// DO-hosted Go WASM pattern verified in spikes/s8-wasm-resident/do-hosted
// (local) and production-verified in spikes/s8-wasm-resident/prod-resident
// (docs/platform-verification.md's S8 section), now combined with the
// ASSETS+LOADER code-supply path verified in
// spikes/s14-loader-external-fetch: the compiled kube-controller-manager
// WASM is far too large to bundle into this script (gzip ~13MB vs
// Workers' 10MiB deploy cap), so it is served from this Worker's own
// Static Assets in <=24MiB chunks (25MiB ASSETS per-file cap), assembled
// at Loader-factory time, and run as a Dynamic Worker via the
// `modules.wasm` field -- the platform's sanctioned route around both the
// deploy cap and the dynamic-code-generation restriction (S14 Part 1).
// The wasm-opt'd binary must stay under the Loader's own hard 64MiB
// total-module-bytes cap (S14 Part 2) -- `make wasm` enforces that at
// build time.
//
// The Controllers DO below keeps its previous role (single resident
// instance, event-armed alarm safety net); it just dispatches into the
// Loader-loaded dynamic worker instead of instantiating the WASM in its
// own isolate. Two S2/S14-verified constraints shape the wiring:
//   - `Fetcher`s (service bindings) pass through WorkerCode.env; DO
//     namespaces/stubs do NOT (S2 item 3a). pkg/controllers.RestConfig
//     only needs the GATEWAY Fetcher by name, so the dynamic worker gets
//     exactly { GATEWAY, K3S_TOKEN }.
//   - The Loader binding itself cannot be forwarded (S14 Part 6), so all
//     LOADER.get() calls happen here, never inside loaded code.
//
// Post-consolidation, the chunk assembly and bootstrap-module source are
// shared with the apiserver's loader path -- see ../loader/. The former
// GATEWAY cross-Worker service binding is now SELF (the same script's
// public fetch handler).
import type { Env } from "./env.ts";
import { pumpTraceEnabled } from "../trace.ts";
import { clusterSecrets } from "../clusters/tokens.ts";
import { makeResidentBootstrapJS } from "../loader/bootstrap.ts";
import { assembleWasm, fetchWasmAsset, fetchWasmManifest } from "../loader/chunks.ts";

// Safety-net alarm interval: mirrors workers/storage's Cluster DO
// SAFETY_NET_INTERVAL_MS (packages/k8flare-worker/src/storage/index.ts) -- this is purely
// a liveness/resurrection check (is the resident controller-manager
// still alive after a redeploy/panic/eviction?), not a reconciliation
// trigger: the real controllers, once running, watch continuously via
// their own client-go informers and need no periodic nudging from here.
const SAFETY_NET_INTERVAL_MS = 60_000;

// How long each poke keeps the KCM dynamic worker's event loop serviced
// after its own response returns (see ../loader/bootstrap.ts's resident
// shape for the mechanism and why it exists -- S14). Event-armed and
// bounded: a window opens only when something pokes us (storage's
// pingControllers on relevant writes, or this DO's safety-net alarm),
// and KCM's own resulting writes re-ping and chain further windows
// while there is real work, then go quiet. I/O waits inside the window
// are not CPU-billed (cost invariant #2).
const PUMP_WINDOW_MS = 25000;

// LoadedComponent tracks one Loader-loaded control-plane binary (kcm,
// sched, or gc) through manifest fetch, chunk assembly, and
// dynamic-worker load.
interface LoadedComponent {
  entrypoint: Fetcher | null;
  // Resolves null when the component's manifest is absent from ASSETS
  // (component not shipped -- e.g. "sched" today).
  loading: Promise<Fetcher | null> | null;
  // Fingerprint of the token this component was LOADED with. The token is
  // baked into the dynamic worker's env at factory time, so a rotation
  // must invalidate the in-memory entrypoint too -- see ensure().
  tokenTag: string | null;
}

