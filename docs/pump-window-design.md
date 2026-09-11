# P0-4: pump window を跨ぐコントローラーの継続性

日付: 2026-09-11。対象: `design/pump-window`、調査時 HEAD
`f03d2a6f9deb52552e0b21c7dfa4bcc24b9681a6`。**設計案・レビュー待ち。実装承認や
P0-4 完了の記録ではない。** この変更は文書だけであり、本番への操作・新しい本番測定は行っていない。

## 判断

**有界 WASM 実行を残し、物理接続の寿命と論理 watch の継続性を分離する。**
まず resourceVersion (RV) による欠落のない再開を共通 Go watch 境界に実装する方向を推奨する。
その前提として storage の snapshot/replay/live 接続を検証・修正し、続いて
storage の確定済み変更を durable な起床理由として渡す。受信完了と処理完了を区別し、
再開時の cache catch-up、コントローラーの遅延キュー、時刻起因の仕事まで含めて park を決める。
実 KCM / GC / scheduler は残し、reconcile を apiserver や TS に移植しない。

ただし、**再開できる watch だけで四つの削除ガードを撤去できるとはまだ証明できない**。
最大の不確実性は、upstream の非同期 informer handler・GC グラフ・scheduler snapshot まで
鮮度を確認する境界を、小さな共通アダプターで作れるかである。ここが大量の upstream 改造を
要するなら、その段階は採用しない。前段の replay 改善は独立して出荷できる。

## 制約と証拠の読み方

CLAUDE.md 全文、とくにコンセプト・不可侵ルール・コスト不変条件を前提とする。
常時 Containers、固定周期による永久 wake、非 hibernation DO socket は採用しない。
`docs/cost-model.md:124` のユーザー決定も controller の Containers fallback を除外している。
API の CRUD / finalizer / UID / RV と実 upstream パッケージを使い、手書き制御の総量を減らす。

本書の「コード確認」はその分岐・契約を読んだ意味であり、本番での到達頻度や SLA の実測ではない。
「既存実測」は `docs/platform-verification.md` の記録を引用する。「提案」「目標」「未検証」は
今後証明が必要な事項である。不可侵ルール2に従い、推測を本番原因の確定扱いにしない。
ルール1/5に従い required conformance を減らさず、失敗を再実行で隠さない。
訂正は履歴を残す（ルール4）。この設計のレビューを通す前に実装を開始しない。

ファイル参照の `path:line` は調査時 HEAD の行番号。upstream は `go.mod:40` が pin する
`github.com/k3s-io/kubernetes/staging/src/k8s.io/client-go@v1.36.3-k3s1` を module cache で読んだ。
以下の `client-go/tools/cache/reflector.go` と `apimachinery/pkg/watch/streamwatcher.go` は
同じ `v1.36.3-k3s1` のソースを指す。この worktree に `.build` はなく、生成後 WASM の
reflector と完全一致することは今回ビルドして確認していない。

## 1. 現在の実行モデル

### 起床と park

| 経路 | コードで確認した動作 |
|---|---|
| storage 書き込み | `packages/k8flare-worker/src/storage/index.ts:56` の prefix 群（Node、Pod、Service、Endpoints、EndpointSlice、Lease、RS、RC、Deployment、DaemonSet、Job、CronJob、Cluster）で controllers を poke。Pod は nodes 側も poke。Node は lifecycle alarm も早める。`afterWrite` は persisted pending flag → 最短約1秒の alarm → detached ping の順（同:184,228）。 |
| 通常 poke | `packages/k8flare-worker/src/controllers/index.ts:335` の fetch は backoff を0にし、必要なら60秒後に alarm、kcm/sched/gc/clusterop を dispatch（同:375–388）。テナントの clusterop はロードしない（同:247）。 |
| コールドロード | ensure はコンポーネントごとに非同期ロードを共有（同:112）。Loader id は cluster identity / binary hash / token tag を含む（同:277）。最初の healthz fetch 自体が起床（同:311）。書き込み起因のロード完了は warmup を3分にし5秒後の alarm を張る（同:156）。alarm 起因では warmup を伸ばさない。 |
| Controllers alarm | 最初に warmup と `hasUnconvergedWork` を確認。収束済みなら次の CronJob 時刻だけを予約するか park（同:417–443）。仕事ありならロードして poke。warmup 中は15秒、以後15秒から最大600秒に backoff（同:477–490）。 |
| 仕事判定 | Deployment/RS/Job/RC の4 LIST、cron probe、pending deletion probe、管理クラスタでは Cluster LIST（同:550）。コメントの「2–3 list」より多い。失敗した workload/deletion probe は仕事あり。cron probe 単体の失敗は null（同:522）。StatefulSet や DaemonSet 全般の収束、全遅延キューの網羅的判定ではない。 |
| storage alarm | 未配達 ping を再送。**live Node が1つでもあれば** kcm 専用 poke を行い60秒後に再武装（`storage/index.ts:125,511`）。これは Node の存在に依存する周期監視であり、期限ごとの one-shot にはなっていない。 |

pending flag の削除は Controllers の HTTP 応答後であって、informer 同期や reconcile 完了後ではない
（`storage/index.ts:274`、`controllers/index.ts:401` は loading/202 も返す）。従って「poke 配達済み」
から「変更を処理済み」は導けない。また prefix には GC の全対象や StatefulSet/Namespace/
ControllerRevision/PVC 等が揃っていない。各 resource の write のみで起床できる保証はない。
`storage/index.ts:47` の「watch continuously、ping は初回だけ」というコメントも現在の窓契約と一致しない。

