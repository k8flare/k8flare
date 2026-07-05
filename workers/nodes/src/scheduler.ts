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
}

// A VM whose kubelet hasn't registered within this window is considered
// failed (image pull dead, account concurrency cap, ...): emit
// FailedScheduling and destroy so the next poke can retry fresh.
const NODE_READY_TIMEOUT_MS = 5 * 60_000;

function vmBinding(env: Env, tier: SizeTier): DurableObjectNamespace<NodeVMBase> {
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
  async fetch(_request: Request): Promise<Response> {
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
    await this.reconcile().catch((err) => console.log(`cf-containers-scheduler alarm: ${err}`));
    // Re-arm only while there is work in flight; park on an idle cluster.
    const tracked = await this.trackedVMs();
    if (Object.keys(tracked).length > 0) {
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

  private async reconcile(): Promise<void> {
    const tracked = await this.trackedVMs();
    let dirty = false;

    // 1) New work: unscheduled pods addressed to this scheduler.
    const pending = (await listUnscheduledPods(this.env)).filter(
      (p) => p.spec?.schedulerName === SCHEDULER_NAME && !p.metadata.deletionTimestamp,
    );
    for (const pod of pending) {
      const uid = pod.metadata.uid ?? `${pod.metadata.namespace}/${pod.metadata.name}`;
      if (tracked[uid]) continue; // VM already booting/bound for it
      const tier = resolveSizeTier(pod);
      if (!tier) {
        await createSchedulingEvent(
          this.env,
          pod,
          "FailedScheduling",
          "pod resources exceed the largest cf-containers size tier",
        );
        continue;
      }
      const nodeName = `cf-${pod.metadata.name}-${uid.slice(0, 8)}`;
      const stub = vmBinding(this.env, tier).get(vmBinding(this.env, tier).idFromName(uid));
      await stub.up(nodeName);
      tracked[uid] = {
        namespace: pod.metadata.namespace,
        podName: pod.metadata.name,
        podUID: uid,
        nodeName,
        tier,
        bound: false,
        bootedAt: Date.now(),
      };
      dirty = true;
      console.log(`cf-containers-scheduler: booting ${nodeName} for ${uid}`);
    }

    // 2) Advance/reap tracked VMs.
    for (const [uid, vm] of Object.entries(tracked)) {
      const pod = await getPod(this.env, vm.namespace, vm.podName);
      const podGone =
        !pod ||
        pod.metadata.uid !== vm.podUID ||
        !!pod.metadata.deletionTimestamp ||
        pod.status?.phase === "Succeeded" ||
        pod.status?.phase === "Failed";
      if (podGone) {
        await this.teardown(vm);
        delete tracked[uid];
        dirty = true;
        continue;
      }
      if (!vm.bound) {
        const node = await getNode(this.env, vm.nodeName);
        const ready = node?.status?.conditions?.some(
          (c) => c.type === "Ready" && c.status === "True",
        );
        if (ready) {
          await bindPod(this.env, vm.namespace, vm.podName, vm.nodeName);
          await createSchedulingEvent(
            this.env,
            pod,
            "Scheduled",
            `Successfully assigned ${vm.namespace}/${vm.podName} to ${vm.nodeName}`,
          );
          vm.bound = true;
          dirty = true;
        } else if (Date.now() - vm.bootedAt > NODE_READY_TIMEOUT_MS) {
          await createSchedulingEvent(
            this.env,
            pod,
            "FailedScheduling",
            `node VM ${vm.nodeName} did not become Ready within ${NODE_READY_TIMEOUT_MS / 1000}s`,
          );
          await this.teardown(vm);
          delete tracked[uid];
          dirty = true;
        }
      }
    }

    if (dirty) await this.ctx.storage.put("vms", tracked);
  }

  private async teardown(vm: TrackedVM): Promise<void> {
    try {
      const ns = vmBinding(this.env, vm.tier);
      await ns.get(ns.idFromName(vm.podUID)).destroyVM();
    } catch (err) {
      console.log(`cf-containers-scheduler: destroy ${vm.nodeName}: ${err}`);
    }
    try {
      await deleteNode(this.env, vm.nodeName);
    } catch (err) {
      console.log(`cf-containers-scheduler: delete node ${vm.nodeName}: ${err}`);
    }
  }
}
