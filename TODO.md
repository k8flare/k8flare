# TODO — the road from "alpha" to "a team can run this"

The bar this file is written against: **a team can put a workload they care
about on k8flare and not be paged about the control plane.** Nothing here is
style or polish; every item is something that, left alone, either loses data,
hides an outage, or makes a green CI run mean less than it appears to.

Source: an adversarial product review on 2026-09-11 against `main` @ `43011df`,
plus the open items already recorded in `docs/platform-verification.md`
(S28–S36). Where a claim has evidence, the evidence is cited; where the review
declined to judge for lack of reading, that is marked too.

The review's one-sentence verdict, kept here because it sets the priorities:

> k8flare's correctness rests on Cloudflare runtime behaviours that only
> manifest in production, and nothing in the repo observes production. The
> deployed control plane was broken from 2026-07-26 to 2026-09-09 while every
> local gate stayed green.

Status legend: `[ ]` not started · `[~]` in progress · `[x]` done (link the
commit) · `[!]` blocked or deliberately deferred (say why).

---

## P0 — blocks "a team can run this"

### P0-1 `[x]` Observe production, not just `wrangler dev`

**Problem.** There is no signal that the deployed control plane works. S30 sat
undetected for six weeks: DO-origin Loader calls failed, dynamic workers
reloaded every minute, `kubectl scale` was ignored for 3+ minutes, and the
safety-net alarm never parked — a standing violation of cost invariants #1/#3.
Every local gate was green throughout, because `cost-gate.yml` and every test
lane run against `wrangler dev`, which does not reproduce the production
runtime semantics that broke (`pkg/cfruntime/cloudflare/window.go` says so in
its own doc comment; the `PUMP_WINDOW_DROP_CLOSE` knob in
`packages/k8flare-worker/src/controllers/index.ts` exists only because dev
cannot produce the fault).

**Do.** A scheduled synthetic convergence probe against a real deployment:
create a Deployment → wait for a Pod to reach Running → delete it → assert the
cluster parks within N minutes → assert zero Worker/DO requests for the
following M minutes. Alert on failure. It must run on a schedule against the
real deployment, not in `wrangler dev`.

**Acceptance.** The probe fails when pointed at a deployment with the S30
defect reintroduced (verify by deploying a version with the fix reverted, or by
an equivalent fault injection), and passes against current `main`. Failure
reaches a human without anyone watching a dashboard.

**Done 2026-09-11**, except the scheduled half. Run end to end against the real
deployment: readyz passed with every component reported by size and sha256 and
the verdict cache visibly bounding the cost; a Pod reached Running in 5m13s on
a demand-started NodeVM; the scale to two converged in 2m47s; the workload was
gone 8m05s in. It then refused to assert idle cost because its own
demand-started nodes were still attached -- correct behaviour, wrong timing,
now fixed by waiting for them to detach (they took about two minutes).
**Still open**: `prod-probe.yml` has never run on a schedule, because that
needs a maintainer to set the secrets it documents. Until then nothing watches
production between manual runs.

### P0-2 `[x]` `/healthz` reports nothing about health

**Problem.** The gateway answers `/healthz`, `/livez`, `/readyz` without
touching storage or any component
(`packages/k8flare-worker/src/gateway/index.ts:45`), deliberately, to keep the
probe cheap. The consequence is that an operator's liveness check cannot
distinguish "control plane serving" from "control plane wedged" — which is
exactly the state production was in for six weeks.

**Do.** Keep a zero-cost liveness path, and add a real readiness path that
exercises storage and reports per-component status (apiserver, kine/DO, the
resident workers that are supposed to be loadable). Bound its cost and its
frequency so it cannot itself violate the idle-cost invariant; an endpoint that
only does work when asked is fine, a self-polling one is not.

**Acceptance.** With a deliberately broken Loader path, readiness goes
not-ready while liveness stays cheap. Documented in `docs/admin-guide.md` with
the cost per call.

### P0-3 `[x]` UID/resourceVersion preconditions are dropped on DELETE

**Problem.** `ResourceStore.upstreamMarkForDeletion`
(`pkg/apiserver/upstreamregistry.go:263-266`) constructs a fresh
`&metav1.DeleteOptions{PropagationPolicy: &policy}` and passes that to the
upstream store, discarding the caller's `Preconditions`. A client that sends a
UID-guarded DELETE — which is how clients avoid deleting a *recreated* object
with the same name — has that guard silently ignored. Upstream's own GC sends
UID preconditions. This is a data-loss shape, not a latency bug. Recorded as
open in S33.

**Do.** Thread the caller's full `DeleteOptions` (preconditions, grace period,
dry-run) through to the upstream store. Check every other place this repo
rebuilds an upstream options struct by hand for the same class of bug.