「アイドル ~0」は Node / CronJob / 未処理仕事 / 外部監視トラフィックがない静止状態について
区別して述べる必要がある。現行は Node を残すだけで毎分 alarm が続く。コメントが event-armed と
呼んでいても、CLAUDE.md #1/#3 の厳密な目標を満たしたとは扱わない。

### 窓と Go の寿命

1. JS は isolate ごとに `bindingPromise` を1つ持ち、各 dispatch で env と25,000msを
   `openPumpWindow` に渡す。`ctx.waitUntil(setTimeout(...))` で25秒後に close する
   （`packages/k8flare-worker/src/loader/bootstrap.ts:57–101`、`controllers/index.ts:57`）。
   窓は排他的でなく、複数 poke なら**既に重なる**。
2. Go bridge はその open/close を registry に渡す（`pkg/cfruntime/handler_js.go:76`）。
   registry は窓ごとの env / Done / expiry を保持し、新規 I/O には最新の生存窓を渡す。
   JS close が失われても25+2秒で Go 側期限が来る（`pkg/cfruntime/cloudflare/window.go:75–97`）。
   Go timer が凍結で遅れる可能性は同:90のコメントにあり、27秒は本番壁時計での解放 SLA ではない。
   `newestLive` は選択時にも期限を検査する（同:159）。
3. `ResidentService` は isolate 内で `sync.Once` により controller を1回起動する。
   context は窓の close では cancel されない。healthz の running は同期完了を意味しない
   （`pkg/cfruntime/residentservice.go:32–89`）。heap/cache/queue は isolate が残れば残る。
   eviction・token rotation・別 Loader id の起動では再構築になる。
4. `RestConfig` は `WithLiveBinding`、QPS=20/Burst=30（`pkg/controllers/restconfig/restconfig.go:68`）。
   実 KCM に渡す共有 informer、12時間 resync、Node monitor 5秒、grace 50秒は
   `pkg/controllers/controllermanager.go:62,70,111,241`。resync は窓の25秒と別である。

訂正: CLAUDE.md は apiserver を per-request fresh instance と説明するが、現行 bootstrap の
冒頭と実装（`bootstrap.ts:1,62`）は apiserver も isolate resident。**リクエスト単位の I/O** と
**Go インスタンス単位の寿命**は異なる。ここでは binding を都度解決する controller transport が対象。

### watch の断裂と次回 poke

`pkg/cfruntime/cloudflare/fetch/fetch.go:154` は RoundTrip 開始時に窓を選ぶ。
受け取った `streamBody` はその窓を保持する（同:204–210,234）。以後の body Read は
**最新窓へ移らず、元の窓の Done を待ち合わせる**（同:242–263,331–368）。窓 A が閉じたとき
窓 B が開いていても A の watch はエラーになる。旧窓の残り buffer が読める場合はあり、
close と同時刻に全イベントが消えるという意味ではない。

close は registry から窓を外して Done を閉じる。ネットワークをその場で強制 abort するわけではない。
`streamBody.Close` は EOF または窓終了後には JS cancel を呼ばない（同:289）。
従って「論理 watch 読み取り終了」と「WatchHub socket 解放完了」は別の計測が必要。
`k8s/watch.ts:233` の response cancel は socket retire へ繋がるが、どの終了でも直ちにそこへ
到達する保証ではない。S31 のローカル1306接続/11分・close 0 の記録を、現在の本番 leak 数とはしない。

lean client はこの body を upstream StreamWatcher へ渡す（`pkg/leanclient/watch.go:24–41`）。
通常 EOF 以外の decode error は watch.Error になる（`apimachinery/pkg/watch/streamwatcher.go:110–137`）。
reflector は正常な watch 終了なら `LastSyncResourceVersion` から watch を再開する能力を
**既に持つ**（`client-go/tools/cache/reflector.go:561–643`）。しかし一般の watch error は
watch loop を抜け、外側の delay loop が ListAndWatch を再実行する（同:423–435,644–670）。
WatchList 有効時は initial events を一時 store に集め、bookmark 後に Replace（同:470,804–909）。
S31 では window error → 次窓で全件再 list を実測している（`docs/platform-verification.md:4511`）。
全切断が必ず re-list になるという一般論ではなく、この error 分類が増幅を起こす。

S31 の修正後は `sendInitialEvents=true` を replay cursor として使わず、現状態 snapshot を
返す（`packages/k8flare-worker/src/k8s/watch.ts:96–108`）。次回 poke は停止した Go を動かし
新しい I/O anchor を供給するが、watch 再試行・snapshot 収集・handler/queue 処理が終わるまで
controller が見る cache は古い。初回の HasSynced は「この新しい窓の現在時点まで同期済み」の証明ではない。

ヘッダ前の window error には最大**2 attempts 合計**の再送がある。GET/HEAD/PUT/PATCH/DELETE に
限り、POST はしない（`fetch.go:104–142`）。ヘッダ待ちは10秒。body read の復旧にはこの再送は
効かない。また deadline も同じ ErrPumpWindowClosed を返し、次 attempt が同じ生存窓を使うことも
ある。「必ず別窓へ移る」「任意の要求が20秒以内に完了」は現コードからは言えない。
PUT/PATCH/DELETE の一括許可も、任意の副作用の安全な replay を証明するものではない。

## 2. 障害の機械的な関係と未確定部分

