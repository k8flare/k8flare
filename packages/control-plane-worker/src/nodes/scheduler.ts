// CFContainersScheduler: the NodeVM lifecycle manager Durable Object for
// the Pod-on-Containers backend. Despite the class name (kept as-is --
// renaming it would need a wrangler.jsonc DO migration entry, not
// justified by this change alone), it no longer schedules or binds
// anything: pkg/apiserver/computeclass.go's admission-time
// AssignContainersNode already picked each Pod's size tier and pinned it
// to a dedicated, not-yet-existing Node name (the standard
// kubernetes.io/hostname nodeSelector key), and the real, unmodified
// kube-scheduler (pkg/controllers/sched, the sched dynamic worker) binds
// it via the official Binding subresource once that Node exists and is
// Ready -- exactly like any other Pod, no dedicated binder needed. What
// remains here is pure demand-driven infrastructure lifecycle: given a
// Pod already pinned to a Node name nobody has booted yet, start that
// NodeVM (nodevm.ts); given a tracked NodeVM whose Pod is gone, tear it
// down. There is still no predicate engine -- every Pod gets its own
// dedicated microVM node -- just start/stop.
//
// Wake model (cost invariants #1/#3): event-armed pokes from storage's
// pingNodes write-hook (any /registry/pods/ write) plus a safety-net
// alarm that re-arms ONLY while there is in-flight work (pending
// cf-containers pods or live VMs to reap) and parks otherwise.
import { DurableObject } from "cloudflare:workers";
import type { Env } from "./env.ts";
import { clusterSecrets } from "../clusters/tokens.ts";
import { deleteNode, getNode, getPod, listPendingContainersPods, mergePodEnv, planNodeVMs, type NodeObject, type PodObject } from "./client.ts";
import { createMeshConnector, deleteMeshConnector, meshConnectorToken } from "./meshconnector.ts";
import { r2PodEnv } from "./r2creds.ts";

/** Mirrors pkg/apiserver/computeclass.go's SizeTier naming exactly. */
export type SizeTier = "small" | "medium" | "large";

// Matches pkg/apiserver/computeclass.go's NodeVMTierAnnotation -- Go owns
// the tier decision (real resource.Quantity math against the fixed
// Cloudflare Containers instance_type tiers), this DO just reads it.
const NODEVM_TIER_ANNOTATION = "k8flare.com/nodevm-tier";

const COMPONENT_NAME = "cf-containers-scheduler";

/** One tracked pod->VM assignment, persisted across DO restarts. */
interface TrackedVM {
  namespace: string;
  podName: string;
  podUID: string;
  nodeName: string;
  tier: SizeTier;
  bound: boolean;
  started: boolean;
  bootedAt: number;
  // Per-Pod Cloudflare Mesh membership (spikes/s17-mesh-nodevm/FINDINGS.md's
  // per-Pod-Mesh entry): the warp_connector id minted for this Pod's
  // NodeVM, if CLOUDFLARE_API_TOKEN/ACCOUNT_ID are configured. Tracked so
  // teardown() can delete it -- the 50-connectors/account cap must not
  // leak per Pod.
  meshConnectorId?: string;
}

// A VM whose kubelet hasn't registered within this window is considered
// failed (image pull dead, account concurrency cap, ...): destroy it so
// the next poke can retry fresh (see the timeout branch in reconcile()).
const NODE_READY_TIMEOUT_MS = 5 * 60_000;

type VMEnv = Env & {
  NODE_VM_SMALL?: DurableObjectNamespace;
  NODE_VM_MEDIUM?: DurableObjectNamespace;
  NODE_VM_LARGE?: DurableObjectNamespace;
};

type NodeVMStub = {
  up(nodeName: string, mesh?: string, join?: string, extra?: Record<string, string>): Promise<void>;
  destroyVM(): Promise<void>;
};

