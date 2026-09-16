# Plan: A minimal k8s control plane on Cloudflare that a stock k3s agent can join

## Goal

Run the Kubernetes control plane on Cloudflare Workers and Durable Objects
with as little code of our own as possible: k8s.io/apiserver serves the API,
k3s's own agent runs the node, and k8flare is only the glue between them and
the platform. The first milestone, reached on 2026-09-13, is one OrbStack VM
joining, going Ready, running a Pod that answers on its Pod IP and streams
its log, and removing it again.

## Locked decisions

- **k8flare is glue, not a reimplementation.** Where upstream code has to
  change, the change is a sha256-pinned overlay applied by `scripts/mirror` at
  build time, never an edited copy. No per-resource handling in k8flare code
  where upstream can do it; the remaining per-kind branches are listed under
  Known limitations as debt.
- **External types only.** Linking upstream's registries (internal types,
  `printers/internalversion`) was measured at 92MB after wasm-opt against
  the Worker Loader's 64MiB cap. The API is k8s.io/apiserver's installer over
  `genericregistry.Store`, and only the defaulters from
  `k8s.io/kubernetes/pkg/apis/*/v1` are linked (+3.5MB).
- **Size is a gate, not a guideline.** `make wasm` fails above 67,108,864
  bytes per binary. `wasm-opt -Oz` is the slow step (43s for the
  scheduler on 16 cores; `-Os` would save 7s and cost 5.3MB, `-O1` lands
  at 63.8MB, so -Oz stays); a warm `go build` is 1.5s and byte-identical
  when the binary's own inputs did not change, so the Makefile keeps the
  intermediates (`.SECONDARY`), records the raw binary's sha256 next to
  each `.opt.wasm` and skips wasm-opt when it matches, and bounds
  wasm-opt to `BINARYEN_CORES=2` under `--jobs=8` (8 unbounded wasm-opt
  processes on 16 cores took over 30 minutes for the full set). Current: front 39.0MB (with the RBAC authorizer);
  group workers 28.7–43.1MB; openapi 58.5MB; customresources 59.1MB;
  controllers 48.8MB (with the garbage collector);
  node-tunnel 14.4MB (bundled into the shell); scheduler 55.1MB (109.9MB before the lean clientset and informer factory overlays and the two files that dragged the fake clientset and cri-client in);
  printers-core 43.0MB, the other printer groups 13–27MB. Before the
  clientset-scheme / APF / StorageVersion overlays the one-binary apiserver
  was 57.5MB and the CRD handler 73.5MB; `k8s.io/api` alone was 21.4MB of
  code in every binary because those packages' `init` registrations keep
  every type reachable once the package is imported.
- **One dynamic worker per API group, loaded on first use.** The front
  worker only authenticates and routes; each served group, the CRD handler,
  OpenAPI, the scheduler and each printers group is its own binary, so a
  request loads only what it touches and every binary stays under the cap.
  Two js overlays make that possible without touching what upstream does:
  the clientset scheme registers nothing (each worker registers the groups
  it imports), and the APF filter and StorageVersion manager no longer pull
  every group's informers and typed clients.
- **One resident Go instance per isolate**, dispatched per request by the
  Loader bootstrap. Go timers and fetches only live inside a request
  context, and the Go runtime keeps a single scheduled wake-up, so once a
  context ends every goroutine waiting on a timer stalls until the next
  request enters the instance (observed 2026-09-13: a custom resource
  create finished only when the next request arrived, and workerd reported
  the isolate as hung). The bootstrap therefore keeps each request's
  context open for a pump window after responding and re-enters Go every
  250ms during it: 5s for group workers, 30s for customresources, the
  scheduler and the controllers, whose controllers otherwise only run
  while pumped. A watch stream whose request context ended is not reported
  closed to Go either (a body read blocks forever), so the resident
  scheduler and controllers give every informer watch a 15s lifetime
  through `BindingTransport.WatchLifetime`; the reflector re-watches from
  its last resource version, and the instances themselves stay resident
  (one `RegisteredNode` event per node, node health probes that outlive a
  poke). A Go instance that exits or fails to instantiate is logged by the
  Loader bootstrap and dropped, so the next poke instantiates again, and
  the poke chains log their failures; before that a dead controllers
  instance was invisible (2026-09-14, after a burst of node and namespace
  deletes, no controller acted for 15 minutes and nothing was logged). The
  first death the logging caught was the core group worker: Go's wasm
  runtime reported "all goroutines are asleep - deadlock!" with the
  `ws-message` callback blocked on a full 256-slot kine watch channel
  while every other goroutine waited on a `setTimeout(0)` that could not
  fire with JS stuck inside Go; the isolate had been frozen for the whole
  advisory run before the detector fired and the bootstrap
  re-instantiated it. No JS callback in the bridge may block now: the
  WebSocket callback appends to a queue and a Go goroutine drains it into
  `Messages`; a watch whose consumer never drains (a response stream that
  died unnoticed) is closed once 32 MiB is queued, which ends the kine
  watch and makes the reflector relist. The resident scheduler and
  controllers also use upstream's client QPS/burst (50/100 and 20/30)
  instead of rest.Config's 5/10, which throttled them under e2e load. The
  next hour-long run ended with the core group worker at 4.1 GB of Go
  heap ("fatal error: out of memory") because a watch's server side never
  learned that its client had gone: the DO's `watchers` count (now in
  `GET /stats`) grew by 36 every 30s at idle, one per informer re-watch,
  and killed `kubectl -w` sockets stayed too. `BindingTransport` now ties
  every fetch to an `AbortController` fired by the request context or by
  closing the body, so a client's cancel reaches the front, the group
  worker's `stream-cancel`, and the DO socket, and the Loader bootstrap
  hands each request's `signal` to Go. Measured in wrangler dev
  (2026-09-14): neither the ReadableStream `cancel` nor the request abort
  fired once, for a killed external `curl -N` watch or for a Go-side
  abort across a service binding (0 callbacks over 3 minutes while the
  socket count rose from 55 to 112), so no cancellation signal reaches a
  callee in this runtime and the hooks stay in place only for Track 8 to
  test at the edge. The constraint is per request, not per isolate: a
  subrequest stream is delivered only while the request that opened it is
  alive (its handler plus the pump `waitUntil`); once that poke's window
  closes, every stream it opened is silently dead even while later pokes
  keep the isolate busy (measured: an external `curl -N` saw the DO close
  its watch as EOF after 69s, while the continuously poked controllers
  isolate never saw the same close on its own streams and its caches went
  stale, 0/15 required specs). So each accepted poke ends every stream the
  resident worker still holds (`BindingTransport.AbortOnWake` tracks them,
  `bridge.EndTrackedStreams` closes the pipes and aborts the fetches) and
  the informers re-watch from their last resource version inside the new
  request. The server side is bounded by the DO: any watch socket older
  than `WATCH_LEASE_MS` (60s) is closed on the next write or watch accept,
  and a socket whose reader falls 1 MiB behind is closed by the group
  worker (the 4.1 GB heap was about 120 dead sockets each buffering to
  the earlier 32 MiB cap). Node status writes poke the resident workers
  only when the spec, labels, allocatable or Ready condition changed, so
  kubelet heartbeats no longer drive the re-watch rate. wrangler dev does
  not enforce the 128 MB isolate limit; production does, so the leak
  would have taken minutes there instead of an hour. A goroutine parked
  in `await(reader.read())` on a dead stream never wakes; that leaks a
  few KB per informer per wake and is accepted.
- **Open: the first namespaced write after a cold start fails.** With a
  fresh store and no node agent, `POST` to a namespaced resource returns
  500 ("Network connection lost" from the runtime) after 16-26s, and the
  next identical write succeeds in tens of milliseconds; the failure
  coincides with the controllers worker's cold start, whose informers
  flood the single-threaded Cluster DO. It is not new: bisecting to
  `b88f350`, before the finalizer and socket work, reproduces it exactly
  (500 after 23.7s, then 97ms). The node agent's steady traffic keeps the
  workers warm, which is why kubectl and `make e2e` almost never see it
  and `go test ./packages/apiserver/` almost always does. Reproduce with:
  `rm -rf .wrangler/state`, stop the agent, `make dev`, then one GET and
  two POSTs against `/api/v1/namespaces/default/configmaps`.

  Upstream's client rates assume an apiserver that scales out; here every
  request funnels into one DO thread, so the resident workers now use
  rates chosen for the store (controllers 5/10, its metadata client 5/5,
  scheduler 10/20) rather than kube-controller-manager's 20/30 and
  kube-scheduler's 50/100. One measurement had that turn the cold write
  into a 3.9s success and take the harness lane from nine failures to
  three, but the lane is bistable: the same tree gives three failures in
  one run and eight in the next, so treat the rates as a cost decision
  that is right on its own terms and the cold-start failure as open. A
  fire-and-forget poke (`bridge.Notify`) was tried and reverted; it did
  not help, and an un-awaited subrequest is its own hazard.

- **The 128MB isolate cap was hit by discovery, and is fixed.** A client
  walks every API group before its first request. Forwarding that walk
  cold-started all twelve group workers at once, and several 28-43MB wasm
  modules in flight crossed the isolate's memory limit: production
  reported 98 `exceededMemory` outcomes and 101 "Worker exceeded memory
  limit" exceptions over one run, and `kubectl api-resources` failed for
  every group. The 64MiB the group split was sized against is the Worker
  Loader's *code* cap, a separate budget from isolate memory. The front
  now answers `/api/v1`, `/apis/<group>` and `/apis/<group>/<version>`
  from the generated `registry.Served` table through upstream's
  `discovery.NewAPIVersionHandler`, so a group worker loads only for real
  resource traffic. After the fix, the same run reports 0
  `exceededMemory` and `kubectl api-resources` returns 37 rows with no
  errors.

- **Two more faults that only production shows.** A `scheduled()` handler
  that returns before its work is done has that work cancelled: the cron
  fired in 2ms and the wake it asked for never happened, so the
  entrypoints gained `run()`, which awaits the poke, and the Cron Trigger
  calls that instead of the fire-and-forget `poke()` a write uses.
  Separately, `SharedInformerFactory.WaitForCacheSync` reports only
  *started* informers, so a factory that has not started yet returns an
  empty map, which read as idle: a cold worker answered its own poke 204
  after 1.5s and the window closed before anything ran. `Idle()` now
  returns false until `Run` has started every controller, and the cron
  invocation went from 2ms to a full 20.6s window.