| 症状・証拠 | 機構と確定範囲 | 設計で除くもの |
|---|---|---|
| S31 初期の永久 watch 失速（`docs/platform-verification.md:4367`） | instantiate 元の binding の IoContext 消滅 → fetch promise が settle しない → reflector が待ち続ける。live binding と Done による解放で修正。cross-request AbortController は panic した（`window.go:23–46`）。 | I/O object の持ち越しを禁止。永続化するのは cursor と仕事であり binding ではない。 |
| S31 の phantom Node deletion（同:4490–4576） | boundary error → WatchList 再初期化 → rv を delta cursor と誤解 → 空 snapshot + bookmark → Replace → Node health map から除去。25/50秒窓で失敗、300秒で成功した A/B はこのバグを隠しただけ。rv=0 snapshot 修正後は25秒窓で36秒収束。 | snapshot と delta の意味を分離し、catch-up 完了まで不完全な snapshot を公開しない。 |
| 本番 Node 復帰5–6分（同:4705） | Ready=True の commit と KCM が読む Node/Lease の更新は別。古い cache・再 list・失敗/backoff・実行機会不足はこの間を伸ばせる。しかし原因の内訳は**未確定**。追記2は「close 欠落で永久停止」を本番真因として撤回。追記3（同:4826）では DW 10–20秒、host 10秒であり、窓の存在だけでは6分を説明できない。 | commit → replay → informer → health observation → taint PATCH の時刻を分解。改善は本番でのみ判定する。 |
| S32 の tainted Node への bind（同:4808） | taint commit 30秒超後も bind を測定。scheduler が古い Node snapshot を使えば TaintToleration filter は taint を見ず通る。しかし実際の cache RV を採っておらず、**stale informer は疑い**。一般の taint commit と bind の競合も区別する必要がある。 | scheduler の入力 barrier と snapshot RV、選定→binding の記録。厳密な bind 時点保証は別途必要（後述）。 |
| S32 784 writes/min（同:4736–4806） | NoExecute の即時 eviction → RS 補充 → tainted node へ bind → 再 eviction。実際の create/delete/bind を各61回/約30秒観測。no-op PUT が原因という仮説は撤回済み。DefaultTolerationSeconds 欠落が即時 eviction を可能にし、追加後27 writes/min。 | watch 改善だけで admission 欠落は治らない。300秒 default toleration と no-op 抑止は保持。 |
| foreground/orphan の保護（`pkg/apiserver/gracefuldelete.go:100,185,406`） | owner stream と Pod stream の lag により GC graph の依存欠落、KCM の補充が起こる。finalizer の早期除去拒否と orphan の後追い sweep は graph と storage の差を埋める。cross-GVR ordering は upstream も一般には保証しない。「upstream は全く lag しない」は証明ではない。 | 再開時の stream 間の追いつきと graph 処理完了を検証してから guard を減らす。 |
| S36 90秒超過（`docs/platform-verification.md:5383–5644`） | CI には90秒でLease書込9件があり poke 欠乏説は否定。guard の409、windowで放棄された live GET等が GC の RetryOnConflict と item exponential backoff を進めた。記録の73秒間隔は複合実測で、5ms×2^11そのものではない。FinishUnblockedForegroundOwners は retryを迂回。残る1/5は最後のPod未削除で原因未特定。 | 通常の境界を API failure にしない。未知の apiserver 応答停滞は独立に残す。 |
| S27 deadline 消失（同:4016–4077）、S36 park | isolateのtimer頼みでは eviction後に CronJob を逃す。時刻に一度起こすだけでも cold sync が終わらない。削除は workload spec/status 差分に現れず park。 | deadline と未処理理由を durable に持つ。単なるイベント受信 ack で park しない。 |

依頼文の補正: S36 の最後の1/5は「90秒に近い」だけでなく**90秒以内に消えなかった**。
四つの guard は全て foreground の90秒対策として同時に導入されたわけではない。
`sweepOrphanStragglers` は orphan のデータ保護、RejectCreate は補充抑止である。
また四つ全てが毎回全 store を LIST するわけではなく、RejectCreate は owner kind の point GET
（`gracefuldelete.go:430–438`）。残りの LIST 群も早期 return がある。削除対象と削減 rows を分けて測る。

## 3. コスト見積もりの基準

金額は新しい価格の断言ではなく **repo のモデルによる見積もり**。
`docs/cost-model.md:17–25,194–225,1228` の単位を使う。月=30日=2,592,000秒、
Loader=$0.002/unique loaded worker/day。公式 DO pricing の再取得は DNS 解決に失敗したため、
新しい $/request・$/GB-s・$/row 単価は未検証で記入しない。実装前に最新公式料金を確認し、
この見積もりを **docs/cost-model.md に追記してレビューする**（#5）。本変更では設計文書だけを追加する。

記号: `S`=保持 GB、`pS`=storage $/GB-month、`pD`=DO $/GB-s、`pR/pW`=read/write $/row、
`m`=duration課金の対象になる DO メモリGB、`T`=実測 billable active秒（I/O待ちを勝手に差し引かない）。
`U`=全 unique load の active-day 合計、`R/W`=課金対象 rows、`Q`=課金対象 requests。
月費は `S*pS + 0.002*U + R*pR + W*pW + Σ(m*T)*pD + requests料金 + Workers CPU料金`。
alarm invocation は件数を併記し、料金換算時に DO requests と二重計上しない。

CLAUDE.md #2 の「Workers/DO は実 CPUのみ」を DO duration 全般に一般化しない。
repo 自体が DO の GB-s を列挙し（cost-model:22）、非hibernating socket の常駐を警戒している
（同:88）。hibernation が成立する DO は idle duration を0にできるという設計目標であり、
ストリームを await し続ける DO を無料とは見積もらない。

