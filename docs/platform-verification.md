# プラットフォーム検証スパイク(S1–S8)

k8flare v2 リライト(`feat/v2-rearchitecture`)は Dynamic Workers・DO
Facets・Cloudflare Containers・R2・(検証次第)Cloudflare Mesh といった
2026年の Cloudflare 新機能を前提に設計している。このドキュメントは、それら
の機能について公式ドキュメントを読んだだけで終わらせず、**実際に動かして
確認した**結果を記録する場所である。

CLAUDE.md の不可侵ルール#2「実際に動かして検証する。ソースを読んだだけの
結論を事実として書かない」の実践場所がここになる。過去に「kube-proxy が
Worker をハングさせる」という結論が誤りだった(実際の原因は古い DO state)
前例があり、ドキュメントの記述や推測だけで設計判断を確定させることのコスト
は実証済みである。

## 読み方

- 各スパイクは v2 リライトプランの Phase 1 で「使い捨てコードで可・結果だけ
  記録」と定義されている。検証コード自体はリポジトリに残さなくてよい。
- **ステータス**は次のいずれか: `未着手` / `一部確認済み` / `検証済み`。
- **確定した事実**には必ず出典(コミットハッシュ・公式ドキュメント名・
  changelog 日付)を付ける。出典の URL が本ドキュメントに未記録の場合は
  推測で埋めず、その旨を明記する。
