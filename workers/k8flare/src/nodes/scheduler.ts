// CFContainersScheduler: the per-Pod-node binder Durable Object that
// replaces the old VirtualNode backend. Kubernetes-conventional: pods
// select it via spec.schedulerName "cf-containers-scheduler" (injected by
// the apiserver's compute-class admission, pkg/apiserver/computeclass.go,
// namespace-first); it binds via the official Binding subresource and
// records standard Scheduled/FailedScheduling Events. The scheduling
// decision itself is trivial by design -- every pod gets its own
// dedicated microVM node (nodevm.ts) -- so there is no predicate engine
// here, just lifecycle: boot VM, wait for its kubelet to register, bind,
// destroy on pod termination.
//
// Wake model (cost invariants #1/#3): event-armed pokes from storage's
// pingNodes write-hook (any /registry/pods/ write) plus a safety-net
// alarm that re-arms ONLY while there is in-flight work (pending
// cf-containers pods or live VMs to reap) and parks otherwise.
import { DurableObject } from "cloudflare:workers";
import type { Env } from "./env.ts";
import { clusterSecrets } from "../clusters/tokens.ts";
import type { NodeVMBase } from "./nodevm.ts";
import {
  bindPod,
  createSchedulingEvent,
  deleteNode,
  getNode,
  getPod,
  listUnscheduledPods,
  type PodObject,
} from "./client.ts";
import { resolveSizeTier, type SizeTier } from "./images.ts";
import { createMeshConnector, deleteMeshConnector } from "./meshconnector.ts";

export const SCHEDULER_NAME = "cf-containers-scheduler";
const SAFETY_NET_INTERVAL_MS = 15_000;

/** One tracked pod->VM assignment, persisted across DO restarts. */
interface TrackedVM {
  namespace: string;
  podName: string;
  podUID: string;
  nodeName: string;
  tier: SizeTier;
  bound: boolean;
  bootedAt: number;
  // Per-Pod Cloudflare Mesh membership (spikes/s17-mesh-nodevm/FINDINGS.md's
  // per-Pod-Mesh entry): the warp_connector id minted for this Pod's
  // NodeVM, if CLOUDFLARE_API_TOKEN/ACCOUNT_ID are configured. Tracked so
  // teardown() can delete it -- the 50-connectors/account cap must not
  // leak per Pod.
  meshConnectorId?: string;
}

// A VM whose kubelet hasn't registered within this window is considered
// failed (image pull dead, account concurrency cap, ...): emit
// FailedScheduling and destroy so the next poke can retry fresh.
const NODE_READY_TIMEOUT_MS = 5 * 60_000;

export function vmBinding(env: Env, tier: SizeTier): DurableObjectNamespace<NodeVMBase> {
  switch (tier) {
    case "small":
      return env.NODE_VM_SMALL;
    case "medium":
      return env.NODE_VM_MEDIUM;
    case "large":
      return env.NODE_VM_LARGE;
  }
}

export class CFContainersScheduler extends DurableObject<Env> {
  // Multi-cluster identity: this DO's instance name IS the cluster's
  // doName ("<id>@<uid>", or "default") -- storage's pingNodes and the
  // gateway's wrapped SCHEDULER namespace both address it that way.
  private clusterName(): string {
    return this.ctx.id.name ?? "default";
  }

  // NodeVM DOs are scheduler-scoped: "<doName>/<podUID>", uniformly with
  // clusters/clusterenv.ts's prefixNs (so the gateway's kubelet bridge
  // resolves the same instance).
  private vmStub(tier: SizeTier, podUID: string) {
    const ns = vmBinding(this.env, tier);
    return ns.get(ns.idFromName(`${this.clusterName()}/${podUID}`));
  }

  // Cluster-scoped env for every apiserver call (client.ts): stamps this
  // cluster's storage routing and authenticates with its vault token
  // (default keeps the env token; see clusters/tokens.ts).
  private async apiEnv(): Promise<Env> {
    const name = this.clusterName();
    if (name === "default") return this.env;
    const [secret] = await clusterSecrets(this.env, name);
    return {
      ...this.env,
      CLUSTER_DO_NAME: name,
      ENV_K3S_TOKEN: this.env.K3S_TOKEN,
      K3S_TOKEN: secret ?? this.env.K3S_TOKEN,
    };
  }