静止月（期限・外部入力・未処理仕事なし）は、全採用候補で `Q=R=W=alarms=CPU=GB-s=U=0`、
月額 `S*pS` を要求する。最後の仕事の drain とその日の Loader 費用は別枠。
controller はテナント3、管理4なので、毎日使えば $0.18/$0.24/月、1日だけなら $0.006/$0.008。
apiserver や facet loader は既存の別勘定。poke ごとに新しい Loader id を作らない。

現行の Node を残すだけの60秒周期は最低43,200 alarm/月と同数の kcm 専用 dispatch を生む
（Node 書き込みによる前倒しと他の poke は別）。これは静止月の0とは違う。
未収束 backoff が600秒まで伸びても約4,320 alarm/月 + 初期段階が残る。
現行約60 watch（KCM+GC、scheduler等は別、S31の当時計数）を25秒ごとに再初期化する例では
1時間に8,640 subscription、snapshot 1回の全費用を `R_snap` rows とすると `144*R_snap` rows/h。
これは連続活動を仮定したモデルで、実測件数でも60倍の課金 request と同一でもない。
WatchHub upgrade → Cluster replay → namespaces facets の各 hop、SQL走査行数も数える。

## 4. 候補の比較

### A. 長い窓・重ねた窓

既存25秒を延ばし、仕事がある間だけ次窓を前倒しする。snapshot再取得頻度を減らす局所策。
物理 watch は元の窓に固定されるので、重複は既に存在し、それだけでは継続性を作らない。
S31 の300秒ローカル成功は誤った snapshot を隠した結果で採用根拠にならない。
本番の約60秒という S30/S31 観測を、保証された API 上限とも延命許可とも扱わない。

静止月は event-armed なら `S*pS`。連続20秒間隔の予防的延命なら129,600起床/月、
controller3本で388,800 dispatch/月となり #1/#3 違反。仕事中の1時間では再初期化回数の
理想値が25秒で144、50秒で72となるが、早期 IoContext teardown 次第で改善ゼロにもなる。
DO duration は実測 active秒×m。**恒久解として却下、比較実験の対照群にのみ使う。**

### B. 物理 watch を境界を越えて保持

旧 JS reader / binding をそのまま次窓で使用する案は、S31 の cross-request I/O / settleしない
promise を再導入する。DO に長い streaming call を保持させる案も hibernation を証明できず、
DO が awake のままなら月 `m*2,592,000 GB-s`（仮にm=0.128なら331,776 GB-s/DO/月）。
無イベントでも長期接続・周期timerが残るため、「Workersの待ちCPUが0」だけでは #1 を満たさない。

WatchHub サーバー側の hibernation socket 自体は既存の `acceptWebSocket`（watchhub.ts:89）でよい。
しかしそれは Go isolate の RAM / goroutine / クライアントreader の hibernation・再入を提供しない。
DO が別 DO から取得した socket を hibernation accept する迂回も実測で不可（同:4–13）。
静止時に全部閉じれば `S*pS` だが、それは物理継続を捨ててC/Eになる。
**物理接続保持は却下。接続を閉じても存続する論理 watch はCとして採用候補。**

### C. RV から論理 watch を再開

通常境界を、decoded event 単位の安全な終了・再 watch にする。upstream reflector の既存の
正常終了→RV watch を優先し、足りなければ共通 Go ListWatch/watch.Interface アダプターで
物理接続だけを張り直す。raw body のJSON途中からの継ぎ足しはしない。
LastSyncRV は store/decoder のどの地点まで届いた値かを明示し、最初の snapshot が未完なら
再 snapshot、410/期限外/evictionでも再 snapshot。初期同期なしで durable cursor だけを読んで
空 cache に delta を適用してはいけない。

静止月は `S*pS`、追加 durable consumer metadata があれば `(S+ΔS)*pS`。
活動費は境界あたり `1 subscription + replay hop群 + Δrows`、cold/410では `R_snap`。
旧設計の `144*R_snap/h` を `144*R_delta/h + cold回数*R_snap` に置換する狙い。
切断回数自体、namespace fan-out、全logをscanするSQLは自動で減らない。
新 Loader id / Container / 恒久 alarm は不要。**最小の独立改善として採用。**
ただし lost poke、遅延キュー、cross-stream lag、bind TOCTOU は単独では解決しない。

### D. 各 controller を durable な resumable/idempotent job にする

apiserver上の spec/status/finalizers/UID/RV を再開点にし、1回の有界 reconcile に分ける。
意味論としては正しい方向だが、upstream Run のインメモリ expectations、GC graph、scheduler
assumed Pod、taint evictionのtimer、遅延queueを個別に保存しようとすると巨大な別実装になる。
毎回全 cold start なら upstream は保てても LIST税と初期化遅延が重い。

静止月 `(S+S_job)*pS`、仕事batchあたり job checkpoint の `O(1)` rows/遷移 + APIの実書込、
cold start のたび `R_snap`、retry deadlineごと1 alarm、CPU/DO GB-sは活動時のみ。
「未収束」として永久ポーリングをすればこの見積もりは成立しない。
**全controllerの書き直しはルール3で却下。既存 API 状態を再構築の権威にする原則だけ採用。**

### E. storage の確定変更から consumer を起こす

best-effortな「全員 poke」を、cluster世代・対象resource・committed RVを持つ durable 通知にする。
コントローラーへ reconcile 指示をTSで作るのではなく、既存kine履歴と Go informerへ
exact change を届ける。依存関係の解釈・filter・eviction・bindingは upstream のまま。
通知は coalesce できるが、再生が必要なイベントの履歴を消さない。

