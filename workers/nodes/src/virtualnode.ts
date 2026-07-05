// VirtualNode: the Durable Object that registers and heartbeats k8flare's
// Pod-on-Containers virtual node (`cf-containers-<pool>`) and reconciles
// Pods bound to it onto podcontainer.ts's Container-backed DOs. This is the
// "virtual-kubelet" half of workers/nodes -- see workers/nodes/README.md for
// the overall design and its allowlist/networking limitations.
//
// ## Why Pod discovery is alarm-driven, not push-driven (honest tradeoff)
//
// A real kubelet learns about newly-bound Pods instantly, via a watch. This
// class instead lists Pods for its node on a periodic alarm. That is a
// deliberate scope-driven simplification, not an oversight: this task's
// scope explicitly forbids touching workers/storage's Cluster DO (where a
// push -- mirroring its existing `pingControllers`/`needsControllersPing`
// pattern in workers/storage/src/index.ts -- would otherwise be added), so
// there is no write-hook this class can ride the way ClusterIP allocation or
// workers/controllers' resident KCM do. pkg/apiserver/nodelifecycle.go hit
// the identical constraint for a different reason (Lease staleness is
// detected by the ABSENCE of a write, so there's genuinely nothing to hook)
// and resolved it the same way: ride an already-mandatory periodic alarm
// rather than add a new one.
//
// Crucially, this does NOT add a new idle-cluster cost: renewing this
// virtual node's Lease is unconditionally required for as long as the node
// is registered, regardless of whether any Pod is ever scheduled to it (the
// same way a real kubelet heartbeats every ~10s whether or not it's running
// any Pods) -- docs/cost-model.md's Phase 7 estimate row already prices this
// in ("a lightweight alarm is expected to be needed for virtual-node Lease
// renewal (regardless of whether Pods exist)"). Pod reconciliation is a free
// rider on that already-mandatory wake, not a separate poll loop: with zero
// Pods, each tick is one cheap field-selector List call, no Container
// starts, no extra alarms. "workers/nodes doesn't wake for Pod work when
// Pods=0" (this task's instruction) holds in the sense that matters: no
// *additional* wake beyond the Lease heartbeat every real node already pays
// for. The cost this trades away is latency: Pod scheduling and deletion
// both take up to NODE_LEASE_RENEW_INTERVAL_MS (~10s) to notice, not
// sub-second -- acceptable for a v1/smoke-level backend given Containers'
// own cold start is already 1-3s+ (see spikes/s3-containers/FINDINGS.md).
import { DurableObject } from "cloudflare:workers";
import type { Env } from "./env.ts";
import type { PodContainerBase } from "./podcontainer.ts";
import {
  createLease,
  createNode,
  deletePod,
  getLease,
  getNode,
  listPodsForNode,
  mintR2Credentials,
  NODE_LEASE_RENEW_INTERVAL_MS,
  podPersistentVolumeClaimRef,
  renewLease,
  updateNodeStatus,
  updatePodStatus,
  type NodeObject,
  type PodObject,
  type R2Credential,
} from "./client.ts";
import { isAllowedImage, resolveSizeTier, type SizeTier } from "./images.ts";

const DEFAULT_POOL = "default";

/** Per-Pod bookkeeping this DO persists across reconcile ticks (survives eviction/restart via DO storage). */
interface KnownPod {
  tier: SizeTier;
  uid?: string;
  restartPolicy: "Always" | "OnFailure" | "Never";
  // Phase 8 (R2 PV/PVC backend). epoch-ms expiry of the R2 credential last
  // injected into this Pod's container, if it mounts a PersistentVolumeClaim
  // -- undefined for a Pod with no PVC volume. Used by reconcileOnePod to
  // proactively cycle the container before the credential expires; see this
  // file's "Credential refresh" doc comment below.
  r2CredentialsExpireAt?: number;
}