// The control-plane binaries this DO hosts as dynamic workers. Separate
// binaries, not one combined: each is already close to the Loader's
// 64MiB cap alone (pkg/controllers/cmd/kcm-wasm/scheduler/main.go's doc comment
// has kcm/sched's numbers; pkg/controllers/gc's doc comment has gc's --
// combining any two would exceed it). All LOADER.get() calls happen
// here in the parent -- a loaded worker cannot load further workers
// (S14 Part 6). gc (the real garbagecollector controller, replacing
// pkg/apiserver's old synchronous CascadeDeleteDependents) is
// deliberately its own isolate rather than folded into kcm: its
// dependency-graph builder needs an informer/watch per resource type
// across apidef.Table, which would compete for kcm's own 128MiB isolate
// budget (see pkg/controllers/controllermanager.go's doc comment).
// clusterop (the cluster operator, pkg/controllers/clusterop) is the
// fourth: it reconciles k8flare.com Cluster objects into tenant control
// planes. Unlike the other three it loads ONLY in the management
// ("default") cluster -- a tenant cluster has no Cluster objects, and
// loading an operator per cluster would charge Loader for nothing.
const COMPONENTS = ["kcm", "sched", "gc", "clusterop"] as const;
type ComponentName = (typeof COMPONENTS)[number];

export class Controllers {
  state: DurableObjectState;
  env: Env;
  // Resolved entrypoint once each dynamic worker is loaded, and the
  // in-flight load. Loading ~60MB through the Loader factory takes long
  // enough in production that awaiting it inline from a poke is wrong:
  // storage's pingControllers runs inside the write path, the canceled
  // ping tears down the load with it, and the next write starts the
  // whole load over -- observed live as an endless "GET / - Canceled"
  // storm with KCM never coming up. Pokes therefore return immediately
  // and loads run detached (DO lifetime is not tied to any one request);
  // each load's own completion handler delivers the first poke.
  components: Record<ComponentName, LoadedComponent> = {
    kcm: { entrypoint: null, loading: null, tokenTag: null },
    sched: { entrypoint: null, loading: null, tokenTag: null },
    gc: { entrypoint: null, loading: null, tokenTag: null },
    clusterop: { entrypoint: null, loading: null, tokenTag: null },
  };

  constructor(state: DurableObjectState, env: Env) {
    this.state = state;
    this.env = env;
  }

  private async ensure(
    name: ComponentName,
    opts?: { armWarmup?: boolean },
  ): Promise<Fetcher | null> {
    const armWarmup = opts?.armWarmup !== false;
    const tag = await this.dropRotatedComponents();
    const c = this.components[name];
    if (!c.loading) {
      c.tokenTag = tag;
      // kcm/sched/gc load independently and concurrently -- NOT chained
      // behind one another. An earlier version serialized every load
      // behind a single shared promise, diagnosed at the time as
      // avoiding concurrent multi-10MB WebAssembly compiles starving
      // wrangler dev's single-threaded event loop (2026-07-12 dw-variant
      // e2e bring-up). That diagnosis was itself superseded the same day
      // (docs/platform-verification.md S23's addendum): the real cause
      // was the per-request apiserver dynamic worker's instantiation
      // stampede under an unbounded controller QPS, fixed at the source
      // by capping QPS/Burst (pkg/controllers/restconfig), not by
      // serialization here -- and S23 says outright that production's
      // real Loader does not share one event loop the way wrangler dev
      // does, so the original justification never applied there.
      // Serializing anyway cost real latency for no benefit: kcm and
      // sched are independent controllers with no reason for one to
      // wait on the other, and doing so measurably delayed the
      // scheduler becoming dispatchable behind an unrelated kcm load.
      c.loading = this.loadComponent(name).then(async (f) => {
        c.entrypoint = f;
        // Warmup window: a freshly loaded component needs several pump
        // windows to get through informer sync plus the initial
        // reconcile of whatever write triggered the load -- and on a
        // cluster with zero nodes the safety-net alarm is parked, so
        // without this nothing would ever poke it again (observed live:
        // a Deployment created on an idle cluster never got its
        // ReplicaSet until a redeploy forced a reload). Bounded and
        // event-armed: only arms after a load triggered by a real WRITE
        // poke -- NOT by the safety-net alarm itself. Alarm-triggered
        // loads must not re-arm the window: every alarm wakes a fresh
        // (hibernated) DO instance whose in-memory components are empty,
        // so its ensure() always reloads, and a load that re-arms warmup
        // re-arms the alarm -- a self-perpetuating chain measured live
        // at 94 alarm firings / 25 idle minutes (2026-07-26, wrangler
        // tail against an EMPTY production cluster; cost invariants
        // #1/#3 violated).
        if (armWarmup) {
          await this.state.storage.put("warmupUntil", Date.now() + 3 * 60_000);
          await this.state.storage.setAlarm(Date.now() + 5_000);
        }
        return f;
      });
      // Don't cache failures -- the next poke retries the load.
      c.loading.catch((err) => {
        console.log(`controllers: ${name} load failed: ${err}`);
        c.loading = null;
      });
    }
    return c.loading;
  }