静止月 `(S+S_cursor+S_outbox)*pS`。1 committed batch に dirty世代更新1、consumer受付・ack等
**論理metadata更新は上限 `1+2C`/batch を予算**（C=対象component数、実SQL/index/削除行は別測定）。
CPUとrowsは変更量に依存し、未ack batchのretryだけ alarm。B batchesなら配達 `≤B*C` が
通常経路の目安、失敗再送は別。全poke 3–4 dispatch/write と毎alarm全LISTを減らせる。
**Cを支える本命として段階採用**。通知は exactly-onceではなくat-least-once、queue受領だけで
「処理済み」とはしない。新しいconsumer databaseや常駐プロセスは増やさない。

### 全8不変条件に対する判定

○=構成上可能、条件=以下の証明が必要、×=その候補のままでは不採用。#5は全候補とも本書の
見積もりを cost-model に転記し単価確認するまで実装不可。#6/#7は全候補でPod費用を別枠、
API/gateway/watchはWorkers/DOのまま。

| 条件 | A 長窓 | B 物理保持 | C RV再開 | D job化 | E storage通知 |
|---|---|---|---|---|---|
| #1 idle=storage | 条件:仕事終了で閉鎖。予防延命× | ×:保持のため常駐 | ○:入力なしで停止 | ○:jobなしで停止 | ○:未ack/期限なしで停止 |
| #2 CPU対壁時計 | 条件:DOを長時間awaitさせない | ×:DO awakeと旧I/O寿命 | ○:DW CPU、DO活動GB-sのみ | 条件:WASM限定、Containers不可 | ○:有限dispatch、DO活動GB-sのみ |
| #3 event-armed alarm | 条件:仕事証拠必須 | ×:keepalive chain不可 | ○:新周期なし。起床欠落はEで処理 | 条件:retry対象とdeadline必須 | 条件:世代付き未ack/期限のみ |
| #4 hibernation watch | ○:既存WatchHub維持 | ×:Go readerはhibernatableでない | ○:窓外に物理接続を要求しない | ○:既存watch/有限read | ○:通知は有限fetch、外部watchは既存API |
| #5 事前見積もり | 本節A + 実測必要 | 本節Bで却下費用提示 | 本節C + 実測必要 | 本節D + 実測必要 | 本節E + 実測必要 |
| #6 workload別費用 | ○ | ○（常駐をPod費用に転嫁しない） | ○ | ○ | ○ |
| #7 hot path非Containers | ○ | ○（DO保持は別理由で×） | ○ | ○（asyncでもWASM限定） | ○ |
| #8 cost API再検討 | setTimeout/waitUntil増減を審査 | streaming/waitUntilを拒否 | one-shot/retry/cancelを審査 | deadline/parkを審査 | setAlarm/ack/parkを審査 |

## 5. 推奨設計の契約と実装境界

以下は**設計要求であり現行の保証ではない**。

### 5.1 欠落のない replay を先に確立する

Cluster root は revision 権威、namespaced value はfacetにある（storage/store.ts:124–162）。
root envelope→facet apply は跨DO原子transactionではない。resume の watermark は単なる最大idでなく
**そのprefixについて必要な全facet書込が読めると確認した committed cut**を表す必要がある。
失敗したfacet applyを飛び越すbookmark、value-less envelopeを正常eventとして扱うことは禁止。
現在の `replayDelta` は欠けたfacet行をenvelopeで代用する（同:309–315）。この保証を満たすまで
cursor最適化を有効にしない。parent/facet途中失敗のdurable intentと修復・commit可視化方法は
最初の実装レビュー項目であり、Cloudflareが原子化してくれるとは仮定しない。

snapshotはcut Hの完全な現状態、deltaは(cursor,H]の履歴とする。snapshot収集中の新しい変更は
H後のdeltaに流す。subscribe/replay/liveの切替では重複は許しても欠落・逆順・bookmarkの先行は
許さない。現行WatchHubはaccept後にawait replayし、その間のpushを受け得る（watchhub.ts:89–98）。
現行storeReplayもbookmarkを先に読んだ後にasync facetを読む（store.ts:262–269）。
これは競合検証が必要なコード上の懸念であり、今回本番破損を観測したという意味ではない。

既存の push は best-effort（storage/watch.ts:100）。再接続まで同じ接続が残るとmissを検出できない。
通知outboxはcommitと切り離して消えないようにし、cutを保証してから発行する。
RVはクライアントにはopaque文字列。比較は権威storage内だけで行う。cluster UID/世代、GVR、
namespace、selector、表示形式（full/metadata）ごとにcursorを隔離する。
履歴保持の上限を決め、期限外は明示410→完全再同期。無制限retentionでidle storageを増やさない。
compactionは仕事起因のbatchに同乗し、idle vacuumのcronを新設しない。

### 5.1a Stage 0 が実測で持ち帰った、Stage 1 のレビューに渡す入力

以下は 2026-09-12 に Stage 0(S44–S49)を本番で回して分かったことで、5.1 の
契約を確定させる前にレビューが見るべき材料である。**本書を書いた時点では
まだ分かっていなかった。**

**(a) cut は「時刻」では表現できない。** commit を刻むのはシェル Worker、
observed を刻むのは dynamic worker 内の Go で、Workers の `Date.now()` は
直近の I/O 時点で止まる。本番で連続する 3 revision の commit→observed が
揃って **-888ms** になった(S46)。つまり 5.1 の committed cut は、
**storage が採番する単調値**でなければならず、いかなる wall clock の
組み合わせでも代用できない。逆に、その単調値が watch の event に載って
Go 側まで届けば、Stage 0 の `observed` 境界と突き合わせるだけで
storage→controller の遅延が初めて測れるようになる。**Stage 1 の受入測定は
この値の有無に依存する。**