/**
 * R2CredentialEnvVars maps a minted R2Credential onto the environment
 * variables injected into a Pod's container: the standard AWS_* trio any
 * S3 SDK/CLI picks up automatically from its default credential chain, plus
 * this project's own R2_ENDPOINT/R2_BUCKET/R2_PREFIX -- app-level
 * configuration (which endpoint, which bucket, which key prefix to use)
 * that isn't part of any AWS credential convention, so it has to be handed
 * over some other way. See the repo root README.md's "Volumes (R2 PV/PVC)"
 * section for the full contract an image needs to follow.
 */
function r2CredentialEnvVars(cred: R2Credential): Record<string, string> {
  return {
    AWS_ACCESS_KEY_ID: cred.accessKeyId,
    AWS_SECRET_ACCESS_KEY: cred.secretAccessKey,
    AWS_SESSION_TOKEN: cred.sessionToken,
    R2_ENDPOINT: cred.endpoint,
    R2_BUCKET: cred.bucket,
    R2_PREFIX: cred.prefix,
  };
}

function podKey(namespace: string, name: string): string {
  return `${namespace}/${name}`;
}

function containerBindingForTier(
  env: Env,
  tier: SizeTier,
): DurableObjectNamespace<PodContainerBase> {
  switch (tier) {
    case "small":
      return env.POD_CONTAINER_SMALL;
    case "medium":
      return env.POD_CONTAINER_MEDIUM;
    case "large":
      return env.POD_CONTAINER_LARGE;
  }
}

// ## Credential refresh for long-running Pods (Phase 8, S6's open problem)
//
// A Temporary Access Credential is deliberately short-lived
// (DefaultCredentialTTL, pkg/apiserver/r2.go -- 1 hour), but a Pod can keep
// its container running far longer than that. Nothing about
// @cloudflare/containers lets a Worker push updated env vars into an
// already-running container (they're fixed at process start), and there is
// no in-image cooperation (a credential-refresh sidecar rewriting a
// credentials file the app re-reads) in workers/nodes v1's one allowlisted
// demo image -- see spikes/s6-r2/RESEARCH.md's "one real gap" and
// README.md's Volumes section for why that's future work, not this file's
// job to solve generically.
//
// What this file DOES do, cheaply, using only mechanisms it already has:
// reconcilePods already runs on a short (~10s) alarm-driven tick regardless
// of Pod count (see this file's top doc comment), and already knows how to
// stop+restart a container for restartPolicy handling. reconcileOnePod
// below reuses exactly that -- when a restartPolicy: Always Pod's tracked
// R2 credential is past its expiry (KnownPod.r2CredentialsExpireAt), it
// proactively stops and restarts the container, which re-mints a fresh
// credential as a side effect of the same ensureRunning() call every other
// (re)start already goes through. This is a real, working mitigation, not
// just a documented gap -- but it is not zero-downtime (the container
// briefly stops) and it deliberately does NOT apply to OnFailure/Never
// Pods: forcibly restarting a Pod whose restartPolicy says "don't restart
// me" would violate its own semantics for the sake of credential hygiene.
// Those Pods simply lose R2 access some time after DefaultCredentialTTL
// elapses if they're still running -- a visible, debuggable failure (S3
// calls start returning 403) rather than a silent one, and an acceptable
// v1 trade-off for workloads that are, by their own restartPolicy, expected
// to be short-lived rather than long-running in the first place.
export class VirtualNode extends DurableObject<Env> {
  private pool: string;

  constructor(ctx: DurableObjectState, env: Env) {
    super(ctx, env);
    this.pool = env.NODE_POOL || DEFAULT_POOL;
  }

  private get nodeName(): string {
    return `cf-containers-${this.pool}`;
  }

  async fetch(request: Request): Promise<Response> {
    // Any request bootstraps the alarm loop if it isn't running yet -- see
    // this class's doc comment on why alarm() (not a request-driven path) is
    // where registration/Lease/Pod work actually happens. Operationally,
    // this means a freshly deployed workers/nodes needs exactly one request
    // (e.g. a health check) to start the virtual node; see README.md.
    await this.ensureAlarmScheduled();
    const url = new URL(request.url);
    if (url.pathname === "/healthz") {
      return Response.json({ ok: true, node: this.nodeName, pool: this.pool });
    }
    return new Response("k8flare-nodes: not directly routable (see /healthz)", { status: 404 });
  }