  // Loads one control-plane binary as a dynamic worker and returns its
  // Fetcher. Cheap on the warm path: LOADER.get() with an already-loaded
  // id skips the factory entirely (S2 item 4). The binary's own sha256 is
  // the cache id, so a rebuilt binary is a new id and the stale isolate
  // is simply never addressed again.
  //
  // Chunks are fetched sequentially and streamed straight into one
  // preallocated buffer: an earlier fetch-all-then-concat version held
  // both every part and the assembled copy alive at once (~2x binary
  // size ≈ 125MB), which flirts with production's 128MiB isolate memory
  // limit that wrangler dev never enforces.
  // Multi-cluster identity: this DO's instance name IS the cluster's
  // doName ("<id>@<uid>", or "default") -- storage's pingControllers
  // addresses the Controllers DO by its own Cluster DO name.
  private clusterName(): string {
    return this.state.id.name ?? "default";
  }

  private clusterBasePath(): string {
    const name = this.clusterName();
    return name === "default" ? "" : `/c/${name.split("@")[0]}`;
  }

  // The token this cluster's KCM authenticates with: the vault's first
  // token for provisioned clusters, the env token for default (see
  // clusters/tokens.ts).
  private async clusterToken(): Promise<string | undefined> {
    const [secret] = await clusterSecrets(this.env, this.clusterName());
    return secret;
  }

  // Fingerprint of the current cluster token, used both as the loader id's
  // rotation discriminator and as the staleness check below.
  private async tokenTag(): Promise<string> {
    const token = await this.clusterToken();
    return token ? token.slice(0, 8) : "none";
  }

  // Rotation staleness: each component's token is baked into its dynamic
  // worker's env at Loader-factory time, so an ALREADY-LOADED component
  // keeps calling with the token it was loaded with until its isolate is
  // evicted -- and once a rotation revokes that token, every call it makes
  // 401s. The loader id already carries #tokenTag (a rotation is a
  // different id, hence a fresh isolate), but nothing re-derived the id for
  // a component held in memory. So recompute the tag on every ensure() and
  // on the fetch() poke path, and drop any component loaded under a
  // different one; the next load addresses the new id. Cheap: clusterSecrets
  // caches per isolate for 60s, so the steady state is a memory read.
  // Returns the current tag.
  private async dropRotatedComponents(): Promise<string> {
    const tag = await this.tokenTag();
    for (const name of COMPONENTS) {
      const c = this.components[name];
      if (c.tokenTag !== null && c.tokenTag !== tag) {
        console.log(`controllers: ${name} token rotated, reloading`);
        this.components[name] = { entrypoint: null, loading: null, tokenTag: null };
      }
    }
    return tag;
  }