**(b) controller 側の区間はすでに測れている。** `issued`(Go の transport)
と `observed`(Go の informer)は同一 isolate・同一時計なので引き算できる。
本番実測でスケジューラの bind が KCM の informer に届くまで **82〜93ms**
(S49)。Stage 1 の「境界起因 full snapshot 0 / 欠落 0」を測るとき、
controller 側の遅延はこの既存の物差しで切り分けられる。新しい計測機構を
Stage 1 で作る必要はない。

**(c) 観測は resync と区別して数えること。** informer は同じ revision を
約 25 秒おきに再配送する。revision ごとに**最初の観測だけ**を採らないと、
25,858ms や 50,174ms といった偽の「遅延」が出る(S44)。Stage 1 の
「1000 回以上の通常境界」を数えるときも同じ注意が要る。

**(d) 本番の canary は暖機してから測ること。** デプロイ直後の制御プレーンは
約 44MB の WASM をコンパイルし終わるまで何も reconcile せず、それが 8 分の
予算を超えて prodprobe を落とした(S47)。`/readyz` は WASM アセットの存在
しか見ないので**この状態を検出できない**。Stage 1 の canary 手順には明示の
暖機段階を入れる。

**(e) dynamic worker の中を本番で読むには中継が要る。** Loader が起動した
worker の console 出力は、それを読み込んだスクリプトの `wrangler tail` には
現れない(S45)。`WorkerLoaderWorkerCode.tails` に `env.SELF` を渡すと届く
(S46 で本番検証済み)。Stage 1 の受入判定が controller 側の状態に依存する
なら、`PUMP_TRACE=1` を付けた deploy でしか観測できない。

**(f) 計装は常時コストを持ちうる。** `issued` 境界の有効判定を毎回
`globalThis.context.env` から読んでいたため、tracing が無効な本番でも
送信リクエストごとに `syscall/js` の境界を 3 回跨いでいた(S49 追記)。
Stage 1 で cursor や世代を扱う計測を足すときは、同じ罠を踏まないこと。

### 5.2 実 upstream へのアダプター

まず正常境界を decoded event 間で閉じることでupstreamの再watchを使えるか計測する。
使えなければ共通 Go watch.Interface アダプターを `pkg/controllers/restconfig` とは独立した
小パッケージに置く案をレビューする。KCM lean watch、GC metadata watch、schedulerのtransportで
同じcursor契約を使う。TSはI/O glueだけ、Kubernetesのselector等は既存Go資産を使う。

成功済みeventだけcursorを進める。JSONの途中切断はそのeventを捨てて最後の完全eventから再取得。
initial-events-end前の切断は未完成snapshotを捨てる。重複eventをUID/RVで処理しても
delete→同名create、selector退出、ownerRef変更を失わないことを確認する。
窓終了はsuspensionであり、GC itemのAPI失敗にはしない。ただし認証失敗/409/410/本当のtimeoutは
隠さず適切な層へ返す。期限やcontext cancelが届けば論理watchも閉じる。
通常POST、JSON Patch等をverbだけで無条件再送しない。未知のcommit結果はUID/RVで照合し、
controllerがAPI状態から再試行する。全要求を「成功だったはず」と扱う経路は作らない。

### 5.3 起床・同期・処理のackを分離する

storageはcommitからdirty世代をdurableに記録。Controllersは対象componentを起こし、
instance epochを持つ受付ackを返す。informerへの適用ack、reconcileの完了/次deadlineは別。
新epochは古いRAMを持たないので、古いackで同期済みを名乗れない。失われた通知はdirty世代から
alarm再送し、新世代が到着したとき旧ackがdirtyを消せないcompare-and-clearにする。
component集合はGoのresource/informer宣言・生成表から作り、TSの手書きprefix一覧を増やさない。
GCは全対象resourceのownerRef/finalizer変更も起床理由に含める。

再開時には必要streamをcut Hまでcatch-upしてから新しいreconcileを許可する。
**wire受信完了、Reflector store更新、informer event handler完了、GC graph反映は別**。
既存WaitForCacheSyncだけを再開barrierに転用しない。schedulerのcycle開始、GC graph queue処理、
KCM workqueue開始に必要なupstream hookの所在とサイズをspikeで示すことが採用条件。
現在のgoroutineをI/O gateで止めるだけでは、止まる前に古いcacheで作った判断を再開時に送れてしまう。
既に進行中のcycleは再評価/無効化できなければbarrier達成扱いにしない。

全クラスタを永遠に同じcutで停止させず、各work batch開始時にHを固定して追いつく。
H以後の変更は次batchのdirtyとして残す。これでも一般のTOCTOUは残る。とくに
**bind commit時点で既にNoScheduleなNodeへの非許容Podのbindを絶対に0にする保証**は、
watch鮮度だけでは作れない。まずtaint commitより後に始めたcycleのstale bindを0にする。
厳密な受入条件にはNode RV変化をbind確定と同じ権威で条件検証する仕組みが必要で、
Go側でupstream TaintTolerationを再利用できるか、Node更新とPod bindingの跨facet整合性まで
別レビューする。単なるbind前GETでは隙間を閉じられない。これが未実装ならP0-4の「no bind」は未達とする。

### 5.4 時刻の仕事と停止条件