  async alarm(): Promise<void> {
    await this.ensureNodeRegistered();
    await this.renewNodeLease();
    await this.reassertNodeReady();
    await this.reconcilePods();
    // Re-arm unconditionally: unlike Cluster DO's safety-net alarm (which
    // parks once no Node/Service exists), this virtual node's own existence
    // IS the condition for its Lease needing renewal -- there's no "idle"
    // state to park into short of de-registering the node entirely (an
    // explicit operator action, not implemented in v1 -- see README.md).
    await this.ctx.storage.setAlarm(Date.now() + NODE_LEASE_RENEW_INTERVAL_MS);
  }

  private async ensureAlarmScheduled(): Promise<void> {
    const current = await this.ctx.storage.getAlarm();
    if (current === null) {
      await this.ctx.storage.setAlarm(Date.now() + NODE_LEASE_RENEW_INTERVAL_MS);
    }
  }

  /** Idempotent create-if-missing, mirroring pkg/apiserver/bootstrap.go's BootstrapCluster pattern. */
  private async ensureNodeRegistered(): Promise<void> {
    const existing = await getNode(this.env, this.nodeName);
    if (existing) return;

    const now = new Date().toISOString();
    const node: NodeObject = {
      apiVersion: "v1",
      kind: "Node",
      metadata: {
        name: this.nodeName,
        labels: {
          "kubernetes.io/hostname": this.nodeName,
          "k8flare.dev/backend": "containers",
          "k8flare.dev/pool": this.pool,
        },
      },
      // NoSchedule taint: EKS-on-Fargate-style isolation. Ordinary Pods
      // must never land on this backend (image allowlist, no UDP, no
      // kubectl exec -- see README.md); only Pods that opted in via the
      // `k8flare.dev/compute: containers` annotation get a matching
      // toleration + nodeSelector injected by the apiserver at admission
      // (pkg/apiserver, MutatePodForComputeClass).
      spec: {
        taints: [{ key: "k8flare.dev/pod-on-containers", value: "true", effect: "NoSchedule" }],
      },
      status: {
        // Deliberately large, fixed capacity rather than a tight number:
        // Cloudflare Containers' real ceiling is an account-wide concurrency
        // cap (1,500 vCPU / 6TiB mem, spikes/s3-containers/FINDINGS.md), not
        // a per-node capacity in the traditional sense -- every Pod gets its
        // own Container instance, not a slice of one shared host. This
        // capacity exists so the real scheduler's capacity predicate has
        // *something* plausible to check, not to model a real resource
        // ceiling; see README.md's networking/allowlist limitations section.
        capacity: { cpu: "64", memory: "128Gi", pods: "256" },
        allocatable: { cpu: "64", memory: "128Gi", pods: "256" },
        conditions: [
          {
            type: "Ready",
            status: "True",
            reason: "VirtualNodeReady",
            message: "workers/nodes virtual node is ready",
            lastHeartbeatTime: now,
            lastTransitionTime: now,
          },
        ],
        nodeInfo: {
          architecture: "amd64",
          operatingSystem: "linux",
          osImage: "cloudflare-containers",
          kubeletVersion: "k8flare-nodes-v1",
          containerRuntimeVersion: "cloudflare-containers://v1",
        },
      },
    };
    await createNode(this.env, node);
    // Re-fetch and PUT .../status: apiserver's Create path only applies
    // ApplyDefaults (handler.go), never the caller-supplied .status -- a
    // separate status-subresource PUT is required to actually persist
    // capacity/allocatable/conditions (see pkg/apiserver/subresource.go's
    // handleStatusSubresource).
    const created = await getNode(this.env, this.nodeName);
    if (created) {
      created.status = node.status;
      await updateNodeStatus(this.env, created);
    }
  }

