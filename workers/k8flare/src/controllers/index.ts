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
import { clusterSecrets } from "../clusters/tokens.ts";
import { makeResidentBootstrapJS } from "../loader/bootstrap.ts";
import { assembleWasm, fetchWasmAsset, fetchWasmManifest } from "../loader/chunks.ts";

// Safety-net alarm interval: mirrors workers/storage's Cluster DO
// SAFETY_NET_INTERVAL_MS (workers/storage/src/index.ts) -- this is purely
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
}

// The control-plane binaries this DO hosts as dynamic workers. Separate
// binaries, not one combined: each is already close to the Loader's
// 64MiB cap alone (workers/controllers/scheduler/main.go's doc comment
// has kcm/sched's numbers; pkg/controllers/gc's doc comment has gc's --
// combining any two would exceed it). All LOADER.get() calls happen
// here in the parent -- a loaded worker cannot load further workers
// (S14 Part 6). gc (the real garbagecollector controller, replacing
// pkg/apiserver's old synchronous CascadeDeleteDependents) is
// deliberately its own isolate rather than folded into kcm: its
// dependency-graph builder needs an informer/watch per resource type
// across apidef.Table, which would compete for kcm's own 128MiB isolate
// budget (see pkg/controllers/controllermanager.go's doc comment).
const COMPONENTS = ["kcm", "sched", "gc"] as const;
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
    kcm: { entrypoint: null, loading: null },
    sched: { entrypoint: null, loading: null },
    gc: { entrypoint: null, loading: null },
  };

  constructor(state: DurableObjectState, env: Env) {
    this.state = state;
    this.env = env;
  }

  // Serializes loadComponent calls: loading a control-plane binary ends
  // in a multi-10MB WebAssembly compile that runs as NATIVE code on the
  // isolate's thread -- it cannot be interrupted, and wrangler dev runs
  // every isolate on one event loop. Three concurrent ~40-60MB compiles
  // starved a 2-vCPU CI runner for 20+ minutes with every API request
  // (and even the V8 inspector) dead in the meantime, while the same
  // runner loads a single binary fine every host-variant e2e run
  // (found bisecting the dw-control-plane e2e bring-up, 2026-07-12).
  // One-at-a-time keeps the worst case at "one compile blocks briefly",
  // and also flattens the production cold-start CPU spike.
  private loadChain: Promise<unknown> = Promise.resolve();

  private ensure(name: ComponentName): Promise<Fetcher | null> {
    const c = this.components[name];
    if (!c.loading) {
      const queued = this.loadChain.then(() => this.loadComponent(name));
      this.loadChain = queued.catch(() => {});
      c.loading = queued.then(async (f) => {
        c.entrypoint = f;
        // Warmup window: a freshly loaded component needs several pump
        // windows to get through informer sync plus the initial
        // reconcile of whatever write triggered the load -- and on a
        // cluster with zero nodes the safety-net alarm is parked, so
        // without this nothing would ever poke it again (observed live:
        // a Deployment created on an idle cluster never got its
        // ReplicaSet until a redeploy forced a reload). Bounded and
        // event-armed: only arms after an actual load, and alarm()
        // stops re-arming once the window passes.
        await this.state.storage.put("warmupUntil", Date.now() + 3 * 60_000);
        await this.state.storage.setAlarm(Date.now() + 5_000);
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
    // Manifest absent = component not shipped in this deployment.
    // Treated as absent, not an error, so pokes/alarms stay quiet
    // about it.
    const manifest = await fetchWasmManifest(this.env.ASSETS, name);
    if (!manifest) return null;
    const doName = this.clusterName();
    const token = await this.clusterToken();
    // Per-cluster dynamic worker, and the token is baked at factory time
    // (unlike the apiserver's vault-backed TokensFunc) -- so the loader
    // id carries a token fingerprint: rotation = new id = fresh isolate,
    // and the stale one is simply never addressed again.
    const tokenTag = token ? token.slice(0, 8) : "none";
    const worker = this.env.LOADER.get(
      `${name}:${doName}@${manifest.sha256}#${tokenTag}`,
      async () => {
        const wasm = await assembleWasm(this.env.ASSETS, manifest);
        console.log(`controllers: ${name} chunks assembled (${wasm.byteLength} bytes)`);
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
        return {
          compatibilityDate: "2026-07-01",
          mainModule: "index.js",
          modules: {
            "index.js": makeResidentBootstrapJS(PUMP_WINDOW_MS),
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
  private poke(name: ComponentName, request?: Request): Promise<Response> | null {
    const c = this.components[name];
    if (c.entrypoint) {
      return c.entrypoint.fetch(request ?? "http://controllers.internal/healthz");
    }
    void this.ensure(name)
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
      this.components.kcm = { entrypoint: null, loading: null };
      this.components.sched = { entrypoint: null, loading: null };
      this.components.gc = { entrypoint: null, loading: null };
      return Response.json({ destroyed: true });
    }
    // Test kill switch (see Env.KCM_DISABLED): pkg/apiserver's go test
    // suite runs against the consolidated single config, and its Pods
    // must not be touched by controllers.
    if (this.env.KCM_DISABLED === "1") {
      return Response.json({ controllerManager: "disabled (KCM_DISABLED=1)" });
    }
    // Arm the safety net if it isn't already, so a redeploy/panic/
    // eviction that resets the dynamic workers still gets noticed and
    // restarted even if no further relevant write happens to re-trigger
    // storage's pingControllers (workers/storage/src/index.ts). Cheap
    // local check -- no cross-DO call on this hot path. Armed before the
    // dispatch so a still-loading component gets a completion poke even
    // if no further write ever arrives.
    const current = await this.state.storage.getAlarm();
    await this.state.storage.put("unconvergedTicks", 0); // fresh write: reset backoff
    if (current === null || current > Date.now() + SAFETY_NET_INTERVAL_MS) {
      this.state.storage.setAlarm(Date.now() + SAFETY_NET_INTERVAL_MS);
    }

    const kcmResp = this.poke("kcm", request);
    const schedResp = this.poke("sched");
    const gcResp = this.poke("gc");
    const statuses: Record<string, unknown> = {};
    if (schedResp) {
      statuses.scheduler = await schedResp
        .then((r) => r.json<Record<string, unknown>>().then((j) => j.scheduler ?? "up"))
        .catch((err) => `dispatch failed: ${err}`);
    } else {
      statuses.scheduler = "not loaded";
    }
    if (gcResp) {
      statuses.garbageCollector = await gcResp
        .then((r) => r.json<Record<string, unknown>>().then((j) => j.garbageCollector ?? "up"))
        .catch((err) => `dispatch failed: ${err}`);
    } else {
      statuses.garbageCollector = "not loaded";
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
    // Hitting /healthz both confirms liveness and (re-)triggers the Go
    // side's ensureStarted() if a dynamic worker's isolate was evicted
    // since the last request (the Loader factory then reruns too). The
    // alarm context has no client to cancel it, so awaiting the loads
    // here is safe (and is what completes a load whose pokes all died).
    for (const name of COMPONENTS) {
      const c = await this.ensure(name);
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
    // "Work exists" for the slimmed KCM (five workload controllers) is
    // exactly workload convergence: any Deployment/ReplicaSet/Job whose
    // status lags its spec. Checked via the same gateway API the KCM
    // itself uses; 2-3 cheap list calls per tick, and only while ticking.
    const warmupUntil = (await this.state.storage.get<number>("warmupUntil")) ?? 0;
    if (Date.now() < warmupUntil) {
      await this.state.storage.put("unconvergedTicks", 0);
      this.state.storage.setAlarm(Date.now() + 15_000);
      return;
    }
    if (await this.hasUnconvergedWork()) {
      // Exponential backoff bounds the cost of work that will never
      // converge (e.g. a Deployment whose pods are unschedulable on a
      // node-less cluster): 15s doubling to a 10min ceiling. Any fresh
      // relevant write resets the cadence via fetch() below.
      const ticks = ((await this.state.storage.get<number>("unconvergedTicks")) ?? 0) + 1;
      await this.state.storage.put("unconvergedTicks", ticks);
      const interval = Math.min(15_000 * 2 ** Math.max(0, ticks - 4), 600_000);
      this.state.storage.setAlarm(Date.now() + interval);
    } else {
      await this.state.storage.put("unconvergedTicks", 0);
    }
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
    ]);
    const [deploys, rss, jobs] = lists.map((l) => (l?.items as WorkloadItem[] | undefined) ?? []);
    // A list call failing (null) counts as "work exists": staying awake
    // through an apiserver hiccup is cheap; parking on one is not.
    if (lists.some((l) => l === null)) return true;
    for (const d of deploys) {
      const spec = d.spec?.replicas ?? 1;
      const st = d.status ?? {};
      if ((st.observedGeneration ?? 0) < (d.metadata?.generation ?? 0)) return true;
      if ((st.replicas ?? 0) !== spec || (st.availableReplicas ?? 0) !== spec) return true;
    }
    for (const r of rss) {
      const spec = r.spec?.replicas ?? 1;
      if (((r.status ?? {}).replicas ?? 0) !== spec) return true;
    }
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

// (The former standalone-Worker default export that forwarded to the
// Controllers DO is gone: post-consolidation, storage's pingControllers
// reaches the DO binding directly.)