- **Open: the controllers cannot finish a cold start inside one window on
  production.** Deployments do not produce a ReplicaSet there, while the
  same build locally creates one in 2s and the default ServiceAccount in
  3s. Loading 48.8MB of wasm and syncing sixteen controllers over roughly
  twenty informers does not fit the 20s poke window, and the chained
  wake does not appear to inherit a warm isolate, so each attempt starts
  over. This is a startup-budget problem, not memory; the plan's own
  "split if the cap is hit" note (workload controllers versus node and
  endpoint controllers) is the lever to try next.

- **Cost: what the pre-rewrite tree already solved, and what carried over.**
  Billing has four axes and only one of them is over its allowance.
  Cloudflare bills neither subrequests nor service-binding hops, so the
  hop count is an operational number, not a cost; Durable Object duration
  at 128MB is 324k GB-s for a permanently resident object, structurally
  under the 400k allowance for one object; DO requests cost $0.04. Worker
  **CPU** is the whole bill above the $5 floor.

  Dynamic Workers are separate Workers, so the parent's tail never showed
  theirs: attaching the parent as their tail consumer (`tails` on the
  loader code object) revealed 63% of CPU living there, and the true idle
  figure is 90.9M CPU-ms/month, not the 75.9M the parent alone reported.

  Holding the wake with a streaming response instead of `waitUntil` took
  that to 62.1M (-32%): `waitUntil` is capped at 30s while an HTTP
  response has no cap, so watches now live for the window rather than
  being re-established on a 30s treadmill.

  The pump's 250ms tick is **not** the remaining cost. Measured at 250ms,
  500ms and 1000ms, the first two are identical (2,105 and 2,117
  CPU-ms/min) and CPU tracks the invocation count at a flat 10-13 CPU-ms
  each, so an earlier single-window "34% saving" was withdrawn.

  The pre-rewrite tree solved the underlying problem differently and its
  notes are worth reading before trying again
  (`backup/pre-rewrite-2026-09-13`: `pkg/cfruntime/cloudflare/window.go`,
  `packages/k8flare-worker/src/loader/bootstrap.ts`, `docs/cost-model.md`,
  `docs/pump-window-design.md`). Bindings are request-scoped I/O objects:
  one captured at instantiation stops settling its promises once that
  request is torn down, with no error, which is recorded there as S31 and
  is the same silent watch death rediscovered here. Its answer was an I/O
  anchor - `openPumpWindow(env, ms)` hands Go the live request's env, one
  `setTimeout` closes it, the window self-closes on a grace timer in case
  the close is dropped, and background goroutines block on
  `CurrentWindow` rather than ticking. Its cost invariant was that an
  idle cluster dispatches nothing at all: no cron, and a safety-net alarm
  that self-parks when no node exists.

  **Porting the anchor's JS half alone made things worse** and was
  reverted: removing the tick without the Go-side `CurrentWindow`
  integration leaves timers unadvanced, and the reflectors spin on
  immediate retries - 573 invocations/min and 4,110-7,018 CPU-ms/min
  against 128 and 1,438. A full port has to move the Go side onto windows
  too. The other half of that design, returning the wake model to
  event-armed with a self-parking safety net instead of today's
  unconditional per-minute cron, is independent of it and is the larger
  remaining lever for an idle cluster.

- **Track 8 measured on production (2026-09-15, `k8flare.kooffice.workers.dev`).**
  The account already held a `k8flare` worker from the pre-rewrite
  architecture (10 deployments to 2026-09-11) whose `Cluster` DO blocked
  the `new_sqlite_classes` migration; it was deleted on the user's
  instruction and the current tree deployed in its place (868MB of wasm
  assets, 79 files, 160s upload). Tokens are fresh production secrets,
  not the dev ones.

  | measurement | result |
  | --- | --- |
  | namespace to default ServiceAccount and kube-root-ca.crt | 22.5s |
  | CRD to Established | 1.45s |
  | pod to bound | not measured, no node can join |
  | namespaced writes | 201 in 2.7-4.2s |

  Both timed paths completed, so **the 30s pump survives production's
  `waitUntil`** and the planned contingency (drop `pumpMs` to 25s) is not
  needed. The local cold first-write failure did not reproduce here.

  The binding constraint is memory, not the pump. Over the run
  `wrangler tail` recorded 1,544 events: 966 ok, 468 canceled, 98
  `exceededMemory`, 6 exception, with 101 "Worker exceeded memory limit"
  exceptions. Every one of them is a discovery request for
  `/apis/<group>/<version>` against a per-group worker. The group split
  was sized against the Worker Loader's 64MiB *code* cap, but discovery
  asks for every group in turn, so several 28-43MB modules instantiate in
  one isolate and cross the 128MB *isolate memory* cap. wrangler dev does
  not enforce it, which is why this never appeared locally. Remedies to
  weigh: fewer, smaller group workers; evicting a loaded module after
  use; or serving discovery from a static document so the group workers
  are only instantiated on real resource traffic.

  The pod-bind timing needs a node, and none can join production: the
  agent trusts the dev CA that `devtls` presents while the edge presents
  Cloudflare's, and `/v1-k3s/connect` needs the vault seeded with a node
  password.

- A poke must not make the write that triggered it wait for a cold
  worker. `Scheduler.poke` and `Controllers.poke` awaited
  `loadWasmWorker` before handing the fetch to `waitUntil`, so the first
  write against a cold cluster blocked while 48.8MB of wasm was assembled
  and instantiated: measured as a 21s gap in the log between the CRD
  informer starting and the controllers informers starting, long enough
  for the front's subrequest to be dropped ("Network connection lost").
  The load now happens inside `waitUntil`.