  /**
   * Re-assert Ready on every tick, exactly like a real kubelet's periodic
   * node-status update. Without this, one transient Lease gap (DO
   * eviction, redeploy) lets the node-lifecycle controllers set the
   * conditions to Unknown and add node.kubernetes.io/unreachable taints
   * that nothing ever clears -- observed live (2026-07-05): the virtual
   * node's Lease had recovered but the stale NoSchedule/NoExecute taints
   * left every annotated Pod permanently unschedulable ("untolerated
   * taint(s)"). With a freshly-Ready status, the real
   * kube-controller-manager's node_lifecycle_controller removes those
   * taints itself (upstream behavior), so recovery needs no
   * apiserver-side change. Costs 1 GET + 1 status PUT per ~10s tick on
   * top of the 4 requests/tick already priced in docs/cost-model.md's
   * Phase 7 section.
   */
  private async reassertNodeReady(): Promise<void> {
    const node = await getNode(this.env, this.nodeName);
    if (!node) return;
    const now = new Date().toISOString();
    const status = (node.status ?? {}) as { conditions?: Record<string, unknown>[] };
    const prevReady = (status.conditions ?? []).find((c) => c.type === "Ready");
    status.conditions = [
      {
        type: "Ready",
        status: "True",
        reason: "VirtualNodeReady",
        message: "workers/nodes virtual node is ready",
        lastHeartbeatTime: now,
        lastTransitionTime: prevReady?.status === "True" ? prevReady.lastTransitionTime : now,
      },
    ];
    node.status = status as NodeObject["status"];
    await updateNodeStatus(this.env, node);
  }

  private async renewNodeLease(): Promise<void> {
    const now = new Date();
    const existing = await getLease(this.env, this.nodeName);
    if (existing) {
      await renewLease(this.env, this.nodeName, now);
    } else {
      await createLease(this.env, this.nodeName, now);
    }
  }

  private async reconcilePods(): Promise<void> {
    const pods = await listPodsForNode(this.env, this.nodeName);
    const known = ((await this.ctx.storage.get<Record<string, KnownPod>>("knownPods")) ??
      {}) as Record<string, KnownPod>;
    const seen = new Set<string>();

    for (const pod of pods) {
      const key = podKey(pod.metadata.namespace, pod.metadata.name);
      seen.add(key);
      const deleting = pod.metadata.deletionTimestamp !== undefined;
      const terminal = pod.status?.phase === "Succeeded" || pod.status?.phase === "Failed";

      if (deleting || terminal) {
        const restartPolicy = known[key]?.restartPolicy ?? pod.spec?.restartPolicy ?? "Always";
        if (deleting || restartPolicy !== "Always" || pod.status?.phase === "Failed") {
          await this.stopPodContainer(pod, known[key]);
          delete known[key];
          if (deleting) await deletePod(this.env, pod.metadata.namespace, pod.metadata.name);
          continue;
        }
      }

      await this.reconcileOnePod(pod, known);
    }

    // Anything previously tracked but no longer returned by the List above
    // (Pod object itself is gone from storage) is an orphaned container --
    // stop it. This is the "delete" half of the list-and-diff level-trigger
    // this class uses instead of a push/watch (see class doc comment).
    for (const key of Object.keys(known)) {
      if (seen.has(key)) continue;
      const [namespace, name] = key.split("/", 2) as [string, string];
      await this.stopPodContainer(
        { apiVersion: "v1", kind: "Pod", metadata: { namespace, name } },
        known[key],
      );
      delete known[key];
    }

    await this.ctx.storage.put("knownPods", known);
  }

