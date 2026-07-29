# k8flare — CLAUDE.md

## コンセプト(最優先・全判断の前提)

**k8flare はサーバーレスコントロールプレーンである。** アイドル時のコストは
ストレージ代のみに近づけ、書き込みでミリ秒単位にウェイクする——これが他の
マネージド Kubernetes には無い差別化要因(docs 曰く "no other hosted
Kubernetes offers scale-to-zero control planes")。**この特性を壊す変更は
却下する。** 常駐コストが掛かる実行方式(Containers の常時稼働、固定間隔
ポーリングの DO alarm、非 hibernation の WebSocket 等)を新しく導入すると
きは、必ず代替の demand-start / event-armed 設計を検討すること。

再設計の背景: 検証用サブセットの積み重ねでコードが複雑化したため、
`k8flare` 自身のコードを最小化し、k8s/k3s の実物資産(Go パッケージ・
実バイナリ)を最大限再利用する方向に作り直している。手書きで再実装しない。

## アーキテクチャ(v3 ターゲット設計 — 2026-07-10)

本番前の検証フェーズにつき、既存コードや稼働中クラスタの DO データの移行は
設計制約にしない(ユーザー決定 2026-07-10)。k8s 制御プレーンを Cloudflare の
**3 プリミティブ**だけに載せるのが理想形:

1. **Dynamic Worker(LOADER + ASSETS チャンク)= 計算。** 制御プレーンロジック
   (Go WASM)はすべてここで動く。永続状態は持たない(メモ化は isolate 生存期間
   限定)。契約は 2 形態のみ:
   - **per-request**(apiserver): リクエスト毎に fresh な Go インスタンス
   - **resident**(kcm / gc): poke ごとの有界 waitUntil ポンプ、event-armed
   各バイナリは Loader の 64MiB raw cap 制約(Makefile の manifest レシピが
   ゲート)。**エントリポイント毎に pkg サブパッケージを分離する** — 同一
   パッケージに同居させると呼ばれないコントローラーのコードまでリンクされる
   (kcm 実測 +4.15MB、2026-07-09。pkg/controllers/restconfig の doc comment)。
2. **Durable Object + Facets = 状態。** Dynamic Worker が持てないデータはすべてここ。
   - Cluster DO: root = リビジョン権威 + kine ログ。facets = ns/<name>・
     events-log・ca-vault(+ per-cluster token vault)
   - WatchHub DO: watch fan-out(hibernation WebSocket 必須)
   - CFContainersScheduler + NodeVM{Small,Medium,Large} DO(containers):
     Pod 実行(任意 OCI 実行が要るため Containers)
3. **ASSETS = 静的。** デプロイ毎に不変なもの: WASM チャンク(≤24MiB×N +
   sha256 manifest)・patched wasm_exec.js・OpenAPI / discovery 文書。

シェル Worker は packages/k8flare-worker の 1 つだけ(唯一のデプロイ単位・唯一の公開
fetch)。その TS の役割は (1) ルーティング/認証 (2) LOADER 呼び出し(loaded
worker からは不可、S14) (3) DO クラスの器 (4) bindings グルー、に限定し、
**ビジネスロジックを TS に書かない**(Go-first の徹底)。

```
kubectl / kubelet(BYO VM: cmd/agent 無改変 k3s embed)
   │ HTTPS + token
   ▼
packages/k8flare-worker(シェル: 認証一元化・watch ストリーミング・kubelet proxy・
   │            /nodes/* オペレータ面。/internal/* は外部からはクラスタトークン必須)
   ├──► apiserver DW(per-request。apidef テーブル駆動、
   │      pkg/apiserver/cmd/apiserver-wasm。kine への経路は STORAGE 自己バインディング
   │      (ClusterLoopback entrypoint)経由 — Loader env に DO namespace は渡せない、S2)
   ├──► kcm DW(resident。実 kube-controller-manager の 7 ワークロードコントローラー、
   │      pkg/controllers/cmd/kcm-wasm。poke = Cluster DO の pingControllers 直呼び /
   │      event-armed 安全網 alarm)
   ├──► gc DW(resident。実 garbagecollector、pkg/controllers/cmd/gc-wasm。
   │      kubectl delete は非同期 eventually-consistent(実 k8s と同じ挙動、
   │      ユーザー承認済み)。Orphan/Foreground は apiserver 側の最小
   │      graceful-deletion(deletionTimestamp+finalizer スタンプ、
   │      pkg/apiserver/gracefuldelete.go)で実 GC に完全委任。upstream の
   │      GC conformance 7 テストを 7/7 で required 昇格済み 2026-07-11)
   ├──► sched DW(resident。実 kube-scheduler、pkg/controllers/cmd/kcm-wasm/scheduler、
   │      -tags schedwidth。2026-07-10 に 101MB→45MB opt へ削減して cap 内に収め、
   │      wrangler dev で実 Pod bind を実機確認済み — 経緯は
   │      docs/platform-verification.md S21。cmd/scheduler(ホスト)も併存)
   ├─ Cluster DO / WatchHub DO / CFContainersScheduler + NodeVM DO(上記 2.)
   └─ ASSETS(上記 3.)
```

PV/PVC/StorageClass: 汎用 CRUD リソースとしては存在するが、動的プロビジョナーは
現在なし(独自 R2 バックエンドを 2026-07-08 に撤去 — 経緯は git history と
docs/cost-model.md「Phase 8」/docs/platform-verification.md「S6」参照。実クラス
タでプロビジョナー未設定時と同じく PVC は Pending のまま。k8s 標準の CSI ドライ
バーでの作り直しを検討中)。

(訂正 2026-07-06: 旧 6 Worker 分割(gateway/apiserver/storage/runtime/
controllers/nodes + script_name DO binding + service binding 循環)は
S19 の 3 ゲート検証を経て単一 Worker に統合。旧クラスタの DO state は
破棄(ユーザー承認)。経緯は docs/platform-verification.md S19。)

移行中の詳細な設計判断・未検証項目は `docs/platform-verification.md`
`docs/cost-model.md` `docs/multi-tenancy-and-hosting.md`
`docs/control-plane-architecture.md` `docs/general-purpose-k8s-plan.md` を見る
こと。ここでは重複させない。

## コマンド

ローカル作業は `Makefile`(GNU Make のファイル依存関係で、Go ソースに差分が
なければ `make wasm` はビルドをスキップする)を主な入口とする。CI は
`git checkout` 直後で mtime が信用できないため Makefile を経由せず、
`npm run build:wasm` 等を直接呼ぶ(`.github/workflows/*.yml` 参照)。

```
pnpm install                     # 初回のみ
make wasm                        # Go を変更したら必須。apiserver+KCM+GC+sched のチャンクと
                                 # selector.wasm(gateway バンドル用)を
                                 # packages/k8flare-worker/assets/wasm/ に生成(Make のファイル依存関係で
                                 # 差分ベースにスキップ。KCM の wasm-opt 込みで約2分だが、対象バイナリの
                                 # ソースが変わっていなければ即スキップ。強制再ビルドは `make clean-wasm wasm`)
make dev                         # wrangler dev(単一 config: packages/k8flare-worker/wrangler.jsonc)。
                                 # Docker なし環境は `wrangler dev -c packages/k8flare-worker/wrangler.jsonc
                                 # --persist-to .wrangler/state --enable-containers=false` を直接叩く
make check                       # TypeScript 型チェック(vp check)
npx tsc --noEmit -p packages/k8flare-worker/tsconfig.json   # vp check が拾わない型面の直接チェック
make vet                         # go vet ./pkg/... ./cmd/k8flare-gen/...
make test                        # go test ./pkg/apiserver/... (wasm ターゲットに依存、自動で先にビルドされる)
                                 # 自前で `npx wrangler dev` を起動して実 client-go で駆動する
                                 # (単一 config + --enable-containers=false + KCM_DISABLED:1)
make gen                         # コード生成(cmd/k8flare-gen。生成後は git diff --exit-code で検証)
```

CI ゲート(`.github/workflows/`): `ci.yml`(vp check / build:wasm / go vet+test)、
`e2e-conformance.yml`(**Definition of Done**。k8s v1.36.2 の e2e.test を実クラスタで
実行し、required baseline 群 + advisory 群を評価)。

## 不可侵ルール

1. **conformance CI が Definition of Done。** 進捗は required focus set を増やすことで測る。減らしてはならない。
2. **実際に動かして検証する。** ソースを読んだだけの結論を事実として書かない。過去に「kube-proxy が Worker をハングさせる」という結論が誤りだった(実際は古い DO state が原因)前例がある。
3. **upstream の実物(k8s/k3s の実バイナリ・実パッケージ)を再実装より優先する。** 5つの手書き TS コントローラーは実 kube-controller-manager 埋め込みに置き換えて削除された。新しい機能も同じ発想で: 先に「実物を埋め込めないか」を検討する。
4. **訂正は隠さず記録する。** 判断が後で誤りだと分かったら、docs にその旨と発見経緯を書き足す(消して書き直さない)。
5. **flaky なテスト/インフラは直すか revert する。** 「原因不明だが時々失敗する」を放置しない。

## コスト不変条件

1. **アイドルクラスタの維持コストはストレージ代のみに近づける。** アイドル時に動いてよいものはゼロ: 常時稼働プロセス0、DO alarm は停止(パーク)、WebSocket は hibernation。
2. **常駐は「壁時計課金」だとコンセプト違反、「CPU 時間課金」なら許容。** Workers/DO は実 CPU 消費時間のみ課金(I/O 待ちは無課金、ストリーミング応答に壁時計上限なし)。Containers は稼働時間(壁時計)で課金されるため、Containers 上のプロセスは「仕事がある間だけ」動かす(demand-start、`onActivityExpired` で ask-before-sleep して `stop()`)。idle-timeout は固定値で決め打ちしない — 短すぎるとサッシング(頻繁な起動停止による再同期コスト増)、長すぎるとアイドル課金が伸びる。実測してから決める。
3. **DO alarm は event-armed のみ。固定間隔ポーリング禁止。** 安全網 alarm も「未処理の仕事がある時だけ」再武装し、アイドルで自己解除(パーク)すること。
4. **watch/WebSocket は hibernation API 必須。**
5. **新機能・新コンポーネントは実装前にコストを見積もる。** アイドル時の月額と、アクティブ時の単価(requests / duration GB-s / rows read-written / alarm 回数 / Loader $0.002/unique/day / Containers vCPU・メモリ秒)を `docs/cost-model.md` に書いてから実装し、実測で更新する。
6. **Pod ワークロードはユーザーのコスト**(制御プレーンの常駐禁止とは別枠)。ただし k8flare 自身がシステム Pod(CoreDNS 等)を動かす場合はノード側で動かし、制御プレーンのコストに含めない。
7. **ホットパス(kubectl が直接待つ経路: apiserver・gateway・watch)は Containers 化しない。** Containers のコールドスタートは典型 1〜3秒・最悪 3〜15秒(Workers isolate は 5ms 未満)。常時ウォームに保つと月 $50+/クラスタ規模の固定費が発生し「アイドル ~0」と両立しない。Containers は非同期リコンサイラ専用。
8. コスト感応な API(`setAlarm`/`setInterval`/`sleepAfter`/`onActivityExpired`/cron/`scheduled`/`waitUntil` 等)を追加・変更するときは、上記1〜7に照らして一度立ち止まって考えること。

## ローカル開発の落とし穴

- DO の state は `--persist-to .wrangler/state`(repo ルート)で明示する(Worker 分割後の現行規約。旧 `packages/worker/.wrangler/state` は使われない)。
- `.dev.vars` は wrangler の設定ファイルと同じディレクトリでしか読まれない。repo ルートに置いても無視される(雛形は `packages/k8flare-worker/.dev.vars.example`。2026-07-30 以前はルートに置かれていて、コピーしても効かない罠になっていた)。
- トークン未設定時は開発用トークン `k8flare-dev-token` にフォールバックする(Go/CI はこれに依存しているので「直す」対象ではない)。
- `wrangler dev` の alarm エミュレーションは、読み取り専用のポーリングだけでは発火しないことがある。「動いていない」と結論する前に書き込みを1件試すこと。
- `kubectl apply` に `--validate=false` はもう不要(OpenAPI v2/v3 を Static Assets で配信、実 kubectl で確認済み)。ただし plain HTTP(`wrangler dev` そのまま)だと client-go の `clientcmd` が TLS 以外への認証情報送信を拒否するため、kubeconfig 経由の実 kubectl 検証にはローカル TLS 終端(自己署名証明書 + リバースプロキシ)が要る — Go の `rest.Config{BearerToken: ...}` を直接使う `go test` はこの制約を受けない。サーバー側の strict field validation は実装済みで既定 Strict(pkg/apiserver/fieldvalidation.go。この行の旧記述「未実装」は 2026-07-27 の docs 監査で誤りと判明し訂正)。
- `wrangler deploy` / `wrangler secret put` は実アカウントに影響するので、指示なく実行しない(`.claude/settings.json` の deny 設定でもブロックされる)。
- 単一 Worker 統合後(2026-07-06)、`npm run dev` は常に全コンポーネント(実 KCM 含む)を含む。**`go test ./pkg/apiserver/...` は同じ単一 config を `--var KCM_DISABLED:1` 付きで起動する** — テスト内の Pod は KCM に触られない前提で書かれており、このキルスイッチ(storage の pingControllers と Controllers DO の early-return)がその前提を守る。「dev では Pod が動くのに test では KCM が反応しない」はこの差が原因。KCM を動かすには先に `npm run build:wasm`(KCM の wasm-opt 込みで約2分)で `packages/k8flare-worker/assets/wasm/` を生成しておくこと(apiserver チャンクがないと dev は API 応答自体ができない)。
- **Docker が動いていない環境では `wrangler dev` は containers 定義で hard fail する。** `--enable-containers=false` を付けること(go test / e2e / cost-gate の各ハーネスは付与済み)。コンテナイメージには EXPOSE が必須(ないと dev が起動拒否、S19 実測)。
- `go test ./pkg/apiserver/...` は repo ルートの `.wrangler/state` を**クリアせずに**使う。中断された前回実行の残骸があると「already exists」で決定論的に落ちる(2026-07-05 実測)。落ちたらまず `rm -rf .wrangler/state` してから再実行し、flaky と結論しない。
- **`.build/` のミラー(k8s-js-mirror / clientgo-lean-mirror / apiserver-js-mirror)は再生成可能。** `make` のどのターゲットからも毎回走るが APFS clonefile で ~3 秒。**訂正 (2026-07-30)**: この行は以前「再生成前に必ず現物を退避せよ」と要求していた。根拠だった 2026-07-05 の回帰(ディスク上のミラー状態からしか 64MiB 以下の KCM が作れなかった件)は**同日中に解決済み**で、失われていた仕組みは `kubernetes.Interface` の幅であり、`-tags leanwidth` + sha256 ピン付きミラー変換としてコミット済み構成に再構築されている(docs/platform-verification.md の当該節の "RESOLVED for KCM" を参照)。クリーンクローンから全チャンクが cap 内でビルドできることも実測済み。退避は不要になったが、`.build/*.bak-*` が残っていればそれは当時のデバッグ残骸で、削除して差し支えない。

## コード規約

- **Go-first。** 新しい制御プレーンロジックは Go(`pkg/`)に書き、k8s.io / k3s-io のパッケージを再利用する。TypeScript はプラットフォームが要求する部分(Worker エントリポイント、DO クラスのグルー、bindings)に限定する。手書き行数を減らすこと自体が目標。
- **生成コードは手編集しない。** `gen/` ディレクトリ配下、または `Code generated by k8flare-gen. DO NOT EDIT.` ヘッダーを持つファイルは編集禁止(hook でもブロックされる)。変更したい場合はジェネレーター(`cmd/k8flare-gen`)を直して `go generate ./...` を再実行し、生成結果をコミットする。`packages/k8flare-worker/assets/wasm/` などのビルド出力も同様に直接編集しない。
- **レイアウト**: `packages/k8flare-worker/`(唯一のデプロイ単位・唯一の TypeScript ワークスペース。src/ 配下に gateway/ storage/ controllers/ nodes/ clusters(予定)/ k8s(旧 `packages/k8s`、2026-07-08 統合)のサブツリー)+ Go モジュールはルート単一(k3s-io の replace 群をモジュール間で重複させない)。`cmd/` は BYO VM / ホストプロセスで動く独立した実行ファイル専用(`cmd/agent` `cmd/scheduler` `cmd/controller-manager` `cmd/k8flare-gen`)。k8flare Worker 自身の WASM エントリーポイント(独立実行ファイルではなく Worker のビルド成果物の一部)は対応する `pkg/` 配下の `cmd/` サブディレクトリに置く: `pkg/apiserver/cmd/apiserver-wasm` / `pkg/controllers/cmd/kcm-wasm`(2026-07-08、旧リポジトリ直下の `cmd/apiserver-wasm` / `cmd/kcm-wasm` から移動)。
- **ブランチ**: `feat/*` | `fix/*` | `docs/*`。main への直接コミット禁止。

## docs 索引

- `docs/multi-tenancy-and-hosting.md` — マルチテナント・Facets・WatchHub のターゲットアーキテクチャ
- `docs/control-plane-architecture.md` — コントローラーと Cloudflare プリミティブの対応関係
- `docs/general-purpose-k8s-plan.md` — conformance フォーカスセットを広げる段階計画
- `docs/cloudflare-mesh-networking.md` — ノード間ネットワーキングの評価
- `docs/platform-verification.md` — 2026 Cloudflare 機能の実機検証スパイク結果(S1-S8)
- `docs/cost-model.md` — コンポーネント毎のアイドル/アクティブ単価の見積もりと実測
- `docs/custom-code-inventory.md` — 手書きコード vs upstream 再利用の棚卸しと「実物を使わない」判断の記録
- `docs/k8s-version-bump.md` — go.mod の k8s.io/kubernetes pin 更新手順(regen → build → conformance CI)
- `docs/admin-guide.md` / `docs/user-guide.md` — 管理者向け・クラスタ利用者向けガイド
- `README.md` — ユーザー向け API サポート状況・デプロイ手順