  private async loadComponent(name: ComponentName): Promise<Fetcher | null> {
    // Harness kill switch (see Env.SCHED_DISABLED): e2e-conformance runs
    // a HOST kube-scheduler process against the same cluster, and two
    // live schedulers race on Bindings (409s are upstream-tolerated but
    // make sig-scheduling tests flaky). Same treated-as-absent shape as
    // a missing manifest, so pokes/alarms stay quiet about it.
    if (name === "sched" && this.env.SCHED_DISABLED === "1") return null;
    // Same shape for the workload controller-manager: e2e-conformance
    // runs a HOST kube-controller-manager, and two live sets of
    // workload controllers (each with its own informer lag) race each
    // other's back-fills -- exactly the double-scheduler problem
    // SCHED_DISABLED exists for. The gc component stays loadable: the
    // host has no garbage collector, the gc dynamic worker is the only
    // one.
    if (name === "kcm" && this.env.CM_DISABLED === "1") return null;
    // The cluster operator is a MANAGEMENT-cluster singleton: only the
    // default cluster holds Cluster objects. Treated as absent elsewhere,
    // the same shape as a missing manifest.
    if (name === "clusterop") {
      if (this.clusterName() !== "default") return null;
      // Test/harness kill switch, mirroring CM_DISABLED: keeps the
      // operator unloaded so a suite can drive Cluster objects by hand
      // without a live reconciler seeding "default" or provisioning DO
      // trees underneath it.
      if (this.env.CLUSTEROP_DISABLED === "1") return null;
    }
    // Manifest absent = component not shipped in this deployment.
    // Treated as absent, not an error, so pokes/alarms stay quiet
    // about it.
    console.log(`controllers: ${name} load queued/starting`);
    const manifest = await fetchWasmManifest(this.env.ASSETS, name);
    if (!manifest) return null;
    const doName = this.clusterName();
    const token = await this.clusterToken();
    // Per-cluster dynamic worker, and the token is baked at factory time
    // (unlike the apiserver's vault-backed TokensFunc) -- so the loader
    // id carries a token fingerprint: rotation = new id = fresh isolate,
    // and the stale one is simply never addressed again.
    const tokenTag = await this.tokenTag();
    // Fault injection (see Env.PUMP_WINDOW_DROP_CLOSE): production can
    // tear a poke's IoContext down before its ctx.waitUntil timer runs,
    // which leaves a pump window open forever on the Go side. `wrangler
    // dev` never does that (S31 E1), so the only way to cover the
    // resulting wedge locally is to drop the close deliberately.
    const dropCloseEvery = Number(this.env.PUMP_WINDOW_DROP_CLOSE ?? 0) || 0;
    const worker = this.env.LOADER.get(
      `${this.env.LOADER_ID_SALT ?? ""}${name}:${doName}@${manifest.sha256}#${tokenTag}${dropCloseEvery ? `!${dropCloseEvery}` : ""}`,
      async () => {
        const wasm = await assembleWasm(this.env.ASSETS, manifest);
        const wasmExec = await fetchWasmAsset(this.env.ASSETS, "wasm_exec.js").then((r) =>
          r.text(),
        );
        // Only plain values and Fetchers survive the env clone (S2 item
        // 3a) -- this is exactly what pkg/controllers.RestConfig("GATEWAY")
        // and getToken() read on the Go side. SELF is this same script's
        // public fetch handler; CLUSTER_BASE_PATH keeps every API call the
        // KCM makes inside this cluster's /c/<id> URL space.
        const dynamicEnv: Record<string, unknown> = {
          GATEWAY: this.env.SELF,
          CLUSTER_BASE_PATH: this.clusterBasePath(),
        };
        if (token) dynamicEnv.K3S_TOKEN = token;
        if (this.env.PUMP_TRACE) dynamicEnv.PUMP_TRACE = this.env.PUMP_TRACE;
        if (this.env.KCM_VERBOSITY) dynamicEnv.KCM_VERBOSITY = this.env.KCM_VERBOSITY;
        const tails = pumpTraceEnabled(this.env) ? [this.env.SELF] : undefined;
        return {
          tails,
          compatibilityDate: "2026-07-01",
          mainModule: "index.js",
          modules: {
            "index.js": makeResidentBootstrapJS(PUMP_WINDOW_MS, dropCloseEvery),
            "wasm_exec.js": wasmExec,
            "app.wasm": { wasm: wasm.buffer as ArrayBuffer },
          },
          env: dynamicEnv,
        };
      },
    );
    const entrypoint = worker.getEntrypoint();
    // Force the factory to actually run now (getEntrypoint alone is lazy)
    // and prove the dynamic worker is dispatchable before declaring it
    // ready -- this doubles as the "first poke" that starts the resident
    // control loop.
    await entrypoint.fetch("http://controllers.internal/healthz");
    console.log(`controllers: ${name} dynamic worker up`);
    return entrypoint;
  }