- **honest correction 規約**: 一度書いた「確定した事実」や判断が後で誤りと
  判明した場合、該当箇所を書き換えて消すのではなく、末尾の「訂正履歴」に
  日付と経緯を追記する(CLAUDE.md 不可侵ルール#4)。

## スパイク一覧

| # | 検証対象 | ステータス | 主に前提とするフェーズ |
|---|---|---|---|
| S1 | Facets(上限・ストレージ計上・並列性・alarm・delete） | 一部確認済み | Phase 4(storage v2) |
| S2 | Dynamic Workers Loader(WASM同梱・サイズ上限・envバインディング) | 未着手 | Phase 2 / Phase 4 |
| S3 | Containers(起動・onActivityExpired・コールドスタート・任意イメージ・UDP・wrangler dev) | 未着手 | Phase 5 経路B / Phase 7 |
| S4 | Cloudflare Mesh(課金範囲・flannelプロトタイプ・Cluster DNS代替) | 未着手 | Phase 9 |
| S5 | WASM isolate シングルトン化(syumai fork) | 未着手 | Phase 2(apiserver) |
| S6 | R2(PVCアクセス分離・ContainersからのS3アクセス) | 未着手 | Phase 8 |
| S7 | apiserver 常駐化の再検証(却下判断の裏取り) | 未着手 | 却下判断の最終確認 |
| S8 | controllers の WASM 常駐実行可否 | 未着手(検証中) | ★最優先。Phase 5 の実行方式を左右 |

---

## S1: Facets

**検証項目**
- facet 数の実用上限
- ストレージ計上(親子で 10GB 共有か、facet 毎に独立 10GB か)
- facet 実行並列性
- facet 内での alarm 可否
- `delete()` のセマンティクス(namespace 削除への転用可否)
- (検証方法)既存の KOOFFICE 実機検証デプロイを再利用する

**ステータス**: 一部確認済み

**確定した事実**

出典: コミット `46df0c0`("Add empirical Facets verification against a real
KOOFFICE deployment"、2026-07-02)。使い捨ての supervisor-DO Worker を
KOOFFICE 実アカウントにデプロイし、ドキュメント記載の facets パターン
(`cloudflare:workers` の `DurableObject` + `worker_loaders`)を実際に動かして
確認した(この検証結果は `docs/multi-tenancy-and-hosting.md` にも反映済み)。

1. facet の `class` は Dynamic Workers loader
   (`env.LOADER.get(...).getDurableObjectClass(...)`)経由でなければならない。
   素の静的 import した `DurableObject` サブクラスを
   `ctx.facets.get(name, () => ({ class: LocalClass }))` に渡すと実行時に
   `TypeError: Incorrect type for the 'class' field on 'StartupOptions': the
   provided value is not of type 'DurableObjectClass or
   LoopbackDurableObjectNamespace or LoopbackColoLocalActorNamespace'` で
   失敗する。公式ドキュメントのどこにも「必須」とは明記されていない
   (サンプルコードがすべて loader を使っているだけ)が、実機検証で必須要件と
   確認できた。→ 自前 CRD-as-facet 設計も、ユーザー提供コードと同じ
   `worker_loaders` / `LOADER` 機構を通す必要がある。
2. ストレージ分離は実データ量下でも成立する。2つの兄弟 facet と supervisor
   それぞれに独立したキーを書き込み、クロスリードが空を返すことを確認した。
   `abort()` は facet のデータを次の `get()` でも保持する。`delete()` は
   本当に破棄する(以後の `get()` は空の新規 facet を返す)。どちらの操作も
   兄弟 facet や supervisor には影響しない。→ namespace 削除に `delete()`
   を転用する設計(Phase 4)の裏付けが取れた。
3. `ctx.storage.sql.exec()` 内で `PRAGMA` 文は
   `Error: not authorized: SQLITE_AUTH` で拒否される。Cloudflare が意図的に
   この introspection 経路をブロックしている。アプリケーションコードから
   facet や DO の実際の SQLite サイズを直接問い合わせる方法は存在しない
   (wrangler CLI にも相当コマンドなし)。→「10GB にどれだけ近いか」を
   自己申告させるには、SQLite に聞くのではなく書き込み時に自前でバイト数を
   積算する仕組みが要る(未実装、設計課題として残る)。
4. 1 facet に ~1.01GB(1MB 行 × 1,010、`SELECT COUNT(*)` で確認)を書き込み、
   同時に supervisor へ 100MB、新規の兄弟 facet も作成 — いずれもエラーなく
   即座に成功し、1GB facet のデータから分離されたまま保たれた。→ 実データ量
   での共存・分離は確認できたが、**1GB は共有/独立 10GB を区別するには
   小さすぎる**(どちらのモデルでも 10GB 上限に対して無視できる量)。この
   問いに決着をつけるには片側を ~9GB+ まで埋めて他方が制約を受けるか確認
   する境界テストが要るが、未実施。

上記の実機検証を踏まえ、v2 リライトプランでは以下を確定事実として採用する:

- facet は facet 毎に独自 SQLite DB を持つ(実機確認済み、上記2)。
- facet クラスのロードは Worker Loader 経由必須(実機確認済み、上記1。
  公式ブログの記述とも一致)。
- **10GB が親子共有か独立かは公式未文書のまま**。本プランでは
  **非共有(facet 毎に独立 10GB)と仮定して設計する**(理由: 共有だった場合は
  自然に上限で頭打ちになるだけで設計は破綻しないが、非共有を前提にしないと
  本来使えるスケールを設計段階で捨てることになる。ダウンサイドが非対称なので
  楽観側を仮定し、境界テストで外れたら容量設計だけ修正する方針)。
- facet の実行並列性は未検証のまま。全トラフィックが親 DO の単一スレッド
  経由という制約はどちらの仮定でも変わらないため、設計はこれに依存しない。

DO / Facets の一般的なプラットフォーム数値(SQLite DO 10GB storage、
~1,000 req/s ソフト上限、単一スレッド、2MB max value/row、32,768
WebSocket/DO、30日 PITR。DO Facets は Dynamic Workers 上の Open Beta、
Agents Week 2026年4月)は `docs/multi-tenancy-and-hosting.md` の Verified
Cloudflare platform facts 表(引用元: [DO limits](https://developers.cloudflare.com/durable-objects/platform/limits/)、
[DO pricing](https://developers.cloudflare.com/durable-objects/platform/pricing/)、
[facets blog post](https://blog.cloudflare.com/durable-object-facets-dynamic-workers/)、
[facets docs](https://developers.cloudflare.com/dynamic-workers/usage/durable-object-facets/))
で確認済み。

**未解決の問い**

- facet 数の実用上限(未検証。今回は兄弟 facet 2 + supervisor の3者構成のみ)。
- 10GB の親子共有 vs 独立の境界テスト(片側を ~9GB+ まで埋める実験、未実施)。
- facet 内での alarm 可否(未検証)。
- facet 実行並列性(未検証。`docs/multi-tenancy-and-hosting.md` も
  「undocumented」として設計を依存させない方針を取っている)。
- **`docs/multi-tenancy-and-hosting.md` の既存記述との仮定の向きの相違**:
  同ドキュメントは 10GB を「保守的に共有と仮定」している(誤って独立と
  仮定した場合のダウンサイドの方が大きいという理由)。一方、本プラン
  (v2 リライト)は逆に「非共有と仮定」している(理由は上記)。どちらも
  「公式未文書」という同じ事実からの異なるリスク評価であり、実測で決着する
  までは2つの docs で矛盾したまま残る。境界テストが完了し次第、いずれかを
  honest correction として更新する必要がある。

---

## S2: Dynamic Workers Loader

**検証項目**
- 動的ロード Worker に WASM モジュールを含められるか
- ロードコードのサイズ上限
- `env` にバインディングを渡せるか

**ステータス**: 未着手

**確定した事実**

- Dynamic Workers は 2026年3月にオープンベータ化した。出典: v2 リライト
  プランの「調査で確定した重要事実」(一次情報 URL は本ドキュメント未記録 —
  参照時に追記)。
- facet クラスのロードに Dynamic Workers の loader が必須であることは S1 で
  実機確認済み(`46df0c0`)。ただしこれは「facet クラスをロードできる」こと
  の確認であり、S2 が問う「WASM モジュール同梱」「ロードコードのサイズ上限」
  「env バインディングの受け渡し」はいずれも未検証。

**未解決の問い**

- apiserver(Go WASM、gzip 7.07MiB)を Dynamic Workers 経由でロードする場合の
  サイズ上限(Worker 本体の Paid 上限 10MiB と同じ制約か、別枠かは未確認)。
- ロードコードに `env` バインディング(DO namespace 等)を渡せるか。
- S8 で controllers の WASM 常駐案が採用された場合、controllers も Loader
  経由にする必要があるか(Cluster DO からの service binding 呼び出しと
  Loader の関係は未整理)。

---

## S3: Containers

**検証項目**
- `cmd/scheduler` / `cmd/controller-manager` イメージの起動と apiserver への
  長時間 watch 維持
- `onActivityExpired` オーバーライド動作(`stop()` を呼ばなければ常駐するか)
- コールドスタート時間(本プロジェクトのイメージでの実測)
- 任意イメージの動的実行可否(Pod バックエンドの成立条件。不可なら
  「wrangler 定義済みイメージの allowlist」方式に確定)
- outbound UDP 可否
- `wrangler dev` での Containers + DO 開発体験

**ステータス**: 未着手

**確定した事実**

- Cloudflare Containers は GA(2026年4月)。インスタンス上限 4 vCPU / 12 GiB
  / 20 GB disk、コンテナは DO と 1:1 でペアリングされ scale-to-zero
  (timeout 後 sleep)、GA 時点でビルトインのオートスケーリングは未提供。
  出典: `docs/multi-tenancy-and-hosting.md` の Verified Cloudflare platform
  facts 表(引用元: [Containers limits](https://developers.cloudflare.com/containers/platform-details/limits/))。
- `onActivityExpired()` をオーバーライドし `stop()` / `destroy()` を
  呼ばなければコンテナは自動停止しない。出典: 公式 docs の Warning 注記
  (v2 リライトプラン記載。一次情報 URL は本ドキュメント未記録 — 参照時に
  追記)。→ ライフサイクルを自前管理する設計(demand-start/idle-stop、
  経路B)の根拠。
- Containers のコールドスタートは一般値として典型 1〜3秒、実運用で
  3〜15秒に及ぶ例が報告されている(アーキテクチャ記事・実測ブログで確認、
  v2 リライトプラン記載。Workers isolate のウォームアップ 5ms 未満とは
  桁違い)。**これは他プロジェクトの実測に基づく一般値であり、本プロジェクト
  の scheduler/KCM イメージでの実測ではない**(本プロジェクト固有の実測は
  S7 で行う)。

**未解決の問い**

- 任意 OCI イメージの動的実行可否(Pod バックエンドの成立条件そのもの。
  不可なら allowlist 方式に設計変更が要る)。
- outbound UDP 可否(k3s/flannel 系のネットワーキングに影響)。
- `wrangler dev` での Containers + DO のローカル開発体験。
- 本プロジェクトの scheduler/KCM イメージでの実測コールドスタート時間
  (→ S7 で実施予定)。
- `onActivityExpired` の呼び出し粒度(経路Bの idle-timeout チューニングに
  必要)。

---

## S4: Cloudflare Mesh

**検証項目**
- Workers Paid の範囲で使えるか(ライセンス/課金確認)
- 使い捨て2ノードの flannel-over-Mesh プロトタイプ
- Cluster DNS 代替になり得るか(`*.svc.cluster.local` を Mesh/Gateway の
  名前解決で返せるか。不可なら CoreDNS 案を維持)

**ステータス**: 未着手

**確定した事実**

(現時点でなし。`docs/cloudflare-mesh-networking.md` に Mesh 自体の概要調査は
あるが、本スパイクが問う課金範囲・プロトタイプ動作・DNS代替可否はいずれも
実機検証されていない。)

**未解決の問い**

- 検証項目すべて未着手。
- 不採用の場合のフォールバック(CoreDNS Deployment + `kube-dns` Service)は
  `docs/general-purpose-k8s-plan.md` の Phase 4 手順として既に設計されている
  (S4 が失敗した場合はそちらを正とする)。

---

## S5: WASM isolate シングルトン化

**検証項目**
- isolate シングルトン化の実証(syumai fork の doneCh ガード + mutable
  context holder)
- cross-request IoContext エラーの有無
- `WASM_INSTANCE_REUSE` フォールバックフラグ

**ステータス**: 未着手

**確定した事実**

- 現状(v1)は「リクエスト毎に Go ランタイムを再インスタンス化」する設計
  (モジュールコンパイルのみキャッシュ)。全 API 呼び出しに Go 起動税が
  掛かり、並行リクエストで 128MB isolate の OOM リスクがある。出典: v2
  リライトプランの現状分析(「調査で確定した重要事実」)。これが S5 で
  シングルトン化を検証する動機。

**未解決の問い**

- 検証項目すべて未着手。S5 の成否は apiserver(Phase 2)のレイテンシ・安定性
  に直結し、S8(controllers の WASM 常駐)の fork 作業とも地続き
  (同じ fork 作業で両方解決する見込み、詳細は S8 参照)。

---

## S6: R2

**検証項目**
- PVC 単位のアクセス分離(バケット/プレフィックス + スコープ付きトークン)
- Containers からの S3 API アクセス

**ステータス**: 未着手

**確定した事実**

(なし)

**未解決の問い**

- 検証項目すべて未着手。Phase 8(R2 PV/PVC バックエンド)の前提。
- Pod へのボリューム提供が FUSE マウントか S3 互換エンドポイント + 認証情報
  注入になるかは、本スパイクと S3(Containers の制約)両方の結果次第。

---

## S7: apiserver 常駐化の再検証(却下案の裏取り)

**検証項目**
- 本プロジェクトの scheduler/KCM イメージサイズでの実際のコールドスタート
  時間を計測
- 上記一般値(典型1〜3秒・最悪15秒、S3参照)との比較
- apiserver を WASM のまま据え置く判断の最終確認(数値が想定と大きく異なれば
  再協議)

**ステータス**: 未着手

**確定した事実**

v2 リライトプランの「却下した案: apiserver も Containers 化」節に基づく、
S7 実施前の暫定的な却下判断とその根拠(S7 の実測で数値が大きく異なれば
再協議対象):

- レイテンシ比較: Workers isolate のウォームアップは 5ms 未満。Cloudflare
  Containers のコールドスタートは典型 1〜3秒、実運用で 3〜15秒(S3 と同じ
  一般値、アーキテクチャ記事・実測ブログで確認)。桁が3〜4桁違う。
- apiserver はホットパス(kubectl の全コマンドが通る)。scheduler/KCM は
  ユーザーが結果を直接待たない非同期リコンサイラ。同じ Containers でも
  「常時ウォームに保つ必要があるか」がここで分岐する。
- 概算コスト: クラスタ1個・1vCPU+1GiB を24時間常時ウォームに保った場合、
  vCPU代 $0.00002/vCPU秒 × 2,592,000秒/月 ≈ $52 + メモリ代
  $0.0000025/GiB秒 × 2,592,000秒/月 ≈ $6.5 → **月 ~$58/クラスタ**
  (プランの試算)。ホスト型で多数のアイドルクラスタを抱える前提
  (k8flare.com 構想)ではこれが致命的に効く。ウォームに保たない場合は初回
  `kubectl get` が数秒〜15秒待たされ、対話的 CLI として破綻する。
- 「バージョン追随が楽になる」という Containers 化の利点は Runtime
  非依存: `cmd/k8flare-gen` はテーブル + go.mod pin から生成する設計であり、
  生成先が WASM でも Container 用ネイティブバイナリでも同じ恩恵を受ける。
  WASM 固有の負担はサイズ規律(internal 型を避ける・OpenAPI を外出しする等、
  Phase 3)であって、バージョン追随そのものではない。
- **暫定結論(却下)**: apiserver は WASM/Worker のまま(低レイテンシ最優先)。
  S7 はこの結論の最終確認(本プロジェクト固有の実測)であり、まだ実施
  されていない。

**未解決の問い**

- 本プロジェクトの scheduler/KCM イメージでの実測コールドスタート時間
  (未計測)。
- 実測が一般値から大きく乖離した場合の再協議要否。

---

## S8: controllers の WASM 常駐実行可否(★最優先)

Phase 5(controllers 実装)の実行方式(経路A: WASM常駐 / 経路B: Containers
フォールバック)を左右するため、他のスパイクより先に着手する。

**検証項目**

(a) syumai fork して「レスポンスストリームを閉じずに Go プログラムを
    生かし続ける」ことができるか
(b) client-go の informer 相当(複数 goroutine が並行して長時間 watch を
    張り続ける)を模した負荷を数時間かけて、実消費 CPU-ms が想定通り小さく
    収まるか(`wrangler tail` 等で CPU 時間を実測)
(c) 10〜15本規模の同時長時間ストリームが 2026-04 の緩和後の接続制限内で
    安定するか
(d) service binding 経由の内部呼び出しでも同じ「壁時計無制限」が成り立つか

**ステータス**: 未着手(検証中)

**分岐条件**: (a)〜(d) の4点すべてが確認できれば `workers/controllers` は
Containers ではなく Go WASM 常駐として設計する(経路A)。いずれかが致命的に
破綻した場合は Containers + demand-start/idle-stop(経路B)にフォールバック
し、常時ビジーな稼働パターンには BYO VM を代替デプロイ先として案内する
(cmd/scheduler・cmd/controller-manager は元々どこでも動く無改変バイナリ
なので実装追加なしに提供できる)。ダメだった項目は「具体的にどう壊れたか」
をこのドキュメントに追記し、経路B採用の根拠とする。

**確定した事実**

- **Workers/DO の課金は実 CPU 消費時間のみ(壁時計ではない)。I/O 待ちは
  無課金**、HTTP ストリーミング応答に壁時計上限はなく、CPU 時間上限
  (5分/呼び出し)のみが効く。出典: v2 リライトプランの「調査で確定した
  重要事実」(一次情報: Cloudflare Workers pricing ドキュメント、URL は本
  ドキュメント未記録 — 参照時に追記)。→ informer(watch を張りっぱなしで
  大半の時間 I/O 待ち)のような負荷パターンは Workers 上でほぼ無課金に
  なり得る、という S8 の仮説の根拠。
- **2026-04-09 に同時接続制限が緩和**され、「ヘッダー待ち」の瞬間だけ6本
  制限が残り、確立済みの長時間ストリームは無制限になった。出典: Cloudflare
  changelog、2026-04-09付(プランで確認済みと記載。本ドキュメントには具体的
  なエントリ URL は未記録 — 参照時に追記)。→ (c) の「10〜15本規模の同時
  長時間ストリームが安定するか」に直接関係する既知事実(ヘッダー待ちの
  瞬間的な6本制限には抵触しないはずだが、確立後の安定性そのものは未検証)。
- 現在の syumai/workers は「1リクエスト=1実行、レスポンスが閉じたら WASM
  終了」という設計。「閉じないレスポンスストリーム」として Go プログラムを
  生かし続けるには fork が要る(S5 の isolate 再利用 fork と地続きで、
  おそらく同じ fork 作業で両方解決する見込み、プラン記載)。

**未解決の問い**

- (a)〜(d) すべて未検証。
- kube-scheduler/KCM 内部は informer 毎の goroutine + workqueue worker +
  定期 resync タイマーが並行動作する設計。`GOOS=js/wasm` ターゲットで
  この規模の並行 I/O 待ちが数時間〜数日安定するかは未実証。
- client-go の transport(syumai の fetch ベース RoundTripper 経由)が
  長時間の watch 接続の再接続・backoff・resourceVersion 継続を正しく
  扱えるかは未実証。

---

## 訂正履歴(Honest Corrections)

まだ訂正はない。判断や「確定した事実」が後で誤りと判明した場合は、該当
セクションを書き換えず、ここに日付と経緯を追記する(CLAUDE.md 不可侵ルール
#4)。