pokeのpayloadだけではLease更新停止、300秒toleration、CronJob、Job retry、Deployment deadlineを
再現できない。時刻の仕事はupstreamが持つ次期限をdurableに登録し、DOは最早期限だけをsetAlarm。
renewal/削除で期限の世代を更新・取り消し、古い発火はno-op。Nodeが存在するだけの60秒loopは
ここへ置換する。Go runtimeの全timerをDO alarmへ写す案は、upstreamの周期resyncをそのまま
永久課金にするため採用しない。

とくにnodelifecycleのhealth timestamp、taint eviction開始時刻は全てがAPIに永続化されているとは
仮定しない。どの値をAPIから再構築でき、どの最小deadline情報を保存するかをcontrollerごとに
証明する。CronJobは既存cronschedule.goの契約を継承し、catch-up完了までoverdueを残す。
失効NodeがUnknownになりeviction等も終わった後は、新Lease/Node書込まで停止できることを目標とする。

park条件は「dirtyなし AND 処理中batchなし AND 期限到来仕事なし」。未来期限があればその時刻に
one-shotだけを残す。unschedulable Podは資源変更か本当の次retry期限に紐付ける。
未知のqueueは空と決めつけず、移行中は既存hasUnconvergedWorkを安全網として残す。
安全網を消せない状態を「完全にidle」と呼ばない。永続エラーのretryは有限burstとbackoff、
運用アラートを伴い、仕事を捨ててparkして成功扱いにはしない。

### 削除できるものと順番

| 削除候補 | 撤去を許す証拠 |
|---|---|
| `FinishUnblockedForegroundOwners` と専用helper/callsite | 実GCが最後のblocker消滅を受けて自力でfinalizerを外し、55 Pod multi-owner/grace条件で90秒以内。まずこれだけを無効化したcanaryで検証。 |
| `refuseForegroundFinalize` / `blockingDependent` の専用部分 | GC graph反映済みbarrier下でforeground ownerが最後のblocking dependentより先に消えない。上の補助なしでも予算内。 |
| `sweepOrphanStragglers` / `finalizeDeleteWithOrphanSweep` の専用部分 | orphan直後・切断/再起動/二重owner/新規dependentの競合でも対象Pod数とownerRefが正しい。foregroundだけの合格では消さない。 |
| `RejectCreateWithTerminatingController` | KCMがowner削除を反映してからdependent減少を処理し、orphan/foreground時の補充がなく、同名owner再作成も正しい。 |
| TS prefix表、全resource LIST型park probe、3分warmup、Node存在型60秒loop | 全enabled informerと遅延仕事にdurable wake/ack/deadlineの対応が揃い、cold/eviction後も仕事が落ちない。対応した範囲から削る。 |

通常のdeletionTimestamp/finalizer stamping、最後のfinalizer除去による実DELETE、UID/RV precondition、
DefaultTolerationSeconds、no-op更新抑止は削除対象ではない。四guard全撤去は**到達目標**であり、
Cだけを入れた時点の約束ではない。削れないguardは残る理由と測定を記録し、P0-4を完了扱いにしない。

## 6. 独立して出荷・検証できる段階

各段階のコードは本設計レビュー後の別変更。本書は実装しない。全段階でmanifest hashと
compatibility dateを記録し、既存required focusを維持、WASM cap/CIを通す。
追加挙動はcomponent/cluster単位で切替可能とし、失敗時はその段階を戻す。前段の成果は保持できる。

| 段階 | 単独で出荷する成果 | 合否を決める測定 |
|---|---|---|
| 0: 観測と費用契約 **(完了 2026-09-12、S44–S49)** | cost-modelへの見積もり追記、動作を変えないevent/epoch/cursor/error計測、外部prod probe。 | 本番のNode停止90秒/10分と復帰、55 Pod GC、完全idleを現行hashで基準測定。commit→informer→actionのどこに遅延があるか区別できる。probe用trafficと対象clusterの自然trafficを分離できる。 |
| 1: storage watch契約 | snapshot/delta切替・commit cut・欠落検出を修正。従来controllerも利益を得る。 | 本番canaryでsnapshot中にwrite、push失敗、root/facet途中失敗、delete/recreate、selector変化を注入。受信履歴と権威logの差分0、phantom deletion0、無効cursorは410。未完facetは成功bookmarkにならない。 |
| 2: 論理watch再開 | Cをまず1component、次に全componentへ。guard/既存pokeは維持。 | 本番で通常境界1000回以上、idleからwarm/cold再開。warm・履歴有効・初期同期済み条件で境界起因full snapshot0、欠落0。各GVR rows/readとreconnect費用を旧版比較。close欠落・JSON途中切断でもrecovery。孤立した遅延/410は正しく再初期化。 |
| 3: durable変更通知 | Eのoutbox/世代ackと対象component dispatch。旧alarmをfallbackとして段階運用。 | 全served GVRの変更を、それ以外のheartbeatなしで起床させる。本番で通知/応答/ackを落としても再送で到達、重複副作用0、旧ackで新dirty消失0。batch毎metadata論理更新予算1+2C、実課金rowsも記録。 |
| 4: 再開barrierと期限 | upstream hookのspikeをレビューし、componentごとにcatch-up/処理ack、deadlineを導入。対応済みprobeだけ撤去。 | 本番でReady=True commit→両unreachable taint消滅の目標p95≤15秒、全sample≤30秒（healthy peerあり、warm/coldを別記、各30回以上）。taint前後のbind履歴を照合し非許容bind0。strict bind検証がなければ未達。CronJob cold発火、300秒eviction、Job retry、Deployment deadlineを外部pokeなしで検証。 |
| 5: guard削減 | 上表の順でguardを1つずつ撤去。 | 本番のisolated test clusterでorphan/foreground/background/multi-owner/UID再作成を各30回以上、GC owner消滅max<90秒、p95≤60秒、早期owner消滅0、orphan損失/補充0。さらにdw variant約10連続greenを要求しrequiredへ昇格する計画（P1-1/2）。失敗は原因確定またはrevert。 |
| 6: parkの簡素化 | 全対応を確認してhasUnconvergedWork全LIST/warmup/Node周期を撤去。 | 本番で最後の仕事から5分以内にpark、続く30分target invocations/alarms/rows/CPU/duration増分0。24時間の課金metricsで再確認。収束不能workloadは別caseで無意味なwrite増分0、有限の理由付きretryだけ。 |