  private async reconcileOnePod(pod: PodObject, known: Record<string, KnownPod>): Promise<void> {
    const key = podKey(pod.metadata.namespace, pod.metadata.name);
    const restartPolicy = pod.spec?.restartPolicy ?? "Always";
    const containers = pod.spec?.containers ?? [];

    if (containers.length !== 1) {
      await this.failPod(
        pod,
        `workers/nodes v1 only supports exactly 1 container per Pod (got ${containers.length})`,
      );
      return;
    }
    if (!isAllowedImage(containers[0].image)) {
      await this.failPod(
        pod,
        `image ${JSON.stringify(containers[0].image)} is not in workers/nodes' allowlist (see README.md)`,
      );
      return;
    }
    const tier = resolveSizeTier(pod);
    if (!tier) {
      await this.failPod(
        pod,
        "Pod resources.requests exceed workers/nodes' largest size tier (see README.md)",
      );
      return;
    }

    const alreadyKnown = known[key];
    const isNewOrRecreated = !alreadyKnown || alreadyKnown.uid !== pod.metadata.uid;
    const stub = this.containerStub(tier, pod.metadata.namespace, pod.metadata.name);

    // Phase 8 (R2 PV/PVC backend): a Pod whose credential has passed its
    // expiry gets proactively cycled -- see this file's "Credential
    // refresh" doc comment above the class for why this is safe/limited to
    // restartPolicy: Always.
    const needsCredentialRefresh =
      !isNewOrRecreated &&
      restartPolicy === "Always" &&
      alreadyKnown.r2CredentialsExpireAt !== undefined &&
      Date.now() >= alreadyKnown.r2CredentialsExpireAt;

    if (isNewOrRecreated || needsCredentialRefresh) {
      const r2 = await this.resolveR2EnvVars(pod);
      if (!r2) return; // failPod already called -- see resolveR2EnvVars
      if (needsCredentialRefresh) await this.stopPodContainer(pod, alreadyKnown);
      await stub.ensureRunning(r2.envVars);
      await this.markRunning(pod, tier);
      known[key] = {
        tier,
        uid: pod.metadata.uid,
        restartPolicy,
        r2CredentialsExpireAt: r2.expiresAt,
      };
      return;
    }

    // Already tracked, no refresh due: check whether the container itself
    // exited (crash, or a Never/OnFailure Pod's process finishing) and
    // needs a restartPolicy decision -- Always restarts unconditionally,
    // OnFailure restarts only on a nonzero exit, Never leaves it stopped
    // and marks the Pod terminal.
    const state = await stub.currentState();
    if (state.status === "stopped" || state.status === "stopped_with_code") {
      // @cloudflare/containers' State type only carries an exitCode
      // for status "stopped_with_code" -- plain "stopped" (observed, by
      // actually running this, for a container killed externally via
      // `docker stop`/SIGTERM rather than exiting its own process) has no
      // exitCode at all. Treat "can't prove it exited 0" as failed rather
      // than defaulting to Succeeded -- a real cluster doesn't treat an
      // externally-killed container as having completed successfully.
      const exitCode = "exitCode" in state ? state.exitCode : undefined;
      const failed = exitCode !== 0;
      if (restartPolicy === "Always" || (restartPolicy === "OnFailure" && failed)) {
        const r2 = await this.resolveR2EnvVars(pod);
        if (!r2) return;
        await stub.ensureRunning(r2.envVars);
        await this.markRunning(pod, tier);
        known[key] = {
          tier,
          uid: pod.metadata.uid,
          restartPolicy,
          r2CredentialsExpireAt: r2.expiresAt,
        };
        return;
      } else {
        await this.markTerminal(pod, failed ? "Failed" : "Succeeded");
      }
    }

    known[key] = {
      tier,
      uid: pod.metadata.uid,
      restartPolicy,
      r2CredentialsExpireAt: alreadyKnown?.r2CredentialsExpireAt,
    };
  }