  // Poke one component: dispatch if loaded, otherwise kick off (or keep
  // retrying) its detached load, whose completion delivers the first
  // dispatch. Never awaited from fetch() -- see the components field's
  // doc comment.
  private poke(
    name: ComponentName,
    request?: Request,
    opts?: { armWarmup?: boolean },
  ): Promise<Response> | null {
    const c = this.components[name];
    if (c.entrypoint) {
      return c.entrypoint.fetch(request ?? "http://controllers.internal/healthz");
    }
    void this.ensure(name, opts)
      .then((f) => f && f.fetch("http://controllers.internal/healthz"))
      .catch(() => {}); // already logged in ensure()
    return null;
  }

  async fetch(request: Request): Promise<Response> {
    // Cluster teardown (clusters/api.ts): stop the alarm, drop state.
    // The dynamic workers die by disuse -- their loader ids are simply
    // never addressed again. Idempotent.
    if (new URL(request.url).pathname === "/admin/destroy" && request.method === "POST") {
      await this.state.storage.deleteAlarm();
      await this.state.storage.deleteAll();
      for (const name of COMPONENTS)
        this.components[name] = { entrypoint: null, loading: null, tokenTag: null };
      return Response.json({ destroyed: true });
    }
    // Test kill switch (see Env.KCM_DISABLED): pkg/apiserver's go test
    // suite runs against the consolidated single config, and its Pods
    // must not be touched by controllers.
    if (this.env.KCM_DISABLED === "1") {
      return Response.json({ controllerManager: "disabled (KCM_DISABLED=1)" });
    }
    // The Cluster DO's node-lifecycle safety net (storage/index.ts) --
    // ALARM-origin, so it deliberately skips everything the write path
    // below does: no warmup window (an alarm-triggered load must not arm
    // one, see ensure()), no backoff reset, and no alarm of this DO's
    // own. Doing any of those on a 60s tick would revive exactly the
    // "idle BYO node kept a pointless 60s chain alive" regression
    // alarm()'s comment describes (cost invariants #1/#3). All it owes
    // the nodelifecycle controller is one kcm pump window.
    if (new URL(request.url).pathname === "/safety-net/node-lifecycle") {
      await this.dropRotatedComponents();
      const kcm = this.poke("kcm", undefined, { armWarmup: false });
      const status = kcm
        ? await kcm.then(() => "pumped").catch((err) => `dispatch failed: ${err}`)
        : "loading";
      return Response.json({ nodeLifecycle: status });
    }
    // Arm the safety net if it isn't already, so a redeploy/panic/
    // eviction that resets the dynamic workers still gets noticed and
    // restarted even if no further relevant write happens to re-trigger
    // storage's pingControllers (packages/k8flare-worker/src/storage/index.ts). Cheap
    // local check -- no cross-DO call on this hot path. Armed before the
    // dispatch so a still-loading component gets a completion poke even
    // if no further write ever arrives.
    // Corrected 2026-09-11: this used to reset unconvergedTicks to 0 on every
    // poke, reasoning that a fresh write deserves a fresh backoff. On a cluster
    // whose work can never converge the controllers never stop writing, so the
    // reset arrived before the backoff could ever grow -- S26b measured ~40s
    // average intervals and ~65,000 alarms a month against a ceiling of 600s,
    // and recorded the cause as unidentified. Pulling the alarm in still gives
    // the new write prompt attention; the counter now measures how long the
    // cluster has failed to converge, which is what the backoff is for. A
    // cluster that does converge parks, and parking zeroes the counter.
    const current = await this.state.storage.getAlarm();
    if (current === null || current > Date.now() + SAFETY_NET_INTERVAL_MS) {
      this.state.storage.setAlarm(Date.now() + SAFETY_NET_INTERVAL_MS);
    }

    // Before dispatching: poke()'s fast path goes straight to a loaded
    // entrypoint without consulting ensure(), so the rotation check has to
    // happen here too.
    await this.dropRotatedComponents();
    const kcmResp = this.poke("kcm", request);
    const schedResp = this.poke("sched");
    const gcResp = this.poke("gc");
    const clusteropResp = this.poke("clusterop");
    const statuses: Record<string, unknown> = {};
    for (const [field, resp] of [
      ["scheduler", schedResp],
      ["garbageCollector", gcResp],
      ["clusterOperator", clusteropResp],
    ] as const) {
      statuses[field] = resp
        ? await resp
            .then((r) => r.json<Record<string, unknown>>().then((j) => j[field] ?? "up"))
            .catch((err) => `dispatch failed: ${err}`)
        : "not loaded";
    }
    if (!kcmResp) {
      if (this.env.CM_DISABLED === "1") {
        statuses.controllerManager = "disabled (CM_DISABLED=1)";
        return Response.json(statuses);
      }
      statuses.controllerManager = "loading";
      return Response.json(statuses, { status: 202 });
    }
    // Preserve the KCM response shape for callers that read it, but fold
    // the scheduler's status in so /healthz reports both.
    const kcmJson = await kcmResp
      .then((r) => r.json<Record<string, unknown>>())
      .catch((err) => ({ controllerManager: `dispatch failed: ${err}` }));
    return Response.json({ ...kcmJson, ...statuses });
  }