- **A Cron Trigger wakes the resident workers every minute** (decision
  2026-09-14, replacing "no alarms, no polling"): `scheduled()` in the
  shell awaits `Scheduler.poke()` and `Controllers.poke()`, so timers that
  only advance while pumped (nodelifecycle's monitor, workqueue AddAfter
  retries, CronJob, the garbage collector's 30s discovery sync) get one
  window per minute even with no writes. Cost: 1,440 invocations a day
  per worker, each a poke that returns 204 within 1.5s when idle plus its
  pump window. The garbage collector (upstream's, over a metadata
  informer per served resource and a deferred discovery RESTMapper)
  joins the controllers worker on the same wake model; its monitors are
  re-established per wake like every other informer.

- **The cron is gone; a self-parking Durable Object alarm wakes the
  resident workers** (decision 2026-09-16, replacing the per-minute
  cron). The cron held both workers for 55s of every minute, which paid
  near-resident informer CPU *and* a full watch re-establishment per
  minute, and every kubelet lease renewal opened a further 20s window on
  the controllers, so neither worker ever went idle. Now a window closes
  as soon as the worker is idle (`bridge.Hold`: caches synced, scheduler
  activeQ and in-flight empty, controllers workqueue depths zero via
  `workqueue.SetProvider`) and the response reports how soon the worker
  wants to run again: pending work re-arms the `Wake` Durable Object's
  alarm with a delay that doubles while the pending set is unchanged
  (15s to 5min). The controllers keep a 5-minute safety net while a node
  exists (nodelifecycle, CronJob, AddAfter retries) with a 60s minimum
  hold so the node monitor can observe past its grace period, and park
  when no node exists. Lease writes no longer poke; pod deletion now
  pokes the scheduler as well. Verified in wrangler dev: an unschedulable
  pod produces `wake scheduler in 15000ms`, the alarm runs, then
  `30000ms`; the controllers schedule nothing with no node. The
  production cost effect and the isolate-eviction risk (a sparse wake
  may cold-start 48-55MB of wasm) are unmeasured; use the 6-minute
  `wrangler tail` protocol below.

  **Measured on production (2026-09-16, same node `k8flare-c1`, no
  workloads).** A same-day baseline of the previously deployed build
  (the shipped `c3cade6` state) was taken first, then this build was
  deployed and tailed 3 minutes later. The baseline tail was sampled:
  it captured 14 of the kubelet's 36 ten-second lease writes, so its
  numbers are undercounts; the new build's tails captured 35/36 and
  65/65.

  | window | CPU ms/min | /mo | watch/min | hung | cron | notes |
  | --- | --- | --- | --- | --- | --- | --- |
  | baseline, 6 min (sampled) | 3,372 | 145.7M | 145 | 27 | 1 seen | true figure higher; plan's earlier reading of this state was 5,611-5,763 |
  | this build, 6 min, first deploy | 203 | 8.8M | 1 | 3 | 7 | not valid: the cron trigger survived a deploy that merely omitted `triggers`, and no controllers window had run since the deploy, so no safety net was armed |
  | this build + `crons: []` + arm-on-connect, 6 min | 2,824 | 122.0M | 21 | 2 | 1 (pre-redeploy) | contains the controllers' post-deploy cold start: 66s hold at 1,725 ms, then ~60 watches cancelled at 100-800 ms each (relist) |
  | same, 16 min, three safety-net cycles | 1,071 | 46.3M | 24 | 3 | 0 | minute 0 carried 7,866 ms from the previous cycle's teardown; minutes 2-15 average 437 ms/min (18.9M/mo) |

  Per safety-net cycle on a warm isolate (cycles 2 and 3 of the 16-minute
  window): the 60s hold costs 720-725 ms, the ~60 watch streams it opened
  end at 2-16 ms each when the window closes, and the whole cycle lands
  in two minutes at 1,600-1,800 ms each; the other three minutes of the
  cycle sit at 30-150 ms. The first cycle after a deploy is the only one
  with relist-sized cancels (98-368 ms), so the isolate survives the
  5-minute gap and the eviction risk did not materialise at this cadence.
  The scheduler was never woken: nothing was pending. Two mechanisms were
  needed on top of the design to make it hold in production: `"crons":
  []` in `wrangler.jsonc`, because wrangler leaves an existing trigger
  in place when the key is absent, and arming the controllers' safety
  net on `/v1-k3s/connect` when nothing is scheduled, because a node
  joined before the deploy never writes anything that opens a
  controllers window.

- **No periodic wake at all; deadlines are booked by whoever knows them**
  (decision 2026-09-16, replacing the 5-minute safety net and the
  `Wake` Durable Object, which is deleted in migration v4). The Cluster
  DO owns the single alarm: `POST /wake` books a target and a minimum
  hold, and every write under `/registry/leases/kube-node-lease/`
  records the node's last lease time, so a node whose lease stops for
  60s wakes the controllers with a 60s hold (nodelifecycle then needs
  its own 50s grace, so NotReady lands about two minutes after the last
  lease). The batch worker books the CronJob's next schedule with
  robfig/cron on every CronJob write. The controllers count workqueue
  retries in the same provider as depths and report them as pending, so
  a rate-limited requeue re-books a wake with the pacer's backoff. The
  garbage collector's discovery sync moved from 30s to 10 minutes; it
  only advances while a window is open, so in practice it runs once per
  cold start, and a CRD added later is not monitored until then (known
  gap). With a healthy node nothing wakes: the only recurring cost is
  the kubelet's lease write plus one alarm row write each. Verified in
  wrangler dev: a CronJob create logs `wake controllers in 94185ms`, a
  lease left unrenewed logs `lease expired: s2` and `wake controllers`
  after 60s, and the CronJob alarm fires on the next boundary.

  **Required e2e (2026-09-16, dev stack, OrbStack node `k8flare-agent`):
  18 passed / 3 failed.** The three failures are the CRD specs, and the
  wrangler dev log shows the two `customresources` "hung" cancellations
  behind them; the other CRD specs fail on the deadline that follows.
  The previous readings of this set were 16/5 and 17/4, so removing the
  safety net exposed no periodic dependency in the set.

  **Measured on production (2026-09-16, `k8flare-c1`, no workloads).**

  | window | CPU ms/min | /mo | notes |
  | --- | --- | --- | --- |
  | 6 min from 3 min after deploy | 1,647 | 71.2M | post-deploy transient: 14 of 33 lease writes were cold loads of the coordination worker (parent ~280 ms to assemble 33MB + ~330 ms to instantiate); no wake fired |
  | 12 min starting 12 min after deploy | 504 | 21.8M | minute 0 carried 3,657 ms from the transient; minutes 1-11 average 217 ms/min (9.4M/mo); 1 of 69 lease writes cold |

  The steady state is a floor of 70-160 ms/min (the kubelet's lease
  write every 10s, its node status, and the tunnel reconnect) plus one
  spike of about 1.5s every six minutes. That spike is the kubelet's
  watches on `nodes`, `endpointslices`, `runtimeclasses` and
  `csidrivers` being re-dialled after the store's 6-minute
  `WATCH_LEASE_MS` closes them, and each landing on a group worker whose
  isolate went idle in between, so each pays a cold load. Nothing woke
  the scheduler or the controllers in either window. The 3-minutes-after-
  deploy protocol undercounts warm-up for this build: with no resident
  worker touching the group workers, the post-deploy cold loads spread
  over the first ten minutes, so measure from ten minutes after deploy.
  Keeping the kubelet-facing group workers warm would remove that spike,
  but the decision (2026-09-16) is not to: about 6M CPU-ms/month sits
  inside the included 30M, and a cold load delays a kubelet re-watch by
  well under a second.

- **Required e2e against production (2026-09-16), and the two gaps it
  found.** The first run on the deadline-driven build scored 11/10: the
  `default` ServiceAccount never appeared in new namespaces and
  ReplicaSets were never collected, because the controllers never
  finished a cold start. Two mechanisms were missing. (1) Under e2e load
  the parent isolate dies for memory (28 "Worker exceeded memory limit")
  and hung requests (43), which kills a write poke's `waitUntil` and the
  wake it would have booked at its end; the cron used to hide that with
  an unconditional retry. Every poke now books an insured retry 25s
  ahead *before* opening its window, and a window that closes idle
  settles the insured rows for its target. (2) A poke window answered
  204 to the alarm-driven run, so the long hold never happened; a run is
  now refused only by another run, opens a second window beside a poke,
  and gets a 5-minute window so a cold start completes, re-booking
  itself if it fails. With both: 18/3, the three being the `[Serial]`
  scheduling specs, which failed on the node, not the control plane: the
  scheduler bound the pods, and the kubelet reported
  `FailedCreatePodSandBox` because flannel never wrote
  `/run/flannel/subnet.env`, because the `k8flare-c1` VM had no
  `iptables` binary (`iptables binary was not found`); the dev VM has
  it. With iptables installed and the agent restarted: **17/4**, the
  four being two `customresources`/`APIGroups` hung cancellations, one
  cleanup step hit by the same, and one scheduling spec whose bound and
  running pod carried a stale `PodScheduled=False` from a failed attempt
  that landed after the bind. Same standing as the 16/5 and 17/4 the cron
  builds scored; the failure class left is production's hung/memory
  instability, which is prior to this work. CPU during the run: 6,672
  ms/min over 13.9 minutes, 78 hung, 11 memory kills.

  **The production e2e series that followed (2026-09-16, all against
  `k8flare.kooffice.workers.dev`, no local runs).** Each row is one
  required-set run; the fix column is what was deployed before it.

  | run | fix deployed before it | result | what the tail said |
  | --- | --- | --- | --- |
  | 4 | per-request JS keepalive in the loader bootstrap (`setInterval` while Go handles a request, so a handler blocked on a Go channel still has something pending in its own IoContext) | 18/3 | hung 78 → 0, memory kills 11 → 0 in 14 minutes; the three failures were client deadlines |
  | 5 | (none) | 17/4 | GC "orphan pods" spec: `expect 50 pods, got 142`, the replication controller over-created |
  | 6 | prefer the alarm-driven run window for outbound I/O | stalled, killed | API 2-6s per request, a namespace stuck Terminating; reverted |
  | 7 | drain in-flight unary fetches for up to 5s before a window closes, abort only response streams; `pods/status` keeps `PodScheduled=True` once `spec.nodeName` is set; tail forwards `bridge:` / `pods/status:` markers | stalled, killed | 38 memory kills in the first minute, all in the parent isolate: the discovery walk loads every group worker at once and each load holds a 30-56MB assembled binary |
  | 8 | loads serialized per isolate (peak one binary) | **19/2** | hung 0, memory 0; markers showed `fetch lost to window close` for GC controller GET/PATCH/DELETE (the 30s waitUntil cut, which drain cannot reach) and `pods/status: kept PodScheduled=True` ×7 |
  | 9 | (none) | stalled, killed | API 4-6s per request again, store `watchers: 0`, the kubelet failing its own GETs; group workers were fast (APIGroups median 32ms) and the time sat in the front |
  | 10 | pods created as `Pending` with a QoS class (upstream's create strategy; the NodeSelector spec polled a pod with an empty phase and returned early); poke windows 10s so drain fits under the cap | not run: preflight found no Ready node | the node had been `Unknown` since 14:12Z: its lease and status writes were timing out against a 3-7s API, and the store's lease-expiry alarm did its job |

  **Open at the end of the day: every request pays a cold load.** From
  about 14:12Z the API answered simple GETs in 3-7s. A hop-by-hop trace
  of `GET /api/v1/nodes` shows the front's parent invocation at 260-560
  ms CPU (assembling the 37MB front binary), the `APIGroups` invocation
  at 250-320 ms CPU (assembling the core binary) and the core worker's
  own invocation at 480-665 ms CPU (instantiating it), on every probe,
  4s apart, while the coordination worker answered the kubelet's lease
  reads warm at 56 ms. Back-to-back namespace reads went 2.0s, 0.1s,
  5.6s: the loader cache works, but consecutive requests land on
  different parent isolates, each with its own empty cache. Nothing in
  the forwarded logs shows a Go exit, a panic or a memory kill, and
  reverting the serialized loader did not change it (`edec9b1`). This
  is the platform spreading the script over many isolates after a day
  of deploys and e2e load; whether it settles on its own is unmeasured.
  Until it does, the required set cannot complete: the kubelet's 10s
  client timeout and the suite's waits are shorter than the cold loads.

  The NodeSelector failure in run 8 was the empty phase, not the
  scheduler. Run 8's other failure (`expected 25 pods, got 24`) and the
  stalls in runs 6, 7 and 9 share a shape: after a deploy, or under the
  e2e's four-way parallelism, parent isolates churn, every request pays
  a cold load of 3-5s, the kubelet's watches cannot be re-established
  faster than they die, and the suite's namespace and ServiceAccount
  waits (5 minutes each) stack up. That churn is the platform's, and
  the loader design pays for it in full; nothing periodic will fix it.

  Two node-side facts recorded on the way: the agent's tunnel reconnects
  every ~63s with `close 1012: no tunnel session, reconnect`, which is
  the NodeTunnel DO's in-memory `attached` flag not surviving
  hibernation; and `nodes` PATCH/status writes from a fresh agent fail
  for the first ~20s after a deploy while the group workers cold start.

- **Advisory e2e (2026-09-14, per focus group, one wrangler per group):**
  Garbage collector 3/7 after the collector landed (was 1/7): deletion
  cascades work, and the orphan and foreground cases were failing because
  the generic store left `EnableGarbageCollection` false, so it never
  wrote the `orphanDependents`/`foregroundDeletion` finalizers and every
  policy degraded to background; upstream defaults that flag to true
  (`pkg/server/options/etcd.go`) and both the generic store and the CR
  stores now set it. LimitRange defaults 0/1 (no LimitRanger admission,
  internal types); ConfigMap 2/6 and Secrets 1/5 (empty-key validation
  lives in upstream's internal-type strategies; pod log reads returned
  "unknown", unexplained); Namespaces [Serial] 5/8 (100 namespaces do not
  delete fast enough, and a Service-removal case timed out); ReplicaSet
  4/7 and Deployment 8/11 (`resourcequotas` and the scale subresource are
  not served; reaching a replica pod from the runner needs pod
  networking); Pods and Job exceeded the 25-minute cap; Services and
  ServiceAccounts did not run.

- The store's `strategy` does not implement
  `GarbageCollectionDeleteStrategy`, so a delete with no propagation
  policy is background for every resource; upstream's v1
  ReplicationController defaults to orphan instead. kubectl has sent an
  explicit policy since 1.20, so neither kubectl nor e2e sees the gap.

- The CRD specs fail once the cluster has been idle for more than the
  60s watch lease: the establishing controller in the customresources
  worker stops reacting, so new CRDs never reach Established and the
  discovery wait times out (26 creates against 3 status writes in one
  run). Measured 2026-09-14: a CRD created immediately after a restart is
  Established in 1s; after 180s of idle none is, at one or four ginkgo
  processes, with or without load in between, and the worker logs
  nothing at all for two minutes. The lease closes the socket while the
  isolate is suspended, and `WebSocket.Close` only asked JS to close and
  waited for the close event to run `finish()`, so that event never
  arrived and the Go side kept the watch: kine's reader stayed blocked
  and the reflector believed its watch was healthy. `Close` now finishes
  the socket itself, and sockets dialed outside a request (an informer's
  own watch, never one serving a client) are closed at the start of the
  next wake so the reflector re-dials inside a live request.

- The 60s lease also means an external `kubectl -w` is disconnected once
  a minute (measured: EOF at 69s) and reconnects from its last resource
  version, which is within watch semantics but visible in client logs. Watches stream from it (Content-Encoding: identity, or
  the runtime gzips JSON and holds the stream until it closes).
- **The agent is k3s.** `packages/agent` embeds `k3s/pkg/agent` unchanged except
  the `deps.KubeConfigOverride` hook, because TLS terminates at the edge and
  client certificates never reach the control plane. Node identity on the
  API is the bearer token `node:<name>:<node password>`, the secret k3s
  already registers with the supervisor.
- **Local verification only** while GitHub Actions is off: `make test`
  starts `wrangler dev` itself; the node path is checked by hand against
  `devtls` and an OrbStack VM. The harness tests are smoke tests for the
  worker plumbing; the definition of done is upstream's `e2e.test` run by
  `make e2e` with the narrowed focus sets in `scripts/e2e/focus.go`
  (Sonobuoy would need Services, kube-proxy and CoreDNS first). The
  framework's `BeforeEach` waits for the `default` ServiceAccount of each
  test namespace, which only kube-controller-manager's serviceaccount
  controller creates, so the gate depends on the controllers worker: the
  first run (2026-09-14) failed all 16 required specs there, one baseline
  was taken with `--e2e-verify-service-account=false`, and the controllers
  worker carries the serviceaccount controller and is poked on namespace
  writes so the flag is not needed. The same `BeforeEach` then waits for
  `kube-root-ca.crt`, so the worker also runs root-ca-cert-publisher with
  the CA the supervisor serves at `/cacerts` (the cluster's server CA; a
  production edge presents Cloudflare's certificate instead, which only
  matters once pods can reach `kubernetes.default`). Namespace deletion follows
  upstream's life cycle: the core worker's `namespaces` deleter marks the
  namespace Terminating (the `registry.Deleters` hook, transcribed from
  upstream's namespace REST) and serves `namespaces/finalize`, and the
  controllers worker runs upstream's namespace controller, which empties
  the namespace through the metadata client and discovery before the final
  delete; without it, deleted namespaces left their pods behind and the
  scheduling specs' "stable cluster" wait never returned. Every
  group worker rejects creates in a Terminating namespace with the
  Forbidden status upstream's NamespaceLifecycle admission produces
  (`registry.NamespaceLifecycle`, one kine read of the namespace per
  create); without that cause the root CA publisher and serviceaccount
  controllers recreate their objects while the namespace controller
  empties the namespace, and the two back off against each other for
  minutes. Custom resource creates go through the customresources worker
  and are not guarded yet. With the serviceaccount, root-ca-cert-publisher and namespace
  controllers in place the required set passes 15/15 (2026-09-14,
  `PROCS=4`, 104s).

## Current-state anchors

- `packages/apiserver-registry/zz_generated_resources.go` (from `scripts/genresources`): the
  served surface, filtered from upstream's discovery documents.
  `packages/apiserver-registry` builds a generic store for each; pods get upstream's
  graceful-delete rule, nodes a static PodCIDR in `BeginCreate`.
- `packages/apiserver-kine`: `storage.Interface` over the Cluster DO's
  revisioned key-value log; `Watch` dials the DO over a WebSocket and emits
  the WatchList bookmark at the end of the snapshot.
- `packages/apiserver-supervisor`: the nine k3s join endpoints,
  CSR signing, node passwords, CAs in the DO under `/vault`.
- `packages/worker-bridge`: the Go↔Loader bridge (streamed responses, WebSocket client).
- `packages/cluster-store`: the Cluster DO; `packages/control-plane-worker`:
  bootstrap, chunk assembly, routing and the tunnel hand-off. The DO stub is
  handed to the dynamic worker's env directly; the old finding that a
  Loader env cannot carry a DO covered namespaces, not stubs.
- `scripts/mirror/main.go`: the overlays (apiserver storage factory, tracing
  exporter, CEL parser, installer hub version, k3s kubeconfig hook).

## Design

See README.md for the component list. Two things that are not obvious from
the code:

1. **Why WatchList matters.** client-go's reflectors ask for
   `sendInitialEvents=true` and wait for a BOOKMARK annotated
   `k8s.io/initial-events-end`. Without it nothing syncs, and the kubelet's
   protobuf informers were the only ones that appeared to work because the
   runtime does not compress protobuf. The DO sends `snapshot-end`; Go turns
   it into the bookmark.
2. **Why wrangler dev runs without CLAUDECODE.** Its AI-agent mode buffers
   JSON responses for its Local Explorer, which stalls JSON watches the same
   way gzip does.

## Commit sequence

Done, on `feat/minimal-rewrite`:

1. Size gate (`scripts/mirror`, then under hack/; probe measurements in the commit message).
2. apiserver on the Loader with the Cluster DO as its store; ConfigMap verbs
   through client-go.
3. Watch from the resident instance.
4. Supervisor, node identity, the agent embedding.
5. First pod: join, run, reach, log, delete.
6. Auth fixes (node tokens cannot self-register; empty join token opens
   nothing) and the pod lifecycle test.

Next:

7. The `/simplify` review: upstream registrations instead of hand-written
   conversions, generated resource table, shared JS callbacks, the DO's
   single-insert write path.

Next:

8. Repository layout: root `wrangler.jsonc`, `packages/{component}-{part}`
   (plans/repository-layout.md). Done 2026-09-13.

Next: see Known limitations, in this order — compaction of the DO log,
the controllers (kube-controller-manager) and the production deploy check.

## Known limitations

- **Per-kind code that remains**, all in `packages/apiserver-core` and
  registered through the registry's hooks: `podStrategy` (upstream's
  graceful-delete rule, which upstream keeps on the internal Pod type),
  `assignPodCIDR` (the nodeipam controller's job until controllers run),
  the namespace and kubernetes-Service bootstrap, `pods/binding`
  (upstream's BindingREST lives on the internal Pod type), and the
  scheduler and controller wake-ups. PodCIDRs come from the real nodeipam
  controller now. The served resources
  themselves are generated from upstream's discovery documents
  (`scripts/genresources`), and field labels, defaults and PodLogOptions
  come from upstream's `AddToScheme`.
- **The scheduler runs only while woken.** A Pod written without a node
  wakes the scheduler worker, which runs the real kube-scheduler and its
  informers in that isolate for a bounded window per wake-up; nothing keeps
  it alive at idle. A poke holds its request until the active and backoff
  queues drain (20s at most) and answers 202 while work remains, which the
  entrypoint turns into the next poke. Pod writes without a node and every
  Node write poke it, so a Pod created before its Node is retried when the
  Node arrives. The same write hooks poke the controllers worker
  (`packages/controllers`): writes to pods, nodes, services, endpoints,
  replicationcontrollers, apps/batch/discovery resources and leases, so the
  real controllers (deployment → replicaset → pods, nodeipam PodCIDR,
  nodelifecycle taints, endpoints and endpointslices, jobs and cronjobs)
  run only while there is work; `Idle()` reads the workqueue depths through
  client-go's workqueue metrics provider. The scheduler's informers are why apps/v1, policy/v1,
  resource.k8s.io/v1 and replicationcontrollers are served: an informer
  on an unserved resource never syncs and the scheduler never starts. Controllers, Services, kube-proxy and cluster DNS are
  still absent; Pods need `dnsPolicy: Default`.
- **CRD OpenAPI v3 is published** by the customresources worker
  (upstream's openapiv3 controller); the front merges its `/openapi/v3`
  root with the openapi worker's and routes group documents by group.
  `/openapi/v2` still covers built-in groups only. Conversion webhooks are
  untested.
- **Kubelet access goes through the k3s tunnel.** The agent's
  remotedialer WebSocket to `/v1-k3s/connect` is authenticated with its
  node token and handed to the node's `NodeTunnel` Durable Object, which
  runs remotedialer's server (packages/node-tunnel, a wasm module bundled
  into the shell Worker) and proxies `/node/<name>/<kubelet path>` through
  the session. `pods/log` uses it; exec/attach/port-forward and a
  vault-signed kubelet client certificate (today: bearer token +
  InsecureSkipVerify inside the tunnel) are the follow-ups. After
  hibernation the DO closes the agent's socket so k3s reconnects.
- **Authorization is RBAC** (`packages/apiserver-authz`): `system:masters`
  passes unconditionally, everyone else is checked against Roles,
  RoleBindings, ClusterRoles, and ClusterRoleBindings read live from kine,
  unioned with upstream's bootstrap policy. There is no Node authorizer,
  so `system:node` is statically bound to the `system:nodes` group.
- **The Cluster DO compacts on write**: once the log is 1,000 revisions
  past the last compaction the write deletes every older row that is not
  the latest for its key (and every older tombstone) in one statement, so
  the table stays at live keys plus the last 1,000 revisions. A watch from
  a revision below the compaction point gets 410 `Expired` and client-go
  relists; lists always read the latest state.
- **`Content-Encoding: identity`** is verified against workerd and wrangler
  dev only; the production edge is untested.
- `kubectl get` columns come from upstream's printers, one dynamic worker
  per API group (`packages/printers-*`, generated by `scripts/genprinters`
  from the same served-resource list), reached over a Service Binding RPC
  to the `Printers` entrypoint of the same Worker. Only the served kinds are
  linked; a kind added to `api/discovery` needs `make gen`.
- `/openapi/v2` and `/openapi/v3` are computed, not static: the apiserver
  forwards them over the `OPENAPI` Service Binding to the `OpenAPI`
  entrypoint, whose dynamic worker (`packages/openapi`) runs the same
  route installer and kube-openapi's builders. The document set is what
  the installer serves, so CRDs later mean feeding the worker their
  schemas (apiextensions' openapi builder + kube-openapi's aggregator),
  not regenerating a file. Its CRD informer talks to its own handler
  through an in-process loopback client: the same request sent through the
  Service Binding back into the same dynamic worker never returned.

## Known edge cases / watch-fors

- Field selectors read the object's JSON, so an absent boolean field has
  no value; upstream renders the zero value as "false"
  (`spec.unschedulable=false` is how the e2e framework lists schedulable
  nodes). `attrsFor` now treats a missing field as "false" whenever the
  selector compares against "true"/"false".
- `limitranges` is served, but the LimitRanger admission plugin is not
  (it works on internal Pod types); the e2e spec that expects defaults to
  be applied to a Pod is advisory for that reason.

- `wrangler dev` closes an idle keep-alive connection after 5s, and a
  client that reuses it right then gets "connection reset by peer" or EOF
  (measured 2026-09-14: a second request on the same connection succeeds
  after 4.8s idle and fails after 5.0s; a fresh connection always works).
  `TestSchedulerWakesOnNode` sleeps exactly 5s between two POSTs, which is
  why it failed intermittently. The harness clientset therefore caps its
  transport's `IdleConnTimeout` at 2s; kubectl and e2e.test reach the
  worker through `devtls` and were never affected.

- `namespaces` still advertises `deletecollection`: the generic store's
  `DeleteCollection` calls its own `Delete`, not the Terminating deleter,
  so `kubectl delete ns -l ...` removes the namespaces without emptying
  them. Upstream has no such verb on namespaces; hide it or route it
  through the deleter.

- A `k3s` binary must have run once on the node: the agent uses the
  containerd, runc and CNI binaries it unpacks. The VM's stock binary is
  v1.36.2+k3s1; the agent is built against the v1.36.5-dev pin.
- WARP connected inside the VM captures DNS and routing; `host.orb.internal`
  stops resolving.
- After deleting a pod, `crictl pods` keeps the sandbox record until the
  kubelet's GC runs; containers are gone at once.
- The nodes watch with `fieldSelector=metadata.name=` arrives at storage as a
  non-recursive single-key watch (`Store.Watch` optimizes `MatchesSingle`).

## Accepted tradeoffs / future work

- Static PodCIDR allocation in the apiserver instead of the real nodeipam
  controller, until controllers run.
- Hand-written review APIs (SSAR/SAR/TokenReview always allow) until RBAC.
- Edge mTLS with a Cloudflare-managed CA (forwarding the agent's CSR to the
  client-certificate API) remains the candidate that would remove the
  kubeconfig hook entirely; it needs a zone hostname and cannot be tested
  locally.
- **Controllers as a portable core plus two thin `cmd/` entrypoints.**
  `packages/controllers` already has this shape even though only the wasm
  side is built: `controllers.go` imports only client-go and a
  `*rest.Config`, no `syscall/js` or `bridge.*`; `cmd/controllers-wasm`
  is the only file that knows about poke/pump-window/`BindingTransport`.
  Every new controller should start from the same split: the reconciler
  package stays buildable with a plain `GOOS=linux go build` against any
  cluster, `cmd/<name>-wasm` (js) wraps it in `bridge.Serve` plus the
  poke loop, and a second `cmd/<name>` (no build tag) wires
  `rest.InClusterConfig()` (or a kubeconfig flag) and blocks in
  `Run(ctx)` — an ordinary container, deployable on stock k8s with no
  k8flare code in the image. Anything that only makes sense against
  Cloudflare goes behind an interface the core calls; only the wasm
  `cmd/` supplies the Cloudflare implementation, the plain `cmd/` supplies
  a different one (or none).
- **`type: LoadBalancer` Services.** `packages/agent` already sets
  `DisableLoadBalancer: true` (k3s's own ServiceLB/klipper-lb is off).
  Planned: a `loadbalancer` controller built with the pattern above — the
  portable core watches Services of `type: LoadBalancer` and writes
  `.status.loadBalancer.ingress`; a `Provisioner` interface it calls is
  what actually exposes the Service, and only the wasm `cmd/` gets a
  Cloudflare-backed implementation. First cut, since the Worker is already
  the single edge ingress: expose the Service as another routed
  hostname on the same Worker rather than provisioning a real external
  LB (one hostname per Service, e.g. derived from name/namespace, matches
  real `LoadBalancer` semantics — one external identity per Service —
  better than a single shared hostname with path routing). On stock k8s
  the same core, given a host-network or MetalLB-style `Provisioner`,
  behaves like an ordinary ServiceLB replacement (or is skipped where a
  real cloud LB controller already owns the class).
- **Cloudflare Access as a third authenticator.** Today authentication is
  only `AdminToken` and `NodeToken` (`packages/apiserver-auth`); there is
  no notion of a human user. Planned: an `authenticator.Request` that
  reads the `Cf-Access-Jwt-Assertion` header (not `Authorization`, per
  Cloudflare's own guidance — the `CF_Authorization` cookie is
  browser-only and not guaranteed to arrive), verifies against the team's
  JWKS (`https://<team>.cloudflareaccess.com/cdn-cgi/access/certs`,
  selecting the key by the JWT's `kid`), checks `iss` and `aud`, and
  returns `user.Info{Name: <email claim>}`. RBAC needs no new code for
  this — bind the email directly as a `User` subject in an ordinary
  RoleBinding/ClusterRoleBinding, the same as OIDC users on stock k8s.
  Group-based bindings need one more call to `/cdn-cgi/access/get-identity`
  with the same JWT, since group membership is not guaranteed to be in the
  compact JWT itself. This authenticator only fires for requests that came
  through an Access-protected hostname; kubectl needs a way to carry that
  header (a service token, or an exec credential plugin that runs
  `cloudflared access login`-style token retrieval) — it is a path for
  human users alongside, not instead of, the admin and node tokens.
- **Pod-on-Containers: a second, Cloudflare-Container-backed node type.**
  Already built once, pre-`feat/minimal-rewrite` (see `main`,
  `main-legacy-full-history`, `backup/pre-rewrite-2026-09-13`; commits
  around `7d122b5`/`29ace6a`), as `workers/nodes`: a `VirtualNode`
  Durable Object registers a fake Node (`cf-containers-<pool>`), heartbeats
  its Lease on a ~10s alarm, and reconciles Pods scheduled to it by
  list-and-diff against the apiserver on that same alarm (a documented
  latency tradeoff — no push path from the storage layer). Each Pod became
  a `PodContainer{Small,Medium,Large}` DO wrapping one real Cloudflare
  Container instance; **image and instance size are fixed at deploy time
  on Cloudflare Containers** (a platform constraint, not k8flare debt), so
  a Pod's summed resource requests rounded up to the nearest size tier and
  were checked against a deploy-time image allowlist. It was verified
  end-to-end against real wrangler dev + real Docker at the time (two real
  bugs found and fixed that way: Lease `renewTime` needing microsecond
  precision, and a SIGTERM'd container misclassified as Succeeded). v1's
  known gaps: no Pod IP, `restartCount` stuck at 0, ~10s reconciliation
  latency, no UDP (blocks CoreDNS on this node type). **`main` later
  recorded this as broken** (a correction to the provisioning explanation,
  then a recorded regression) — reviving it starts with reading that
  history, not re-diagnosing from scratch against the current, much-changed
  apiserver/controllers/RBAC. Unlike the portable-controller pattern above,
  this backend is Cloudflare-only by nature — there is no vanilla-k8s
  equivalent to running a Pod as a Cloudflare Container, so only the
  wasm/DO side is meaningful; no matching plain `cmd/` to design.
- **R2 storage for Pod-on-Containers Pods**, also already built and
  verified once (`0de2d66`): on PVC mount, `VirtualNode.reconcileOnePod`
  called an internal `mint-r2-credentials` endpoint (namespace + claim name
  only — no CSI attribute schema needed on this path) and injected the
  scoped, temporary result as standard `AWS_ACCESS_KEY_ID`/
  `AWS_SECRET_ACCESS_KEY`/`AWS_SESSION_TOKEN` plus `R2_ENDPOINT`/
  `R2_BUCKET`/`R2_PREFIX` env vars via `@cloudflare/containers`'
  `startOptions.envVars` — the app talked to R2 with a normal S3 SDK, no
  mount at all. Credential refresh was a known rough edge:
  `restartPolicy: Always` Pods past their credential's TTL got proactively
  restarted to re-mint (not zero-downtime; an in-image refresh sidecar was
  left as follow-up). Cloudflare Containers has since gained FUSE support
  (Nov 2025), so a Pod's own image can now additionally mount its scoped
  R2 credential as a real filesystem with a bundled FUSE adapter
  (tigrisfs/s3fs/gcsfuse) instead of, or alongside, using the S3 SDK
  directly — same POSIX/performance caveats as any object-storage-over-FUSE
  setup apply
  (https://developers.cloudflare.com/containers/examples/r2-fuse-mount/,
  https://developers.cloudflare.com/changelog/post/2025-11-21-fuse-support-in-containers/).
  Minting a scoped credential and handing it to a Container's env is just
  an API call, no privileged node access, so this whole CSI-equivalent role
  for Pod-on-Containers is DynamicWorker-native already — unlike the
  `mountpoint-s3-csi-driver` track above, which necessarily runs on the
  real k3s node.
- **A Cloudflare-native alternative to the custom node-tunnel.** Today's
  `packages/node-tunnel` is a from-scratch reimplementation of remotedialer's
  server (patched into `github.com/rancher/remotedialer` by `scripts/mirror`
  to add `ServeConn`, since a DO's hibernatable WebSocket isn't an
  `http.Hijacker`) solely because a Worker cannot otherwise dial an
  unroutable node. Cloudflare Tunnel alone doesn't remove that constraint —
  it's origin-to-Cloudflare only, and a Worker still reaches it either via a
  routed public hostname (back out through the internet, not a direct bind)
  or through **Workers VPC** (`vpc_services`/`vpc_networks` bindings,
  supporting both `fetch()` and raw `connect()` — the same shape
  `BindingTransport.DialTLSContext` already needs), which can reach a
  destination "regardless of how it's connected: Tunnel, Mesh node, or WAN
  on-ramp." **Cloudflare Mesh** (GA'd April 2026) is the better conceptual
  match specifically: every enrolled node gets a stable, private per-node
  "Mesh IP" reachable over TCP/UDP/ICMP with either side initiating, which
  fits "the control plane dials this exact node" better than Tunnel's
  hostname-routing shape. **The one fact that decides whether this is
  viable at all is unconfirmed: Workers VPC's docs only show plain Worker
  `fetch()` handlers, never a call from code running inside a Durable
  Object** — and this project's `TUNNEL` binding is invoked from Go/wasm
  running inside the `NodeTunnel` DO via `worker-bridge`, so DO-callability
  has to be spiked (one OrbStack node enrolled in Mesh, a `vpc_services`
  binding, a `connect()` call from inside a DO) before anything else here
  is worth planning. Workers VPC is also still **beta** ("features and APIs
  may change"), a real risk for the production edge, not just local dev.
  Today's per-node bearer token (`node:<name>:<password>`) selecting one
  `NodeTunnel` DO by name would also need to become
  credential-selects-Mesh-IP/service-id instead — a redesign, not a
  drop-in swap. If the DO spike succeeds, the payoff is real: dropping
  ~200+ lines of custom remotedialer-server/session-adapter code and the
  `ServeConn` mirror overlay in favor of a Cloudflare-maintained connection
  path.
- **Admission control: the hook point already exists empty, not missing.**
  `packages/apiserver-installer/installer.go` and
  `packages/customresources/customresources.go` already call
  `admission.NewChainHandler()` with zero arguments — adding a plugin is
  just constructing it and passing it in, the same "call upstream's real
  constructor" pattern RBAC/controllers already used, at a point that
  already exists. The plugin packages are already vendored under
  `.build/apiserver-mirror/pkg/admission/plugin/` (`webhook/`,
  `namespace/lifecycle/`, `resourcequota/`, `policy/`, `cel/`,
  `authorizer/`) with clean-looking imports (no etcd/grpc/otel), though
  `webhook/generic` pulls in `pkg/admission/plugin/cel` for match
  conditions — a second, real CEL dependency to actually build, unlike the
  ShardSelector CEL this project already stubs out. `Validating`/
  `MutatingWebhookConfiguration` support both a raw `url` (a plain
  outbound HTTPS call, no new networking needed) and a `service`
  reference (blocked on the still-absent Services/kube-proxy path) — url
  webhooks are the clean first cut. Note: the specific "reject a create in
  a Terminating namespace" behavior that would have been NamespaceLifecycle's
  job is already covered by a narrower, hand-written
  `registry.NamespaceLifecycle` hook (one kine read of the namespace per
  create) — so **LimitRanger is the cleanest next full-plugin adoption**
  (still genuinely missing, per Known edge cases above), not
  NamespaceLifecycle. ServiceAccount's admission-time token-volume
  injection (distinct from the serviceaccount *controller*
  `packages/controllers` already runs) wasn't located in this pass and
  needs a follow-up look before assuming it's covered too.
- **Gateway API, not classic Ingress, as routing logic inside the front —
  not a `Provisioner` like LoadBalancer.** Gateway API is GA and still
  active (v1.6, June 2026, added TCPRoute/UDPRoute); classic
  `networking.k8s.io/Ingress` is legacy at this point. Upstream reuse here
  is weaker than the RBAC/controllers precedent: `sigs.k8s.io/gateway-api`
  ships the API *types* as a Go module, not a reusable reconciler the way
  `k8s.io/kubernetes/pkg/controller/*` was — this project would write its
  own HTTPRoute/Gateway reconciliation. **Correction to an earlier draft
  of this note**: dispatch and domain-assignment are two separate problems,
  and only one of them is free. `wrangler.jsonc` has no `routes`/custom-domain
  config today, and `index.ts`'s `fetch` branches on path only
  (`/v1-k3s/connect` vs. everything else) — there is no Host-header
  dispatch yet. Once traffic for a hostname already reaches this Worker,
  Host/path → Service/Pod dispatch can live entirely as internal routing
  logic (one more Loader dynamic worker, keyed by Host header the same way
  `/v1-k3s/connect` is special-cased today) — that part is free, matches
  the user's "DynamicWorker for routing" instinct, and needs no
  `Provisioner`-interface like LoadBalancer's. But *getting* a brand-new
  hostname to reach this Worker at all is a real, one-time Cloudflare API
  call per hostname (or per wildcard), not automatic. Two mechanisms,
  by who owns the domain: **Custom Domains** (API-provisionable — a
  Terraform resource takes `hostname`+`service`+`zone_id` — but needs a
  zone *this operator* owns, e.g. `*.apps.<operator-domain>` as one
  wildcard Custom Domain covering every future Gateway/HTTPRoute with zero
  further calls) for the common case; **Cloudflare for SaaS /
  Custom Hostnames** (GA, bundled non-Enterprise; the tenant CNAMEs to a
  fallback origin and Cloudflare issues the cert) only for genuine
  bring-your-own-external-domain, which is overkill until actually needed.
  Plain Workers **Routes** don't fit — they front an existing non-Worker
  origin, which doesn't apply here. The user's other alternative — a
  second, separately-*deployed* Worker script dedicated to Gateway-routed
  traffic — would buy real blast-radius isolation from the control plane,
  but costs a categorically new capability (calling the Workers API to
  deploy/manage another script is a much bigger permission surface than
  anything built so far, which is entirely self-contained in one Worker +
  its own bindings); defer unless the isolation is actually needed. MVP:
  one wildcard Custom Domain under an operator-owned zone + Host-header
  dispatch as an internal dynamic worker. cert-manager mostly falls away
  regardless (TLS terminates at the edge either way).
  (https://developers.cloudflare.com/workers/configuration/routing/custom-domains/,
  https://developers.cloudflare.com/cloudflare-for-platforms/cloudflare-for-saas/)
- **Cluster DO backup/DR is mostly already solved — the real gap is
  narrower than "disaster recovery."** SQLite-backed Durable Objects (what
  `Cluster` already is) have native point-in-time recovery for the last 30
  days: `ctx.storage.getBookmarkForTime(timestamp)` →
  `ctx.storage.onNextSessionRestoreBookmark(bookmark)` → `ctx.abort()` to
  apply it, covering both SQL and KV storage, scoped per-DO-instance (a
  perfect match — there is exactly one `Cluster` DO). Writes are also
  already synchronously replicated to multiple nearby-datacenter replicas
  before being acknowledged. What this project would actually be adding on
  top is retention *beyond* 30 days and a portable/human-inspectable
  export, not baseline durability. Recommended first step, cheap: wire the
  native PITR API behind one admin endpoint
  (`POST /restore?to=<timestamp>`). Second, lower-priority step: a
  periodic snapshot to R2 needs no new query logic (the existing
  "current value per live key" read already does it, e.g.
  `SELECT name, value FROM kine WHERE id IN (SELECT MAX(id) FROM kine
  GROUP BY name) AND deleted=0`) and needs no revision-continuity
  handling on restore — replaying rows into a fresh DO gets fresh revision
  numbers, which every kine/client-go watcher already treats exactly like
  a big compaction event (410 Expired → relist). No `ctx.storage.setAlarm()`
  is used anywhere in `cluster.ts` today, so a periodic (non-manual)
  snapshot job needs a new wake source — a Cron Trigger is the natural
  fit. Not urgent: the schema/compaction semantics are still moving, and
  locking in an R2 export format now risks a redesign later.
  (https://developers.cloudflare.com/durable-objects/api/sqlite-storage-api/,
  https://developers.cloudflare.com/changelog/2025-04-07-sqlite-in-durable-objects-ga)
- **`k8s.io/kube-aggregator` is already pinned but doesn't fit; metrics-server
  doesn't need it anyway.** `go.mod` already replaces
  `k8s.io/kube-aggregator` with the same k3s-io fork as everything else
  (indirect dep, unused). Its real proxying
  (`handler_proxy.go`) dials a resolved backend address over real TLS —
  built for arbitrary, runtime-registered targets, which doesn't map onto
  this project's Service Bindings, fixed at deploy time in
  `wrangler.jsonc`. No workload can make itself a new routable aggregation
  target at runtime the way real `APIService` allows; this project's
  existing hand-rolled `groupRouter`/`forwardTo` (resolve group name → a
  small, fixed set of Service Bindings → stream-proxy) is the more honest
  fit than importing real kube-aggregator wholesale. **metrics-server
  doesn't need any of that resolved first**: build it exactly like
  `scheduler`/`controllers` — one more fixed-route DynamicWorker (e.g.
  `METRICS`) serving `metrics.k8s.io/v1beta1`, reusing
  `packages/node-tunnel`'s existing `/node/<name>/<kubelet-path>` proxy
  (today used for `pods/log`) to reach each kubelet's `/metrics/resource`.
  The one real mismatch: metrics-server polls every node on a fixed
  ~15s wall-clock interval, not reactively on writes — the same shape as
  the already-identified CronJob gap, needing a Cloudflare Cron Trigger
  (still absent from `wrangler.jsonc`) rather than a poke. HPA is a
  separate, bigger follow-on: `packages/controllers/controllers.go`'s 14
  wired controllers do not include `horizontalpodautoscaler` — it would
  need upstream's real `NewHorizontalController` added the same way the
  other 14 were, plus a metrics client pointed at `METRICS`.
- **ClusterUpgrade has three separable concerns; the Durable Object data
  question is the hard, novel one.** (1) Control-plane code: a plain
  `wrangler deploy` is effectively an instant full cutover for new
  requests by default; Cloudflare's gradual deployments can split traffic
  by percentage with optional version affinity if a slower rollout is
  wanted. This project's own architecture adds a wrinkle generic Workers
  docs don't cover: resident dynamic-worker instances (front `apiserver`,
  `scheduler`, `openapi`, etc.) can stay warm across a deploy, so an old
  front talking to a new group worker (or vice versa) over their internal
  RPC shape is a real skew window this project invented by splitting into
  per-group binaries — worth testing deliberately, not assumed away by
  Cloudflare's own version-skew tooling (which is about *which Worker
  version* handles a request, not this project's own inter-binary
  protocol). (2) **The Cluster DO's data is the hard part.** Current
  `wrangler.jsonc` migration tags (`new_sqlite_classes` for `Cluster`,
  `NodeTunnel`) are confirmed to only govern which DO *classes* exist —
  changing an existing class's code needs no migration tag at all, but
  Cloudflare's own docs say it's the project's responsibility that new
  code stays backwards-compatible with what's already stored; there is no
  `PRAGMA user_version`, so schema versioning is entirely userland (a
  tracking table, `blockConcurrencyWhile()` for a real migration pass).
  With exactly one `Cluster` DO holding the whole cluster's live kine log
  (unlike etcd's N-replica rolling upgrade), there is no "old version
  keeps serving while new version validates" option. Recommended: give
  every kine row a schema-version field from day one and migrate-on-read
  in `cluster.ts`, before there's real production data to be locked out
  of reading. (3) k3s agent/kubelet version skew is the well-trodden part
  — this project adds no skew gating beyond upstream's own installer, so
  it's no worse than real k8s's already-documented kubelet-to-apiserver
  skew policy; can wait.
  (https://developers.cloudflare.com/workers/configuration/versions-and-deployments/gradual-deployments/,
  https://developers.cloudflare.com/durable-objects/reference/durable-objects-migrations/)
- **Durable Object Facets are real (open beta, Apr 2026, Workers Paid,
  under Cloudflare's "Dynamic Workers" umbrella with this project's own
  Worker Loader binding) — and this project already built and verified a
  real design around them**, on `main`/`main-legacy-full-history`, not
  `feat/minimal-rewrite`. Mechanics confirmed both from current Cloudflare
  docs and this project's own historical design doc
  (`docs/multi-tenancy-and-hosting.md` on
  `origin/docs/multi-tenancy-and-roadmap`, dated July 2026): a DO loads a
  named child "facet" lazily (`ctx.facets.get(name, initCallback)`, the
  callback only runs if that facet hasn't started or has hibernated) with
  its **own isolated SQLite** the parent can't read, reached via a local
  RPC hop (not a network round-trip) — but **every facet shares the
  parent's 10 GB storage cap and is reachable only through the parent's
  single thread**. Facets buy isolation and lifecycle, not throughput or
  storage headroom; anything needing its own 10 GB or its own thread has
  to be a top-level DO. The old design used this correctly: the Cluster DO
  hosted only small, isolation-worthy facets (`ca-vault` for CA
  keys/node-password hashes, `events-log` for high-churn Event isolation),
  while the Namespace DO (one per namespace) was deliberately a
  **top-level DO, not a facet**, because namespace data needs real
  throughput headroom — directly relevant to the Cluster-DO-backup/DR note
  above, since splitting `ca-vault`/high-churn writes into facets shrinks
  what one DO instance has to hold without needing the full multi-tenant
  apparatus below. The storage layer also recorded a real correctness
  lesson worth reusing verbatim: after two more clever, abandoned attempts,
  it landed on **parent-first, no inference** — the parent's log row is
  always authoritative, trimmed only after the facet acknowledges receipt;
  never ask the facet first, because "the acknowledgement and the trim can
  both land in between." Decision 2026-09-14: not adopted yet. Go reaches
  storage only through the `STORAGE` binding's HTTP and WebSocket contract
  (`/list`, `/watch`, `/insert`, `/compact`, `/stats`), and the single-DO
  assumption is four `idFromName("default")` lines in
  `packages/control-plane-worker/src`, so a per-namespace DO is a
  cluster-store-internal change that can land later without touching Go.
  Trigger: `GET /stats` storage growth toward the 10 GB cap, or p99
  write/list latency under e2e load showing the one DO thread saturating;
  neither is near today (required set 15/15 in 104s on one DO).
- **Full multi-tenancy (a `Cluster` CRD + `clusterop` controller) was also
  built and verified end-to-end, separately from facets** — reusable
  independently, not a package deal. `k8flare.com/v1alpha1 Cluster` was a
  compiled-in type (no CRD codegen in that era), cluster-scoped in the
  storage keyspace; `clusterop` reconciled `Cluster` objects into full
  tenant control planes (allocates the DO tree name, initializes the token
  vault, publishes credentials as a Secret, maintains `/c/<id>` resolution,
  tears down via finalizer — Scheduler-first, "because Containers are
  wall-clock billed"). Routing was a `/c/<id>` path prefix (the Rancher
  `/k8s/clusters/<id>` precedent) backed by a `ClusterRegistry` DO
  (`id -> uid, state`; the UID indirection means a recreated cluster never
  reuses DO/facet names). DynamicWorker Loader IDs became per-cluster too
  (`apiserver:<doName>@sha`) — loading was tenant-scoped, not just
  storage. The whole seam was one indirection layer
  (`clusterenv.ts` retargeting `idFromName("default")` per-request), so
  every downstream module stayed single-cluster-shaped and unaware of
  multi-tenancy. Verified at the time: cross-cluster token rejection, data
  and watch isolation, rotation/revocation, and teardown + same-id
  recreation yielding a genuinely fresh cluster. No recorded regression or
  abandonment reason was found for this (unlike Pod-on-Containers, which
  has one) — reads as deliberate descoping for `feat/minimal-rewrite`'s
  "as little code of our own as possible" goal, not a technical wall, but
  that's an inference from absence of failure evidence, not a stated fact,
  so verify before assuming it's simply revivable as-is.
- **Is swapping the hardcoded `idFromName("default")` string alone safe
  multi-tenancy? No — it's the one indirection point that has to exist,
  but by itself it's unsafe.** Current code has grown to **five** such
  call sites, not four as stated above (`packages/control-plane-worker/
  src/loader.ts:5`, `apigroups.ts:11`, `customresources.ts:7`,
  `index.ts:34` and `:53` — corrects the count in the bullet above), all
  in the plain TS Worker, none in Go. That split isn't a style choice:
  `pkg/controllers/clusterop/bridge.go`'s own doc comment cites
  `docs/platform-verification.md` S2 for the reason — **a Loader-loaded
  dynamic worker can't be handed a `DurableObjectNamespace` binding at
  all, only plain values and `Fetcher`s survive the env clone** — so
  `packages/controllers`, `packages/apiserver*`, etc. can never call
  `idFromName` themselves; they only ever receive an already-resolved
  `STORAGE` Fetcher-shaped binding, exactly like the old `clusterop`
  controller (also a resident Go/wasm dynamic worker) having to proxy
  every DO-allocation/registry/vault write through an authenticated
  `/internal/clusters/*` HTTP bridge back into the TS layer instead of
  touching `env.CLUSTER` itself. On today's code specifically, three
  things make a bare id-swap unsafe:
  1. **Global secrets, not per-tenant ones.** `ADMIN_TOKEN` /
     `READONLY_TOKEN` / `JOIN_TOKEN` are Worker-wide secrets (absent from
     `wrangler.jsonc`'s `vars`, so `wrangler secret put`-managed — one
     value for the whole deployment), and the apiserver's bearertoken
     auth compares against them directly. Route different requests to
     different Cluster DOs today and every tenant still accepts the
     *same* admin/join token — a full cross-tenant auth bypass, not an
     isolation win. The old design's fix: a per-cluster **token vault
     living inside each tenant's own Cluster DO** (`clusterop.go`'s
     `EnsureVault`/`MintToken`/`RevokeToken`), reusing exactly the
     `/vault/...` kine-KV prefix this repo already uses today for node
     passwords (`checkNodePassword`, `index.ts:33`) — the storage-side
     isolation primitive already exists, it's just not yet used for
     admin/join tokens.
  2. **`NODE_TUNNEL.idFromName(nodeName)` is already parameterized, but
     only by node name.** Two tenants each naming a node `node1` collide
     on the identical NodeTunnel DO today, and the join wire format
     (`node:<name>:<password>`, `index.ts:26`) carries no cluster
     identifier at all — confirmed zero hits for `tenant`/`clusterId`
     anywhere in `packages/apiserver-supervisor`. This is a genuinely new
     gap the old design never had to close, since node-tunnel postdates
     it; closing it means a wire-format change (e.g.
     `node:<clusterId>:<name>:<password>`) before `authenticateNode` can
     even know which vault to check.
  3. **Name reuse would bleed data.** A raw `idFromName(tenantId)` where
     `tenantId` is a user-chosen string means deleting and recreating a
     tenant of the same name resolves to the *same* DO instance,
     inheriting its old SQLite state. The old code's defense was minting
     the actual DO-name suffix from the Kubernetes object's own immutable
     UID (`doNameFor`: `fmt.Sprintf("%s@%s", cl.Name, cl.UID)`,
     `clusterop.go`), allocated exactly once and persisted in
     `.Status.DoName`, with the external-facing `/c/<id>` route keyed
     only on the UID half (`uidFromDoName`) — "switching the id" has to
     mean switching to an operator-allocated, never-reused compound key,
     not a bare tenant-supplied slug.
  What genuinely *is* close to free: Cloudflare's own DO namespace needs
  no provisioning step — `idFromName` on a new string just lazily
  allocates a fresh, fully SQLite-isolated instance on first
  `.get().fetch()`, no platform-side "create cluster" call required.
  Every hard part above is this project's own application-level
  assumption, not a platform limit. Sequencing worth reusing verbatim
  from `clusterop.reconcileActive`: write the `/c/<id>` registry entry
  *before* minting the vault (a routing entry with no credentials yet
  safely 401s; credentials with no route safely 404s forever — never
  serve a half-provisioned tenant), and on rotation, revoke a superseded
  token only after its replacement is fully distributed.
- **What this means for "zero to scale" concretely**: DynamicWorkers and
  DOs are already zero-cost-until-addressed by Cloudflare's own runtime
  (no new design needed there); facets add a third, more fine-grained
  zero-to-one primitive *within* one DO. A user-facing CRD declaring
  "run workload X on DynamicWorker/DO-Facet compute, 0↔N" still needs a
  real controller mapping that onto Cloudflare primitives — the same
  *provisioning* category as Pod-on-Containers' `VirtualNode`/
  `PodContainer` DOs or the LoadBalancer controller's `Provisioner`, not
  the portable-controller-core pattern (DynamicWorkers/Facets are
  Cloudflare-only primitives with no vanilla-k8s equivalent, so — like
  Pod-on-Containers — there is no matching plain `cmd/` to design here
  either). Shape sketch: a supervisor DO (mirroring `VirtualNode`'s
  existing ~10s Lease/reconcile-alarm pattern) lazily creating one facet
  per workload via `ctx.facets.get(name, initCallback)`, trading
  Pod-on-Containers' ~100x-slower/heavier container cold start for a much
  cheaper isolate — a better fit for small, short-lived, event-driven
  workloads than anything needing a full container.
  (https://blog.cloudflare.com/durable-object-facets-dynamic-workers/,
  https://developers.cloudflare.com/dynamic-workers/usage/durable-object-facets/)
- **Pod-to-pod networking: Cloudflare Mesh is a real WireGuard/Tailscale
  replacement candidate for flannel's transport — unlike cluster DNS
  below, this one looks genuinely promising.** `packages/agent` doesn't
  override flannel's backend at all, so it inherits stock k3s's default,
  **`vxlan`** (not WireGuard); more importantly, the only milestone
  reached so far (see Goal) is **one** OrbStack VM — multi-node pod
  networking is entirely unexercised here today, not just undocumented.
  The decisive fact, confirmed from current docs: Mesh enrollment installs
  a **VPN profile** via "a headless version of the Cloudflare One Client,"
  giving the node real OS-level TCP/UDP/ICMP reachability to other Mesh
  IPs for *any local process* — architecturally the same shape as
  WireGuard/Tailscale, and a materially different (better) fact than the
  Workers-VPC-from-a-DO open question above, since this doesn't route
  through Workers/DOs at all. flannel's **`host-gw`** backend adds no
  encapsulation of its own — just a kernel route to each peer's pod CIDR
  via that peer's node IP — so pointing flannel at `host-gw` with each
  node's *Mesh IP* as its node-internal IP should work with no k3s/flannel
  code changes: Mesh supplies the encryption and transport, `host-gw`
  supplies the pod-CIDR routing. Not verified end-to-end and no existing
  Cloudflare precedent exists for Mesh-as-CNI-transport specifically (all
  current framing is dev-agent/SSH/DB-access, not Kubernetes networking) —
  this is new territory. Sequence after single-node work stabilizes
  (multi-node is untested regardless of transport). Cheapest validation:
  enroll two OrbStack VMs in Mesh, confirm plain `ping`/`iperf3` between
  their Mesh IPs as ordinary OS traffic (no Workers/DO involved at all),
  then try `--flannel-backend=host-gw` with those Mesh IPs as the node
  IPs. (https://developers.cloudflare.com/cloudflare-one/networks/connectors/cloudflare-mesh/get-started/,
  https://developers.cloudflare.com/cloudflare-one/networks/connectors/cloudflare-mesh/client-devices/)
- **Cluster DNS: no meaningful Cloudflare-native substitute for CoreDNS
  exists today — don't stretch for one.** The real constraint: kubelet
  points every pod's `/etc/resolv.conf` at the cluster-dns ClusterIP, and
  whatever answers it must be a real UDP/TCP **:53** listener speaking
  plain DNS wire protocol (DNS is UDP-first; TCP is only the &gt;512-byte
  fallback) — a fundamentally different transport shape from a Worker's
  HTTP request/response model. Checked and ruled out: Cloudflare's newest
  Spectrum-routed-to-Worker-socket feature (private beta, Aug 2026) is
  **TCP only** — the announcement itself names UDP as future work, so even
  a mature version misses the half DNS actually needs; plain (GA) Spectrum
  is a dumb byte-forwarder to a real origin, so routing it at a hand-run
  DNS server doesn't eliminate a workload, it just relabels one. Cloudflare's
  DNS product has no dynamic, API-driven per-query answering mechanism —
  only static zone records (useless for Service/EndpointSlice-derived
  answers that change every second) and Cloudflare One's Gateway resolver
  policies (a client's own *outbound* DNS routing/filtering, a different
  problem entirely, not a way to answer *inbound* in-cluster queries).
  Realistic path: real upstream CoreDNS as an ordinary Deployment+Service
  once Services/kube-proxy exist (the same gating dependency already
  flagged for Ingress and admission control) — a small, well-understood,
  upstream-maintained fixed cost, consistent with this project's own
  reuse-upstream philosophy, not overhead worth hand-rolling a replacement
  for. `node-local-dns`-style caching solves a multi-node conntrack/latency
  problem this project doesn't have yet.
  (https://blog.cloudflare.com/grpc-workers/,
  https://developers.cloudflare.com/cloudflare-one/traffic-policies/resolver-policies/)

## Where the idle CPU actually went (measured, one node attached)

Every measurement below is a 6-minute `wrangler tail` window on production,
taken 3 minutes after deploy so the cold start is excluded, with the same
single OrbStack node (`k8flare-c1`) joined and no workloads.

| configuration | inv/min | watch/min | CPU ms/min | /mo | exceededMemory | hung |
| --- | --- | --- | --- | --- | --- | --- |
| 250ms tick (`c044e57`) | 577 | 266 | 5,611 | 242.4M | 32 | 0 |
| I/O anchor, 55s hold (`1cfd478`) | 561 | 258 | 4,622 | 199.7M | 114 | 0 |
| window=dispatch, 290s hold (`2fa87f2`) | 488 | 206 | 5,589 | 241.5M | **0** | 61 |

Attribution of the 4,622 ms/min, by tail event:

| source | ms/min |
| --- | --- |
| dynamic workers, `canceled` (watch re-establishment) | 1,567 |
| parent-side `APIGroups` dispatch | 887 |
| dynamic workers, `ok` (steady-state informer work) | 838 |

Broken down per resource, the traffic is almost entirely re-establishment:
`services` 9.2/min, `pods` 8.7, `replicasets` 7.8, `jobs` 6.6, `leases` 6.5,
and so on across ~20 resources — each informer re-watching 6 to 8 times a
minute. That rate is the kubelet lease renewal (every 10s), not the cron:
a write poke called `OpenWindow`, which called `endTrackedStreams(0)`, which
killed all 43 watches. **The treadmill scaled with node count × heartbeat
rate**, which is why idle-with-zero-nodes cost nothing and one node cost
199.7M CPU-ms/month.

### What the 290s hold taught

Lengthening the resident hold was aimed at the wrong side. Group workers
(`apiserver-<group>`) never hold, so no resident hold length touches their
churn, and raising the resident duty cycle from ~50% to ~97% tripled
steady-state informer work (838 → 2,333 ms/min). Reverted to 55s.

Two findings from that deploy are keepers:

- **`window = dispatch` eliminated the memory exhaustion**: `exceededMemory`
  114 → 0. A window closed by a JS timer outlives the request it anchors —
  a write poke runs under `ctx.waitUntil`, cut at 30s, so its timer never
  fired and a dead env stayed registered as the newest window.
- **Removing the JS timer entirely introduced 61 hangs**, all in
  `APIGroups`: "the Workers runtime canceled this request because it
  detected that your Worker's code had hung". With no Go timer and no JS
  timer, a wasm isolate whose goroutines are all blocked looks hung. If
  these persist, the fix is one long-interval ticker, not a 250ms tick.

### Ruled out by measurement, not by argument

- **Controllers or scheduler inside a Durable Object.** The corrected cost
  model does put one always-resident DO inside the 400k GB-s allowance, so
  the original objection is gone — but `gzip -9` gives controllers 9.9MB
  and scheduler 11.2MB against a 10MB Worker bundle limit. The scheduler
  does not fit.
- **Trimming the GC monitor set.** Of the 14 monitors, only `runtimeclasses`
  and `deviceclasses` are cluster-scoped catalog types that structurally
  cannot be owners or dependents here. `csinodes` and `resourceslices` do
  carry `ownerReferences` to their Node, so ignoring them leaks objects on
  node deletion. Two watches out of 43 is not worth the correctness risk.
- **Dropping the endpoint and endpointslice controllers.** `services`,
  `endpoints` and `endpointslices` are all in `registry.Served`, so those
  controllers do real work even without kube-proxy.

### The window-ownership series (measured on production, one node)

| build | CPU ms/min | /mo | watch/min | hung | exceededMemory |
| --- | --- | --- | --- | --- | --- |
| `c044e57` 250ms tick | 5,611 | 242.4M | 266 | 0 | 37 |
| (store grew from 1,056 to 1,255 rows across the series; each row is one 6-minute window) | | | | | |
| `1cfd478` I/O anchor, 55s | 4,622 | 199.7M | 258 | 0 | 114 |
| `2fa87f2` window=dispatch, 290s | 5,589 | 241.5M | 206 | 61 | 0 |
| `b003276` stream ownership | 3,247 | 140.3M | 135 | 165 | 23 |
| `92b1427`+`62f3c83` keepalive, tracking | 3,267 | 141.1M | 236 | 34 | 0 |
| `12c1d38` owned windows only | 5,763 | 249.0M | 278 | 41 | 0 |
| `41eeed2` borrowed dials scoped to the isolate | 4,635 | 200.2M | — | 25 | 0 |

The `12c1d38` window is not usable: a bulk namespace delete issued for
cleanup was still running through it (`rows` 1,082 → 1,181, `revision`
2,600 → 3,450). It needs re-measuring.

**The required e2e set, same cluster, same day**: `c044e57` 16 passed /
5 failed; `12c1d38` 17 passed / 4 failed; `41eeed2` 11 passed / 10
failed. The `62f3c83` run was killed before it produced a summary, with
the CRD specs failing first; the "18" quoted in that commit message is a
count of `[FAILED]` *lines*, which overcounts specs about threefold, so
treat it only as "CRD-heavy, clearly worse".

One spec between 17/4 and 16/5 is inside single-run noise, so the
shipped build is **not worse** than the point it started from rather
than better. The failure *mode* did change, and that part is a real
regression: `c044e57` hangs zero requests, `12c1d38` hangs 41 per
six-minute window.

**No cost reduction survived into the shipped state, and this is the
honest result of the series.** The two builds that were materially
cheaper are the two that break: `62f3c83` at 3,267 ms/min fails the CRD
specs because an unrelated request closes the CRD informer's socket, and
`41eeed2` at 4,635 ms/min fails 10 of 21. `12c1d38` costs 5,763 ms/min against
the 5,611 it started from — the same, within the noise of a single
window. What it buys is a fully attributed cause for the
CPU, and `exceededMemory` 37 → 0 **in an idle window only**: the same
build showed 54 while a bulk namespace delete was running. Memory
exhaustion is improved, not eliminated.

The three lifetimes tried for a background dial — the dispatch that
borrowed the window, the isolate's live period, and no tracking at all —
trade the store's socket pool against the CRD informer's survival, and
none of them is right. The socket needs to outlive any single dispatch
but die with the env that dialed it, and neither `windows` nor the
isolate is that boundary. The next attempt should give a worker with
background informers a hold of its own, the way the resident workers
have one, rather than letting it borrow.

**Comparable failure counts.** The starting point was not failure-free:
`c044e57` failed 37 requests per 6-minute window with `exceededMemory`.
The window-ownership build fails a similar number with `hung`. The
difference is 42% less CPU and no memory pressure.

### What each change actually did

- **A window is a dispatch** (`2fa87f2`). A window closed by a JS timer
  outlives the request it anchors — `ctx.waitUntil` is cut at 30s, so a
  write poke's timer never fired and a dead env stayed registered as the
  newest window. Fixing this took `exceededMemory` from 114 to 0.
- **Each window owns the streams it opened** (`b003276`). A tracked
  stream used to be reaped 30s after it opened, by whichever dispatch ran
  next, which killed the store socket under a healthy watch. This is the
  single biggest CPU win: 4,622 → 3,247 ms/min.
- **Track every store socket** (`62f3c83`). A group worker dials inside
  the watch request, so `background` was false and the socket was never
  tracked at all. Nothing closed it once the 30s reap was gone and the
  pool grew to 353.
- **Only on a window the caller owns** (`12c1d38`). A worker with no
  resident hold borrows whichever dispatch is newest, so tracking a
  borrowed window let an unrelated request close the `customresources`
  CRD informer's socket. The required e2e set went from 18 failures to 4.

### Open

- **Hangs.** "The Workers runtime canceled this request because it
  detected that your Worker's code had hung": 0 before this series, 34-50
  after, all in `APIGroups`. The 250ms tick had been hiding whatever the
  runtime needs to see; a 10s Go ticker recovers most but not all of it
  (165 → 34). One e2e failure is a configmap delete that hung; the other
  three cascade from the node going briefly unschedulable during it.
- **Watch churn is still 236/min** against roughly 6/min expected from
  the reflector's own 5-10 minute timeout.