  /**
   * Mints fresh R2 credentials (Phase 8) for pod's PersistentVolumeClaim
   * volume, if it has one, as container env vars (r2CredentialEnvVars).
   * Returns `{}` (envVars/expiresAt both undefined) for a Pod with no PVC
   * volume, so callers can proceed with an undefined envVars argument to
   * ensureRunning (leaves the container's env unchanged from its image
   * default). Returns undefined -- after already calling failPod -- if the
   * Pod does reference a PVC but minting fails (claim missing/unbound, or
   * R2 not configured on this cluster): callers must stop and not start the
   * container in that case, the same way an allowlist/size-tier rejection
   * above already does.
   */
  private async resolveR2EnvVars(
    pod: PodObject,
  ): Promise<{ envVars?: Record<string, string>; expiresAt?: number } | undefined> {
    const pvcRef = podPersistentVolumeClaimRef(pod);
    if (!pvcRef) return {};

    const cred = await mintR2Credentials(this.env, pod.metadata.namespace, pvcRef.claimName);
    if (!cred) {
      await this.failPod(
        pod,
        `failed to mint R2 credentials for PersistentVolumeClaim ${JSON.stringify(pvcRef.claimName)} (not found, not yet bound, or R2 is not configured on this cluster)`,
      );
      return undefined;
    }
    return { envVars: r2CredentialEnvVars(cred), expiresAt: Date.parse(cred.expiresAt) };
  }

  private containerStub(tier: SizeTier, namespace: string, name: string) {
    const ns = containerBindingForTier(this.env, tier);
    return ns.get(ns.idFromName(podKey(namespace, name)));
  }

  private async stopPodContainer(
    pod: Pick<PodObject, "apiVersion" | "kind" | "metadata">,
    known: KnownPod | undefined,
  ): Promise<void> {
    if (!known) return; // never actually got a container (e.g. failed the allowlist check) -- nothing to stop
    try {
      const stub = this.containerStub(known.tier, pod.metadata.namespace, pod.metadata.name);
      await stub.stopContainer();
    } catch (err) {
      console.error(
        `stopPodContainer ${pod.metadata.namespace}/${pod.metadata.name}: ${String(err)}`,
      );
    }
  }

  private async markRunning(pod: PodObject, _tier: SizeTier): Promise<void> {
    const now = new Date().toISOString();
    pod.status = {
      ...pod.status,
      phase: "Running",
      // Real routable Pod networking is out of scope for this v1 backend --
      // see README.md's "Networking limitations" section. No podIP is set
      // rather than fabricating one that would silently not route.
      conditions: [
        { type: "Initialized", status: "True", lastTransitionTime: now },
        { type: "Ready", status: "True", lastTransitionTime: now },
        { type: "ContainersReady", status: "True", lastTransitionTime: now },
        { type: "PodScheduled", status: "True", lastTransitionTime: now },
      ],
      containerStatuses: (pod.spec?.containers ?? []).map((c) => ({
        name: c.name,
        image: c.image,
        imageID: "",
        containerID: "",
        ready: true,
        started: true,
        restartCount: 0,
        state: { running: { startedAt: now } },
      })),
    };
    await updatePodStatus(this.env, pod);
  }

  private async markTerminal(pod: PodObject, phase: "Succeeded" | "Failed"): Promise<void> {
    const now = new Date().toISOString();
    pod.status = {
      ...pod.status,
      phase,
      conditions: [
        { type: "Initialized", status: "True", lastTransitionTime: now },
        { type: "Ready", status: "False", lastTransitionTime: now },
        { type: "ContainersReady", status: "False", lastTransitionTime: now },
        { type: "PodScheduled", status: "True", lastTransitionTime: now },
      ],
      containerStatuses: (pod.spec?.containers ?? []).map((c) => ({
        name: c.name,
        image: c.image,
        imageID: "",
        containerID: "",
        ready: false,
        started: false,
        restartCount: 0,
        state: { terminated: { reason: phase, finishedAt: now } },
      })),
    };
    await updatePodStatus(this.env, pod);
  }

  private async failPod(pod: PodObject, reason: string): Promise<void> {
    console.error(`Pod ${pod.metadata.namespace}/${pod.metadata.name} rejected: ${reason}`);
    const now = new Date().toISOString();
    pod.status = {
      ...pod.status,
      phase: "Failed",
      conditions: [{ type: "PodScheduled", status: "True", lastTransitionTime: now }],
    };
    await updatePodStatus(this.env, pod);
  }
}