  async alarm(): Promise<void> {
    if (this.env.KCM_DISABLED === "1") return; // test kill switch; do not re-arm
    // Park check FIRST, before touching any dynamic worker: an alarm
    // firing on an idle cluster used to ensure()+load all four ~40MB
    // components just to then decide to park -- and on a hibernated DO
    // that load re-armed the warmup window, chaining the alarm forever
    // (see ensure()'s warmup comment; measured live 2026-07-26). The
    // convergence probe below costs 2-3 apiserver list calls and no
    // controller loads.
    const warmupUntil = (await this.state.storage.get<number>("warmupUntil")) ?? 0;
    const warmupActive = Date.now() < warmupUntil;
    const unconverged = await this.hasUnconvergedWork();
    if (!warmupActive && !unconverged) {
      await this.state.storage.put("unconvergedTicks", 0);
      // Nothing outstanding -- but a CronJob's next fire is a wall-clock
      // deadline, not a write, so parking here silently drops it (measured:
      // docs/platform-verification.md S27). Arm for that instant instead.
      // Still event-armed: the alarm exists only because a CronJob exists,
      // it is one alarm per occurrence, and deleting the last CronJob parks
      // it again. Not the backoff path below -- that would push the alarm
      // past the schedule.
      const cron = await this.cronProbe();
      if (cron?.wakeMs != null) {
        this.state.storage.setAlarm(cron.wakeMs);
        return;
      }
      return; // park: no work, no fresh load to nurse -- no reload, no re-arm
    }

    // Hitting /healthz both confirms liveness and (re-)triggers the Go
    // side's ensureStarted() if a dynamic worker's isolate was evicted
    // since the last request (the Loader factory then reruns too). The
    // alarm context has no client to cancel it, so awaiting the loads
    // here is safe (and is what completes a load whose pokes all died).
    // armWarmup:false -- alarm-triggered loads must not extend the
    // warmup window (see ensure()).
    for (const name of COMPONENTS) {
      const c = await this.ensure(name, { armWarmup: false });
      if (c) await c.fetch("http://controllers.internal/healthz");
    }

    // Re-arm while there is UNCONVERGED WORKLOAD WORK, or while a fresh
    // load's warmup window is open (see ensure()); otherwise park (cost
    // invariants #1/#3 -- no alarm chain on an idle cluster).
    //
    // The predicate used to be "does any Node exist", which was wrong on
    // both edges (observed live, 2026-07-06): a node-less cluster with a
    // freshly created Deployment parked the alarm the moment warmup
    // expired, leaving the KCM permanently inert (its dynamic worker
    // only makes progress inside pump windows, and nothing else opens
    // them once storage's per-write pokes stop coming); conversely a
    // cluster with an idle BYO node kept a pointless 60s chain alive.
    // "Work exists" for the KCM's twelve controllers is exactly workload
    // convergence: any Deployment/ReplicaSet/Job whose status lags its
    // spec. Checked via the same gateway API the KCM itself uses; 2-3
    // cheap list calls per tick, and only while ticking.
    // Node lifecycle is deliberately OUTSIDE this predicate -- Lease
    // staleness has no spec/status gap to observe, so the Cluster DO's
    // safety net pumps it instead (see the /safety-net/node-lifecycle
    // path in fetch() and storage/index.ts's alarm()).
    if (warmupActive) {
      await this.state.storage.put("unconvergedTicks", 0);
      this.state.storage.setAlarm(Date.now() + 15_000);
      return;
    }
    // unconverged is necessarily true here (the park check returned
    // otherwise). Exponential backoff bounds the cost of work that will
    // never converge (e.g. a Deployment whose pods are unschedulable on
    // a node-less cluster): 15s doubling to a 10min ceiling. Any fresh
    // relevant write resets the cadence via fetch() below.
    const ticks = ((await this.state.storage.get<number>("unconvergedTicks")) ?? 0) + 1;
    await this.state.storage.put("unconvergedTicks", ticks);
    const interval = Math.min(15_000 * 2 ** Math.max(0, ticks - 4), 600_000);
    this.state.storage.setAlarm(Date.now() + interval);
  }

