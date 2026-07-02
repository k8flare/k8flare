# コストモデル

k8flare のコンセプトは「アイドル時コストをストレージ代のみに近づける
サーバーレスコントロールプレーン」である(CLAUDE.md)。コスト不変条件#5
「新機能・新コンポーネントは実装前にコストを見積もる」に従い、コンポーネント
毎のアイドル時/アクティブ時の単価をこのドキュメントに記録する。

- **見積もり**と**実測**は別カラムで管理する。未実測の項目は `TBD` と書き、
  数値を創作しない。
- 見積もりが実測で更新される場合も、見積もり自体は消さずに残す
  (honest correction 規約、CLAUDE.md 不可侵ルール#4)。
- 出典(コミットハッシュ・公式ドキュメント名・changelog 日付)を明記する。

## 課金プリミティブ

| プリミティブ | 課金軸 | 判明している数値 | 出典 |
|---|---|---|---|
| Workers | 実 CPU 消費時間のみ。I/O 待ち無課金。HTTP ストリーミング応答に壁時計上限なし(CPU時間上限 5分/呼び出し) | 単価そのものは本ドキュメント未記載(TBD) | v2 リライトプランの調査結果(一次情報: Cloudflare Workers pricing docs、URL 未記録 — 参照時に追記) |
| Durable Objects | requests / duration(GB-s)/ rows read-written(SQLite)/ alarm 起動回数 | 10GB storage/DO、~1,000 req/s ソフト上限(複雑操作は200-500)、単一スレッド、2MB max value/row、32,768 WebSocket/DO、30日 PITR。$レートは本ドキュメント未記載(TBD) | [DO limits](https://developers.cloudflare.com/durable-objects/platform/limits/)、[DO pricing](https://developers.cloudflare.com/durable-objects/platform/pricing/)(docs/multi-tenancy-and-hosting.md 引用) |
| Cloudflare Containers | 壁時計(稼働時間)ベースの vCPU秒・GiB秒課金 | vCPU代 $0.00002/vCPU秒、メモリ代 $0.0000025/GiB秒。Paid込み枠: vCPU 375分/月・メモリ 25GiB時/月 無料。インスタンス上限 4vCPU/12GiB/20GB disk、DOと1:1ペア、scale-to-zero | v2 リライトプランの試算(Phase 5 経路B)、[Containers limits](https://developers.cloudflare.com/containers/platform-details/limits/)(docs/multi-tenancy-and-hosting.md 引用) |
| Worker Loader(Dynamic Workers) | ユニークロード数課金 | $0.002/unique/day | CLAUDE.md コスト不変条件#5、v2 リライトプラン |
| R2 | ストレージ + オペレーション(想定、詳細未確認) | TBD(未使用。Phase 8 実装前に一次情報を確認して追記) | — |
| Workers KV | 読み書き回数 + ストレージ(想定、詳細未確認) | TBD(未使用。Phase 2 で `CLUSTER_ROUTES` として使用予定、実装前に追記) | — |

## コンポーネント別見積もり

| コンポーネント | アイドル時月額(目標: ~0) | アクティブ時単価 | 見積もり | 実測 |
|---|---|---|---|---|
| gateway(TS Worker) | ~0(ステートレス、DOを持たない) | Workers リクエスト課金(実CPU時間) | TBD | 未実施(未実装、Phase 2) |
| apiserver(Go WASM) | ~0 | Workers リクエスト課金(実CPU時間)。S5(isolateシングルトン化)の成否でWASM起動税の有無が変わる(`docs/platform-verification.md` S5) | TBD(S5待ち) | 未実施(未実装、Phase 2) |
| storage: Cluster DO | ストレージ代のみが目標(alarmパーク時) | DO requests / duration GB-s / rows read-written + alarm起動回数(event-armedのみ) | TBD | 未実施(未実装、Phase 4) |
| storage: WatchHub DO | ~0(hibernation中) | DO requests / duration GB-s(hibernation中のWSは非アクティブ時間課金対象外の想定 — 要実測) | TBD | 未実施(未実装、Phase 4) |
| runtime(TS Worker + LOADER) | ~0(cron未発火時) | Workers リクエスト課金 + Worker Loader $0.002/unique/day | TBD | 未実施(未実装、Phase 2) |
| controllers | 下記「controllers の実行方式: 2経路試算」参照 | 同上 | 同上 | 未実施(`docs/platform-verification.md` S8待ち) |
| nodes(Pod-on-Containers) | 目標 ~0。ただし仮想ノードのLease更新用に軽量alarmが必要見込み(Pod有無に関わらず) | Containers vCPU/GiB秒課金。**Pod自体の稼働コストはユーザーワークロードのコスト**(コスト不変条件#6、制御プレーンコストには含めない) | TBD | 未実施(未実装、Phase 7) |
| R2 PV/PVC | ストレージ代のみが目標 | R2オペレーション課金 | TBD | 未実施(未実装、Phase 8) |

## controllers の実行方式: 2経路試算

scheduler/KCM は「実物コントローラーを使う」ために動かすのであって、
どちらの実行方式でも壁時計課金の常駐はコンセプト違反(CLAUDE.md)。
`docs/platform-verification.md` の S8 の結果でどちらかを採用する。

### 経路A: WASM常駐(S8成功時・優先)

`workers/controllers`(Go WASM)を syumai fork 上で「閉じないレスポンス
ストリーム」として起動し続け、Cluster DO からの内部呼び出しで生かし続ける。
informer は大半の時間 I/O 待ちのため実 CPU 消費は小さく、**課金は実際の
reconcile 量に比例する(壁時計に比例しない)**という設計仮説。起動/停止と
いう概念自体が不要になり、demand-start/idle-stop のオーケストレーションや
idle-timeout チューニング、サッシング対策が要らない分、経路Bより実装が
シンプルになる見込み。

**見積もり**: informer が I/O 待ちの時間は無課金なので、低頻度〜中頻度は
当然経路B(Containers)より安く、常時ビジーでも「実際に reconcile 演算を
した分」しか課金されないため VPS との比較で不利になりにくい、という設計
仮説(未実測)。

**実測**: 未実施。`docs/platform-verification.md` S8 (b) で CPU-ms/時間を
実測し、想定と乖離があれば経路Bに切り替える。

### 経路B: Containers(demand-start/idle-stop、S8失敗時のフォールバック)

1vCPU+1GiB、Containers Paid 込み枠(vCPU 375分/月・メモリ 25GiB時/月 無料)
前提の試算(v2 リライトプランの見積もり、実測ではない):

| 稼働パターン | 想定 | 月額目安 |
|---|---|---|
| 低頻度(個人開発、日20回起動×30秒) | 10分/日 = 300分/月 | 無料枠内で **$0** |
| 中頻度(小規模チーム、日50回起動×60秒) | 50分/日 = 1,500分/月 | 無料枠超過分課金で **~$1.35/月** |
| 常時ビジー(CronJob分単位・HPA頻繁調整でidleが一度も成立しない) | 実質24時間稼働 | vCPU+メモリで **~$58/月、同スペックVPS(月$5〜)より高い** |

「常時ビジー」ケースでは VPS で直接 k3s を動かす方が安いことを README で
正直に明記し、この稼働パターンには BYO VM(cmd/scheduler・
cmd/controller-manager を既存の無改変バイナリのままホスト)を推奨デプロイ
先として案内する(実装追加ゼロで提供できる安全網)。中頻度域では
「サッシング」(idle-timeout が短すぎて起動・停止を頻発し、そのたびに
informer の relist が走ってコストと復帰レイテンシを両方悪化させる)に
注意。idle-timeout は固定値で決め打ちせず、`docs/platform-verification.md`
S3/S7 の実測(relist の実コスト・所要時間)に基づいて初期値を決め、決めた
理由をここに追記する。

**実測**: 未実施(経路Bが採用された場合のみ実装・計測)。

### 却下済み判断: apiserver の Containers 化

controllers を Containers で動かすなら apiserver も Containers 化すれば
バージョン追随が楽になるのでは、という提案を検討し、以下の試算で却下した
(詳細な検証項目・再検証計画は `docs/platform-verification.md` S7 を参照):

- レイテンシ: Workers isolate のウォームアップ 5ms 未満 vs Cloudflare
  Containers のコールドスタート典型1〜3秒・実運用3〜15秒。
- コスト: クラスタ1個・1vCPU+1GiBを24時間常時ウォームに保った場合、
  vCPU代 $0.00002/vCPU秒 × 2,592,000秒/月 ≈ $52 + メモリ代
  $0.0000025/GiB秒 × 2,592,000秒/月 ≈ $6.5 → **月 ~$58/クラスタ**。
  ホスト型で多数のアイドルクラスタを抱える前提では致命的。
- apiserver はホットパス(kubectl の全コマンドが通る)なので、この
  コールドスタート遅延はユーザー体験に直結する。scheduler/KCM(非同期
  リコンサイラ)とはこの点で性質が異なる。

**結論**: apiserver は WASM/Worker のまま(低レイテンシ最優先)。S7 で
本プロジェクト固有の実測が出るまでは暫定判断。

## アイドルクラスタ検証チェックリスト

コスト不変条件#1「アイドル時に動いてよいものはゼロ」を機械的に確認する
チェックリスト。Phase 6 で CI のコストゲート化を予定している(v2 リライト
プランの「コストゲートを新設」参照)。

- [ ] Container インスタンス数が 0(経路B採用時。経路Aの場合は controllers
      Worker の実消費CPU時間が実質ゼロであることを `wrangler tail` 等で確認)
- [ ] 次回の DO alarm が未予定(パーク状態。イベント待ちの再武装のみで、
      固定間隔ポーリングが残っていないこと)
- [ ] WebSocket 接続が hibernation 状態(WatchHub DO。接続保持中の
      `ctx.waitUntil` keep-alive がアクティブでないこと)
- [ ] DO 課金対象操作(alarm発火・facetアクセス等)が一定時間発生しない

各項目は本ドキュメント作成時点(Phase 1)で未実装・未検証。Phase 4
(storage v2)完了時に alarm パーク・hibernation を、Phase 5/6 完了時に
controllers のアイドル挙動を実測してこのチェックリストを満たすことを
確認する。