  async fetch(request: Request): Promise<Response> {
    // Cluster teardown (clusters/api.ts): destroy every live VM FIRST
    // (Containers are wall-clock billed), then drop all state. Idempotent.
    if (new URL(request.url).pathname === "/admin/destroy" && request.method === "POST") {
      const tracked = await this.trackedVMs();
      for (const vm of Object.values(tracked)) {
        try {
          await this.vmStub(vm.tier, vm.podUID).destroyVM();
        } catch (err) {
          console.log(`cf-containers-scheduler destroy: ${vm.nodeName}: ${err}`);
        }
        // Cluster teardown must not leak per-Pod Mesh connectors either
        // (same 50-node cap concern as the normal per-Pod teardown()).
        if (vm.meshConnectorId) await deleteMeshConnector(this.env, vm.meshConnectorId);
      }
      await this.ctx.storage.deleteAlarm();
      await this.ctx.storage.deleteAll();
      return Response.json({ destroyed: true, vms: Object.keys(tracked).length });
    }
    // Pokes are cheap and idempotent; real work happens in reconcile().
    // Never let a caller's cancellation tear reconciliation down
    // mid-flight: run it detached, exactly like workers/controllers'
    // poke model (see its index.ts for the production incident that
    // motivated this shape).
    void this.reconcile().catch((err) => console.log(`cf-containers-scheduler: ${err}`));
    await this.armIfIdle();
    return Response.json({ ok: true, scheduler: SCHEDULER_NAME });
  }

  async alarm(): Promise<void> {
    const hasWork = await this.reconcile().catch((err) => {
      console.log(`cf-containers-scheduler alarm: ${err}`);
      // A failed reconcile IS pending work -- stay awake through
      // apiserver hiccups rather than park with the queue unknown.
      return true;
    });
    // Re-arm while there is work in flight; park on an idle cluster.
    // "Work" includes PENDING pods with no VM, not just tracked VMs:
    // after a FailedScheduling teardown the pod is still unscheduled and
    // nothing will write it again, so parking on tracked-VMs-only left
    // it stuck forever (hit live 2026-07-06, same predicate class as
    // the KCM liveness bug fixed in d1a9503).
    if (hasWork) {
      await this.ctx.storage.setAlarm(Date.now() + SAFETY_NET_INTERVAL_MS);
    }
  }

  private async armIfIdle(): Promise<void> {
    const current = await this.ctx.storage.getAlarm();
    if (current === null) {
      await this.ctx.storage.setAlarm(Date.now() + SAFETY_NET_INTERVAL_MS);
    }
  }

  private async trackedVMs(): Promise<Record<string, TrackedVM>> {
    return (await this.ctx.storage.get<Record<string, TrackedVM>>("vms")) ?? {};
  }

  /**
   * Resolves a tracked VM by pod UID or node name, for the gateway's
   * kubelet bridge (pods/log, nodes/proxy stats) to find the right
   * NodeVM DO. Returns null for unknown/already-reaped VMs.
   */
  async lookupVM(key: string): Promise<{ podUID: string; tier: SizeTier } | null> {
    const tracked = await this.trackedVMs();
    const direct = tracked[key];
    if (direct) return { podUID: direct.podUID, tier: direct.tier };
    for (const vm of Object.values(tracked)) {
      if (vm.nodeName === key) return { podUID: vm.podUID, tier: vm.tier };
    }
    return null;
  }