**Acceptance.** A test that deletes with a stale UID precondition gets `409`
(or upstream's exact behaviour) and the object survives; the same DELETE with
the correct UID succeeds. Plus an audit note listing the other options structs
checked.

### P0-4 `[~]` Resident controllers are discontinuous across pump windows

**Design proposed, awaiting review:** `docs/pump-window-design.md` (2026-09-11).
It recommends keeping bounded WASM execution and separating physical
connection lifetime from logical watch continuity — resourceVersion-resumable
watches first, then durable change notification from storage — and explicitly
refuses to promise that resumable watches alone retire the four
`gracefuldelete.go` guards. Seven stages, each independently shippable, each
with the production measurement that decides it.

**Problem.** The deepest issue and the reason the other symptoms keep
reappearing. kcm/gc/sched run as resident WASM pumped in bounded windows;
informer watches are torn down at every window boundary and re-established on
the next poke. Downstream symptoms already measured: node recovery after a
heartbeat outage takes ~5–6 minutes to clear `unreachable` taints (S31
addendum 2), the scheduler transiently binds Pods onto a node carrying
`unreachable:NoSchedule`, foreground deletion needed four separate apiserver-side
guards to stay inside a 90s budget, and local measurement still shows roughly
1 run in 5 near that budget (S36).

**Do.** This needs a design pass before code. Options to evaluate against the
cost invariants, with measurements: longer or overlapping windows; keeping the
reflector's watch alive across a window boundary; a resume-from-resourceVersion
path so a re-watch does not re-list; or accepting discontinuity and making the
controllers' work idempotent-and-resumable by design. Write the design down and
get it reviewed before implementing (CLAUDE.md's "design first").

**Acceptance.** Taint clearing after a node returns is bounded and measured in
production; no scheduler binding onto `NoSchedule`-tainted nodes; the
foreground-deletion guards in `gracefuldelete.go` can be reduced rather than
added to. Numbers recorded, production-measured.

---

### P0-7 `[ ]` ノードを付けると replicas=2 が Pod を 35 個作る

**Problem (S56).** 本番と同じ構成(全 dynamic worker、ホストプロセス無し)で
**動くノードを 1 台付ける**と、`replicas=2` の ReplicationController に対して
**35 個**の distinct な Pod が作られ、最終的に 2 個へ収束する。ノードが
付いていなければ 2 個で、それが今日までの本番測定の条件だった。

採用者が最初にやること——ノードを 1 台繋いでワークロードを出す——でこれに
当たる。書き込み量、スケジューリング、kubelet の起動が 17 倍になり、コスト
不変条件(rows written / alarm / Containers 起動)にも直接効く。

これが見えなかったのは二つの条件が重なっていたため: 本番の prodprobe
クラスタにノードが無く、ローカルハーネスのノードは S53 まで Pod を起動でき
なかった。両方が今日直って初めて出た。

S55 の required GC conformance の flakiness(動くノードで 6 回中 0 回しか
7/7 にならない)も、おそらくこの churn の下流である。

**First measurement taken (S56).** 動くノードでも informer の取りこぼしは
**1%** で、expectations の破綻ではない。kcm は Pod を POST 6 / DELETE 60、
scheduler は bind を 70 発行していた。**ただし distinct な Pod 名 35 個と
POST 6 件が矛盾しており、計器のほうが合っていない。** 機序は名指しできない。

**Actor identified (S56).** バースト耐性のある `request` 境界(シェル側、
User-Agent 付き)で数え直した: **replication-controller が Pod を 32 個作り
38 個消している**(kcm 全体で作成 35 = commit の distinct 35 と一致)。
GC でも kubelet でも scheduler でもない。informer は届いている(取りこぼし
1%)のに、controller が持つ Pod 集合の像が安定していない。**なぜ像が安定
しないかは未証明。**

**Four candidates eliminated (S56).** 4 つ目: `RejectCreateWithTerminating
Controller` による 403 で作成が失敗扱いになる筋。存在しない owner なら実際に
403 になるが、バーストでは POST 32 件に対して作られた Pod も 32 個で、403 が
混ざった形跡がない。関与なし。

**Three further candidates eliminated (S56).** 291ms で 24 個作っている窓について:
dynamic worker の再ロードは **0 回**(expectations が消えたのではない)、
kcm の `observed` は 24 個目の前に **20 行**(informer は届いている)、
`pkg/leanclient/informers` の indexer / lister は client-go の生成コードと
同一(コード読みの範囲で問題なし)。**残る候補は expectations の経路。**

**Narrowed to the dynamic worker (S56).** `cmd/controller-manager -v=4` で
upstream 自身の expectations ログを読んだ。**ホスト CM は同じ条件で Pod を
2 個しか作らず、`expectations fulfilled` が 94 回出て正常**だった。単純な
RC 1 個での比較:

| 構成 | 作られた Pod |
|---|---|
| 本番(全 dw、ノード無し) | 2 |
| ローカル全 dw + 動くノード | **35** |
| ローカル host CM + 動くノード | **2** |

**過剰生成するのは resident な dynamic worker の KCM だけ。** 同じ upstream の
コードなので、差は k8flare の実行環境(pump window / watch の継続性 /
isolate の寿命)側にある。P0-4 の主題そのもの。

**Done, and it spoke (S57).** `KCM_VERBOSITY` を足して WASM の KCM に
klog を吐かせた: **`Too many replicas` 374 回、`Too few replicas` 0 回**
(ホストプロセスは同条件で 2 / 2)。controller の Pod 集合の像が実際より
多いまま維持されている。消しても像から減らないので、また消す。

**定量では言えない**: 同じ実行で DELETE 91 件に対し `observed.delete` 53 件
だったが、`observed` は Go 側でバーストに落ちる(S56)ので、この 42% 差を
取りこぼしと読んではいけない。使えるのは klog の質的な非対称のほう。

**S59 は取り下げ**(インスタンスは 1 つ、2 行目は tail 中継の診断行だった)。
確定している事実は: 動くノードがあると replicas=2 に対して Pod が 35〜60 個
作られ 2 個に収束する / dynamic worker の KCM 固有(ホストは 2 個)/
controller インスタンスは 1 つ / informer は add・update・delete を完全配送 /
klog は `Too many replicas` ばかりで `Too few` は 0。**機序を特定した (S60)。** 同じワークロード・同じ `-v=4` で実行形態だけを
変えて upstream 自身のログを比べた:

| | WASM KCM | ホストプロセス |
|---|---|---|
| `Warning: watch ended with error` | **477** | **0** |
| `Listing and watching`(relist) | **120** | **15** |

**pump window が閉じるたびに watch が切れ、reflector が 2 分半で 120 回
張り直している。** relist の最中・直後は controller の Pod 集合の像が権威と
一致せず、そこで `manageReplicas` が走ると作りすぎ・消しすぎが起きる。
個々のイベントは落ちていない(S58 訂正)——**落ちているのは連続性**。

**因果まで確かめた**: 隔離ワークツリーで `PUMP_WINDOW_MS` を 4 倍にすると
watch 切断 477→69、relist 120→34、作られる Pod 60→34。**churn は relist に
連動する。** ただし 4 倍にしても 34 個作るので、窓を伸ばすのは解ではない
(しかもコスト不変条件に反する)。

**一つ目の原因を除去した (S61)。** 窓が閉じた watch のボディ読み取りが
エラーを返していたため reflector が LIST からやり直していた。watch に限って
`io.EOF` を返すようにしたところ:

| | watch エラー | relist | 作られた Pod |
|---|---|---|---|
| 変更前 | 477 | 120 | 60 |
| **変更後** | **0** | **15** | **30** |

**relist はホストプロセスと同じ 15 回になった。** ただし **Pod は 30 個
残る**——**P0-7 には relist 以外の第二の原因がある。** 未特定。

**これは P0-4 そのもの。** 設計文書 5.1 の「欠落のない replay を先に確立する」
が未達であることの、実ワークロードでの定量化である。S61 で達成されたのは
「境界が欠落として扱われない」ことだけで、cut の保証はまだ無い。

以下は取り下げた仮説の記録:

**~~Two controller instances found (S59)~~.** `ResidentService` は `sync.Once` で
run を 1 回に抑えるのに、`controllerManager: run starting` が **2 行**出る
実行がある(シェル側の `dynamic worker up` は 1 回)。インスタンス数と過剰
生成の強さが揃う: 1 インスタンス → 4 個、2 インスタンス → 35 個 / 161 POST。
expectations はインスタンスのメモリに載るので、2 つあれば互いの Pod を
「余剰」と見て消し合う。S57 の `Too many` 374 / `Too few` 0 とも整合する。

**確定ではない**: 1 インスタンスでも 4 個作っている(目標 2)ので、二重化は
増幅要因であっても唯一の原因とは限らない。2 つ目がどこから来るかも未特定。

**Do.** Go 側でインスタンス固有 ID を作り `pumptrace` の component 名と
`run starting` に付ける。どのインスタンスが何を書いたかが取れれば、二重化の
実在と各々の振る舞いが同時に分かる。

**Acceptance.** ノードを付けた状態で `replicas=2` に対して作られる distinct
な Pod が 2 個であること。

## P1 — needed before the conformance story is credible

### P1-1 `[ ]` The required gate does not exercise the headline feature

**Problem.** `e2e-conformance.yml`'s REQUIRED variant is `host`: native
kube-scheduler and kube-controller-manager, with the WASM kcm/sched switched
off (`CM_DISABLED` / `SCHED_DISABLED`); only the gc dynamic worker runs. The
README sells resident WASM controllers built from real upstream packages, and
that configuration is **advisory-only** in CI. Reporting "required is green"
therefore says much less than it sounds like it does.

**Do.** Promote the dw variants to required — but only behind P1-2's evidence.
Until then, say so plainly wherever CI status is described (README,
CONTRIBUTING, and any status badge).

**Acceptance.** Either the dw variants are required, or every place that cites
the required gate states which configuration it covers.

### P1-12 `[ ]` required GC フォーカスは、Pod が動くノードの上では flaky

**Found 2026-09-12 (S55).** ローカルハーネスのノードを直して Pod が実際に
起動するようにしたところ、required な GC フォーカス(7 spec、host バリアント)
が **5/7 → 6/7** と揺れ、毎回違う spec が落ちるようになった。Pod が起動しない
ノードでは 7/7 だった。単独実行では通るので、フルの 7 spec の順序・蓄積状態に
依存する。

**CI の失敗と一致する。** 2026-09-09 に CI で落ちた
`should not delete dependents that have both valid owner and owner that's
waiting for dependents to be deleted`(`garbage_collector.go:795`、90 秒予算)は、
ローカルで落ちた 2 件のうちの 1 件である。**CI が止まっている間も、この失敗は
手元で約 6 分ごとに再現できる。**

落ちた 3 件はすべて orphan / Serial 系で、`gracefuldelete.go` のガードと GC の
相互作用が効く領域(P2-1 / P0-4 の対象)。

**Do.** (1) 回数を重ねて落ちる spec の分布を取る。(2) `PUMP_TRACE=1` の
`issued` / `observed` / `commit` を並べてどの境界で時間が消えるかを見る。
(3) `gracefuldelete.go` のガードを 1 つずつ無効化して切り分ける——**隔離
ワークツリーで**(S43)。

**Acceptance.** 10 回連続で 7/7、または落ちる理由が特定されて直っていること。
不可侵ルール #5。

### P1-2 `[~]` Prove the dw variants are stable, don't sample-check them

**Problem.** The dw variants were red in one of the last three runs
(`34445918793`: kcm-dw hit a client-side connection reset, sched-dw blew the
90s foreground-deletion budget) and green in the two after
(`34484825880`, `34491769003`). n=2 is not stability, and local measurement
still shows ~1 in 5 near budget.

**Do.** Run the dw variants repeatedly — target ~10 consecutive green — and
treat any red as a defect to root-cause, not to re-run. Watch Actions quota;
batch or schedule rather than hand-dispatching.

**Acceptance.** ~10 consecutive green dw runs, or a root cause for each red.

**Local evidence 2026-09-12, and it is not symmetric** (`docs/platform-verification.md`
S51). Same harness, same node, same required GC focus:

| variant | result |
|---|---|
| `host` (required) | 7/7 |
| `kcm-dw` (advisory) | 7/7 |
| `sched-dw` (advisory) | **0 of 3 runs passed**, 6/7 each time, on two different specs |

The contradiction with "6 consecutive green" in CI is real and unresolved: this
harness's node cannot start pods (S48), CI's can. But `host` and `kcm-dw` pass
7/7 on that same broken node, so a broken node alone does not explain why only
`sched-dw` fails. Promotion is a CI decision either way — the point here is
that the two advisory variants should not be promoted on the same evidence.

### P1-3 `[x]` The most platform-fragile code has no unit tests

**Problem.** Every `_test.go` lives in `pkg/apiserver` and drives a real
`wrangler dev`. `pkg/cfruntime` — pump windows, the JS boundary, promise
lifetimes, the code that caused S31 and S34 — has **zero** unit tests. There
are **zero** TypeScript tests. A regression in the window registry is caught
only by a 5-minute end-to-end lane, if at all.

**Do.** Unit tests for `pkg/cfruntime/cloudflare` (window open/close/expiry,
`CurrentWindow` racing a close, abandoned promises, `WithLiveBinding` when no
window is open) and for the TS glue that carries logic (the Controllers DO's
poke/park policy, the storage DO's `afterWrite` predicates, watch stream
lifecycle).

**Acceptance.** The S31 and S34 fault shapes are each covered by a unit test
that fails when the fix is reverted. TS tests run in `ci.yml`.

**Done 2026-09-12, and the acceptance criterion was checked by actually
reverting each fix** in a throwaway worktree rather than by reading the tests:

| Fix reverted | Tests that failed |
|---|---|
| S31 — `EnvFromContext`'s fallback to the open pump window (`pkg/cfruntime/cloudflare/env.go`) | `TestEnvFromContextFallsBackToTheOpenWindow`, `TestBindingFromContextResolvesOnTheOpenWindow` |
| S34 first shape — `toJSResponse` returning plain values instead of a `Response` | `TestToJSResponseReturnsPlainValuesNotAResponse` |
| S34 second shape — no body on the Fetch spec's null-body statuses | `TestToJSResponseSendsNoBodyForBodilessStatuses` |

Each mutation failed only its own tests, so they discriminate rather than
tripping on any change. Coverage now stands at 17 Go unit tests across
`pkg/cfruntime` (3), `pkg/cfruntime/cloudflare` window registry (9) and env
resolution (5), plus 50 TypeScript tests; `make test-ts` runs in `ci.yml`
(line 109). One of those TS tests was itself flaky and was fixed the same day
— it compared an alarm interval against the 600s ceiling using a timestamp
sampled before the call, so it failed by exactly 1ms whenever the call was
slow enough.

### P1-4 `[x]` Destructive DO migrations replay on deploy

**Problem.** `packages/k8flare-worker/wrangler.jsonc` carries `migrations`
including delete+recreate cycles, `wrangler deploy` applies whatever is in the
file, there is no backup path, and `docs/adopter-quickstart.md` mitigates this
by telling operators to `git diff` before upgrading. That is a footgun handed
to the user.

**Do.** Make a destructive migration impossible to apply by accident: gate it
behind an explicit opt-in, or move already-applied migrations somewhere they
cannot be re-run, or provide an export/import path so state loss is
recoverable. Document the recovery story.

**Done 2026-09-11** for the first half. `npm run check:migrations` hashes the
block against `migrations.sha256`, runs in CI and ahead of `make deploy` /
`npm run deploy`, and refuses when it changed. Verified both directions: adding
a `deleted_classes` tag exits 1 with the recorded and actual hashes, the
override env var passes it, reverting passes again. It does not cover a direct
`wrangler deploy`, which is documented.

**Backup added 2026-09-11**: `cmd/k8flare-backup dump|restore` walks discovery
and round-trips every served object. Verified against the real deployment — 21
objects dumped, the namespace deleted, then restored with ConfigMap data,
labels and Deployment replicas intact. **Still open**: it cannot cover the CA
keypairs or the token vault, which live in Durable Object facets the
Kubernetes API does not serve, so a restore into a fresh deployment returns
workloads but not cluster identity. Nothing schedules it.

**Acceptance.** An operator who pulls and deploys cannot lose cluster state
without an explicit, separate action. A documented way to export and restore a
cluster's DO state.

### P1-5 `[x]` User-facing docs describe intent, verification docs describe reality

**Problem.** `docs/admin-guide.md` §1 says nothing runs when idle — false in
production for six weeks. §5 documents ~65k alarms/month on an unconverged
cluster with the cause "未特定", which is a standing cost-invariant #3
violation living in the operator guide. `README.md`'s doc table says
platform-verification covers "(S1–S26)"; the file runs to S36.

**Do.** Reconcile every user-facing claim against what the verification docs
actually establish, and mark anything verified only in `wrangler dev` as such.
Fix the stale index. Add a "currently known broken" page an adopter can read in
two minutes.

**Acceptance.** No user-facing claim is contradicted by
`docs/platform-verification.md`. A known-issues page exists and is linked from
the README.

### P1-6 `[x]` `platform-verification.md` has trustworthy history and an unusable present

**Problem.** 5,663 append-only lines, no current-state index. Rule 4 (never
rewrite a correction away) is right and should stay, but a newcomer cannot
learn what is true today without reading the whole archaeology layer.

**Do.** Add a current-state summary at the top — what is verified in
production, what is verified only locally, what is known broken — each line
pointing at the section that establishes it. The history stays untouched below.

**Acceptance.** A reader can answer "what works today?" from the first screen.

---

### P0-6 `[~]` Stage 0 of the pump-window design (instrumentation and cost contract)

Attempted 2026-09-11 and **discarded**. Two delegated agents were each cut off
by provider rate limits mid-task and left unverified work; the salvaged result
passed `vet` and the unit lanes but killed the Worker at startup — every
wrangler-lane test failed with `connection refused`, so `wrangler dev` never
came up. The changes had also strayed past Stage 0 into the clientgo-lean
mirror and `pkg/controllers/restconfig`, which is Stage 2 territory.

Redo it scoped tightly: the three-boundary instrumentation (commit → informer
observed → controller acted) attributable to a pump window and component, with
no always-on cost, plus the cost-model entries and a probe-traffic
discriminator. Note that S37 removed the urgency: the node-recovery latency
this instrumentation was meant to localise is no longer a defect.

**Redone 2026-09-12, scoped as written above** (`docs/platform-verification.md`
S44). Done:

- Baseline at the deployed hash **before** instrumenting: convergence 16s,
  55-pod foreground GC owner gone in 9s / all pods in 12s, ten idle minutes
  with zero writes.
- All three boundaries emit under `PUMP_TRACE=1` and were driven in a real
  Worker, not just unit-tested: request 161 / commit 38 / observed 6 lines for
  one Namespace plus a 2-replica ReplicationController.
- Attribution works: each `observed` line names the pump window it arrived in.
  Measured commit → informer observed at 4–29 ms **under `wrangler dev` only**.
  That figure does not hold in production: the two boundaries are stamped in
  different isolates and Workers' clocks disagree (S46). The production figure
  that does hold is `issued` → `observed`, both on the Go clock: **82–93 ms**
  (S49).
- All four resident components emit observations, not just KCM: `kcm`, `sched`,
  `clusterop`, and `gc/<resource>` split per GVR, which shows exactly which
  resource kinds the real garbage collector walks (the 27 kinds S41 measured).
  The helper lives in the leaf package `pkg/pumptrace` so importing it does not
  drag the controller-manager into the gc, sched and clusterop chunks.
- Zero always-on cost, asserted both ways: one string comparison when unset,
  no informer handler registered at all, and 0 `pumptrace` lines across every
  test lane with the var unset.
- Cost-model entry with the per-boundary line counts and the Workers Logs
  budget that follows from them.
- Probe-traffic discriminator: `cmd/prodprobe` now sends
  `k8flare-prodprobe/<run>` as its User-Agent, verified over a real HTTP round
  trip.
- **An error in the instrument itself, found and fixed**: the commit timestamp
  was taken after `broadcastEvent`, so commit → observed came out *negative*
  (-3 to -1 ms). Moved to immediately after the revision is assigned.

Not done, so this stays `[~]`:

- ~~The `observed` boundary is invisible in production~~ — **fixed and verified
  there** (S46). A Loader-spawned worker's console output does not reach the
  loading script's `wrangler tail` (S45); `WorkerLoaderWorkerCode.tails` with
  `env.SELF` does deliver it. Measured in production: `request` 262, `commit`
  50, `observed` kcm 20 / sched 5 / gc per-GVR 13. With the knob off: zero
  trace lines, zero relay lines, **zero tail-handler invocations**, so nothing
  is billed for a cluster nobody is measuring.
- **Duration IS measurable in production for the controller-side loop** (S49).
  An `issued` boundary in the Go transport shares a clock with `observed`, so
  the two can be subtracted: measured in production, a scheduler bind reaches
  the controller manager's informer in **82–93 ms**. Read the low end of the
  distribution — informer resync re-delivers the same revision seconds later.
  Writes whose object is not named in the path (a create with a generated
  name) cannot be paired this way.
- **Duration across the shell/Go boundary is still not measurable** (S46).
  `commit` is timestamped by the shell Worker and `observed` by Go inside the
  dynamic worker; Workers' `Date.now()` freezes at the last I/O, so the two
  clocks disagree — three consecutive revisions all came out at **-888 ms**, a
  systematic offset rather than noise. S44's 4–29 ms was a `wrangler dev`
  number, where one process means one clock, and is **not** a production
  figure. Ordering, attribution (window / component / revision) and
  same-clock deltas do work. Carrying a monotonic commit marker through the
  watch to the Go side is Stage 1's job, not Stage 0's.
- The node-stop 90s/10min baseline — it means stopping the agent on the
  maintainer's VM, deferred to a daytime window rather than done at 03:00.
- Idle request and alarm counts still need the Cloudflare Analytics token.

## P2 — known defects and accidental complexity

### P2-1 `[~]` `gracefuldelete.go`'s guards are compensating for the platform

Four hand-written guards (`RejectCreateWithTerminatingController`,
`refuseForegroundFinalize`, `FinishUnblockedForegroundOwners`,
`sweepOrphanStragglers`), each full-listing every namespaced store per
create/finalize, each with a doc comment ending "upstream needs neither
guard". Rule 3 is satisfied in letter — the real GC is unmodified — while its
cost migrates into hand-written apiserver code compensating for
platform-induced informer lag. Revisit after P0-4; the guards should shrink,
not grow. Also measure their rows-read cost, which is currently unmeasured.

**Measured 2026-09-11, and the answer is no — not yet.** The experiment
disabled `FinishUnblockedForegroundOwners` and repeated the required
garbage-collector focus locally. Five runs passed, then the wall time climbed
253s → 585s → 899s and run 6 died in `BeforeSuite` at the 900s timeout.

The decisive number came from the machine after the experiment stopped: 66
minutes later, with no test running, the Worker was still serving **451
requests per minute**. 1260 of the last 1289 were `GET
/api/v1/namespaces/gc-8627/pods/<name>` returning 404, against a namespace
that no longer exists. The real garbage collector retries forever, and nothing
completes the owner it is blocked on — which is precisely the job the guard
does (S36).

That is a cost-invariant #1 violation (~650k requests/day on a cluster that
can never converge), not a latency regression. **Removal stays blocked behind
P0-4**; re-measure once pump windows are continuous. Full write-up and the two
caveats (the run was killed by `timeout` so framework cleanup never ran; the CI
failure was a 90s timing budget that a fast laptop does not reproduce) are in
`docs/platform-verification.md` S43.

### P2-2 `[x]` `pendingPing` asymmetry in the storage DO

`packages/k8flare-worker/src/storage/index.ts:261`: `afterWrite` sets
`pendingPing:controllers` regardless of whether the `CONTROLLERS` binding
exists, but `pingControllers` does not clear it when unbound, so an unbound
config re-arms the safety-net alarm every 60s forever. `pingNodes` clears it.
Pre-existing, recorded in `docs/custom-code-inventory.md` §5.

### P2-3 `[x]` Event POSTs 400 on first write

The real kcm's event broadcaster gets `400 "Object 'Kind' is missing"` on its
first `POST events` because this apiserver does not infer kind from the URL
path the way upstream does. Retries succeed, so events are only partly lost.
Recorded in S32.

### P2-7 `[x]` Pod-on-Containers does not work in production

Found by the first scheduled run of the production probe (S39 and its
correction). A Pod annotated `k8flare.com/compute=containers` is admitted
correctly and the scheduler is poked — `CFContainersScheduler` answers `ok` —
but every `NodeVMSmall` request ends `canceled` with no log and no exception,
no node ever registers, and the Pod stays `Pending` with
`Unschedulable: no nodes available`. The same path worked earlier the same day,
so it is a regression or intermittent, not unimplemented. Two hypotheses were
tested and disproved: a `provisioning` container application (all three are
`ready`), and the scheduler never being reached (it is, the tail filter was
too narrow).

The probe no longer depends on this capability, so it is not blocking daily
monitoring — but Pod-on-Containers is a headline feature that currently does
not work.

**Fixed 2026-09-11** (S39 続報). `reconcile()` issued `stub.up()` once from the
poke's detached context and never retried, because the claim it persists first
made every later pass skip the Pod. An abandoned promise neither resolves nor
rejects, so nothing surfaced. Tracking `started` separately from `bound` lets
the existing 15s safety-net alarm re-issue the boot. Verified in production:
node registered at t+100s, Pod Running at t+160s, where before it stayed
Pending indefinitely. **Still open**: the mechanism behind the `canceled`
outcome itself is not identified, and `wrangler dev` cannot reproduce it — it
does not abandon detached DO subrequests, which is why no gate caught this.

### P2-4 `[x]` k3s agent's remotedialer tunnel 401-loops

**Done 2026-09-11 (S38).** The agent dials with no Authorization header at all
— pinned k3s calls `ConnectToProxyWithDialer(ctx, wsURL, nil, ...)` and relies
on an mTLS client certificate that Cloudflare strips — so the gateway's door
rejected every attempt: about 28,800 billed requests per day per attached node.
`/v1-k3s/connect` is now on the unauthenticated allowlist, which is what the
stub's doc comment always claimed. Measured in production: 401 retries 0,
`Remotedialer connected to proxy` once, 3m41s after the node started.
**Still open**: it admits an unauthenticated socket, and holds it in the shell
Worker rather than a hibernating Durable Object. The tunnel is unused, so the
right answer is for the agent not to dial at all; k3s has no switch for that.

### P2-5 `[x]` `PUMP_WINDOW_DROP_CLOSE` is a test knob in production code

Added so `wrangler dev` could reproduce a production-only fault
(`packages/k8flare-worker/src/controllers/index.ts`). Keep it only if it is the
cheapest way to hold that regression; if so, document it as a test seam and
make sure it cannot be enabled in a real deployment by accident.

**Kept, and fenced.** It is the cheapest seam: the fault is that production
tears a poke's IoContext down before `ctx.waitUntil`'s timer runs, and
`wrangler dev` never does that (S31 E1), so three regression tests
(`gcmultiowner_test.go`, `kcmdw_test.go`, `ioctxprobe_test.go`) can only reach
the wedge by dropping the close deliberately. Each passes it per invocation as
`wrangler dev --var`, so nothing about it lives in a deployment.

What was missing was the guard. `packages/wasm-build/src/check-test-vars.ts`
now refuses to deploy if `wrangler.jsonc` declares any harness-only var, wired
into `npm run deploy`,
`make deploy` and `ci.yml` alongside the migrations check. Verified both ways:
it passes on `main` and fails with the var added. It does not — and cannot —
catch a deliberate `wrangler secret put` of the same name; the accident it is
built for is a test invocation's `--var` being copied into a config.

The guard covers three more names than P2-5 asked for, because the same
accident has the same consequence for all of them: `KCM_DISABLED`,
`SCHED_DISABLED` and `CM_DISABLED` are harness kill switches that let a host
process stand in for a resident controller. Checked 2026-09-11 that all four
appear only as per-invocation `--var` in test lanes and `e2e-conformance.yml`,
never as deployment configuration, so the guard cannot block a legitimate
deploy. Unlike the fault knob they do not corrupt behaviour, they remove a
controller -- the error message says so rather than calling them all fault
injection.

### P2-6 `[~]` `deps-k3s-update` is failing on `main`

Diagnosed 2026-09-11 (run 34169780304): the bump job pushes the branch fine —
`deps/k3s-v1.36.4-k3s1` is on the remote — and then `gh pr create` fails,
because the repository has "Allow GitHub Actions to create and approve pull
requests" off (`can_approve_pull_request_reviews: false`) and no
`DEPS_UPDATE_TOKEN` secret is set. The step now says exactly that instead of
failing opaquely.

**Needs a human decision**: either enable that repository setting (which also
permits Actions to approve PRs — a security consideration), or create a
`DEPS_UPDATE_TOKEN` with `pull-requests: write`. Until then the workflow will
keep going red weekly, correctly. There is also a real pending k3s patch bump
sitting unmerged on that branch.

---

### P0-5 `[x]` `?dryRun=` was ignored entirely

Found by the P0-3 audit and fixed in the same pass: the apiserver parsed
`dryRun` nowhere, so a server-side dry run created the object, allocated its
ClusterIP and ran the post-create effects. Now validated with upstream's
`ValidateDryRun` and threaded through to the store, with the side effects
suppressed. Left open here because only the create/update/patch paths were
covered -- delete, deletecollection and the subresources still need the same
treatment, and none of it is verified against a real `kubectl --dry-run=server`.

### P1-7 `[x]` `pkg/cfruntime`'s root package cannot be unit-tested

`handler_js.go`'s `init()` reads `globalThis.context.binding` at program start,
so merely adding a test file to the package panics with
`syscall/js: call of Value.Get on undefined`. The consequence is that S34's
first fault shape -- the `toJSResponse` fix that builds the Response in JS
rather than Go -- has no unit test. Making it testable is a restructure of the
package's initialisation, not a seam.

**Done 2026-09-11.** `init()` now calls `registerBinding()`, which returns
false instead of dereferencing an absent `globalThis.context.binding`, so the
package accepts test files. `toJSResponse` is covered in both directions:
rebuilding it as a real `Response` (S34's first fault shape) fails the suite
with "toJSResponse built a Response; it must return plain values so the
bootstrap can construct one in the request's own context".

**Still open**: the unit tests run in node, not workerd, so they cover logic and
promise/stream semantics but not input gates, real IoContext teardown or DO
storage semantics. `@cloudflare/vitest-pool-workers` would close that gap at
the cost of a dependency.

### P1-8 `[~]` A k3s patch bump is sitting unmerged

`deps/k3s-v1.36.4-k3s1` (from the weekly automation on 2026-09-07) moves the
pin from k3s v1.36.3 to v1.36.4 and the `k8s.io/*` staging replaces with it.
It never became a PR because of P2-6's repository setting, so it has been
sitting on the remote while `main` moved on. A dependency bump that carries
upstream fixes should not rot.

**Verified locally 2026-09-11** on `deps/k3s-136-4` (that branch with current
`main` merged in): `make vet`, `make check`, `tsc` clean; `make gen` produces
no drift; all five WASM chunks under the Loader cap (apiserver headroom
2,389KiB, down 177KiB from 2,566KiB); `make test-unit`, `test-apiserver`,
`test-kcm` and `test-clusterop` all pass.

**Not merged**: the Definition of Done is conformance, and Actions capacity is
exhausted (see below). Merge once `e2e-conformance.yml` has run green against
this branch.

**Local verification 2026-09-12** (GitHub Actions capacity is still exhausted,
so this is the substitute for the conformance gate):

- Current `main` merged in; `make check`, `make vet` clean.
- `make clean-wasm wasm` reproduced every chunk byte-identical to the branch's
  existing build, and all five are under the 64MiB Loader cap — apiserver has
  the least headroom at 2389 KiB.
- `make test` green: apiserver 69.9s, `TestKCMDynamicWorkerControlPlane`
  414.3s, `TestClusterOperatorLifecycle` 70.1s, 50 TypeScript tests.
- The **required** garbage-collector focus: `Will run 7 of 7579`, then
  **7 Passed / 0 Failed in 141s**.
- The baseline focus fails locally — but see the rule below before reading
  anything into that.

**Decision rule, written before the comparison came back.** The local harness
cannot run the `host` variant that gates baseline in CI (no host scheduler or
controller-manager process; `docs/development.md`), so a local baseline
failure is unattributed on its own. Running the same focus against `main`:

- main fails the same specs → environmental, merge on the strength of the GC
  focus and `make test`, and say plainly that the host baseline was not
  locally reproducible.
- main passes them → a real regression in the bump; do not merge.
- main fails one and passes the other → not an average. Inviolable rule #5
  applies: take a second sample of each before concluding anything.

**The comparison came back: environmental.** Same harness, same node, same
focus, `main` at k3s v1.36.3:

| | specs completed | passed | failed | ended by |
|---|---|---|---|---|
| deps/k3s-136-4 (v1.36.4) | 6 in 2400s | 3 | 3 | a 40-minute tool timeout, not the suite |
| main (v1.36.3) | 4 in 3600s | 2 | 2 | ginkgo's own one-hour suite timeout |

`main` is not better — it completed fewer specs in more time, and it failed
`SchedulerPredicates` at `predicates.go:1041`, which the bump failed too.
Neither version got through the focus. The baseline focus is therefore **not a
usable local signal on this harness**, exactly as the decision rule anticipated,
and it says nothing for or against v1.36.4.

**Re-verified 2026-09-12 in the REQUIRED configuration.** The earlier run used
the harness's default, which reproduces the advisory `sched-dw` variant. S48
showed the required `host` variant can be reproduced locally too, so the bump
was put through it: host `kube-scheduler` and `kube-controller-manager` built
from the v1.36.4 tree, `wrangler dev` with `SCHED_DISABLED:1 CM_DISABLED:1`,
apiserver reporting `v1.36.4+k8flare`.

| focus | k3s v1.36.4 | current `main` (v1.36.3) |
|---|---|---|
| required GC, host variant | **7 Passed / 0 Failed, 101s** | 7 Passed / 0 Failed, 91s |
| baseline, host variant | 8 Passed / 3 Failed, 583s | 8 Passed / 3 Failed, 583s |

The three baseline failures are the same three `SchedulerPredicates` specs on
both versions, for the same reason, which is not the control plane: on Docker
Desktop the containerised node cannot create a pod sandbox (`seccomp is not
supported`, S48), so any spec needing a pod to actually run cannot pass here.
**The bump changes nothing the local harness can measure.** Chunks rebuilt and
all five under cap (apiserver has the least headroom at 2388 KiB).

**Still not merged, and the blocker is not technical.** Inviolable rule #1
makes conformance CI the Definition of Done, and it cannot run: every job on
the repository fails in 4 seconds with zero steps. The annotation says why —
*"The job was not started because recent account payments have failed or your
spending limit needs to be increased."* This is a **Billing & plans setting on
the `k8flare` org that a maintainer has to fix**; it is not capacity that
recovers on its own, and the earlier note in this file that called it
"exhausted Actions capacity" was wrong about the cause.

The branch is ready: rebased on current `main`, every local gate green, the
required GC focus 7/7. Merge it as soon as a conformance run can be
dispatched.

### P1-9 `[x]` Two compiled binaries were committed by accident

`k8flare-backup` (34.0 MB) and `prodprobe` (34.6 MB, twice) were committed to
`main` on 2026-09-11 before `.gitignore` covered them. Untracked and ignored the
same day; the blobs remain in history.

**Decided 2026-09-11: do not rewrite history.** Measured rather than assumed —
a fresh clone is **45 MiB** today, and the three blobs are most of it, so a
rewrite would bring it to roughly 12 MiB. Against that, the project's own audit
trail cites **24 commit SHAs** in `docs/platform-verification.md` alone, and
rule 4 exists precisely so those references stay followable. Rewriting
invalidates every one of them, and every descendant SHA, to save 33 MiB on a
repository that nobody has cloned yet. That trade is not worth it.

Revisit only if the repository grows another accidental blob — at which point
one rewrite can clear them all, and should be done *before* the docs accumulate
more references, not after.

### P1-10 `[ ]` デプロイ後の暖機を運用者の手作業にしない (提案・要承認)

**Problem.** S50 で実測: **コントローラーの wasm が変わった**デプロイの後、
最初に来たワークロードは reconcile が始まるまで **20〜24 分**待つ
(wasm が同一のデプロイなら 23 秒で、この問題は起きない)。約 44MB のコントローラー WASM を、最初に使われた
時点で初めてコンパイルするため。2 個目以降は 11 秒。`/readyz` はこの間も 200 を
返す(S47)ので、ロードバランサや運用スクリプトからは区別できない。

現状の緩和は docs/admin-guide.md に書いた手順書きで、デプロイした人が自分で
Deployment を 1 個作って消す。**運用者が忘れたら利用者が 20 分待つ。**

**Proposal, not implemented — needs a decision.** `npm run deploy` の最後に
暖機を自動で起こす。候補:

- `/internal/warm` のような新しい認証付きルートを足し、Controllers DO に
  コンポーネントをロードさせる。書き込みを伴わないので revision を汚さない。
  代償は**新しい本番エンドポイント 1 本**で、クラスタトークン保持者なら誰でも
  44MB のコンパイルを起こせる(ただしトークン保持者は書き込みでも同じことが
  できるので、権限としては新規ではない)。
- deploy スクリプトが ConfigMap を 1 個作って消す。新しいエンドポイントは
  不要だが、deploy にクラスタトークンが必要になり、revision が 2 進む。

どちらも本番の挙動を変えるので、実装前に承認を得る。S50 を測っただけの現状
では**利用者が 20 分待つ既定のまま**であることを明記しておく。

### P1-11 `[x]` required gate が「Pod を 47 個作る制御プレーン」を通している — **誤報、取り下げ**

**Observed 2026-09-12 (S52).** `replicas=2` の Deployment 1 個に対して、
required な `host` バリアントでは 1 namespace に **47 個**の distinct な Pod が
作られ、それから 2 個に収束していた。`sched-dw` では 35 個で、そちらは
conformance の spec が落ちる。**host が通っているのは、spec がその瞬間の数しか
見ないからにすぎない。**

**訂正済み**: 当初これをホストプロセスの controller-manager に帰属させたが、
ホストプロセスを 1 つも使わない本番同形の構成(全 dynamic worker)でも
**18 個**作られた。分かれ目はバリアントではなく**ローカルか本番か**である
(本番は 2 個、`--cascade=orphan` 後 120 秒安定)。機序は未特定。ローカルには
Pod を bind するが sandbox を作れないコンテナノードがあり(S48)、本番の
prodprobe クラスタにはノードが無い——が、`FailedCreatePodSandBox` は Pod を
Failed にしないので、置き換えの引き金としては説明が足りない。

**Resolved the same day — this was not a defect.** ノードを止めて同じ spec を
回すと `replicas=2` に対して Pod は **2 個**になった。計装で機序も取れた:
Pod の commit 53 件のうち kcm の informer が取りこぼしたものは **0 件**、
一方で kcm は Node への PATCH を 24 件発行しており、同じ時間帯に kubelet が
`PLEG is not healthy` を 244 回出していた。**ノードがフラップして Pod が
evict され、replicaset-controller が正しく補充していた**だけである
(`docs/platform-verification.md` S52 訂正 2)。

required gate は「暴走する制御プレーン」を通していたのではなく、「Pod を
起動できないノードに対して正しく振る舞う制御プレーン」を通していた。
S51 の sched-dw 失敗も同じ補充サイクルのタイミング差に還元される。
**ローカルハーネスのノードが Pod を起動できるようになるまで、バリアント間の
差に意味を読み取ってはいけない**——これが S48 から変わらない本当の限界。

**Acceptance.** CI の host 変種で `replicas=2` に対して作られる distinct な
Pod が 2 個であること、またはそうでない理由が特定されていること。

## Out of scope / deliberately not doing

- `[!]` Replacing the hand-written REST layer with upstream
  `k8s.io/apiserver/pkg/endpoints`: measured NO-GO. Linking it costs +3.5MB
  after `wasm-opt` against ~2.5MB of headroom under the Worker Loader's
  67,108,864-byte cap, and the cap was re-measured on 2026-09-10 and is
  unchanged. See S29 and S35. Revisit only if the SSA/fieldmanager closure
  that `genericregistry` imports unconditionally can be trimmed.

## Not assessed

The review explicitly declined to judge these for lack of reading, so their
absence from this list is not a clean bill of health: `docs/cost-model.md`
body, S28/S29, `docs/user-guide.md`, `docs/development.md`, the `nodes/` and
`clusters/` TypeScript subtrees, and `cmd/agent`.