export function vmBinding(env: Env, tier: SizeTier): DurableObjectNamespace | undefined {
  const e = env as VMEnv;
  if (tier === "small") return e.NODE_VM_SMALL as DurableObjectNamespace | undefined;
  if (tier === "medium") return e.NODE_VM_MEDIUM as DurableObjectNamespace | undefined;
  return e.NODE_VM_LARGE as DurableObjectNamespace | undefined;
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
  private vmStub(tier: SizeTier, podUID: string): NodeVMStub {
    const ns = vmBinding(this.env, tier);
    if (!ns) throw new Error(`NODE_VM_${tier} is not bound`);
    return ns.get(ns.idFromName(`${this.clusterName()}/${podUID}`)) as unknown as NodeVMStub;
  }

  private apiEnv(): Env {
    return this.env;
  }

  async fetch(request: Request): Promise<Response> {
    // Cluster teardown (clusters/teardown.ts): destroy every live VM FIRST
    // (Containers are wall-clock billed), then drop all state. Idempotent.
    if (new URL(request.url).pathname === "/admin/destroy" && request.method === "POST") {
      const tracked = await this.trackedVMs();
      const failed: string[] = [];
      for (const vm of Object.values(tracked)) {
        try {
          await this.vmStub(vm.tier, vm.podUID).destroyVM();
        } catch (err) {
          console.log(`cf-containers-scheduler destroy: ${vm.nodeName}: ${err}`);
          failed.push(vm.nodeName);
          continue;
        }
        // Cluster teardown must not leak per-Pod Mesh connectors either
        // (same 50-node cap concern as the normal per-Pod teardown()).
        if (vm.meshConnectorId && !(await deleteMeshConnector(this.env, vm.meshConnectorId))) {
          failed.push(`${vm.nodeName} (mesh connector)`);
        }
      }
      // A partial destroy must NOT answer 200: teardown.ts propagates the
      // failure so the operator keeps the Cluster's finalizer and retries.
      // Re-running is safe -- the surviving tracked VMs are still in
      // storage, which is why state is only dropped on full success.
      if (failed.length > 0) {
        return Response.json(
          { destroyed: Object.keys(tracked).length - failed.length, failed },
          { status: 500 },
        );
      }
      await this.ctx.storage.deleteAlarm();
      await this.ctx.storage.deleteAll();
      return Response.json({ destroyed: true, vms: Object.keys(tracked).length });
    }
    // NodeVM leak guard (nodevm.ts onActivityExpired): is this pod UID
    // still a tracked VM? Read-only, no reconcile side effects.
    {
      const url = new URL(request.url);
      if (url.pathname === "/internal/vm-tracked") {
        const uid = url.searchParams.get("uid") ?? "";
        const tracked = await this.trackedVMs();
        return Response.json({ tracked: Boolean(tracked[uid]) });
      }
    }
    // Pokes are cheap and idempotent; real work happens in reconcile().
    // Never let a caller's cancellation tear reconciliation down
    // mid-flight: run it detached, exactly like workers/controllers'
    // poke model (see its index.ts for the production incident that
    // motivated this shape).
    const hasWork = await this.reconcile().catch((err) => {
      console.log(`cf-containers-scheduler: ${err}`);
      return true;
    });
    return Response.json({ ok: true, scheduler: COMPONENT_NAME, hasWork });
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
    const env = this.apiEnv();
    let dirty = false;
    const nowMs = Date.now();
    const pending = await listPendingContainersPods(env);
    const pods: Record<string, PodObject | null> = {};
    const nodes: Record<string, NodeObject | null> = {};
    for (const vm of Object.values(tracked)) {
      pods[`${vm.namespace}/${vm.podName}`] = await getPod(env, vm.namespace, vm.podName);
      if (!vm.bound) nodes[vm.nodeName] = await getNode(env, vm.nodeName);
    }
    const plan = await planNodeVMs(env, { nowMs, tracked, pending, pods, nodes });

    // 1) New work: Pods admission already pinned to a dedicated Node
    // name (pkg/apiserver/computeclass.go's AssignContainersNode) that
    // this DO hasn't started a NodeVM for yet. Tier resolution and node
    // naming already happened in Go at admission time -- this loop only
    // ever starts infrastructure, never rejects a Pod (a Pod whose
    // resources don't fit any tier is rejected synchronously at create
    // time instead, a kubectl-visible 403, so it never reaches here).
    for (const boot of plan.boots) {
      const uid = boot.uid;
      const nodeName = boot.nodeName;
      const tier = boot.tier;
      // Claim this UID and persist it BEFORE any awaited network call
      // below, not after. A DO's single-threaded execution can still
      // interleave two reconcile() invocations at an await point (e.g. a
      // pod-create poke and the safety-net alarm landing close
      // together); both call trackedVMs() and see the same
      // pre-this-loop storage snapshot, so the old code -- which only
      // wrote `tracked[uid]` back at the very end of reconcile() --
      // let both invocations pass the `if (tracked[uid]?.started)
      // continue` guard and independently call createMeshConnector/
      // stub.up() for the same pod. Reproduced live (2026-07-08, real
      // KOOFFICE account): the second, interleaved call's
      // createMeshConnector 409'd (Cloudflare rejects a duplicate
      // connector name), returned
      // `mesh: undefined`, and then overwrote this entry's
      // meshConnectorId with undefined when both invocations' dirty
      // writes landed -- leaking the FIRST (successful) connector past
      // teardown, since `if (vm.meshConnectorId)` saw nothing to
      // delete. Persisting the claim immediately closes the race: the
      // second invocation's trackedVMs() re-read now (once it resumes
      // past its own earlier await) sees this UID already present and
      // takes the retry branch below, which never mints a second
      // connector.
      let meshToken: string | undefined;
      const claimed = boot.claimed ? tracked[uid] : undefined;
      if (claimed) {
        meshToken = claimed.meshConnectorId
          ? await meshConnectorToken(env, claimed.meshConnectorId)
          : undefined;
      } else {
        tracked[uid] = {
          namespace: boot.namespace,
          podName: boot.podName,
          podUID: uid,
          nodeName,
          tier,
          bound: false,
          started: false,
          bootedAt: nowMs,
        };
        await this.ctx.storage.put("vms", tracked);
        // Mint this Pod's own Mesh connector before booting its NodeVM
        // (spikes/s17-mesh-nodevm/FINDINGS.md's per-Pod-Mesh entry): the
        // token must be present in the VM's very first env, same reasoning
        // cmd/agent's own comment gives for joining Mesh before building
        // agentConfig. Returns undefined (not an error) if
        // CLOUDFLARE_API_TOKEN/ACCOUNT_ID aren't configured -- the Pod
        // still boots, just without Mesh membership.
        const mesh = await createMeshConnector(env, nodeName);
        // Persist the real connector id immediately once known, before the
        // next awaited call (stub.up(), which can take seconds waiting for
        // the container's ports) -- not after. Reproduced live
        // (2026-07-08): deleting the Pod while stub.up() was still
        // in-flight let a second, interleaved reconcile() (triggered by
        // the delete's own poke) see this uid's podGone with
        // meshConnectorId still unset (the old code only wrote it after
        // stub.up() returned), so teardown()'s `if (vm.meshConnectorId)`
        // found nothing to delete and leaked the connector createMeshConnector
        // had already created above.
        tracked[uid].meshConnectorId = mesh?.id;
        await this.ctx.storage.put("vms", tracked);
        meshToken = mesh?.id ? await meshConnectorToken(env, mesh.id) : undefined;
      }
      const stub = this.vmStub(tier, uid);
      const [joinSecret] = await clusterSecrets(this.env, this.clusterName());
      const extra = await r2PodEnv(this.env, boot.namespace, boot.podName);
      try {
        await mergePodEnv(this.env, boot.namespace, boot.podName, extra);
        await stub.up(nodeName, meshToken, joinSecret, extra);
      } catch (err) {
        console.log(`cf-containers-scheduler: boot ${nodeName} for ${uid}: ${err}`);
        continue;
      }
      tracked[uid].started = true;
      await this.ctx.storage.put("vms", tracked);
      dirty = true;
      console.log(`cf-containers-scheduler: booting ${nodeName} for ${uid}`);
    }

    // 2) Advance/reap tracked VMs.
    // "bound" here means "this VM's kubelet has registered a Ready
    // Node at least once", purely to skip the getNode() call below
    // on future passes -- NOT that the real scheduler has bound the
    // Pod to it (that's now entirely the real scheduler's own job,
    // watched via its own Pod/Node informers, not polled here).
    for (const uid of plan.markBound) {
      if (!tracked[uid]) continue;
      tracked[uid].bound = true;
      dirty = true;
    }
    for (const vm of plan.teardowns) {
      // The VM never came up (image pull dead, account concurrency
      // cap, ...). No FailedScheduling event to synthesize here
      // anymore -- the real scheduler already reports the Pod as
      // Unschedulable on its own (verified live 2026-07-11) for as
      // long as no Ready Node named vm.nodeName exists. Reap the
      // failed VM and drop tracking; the Pod keeps the same
      // (deterministic) hostname pin, so it re-enters `pending`
      // above on the next pass and gets a fresh boot attempt.
      const cur = tracked[vm.podUID] ?? vm;
      await this.teardown(env, cur);
      delete tracked[vm.podUID];
      dirty = true;
    }

    if (dirty) await this.ctx.storage.put("vms", tracked);
    return plan.hasWork;
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