  private async apiGet(path: string): Promise<Record<string, unknown> | null> {
    try {
      const token = (await this.clusterToken()) || "k8flare-dev-token";
      const resp = await this.env.SELF.fetch(
        `http://gateway.internal${this.clusterBasePath()}${path}`,
        {
          headers: { Authorization: `Bearer ${token}` },
        },
      );
      if (!resp.ok) return null;
      return await resp.json();
    } catch {
      return null;
    }
  }

  /**
   * Absolute ms at which to wake for the earliest CronJob fire, or null if
   * no CronJob needs one. The schedule maths lives in Go
   * (pkg/apiserver/cronschedule.go) because the real cron parser and the
   * CronJob semantics are upstream's; this only turns its answer into an
   * alarm time.
   *
   * A schedule already in the past (the cluster was down across it) comes
   * back as a past instant and is clamped to "now-ish" so the fire is not
   * lost. A probe failure returns null rather than holding the alarm open:
   * whatever write eventually arrives will re-arm, and an apiserver hiccup
   * must not turn into a permanent alarm chain.
   */
  private async cronProbe(): Promise<{ wakeMs: number | null; overdue: boolean } | null> {
    const resp = await this.apiGet("/internal/next-cron-schedule");
    if (resp === null) return null;
    const iso = resp.nextScheduleTime;
    let wakeMs: number | null = null;
    if (typeof iso === "string" && iso !== "") {
      const at = Date.parse(iso);
      if (!Number.isNaN(at)) wakeMs = Math.max(at, Date.now() + 1_000);
    }
    return { wakeMs, overdue: resp.overdue === true };
  }

  /**
   * Whether a graceful deletion is still in flight anywhere in this
   * cluster. That is real outstanding work for the garbage collector and
   * the workload probe below cannot see it: a foreground-deleted owner
   * still matches its own spec, so nothing about its spec/status lags.
   * Without this the alarm parks mid-cascade and `kubectl delete
   * --cascade=foreground` never completes on a cluster with no other
   * write traffic (docs/platform-verification.md S36). Same "a failed
   * probe means stay awake" rule as the lists below.
   */
  private async pendingDeletions(): Promise<boolean> {
    // The probe behind this costs one LIST per namespaced resource across every
    // namespace -- 28 of them, measured at 440 Durable Object LISTs for a
    // single 10-pod foreground delete (S41) -- and it ran on every alarm tick.
    // A deletion can only appear through a write, and every write advances the
    // cluster revision, so a revision that has not moved since the last empty
    // answer cannot have grown one. Reading the revision is a single call.
    const revision = await this.clusterRevision();
    if (revision !== null) {
      const clean = await this.state.storage.get<number>("noPendingDeletionsAt");
      if (clean === revision) return false;
    }
    const resp = await this.apiGet("/internal/pending-deletions");
    if (resp === null) return true;
    const pending = typeof resp.pending === "number" && resp.pending > 0;
    if (!pending && revision !== null) {
      await this.state.storage.put("noPendingDeletionsAt", revision);
    }
    return pending;
  }

  private async clusterRevision(): Promise<number | null> {
    const ns = this.env.CLUSTER;
    if (!ns) return null;
    try {
      const stub = ns.get(ns.idFromName(this.clusterName()));
      const resp = await stub.fetch("http://do.internal/revision");
      if (!resp.ok) return null;
      const body = (await resp.json()) as { revision?: number };
      return typeof body.revision === "number" ? body.revision : null;
    } catch {
      return null;
    }
  }