Stage 4で30秒復帰を満たさない場合、窓を大きくして隠さず各区間の測定から修正する。
30回/1000境界/10連続greenは受入sample数であり数学的保証ではない。P0-4完了は
本番測定とguard削減、conformanceをセットで判定する。S36の1/5を「許容flaky」として残さない。

## 7. 本番でのリスク検出

計測は採用時に追加する提案。外部のsynthetic runnerが専用clusterで実行し、失敗通知は既存運用経路へ
接続する。idle検証中は対象APIをpollしない。別の管理経路からlogs/billing metricsを読み、
区間後に状態を取得する。synthetic自身の起床費用はprobe/回として別計上し、毎clusterの
self-pollingを導入しない。`/healthz` runningだけを成功判定に使わない。

| リスク | 本番で採る信号と対処 |
|---|---|
| IoContext早期消滅・close欠落 | dispatch/window id、open/JS-close/expiry、header timeout、未完waiter数、次poke後の最初の成功I/O。close logなしだけで原因確定しない。prod canaryの限定fault seamで旧挙動を再現し、進捗停止を検出して当該stageを戻す。 |
| Node復帰の未知の数分遅延 | Node/Lease commit RVと時刻、KCM store適用、health observation、taint PATCH/commitを相関。Readyだけ即時でtaintが30秒残れば失敗。healthy peer付き90秒/10分停止、Loader age・cold/warmを分ける。 |
| scheduler stale snapshot / TOCTOU | cycle id、参照Node UID/RV、taint commit、filter結果、binding commitを収集。非許容bindが1件でも出れば警報。cycle前のstalenessと選定後taint変更を分け、後者をwatch改善の成功に混ぜない。 |
| replay順序・欠落・facet未commit | 権威logに対するconsumer到達cutと欠番、initial bookmark前の件数、410、value-less event、replay/live重複数。fault中に差分があればconsumerを未同期に戻す。変更のないprefixも「何も変わらずHまで到達」の証明が必要。 |
| cursorだけ残ってcache/graph消失 | instance epoch、snapshot完了、handler/graph ack。実際のprod cold startやtoken rotationを区別し、古いackによる書込を拒否。任意isolate evictionの強制APIは未確認なので、別Loader epoch試験と自然eviction両方を記録する。 |
| GCのasync graph lag・残るitem backoff | owner/dependent commit、graph反映、queue retries/next eligible、finalizer除去時刻、guard発動数、全LIST rows。90秒超・早期削除・orphan数不一致は即失敗。S36の55 Pod/grace20秒条件を落とさない。 |
| APIのcommit済み・応答欠落 | request operation idとUID/RV、server commitとclient outcomeを対応付ける。POST二重生成、非冪等PATCH重複適用、UID違いDELETEは0を要求。watch再開ではapiserver無応答の根因は治らないため独立に追う。 |
| lost wake / 誤park / timer再始動 | committed dirty世代とack世代、deadline世代、alarm arm理由、処理中batchのageを出す。期限を過ぎて仕事が残れば通知。Node残存のみで一定周期alarmが続けば#3違反。 |
| watch leak・メモリ・DO常駐費 | WatchHub connection増減、consumer数、DW heap、128MiB failure、DO activeTime/GB-s、CPU-ms、rows read/writtenをnamespace別に計測。無イベントの長期watchでdurationが増えたらhibernation不成立として戻す。 |
| 無制限replay/catch-up・飢餓 | backlog bytes/rows、snapshot頻度、1batch所要、cut lag。上限超は明示resnapshot、slow consumerが全clusterを止めない。target cut固定でcatch-up可能か負荷試験する。 |
| サイズとupstream drift | 全WASM manifestの64MiB gateと128MiB本番heapを確認。共通hookをpin付きでレビューし、version bump後同じprod canaryを要求。大量controller forkになるならstage4の設計を再検討する。 |

`wrangler dev` は整列・重複・cancelなどの回帰を早く見つける場所として使うが、
`window.go:23–46` のrequest寿命・cross-request I/O・close欠落やproduction evictionを保証しない。
S27では再起動しない実験が誤った成功を返し、S31追記3では窓ありでも6分を再現できなかった。
ローカルgreenを本番の速度・課金・正しさの証明にしない。

## レビューの決定点

この文書で求めるのはC→Eの段階設計のレビューである。長窓や旧readerの延命は根因を残し、
controllerごとの再実装は守りたいupstream再利用を壊す。一方、CはAPI互換性とrows-read改善だけでも
独立の価値があり、Eは停止しても仕事が失われない土台になる。

レビュー時に最初に詰めるべき点は、committed cutの跨facet実現と、upstreamを大幅改造せずに
handler/GC graph/scheduler cycleまでの処理barrierを置けるかである。そこが未証明なまま
「watchが再開するのでguardを全撤去できる」とは承認しない。