  /** Returns whether there is still work in flight (pending pods or live VMs). */
  private async reconcile(): Promise<boolean> {
    const tracked = await this.trackedVMs();
    const env = await this.apiEnv();
    let dirty = false;

    // 1) New work: unscheduled pods addressed to this scheduler.
    const pending = (await listUnscheduledPods(env)).filter(
      (p) => p.spec?.schedulerName === SCHEDULER_NAME && !p.metadata.deletionTimestamp,
    );
    for (const pod of pending) {
      const uid = pod.metadata.uid ?? `${pod.metadata.namespace}/${pod.metadata.name}`;
      if (tracked[uid]) continue; // VM already booting/bound for it
      const tier = resolveSizeTier(pod);
      if (!tier) {
        await createSchedulingEvent(
          env,
          pod,
          "FailedScheduling",
          "pod resources exceed the largest cf-containers size tier",
        );
        continue;
      }
      const nodeName = `cf-${pod.metadata.name}-${uid.slice(0, 8)}`;
      // Mint this Pod's own Mesh connector before booting its NodeVM
      // (spikes/s17-mesh-nodevm/FINDINGS.md's per-Pod-Mesh entry): the
      // token must be present in the VM's very first env, same reasoning
      // cmd/agent's own comment gives for joining Mesh before building
      // agentConfig. Returns undefined (not an error) if
      // CLOUDFLARE_API_TOKEN/ACCOUNT_ID aren't configured -- the Pod
      // still boots, just without Mesh membership.
      const mesh = await createMeshConnector(env, nodeName);
      const stub = this.vmStub(tier, uid);
      await stub.up(nodeName, mesh?.token);
      tracked[uid] = {
        namespace: pod.metadata.namespace,
        podName: pod.metadata.name,
        podUID: uid,
        nodeName,
        tier,
        bound: false,
        bootedAt: Date.now(),
        meshConnectorId: mesh?.id,
      };
      dirty = true;
      console.log(`cf-containers-scheduler: booting ${nodeName} for ${uid}`);
    }

    // 2) Advance/reap tracked VMs.
    for (const [uid, vm] of Object.entries(tracked)) {
      const pod = await getPod(env, vm.namespace, vm.podName);
      const podGone =
        !pod ||
        pod.metadata.uid !== vm.podUID ||
        !!pod.metadata.deletionTimestamp ||
        pod.status?.phase === "Succeeded" ||
        pod.status?.phase === "Failed";
      if (podGone) {
        await this.teardown(env, vm);
        delete tracked[uid];
        dirty = true;
        continue;
      }
      if (!vm.bound) {
        const node = await getNode(env, vm.nodeName);
        const ready = node?.status?.conditions?.some(
          (c) => c.type === "Ready" && c.status === "True",
        );
        if (ready) {
          await bindPod(env, vm.namespace, vm.podName, vm.nodeName);
          await createSchedulingEvent(
            env,
            pod,
            "Scheduled",
            `Successfully assigned ${vm.namespace}/${vm.podName} to ${vm.nodeName}`,
          );
          vm.bound = true;
          dirty = true;
        } else if (Date.now() - vm.bootedAt > NODE_READY_TIMEOUT_MS) {
          await createSchedulingEvent(
            env,
            pod,
            "FailedScheduling",
            `node VM ${vm.nodeName} did not become Ready within ${NODE_READY_TIMEOUT_MS / 1000}s`,
          );
          await this.teardown(env, vm);
          delete tracked[uid];
          dirty = true;
        }
      }
    }

    if (dirty) await this.ctx.storage.put("vms", tracked);
    return pending.length > 0 || Object.keys(tracked).length > 0;
  }

  private async teardown(env: Env, vm: TrackedVM): Promise<void> {
    try {
      await this.vmStub(vm.tier, vm.podUID).destroyVM();
    } catch (err) {
      console.log(`cf-containers-scheduler: destroy ${vm.nodeName}: ${err}`);
    }
    // Free this Pod's Mesh connector so the 50-connectors/account cap
    // doesn't leak (spikes/s17-mesh-nodevm/FINDINGS.md's per-Pod-Mesh
    // entry) -- best-effort, like the destroy/deleteNode calls around it.
    if (vm.meshConnectorId) await deleteMeshConnector(env, vm.meshConnectorId);
    try {
      await deleteNode(env, vm.nodeName);
    } catch (err) {
      console.log(`cf-containers-scheduler: delete node ${vm.nodeName}: ${err}`);
    }
  }
}