  private async hasUnconvergedWork(): Promise<boolean> {
    interface WorkloadItem {
      metadata?: { generation?: number };
      spec?: { replicas?: number; completions?: number };
      status?: {
        observedGeneration?: number;
        replicas?: number;
        availableReplicas?: number;
        readyReplicas?: number;
        active?: number;
        succeeded?: number;
        failed?: number;
        completionTime?: string;
      };
    }
    const lists = await Promise.all([
      this.apiGet("/apis/apps/v1/deployments"),
      this.apiGet("/apis/apps/v1/replicasets"),
      this.apiGet("/apis/batch/v1/jobs"),
      this.apiGet("/api/v1/replicationcontrollers"),
    ]);
    const [deploys, rss, jobs, rcs] = lists.map(
      (l) => (l?.items as WorkloadItem[] | undefined) ?? [],
    );
    const cron = await this.cronProbe();
    // A list call failing (null) counts as "work exists": staying awake
    // through an apiserver hiccup is cheap; parking on one is not.
    if (lists.some((l) => l === null)) return true;
    if (await this.pendingDeletions()) return true;
    for (const d of deploys) {
      const spec = d.spec?.replicas ?? 1;
      const st = d.status ?? {};
      if ((st.observedGeneration ?? 0) < (d.metadata?.generation ?? 0)) return true;
      if ((st.replicas ?? 0) !== spec || (st.availableReplicas ?? 0) !== spec) return true;
    }
    for (const r of [...rss, ...rcs]) {
      const spec = r.spec?.replicas ?? 1;
      if (((r.status ?? {}).replicas ?? 0) !== spec) return true;
    }
    // Cluster provisioning/teardown is real unconverged work too, and it
    // is the ONLY work the operator does -- without this, a Cluster
    // created on an otherwise idle management cluster would park the
    // alarm mid-provision the moment the warmup window closed. One extra
    // list, and only on the management cluster (nothing else has Cluster
    // objects). Same "a failed list means stay awake" rule as above.
    // Skipped when the operator is not running at all (a suite driving
    // Cluster objects by hand), and on tenant clusters, which hold none.
    if (this.clusterName() === "default" && this.env.CLUSTEROP_DISABLED !== "1") {
      const clusters = await this.apiGet("/apis/k8flare.com/v1alpha1/clusters");
      if (clusters === null) return true;
      interface ClusterItem {
        metadata?: {
          generation?: number;
          deletionTimestamp?: string;
          annotations?: Record<string, string>;
        };
        status?: { observedGeneration?: number; phase?: string };
      }
      for (const c of (clusters.items as ClusterItem[] | undefined) ?? []) {
        if (c.metadata?.deletionTimestamp) return true;
        // A pending rotation is unconverged work even on an otherwise Ready
        // cluster: the operator has a token to mint, a Secret to rewrite,
        // superseded tokens to revoke, and the annotation to clear.
        if (c.metadata?.annotations?.["k8flare.com/rotate-token"]) return true;
        if ((c.status?.observedGeneration ?? 0) < (c.metadata?.generation ?? 0)) return true;
        if (c.status?.phase !== "Ready") return true;
      }
    }
    // A CronJob past its slot without having run it is outstanding work,
    // and treating it that way rather than as a wake time is the point:
    // waking exactly at the schedule was measured NOT to be enough (S27) --
    // the alarm fired, re-armed for the next occurrence, and the fire was
    // dropped, because a cold KCM needs longer than one pass to load, sync
    // its informers and act. Routing it here instead hands it to the
    // machinery that already holds a window open until work converges, and
    // it self-clears: once the Job exists, lastScheduleTime advances,
    // overdue goes false, and the Job itself becomes the outstanding work.
    if (cron?.overdue) return true;

    for (const j of jobs) {
      const st = j.status ?? {};
      if (!st.completionTime && (st.failed ?? 0) === 0) {
        // Job not finished: active work unless it already succeeded.
        if ((st.succeeded ?? 0) < (j.spec?.completions ?? 1)) return true;
      }
    }
    return false;
  }
}
