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

## アーキテクチャ(1画面)

```
kubectl / kubelet(BYO VM: cmd/agent 無改変 k3s embed)
   │ HTTPS + token
   ▼
workers/gateway(TS, 唯一の公開 Worker)
   │ 認証一元化・watch ストリーミング・kubelet proxy(VPC)
   ├──► workers/apiserver(Go WASM, apidef テーブル駆動)
   ├──► workers/runtime(TS, CRD/DynamicWorker/WorkerTrigger, ユーザーコードは LOADER)
   ▼ DO binding(script_name)
workers/storage(TS)
   ├─ Cluster DO: リビジョン権威・kine ログ・facet(ns/<name>, events-log, ca-vault)
   └─ WatchHub DO: watch fan-out(hibernation 必須)
workers/controllers: 実 kube-controller-manager(DO-hosted Go WASM 常駐、
   fetch()/alarm() ディスパッチ + waitUntil。Containers フォールバックなし)。
   **実 kube-scheduler は GOOS=js で構文コンパイル不可**(k8s 本体フォーク
   禁止と衝突するため断念、判断根拠は docs/platform-verification.md 参照)
   — BYO VM / ホストプロセス専用に固定。KCM も client-go 型付き
   Clientset+Informers だけで Workers 10MiB 予算の 89% を消費し、軽量
   クライアントに置き換えても実コントローラー本体が +6MiB 級を要求する
   ため 5 コントローラー構成で 15〜16MiB(予算超過)——**KCM も現状 BYO VM /
   ホストプロセス専用**(判断根拠は docs/platform-verification.md 参照)。
workers/nodes: Pod-on-Containers ノードバックエンド(任意 OCI 実行が要るため Containers)
R2: PV/PVC/StorageClass バックエンド
```

移行中の詳細な設計判断・未検証項目は `docs/platform-verification.md`
`docs/cost-model.md` `docs/multi-tenancy-and-hosting.md`
`docs/control-plane-architecture.md` `docs/general-purpose-k8s-plan.md` を見る
こと。ここでは重複させない。

## コマンド

```
pnpm install                     # 初回のみ
npm run build:wasm               # Go を変更したら必須。Worker は app.wasm を実行する、ソースではない
npm run dev                      # wrangler dev(複数 Worker 分割後は multi-config になる)
vp check                         # TypeScript 型チェック(vite-plus)
go vet ./pkg/apiserver/...
go test ./pkg/apiserver/...      # 自前で `npx wrangler dev` を起動して実 client-go で駆動する。
                                 # pnpm install と build:wasm を先に済ませておくこと
go run ./cmd/k8flare-gen         # コード生成(存在する場合。生成後は git diff --exit-code で検証)
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
- `.dev.vars` は wrangler の設定ファイルと同じディレクトリでしか読まれない。repo ルートに置いても無視される。
- トークン未設定時は開発用トークン `k8flare-dev-token` にフォールバックする(Go/CI はこれに依存しているので「直す」対象ではない)。
- `wrangler dev` の alarm エミュレーションは、読み取り専用のポーリングだけでは発火しないことがある。「動いていない」と結論する前に書き込みを1件試すこと。
- `kubectl apply` に `--validate=false` はもう不要(OpenAPI v2/v3 を Static Assets で配信、実 kubectl で確認済み)。ただし plain HTTP(`wrangler dev` そのまま)だと client-go の `clientcmd` が TLS 以外への認証情報送信を拒否するため、kubeconfig 経由の実 kubectl 検証にはローカル TLS 終端(自己署名証明書 + リバースプロキシ)が要る — Go の `rest.Config{BearerToken: ...}` を直接使う `go test` はこの制約を受けない。サーバー側の strict field validation(`fieldValidation=Strict`)は未実装なので、未知フィールドはクライアント側 OpenAPI 検証をすり抜けても現状はサーバーで黙って受理される。
- `wrangler deploy` / `wrangler secret put` は実アカウントに影響するので、指示なく実行しない(`.claude/settings.json` の deny 設定でもブロックされる)。

## コード規約

- **Go-first。** 新しい制御プレーンロジックは Go(`pkg/`)に書き、k8s.io / k3s-io のパッケージを再利用する。TypeScript はプラットフォームが要求する部分(Worker エントリポイント、DO クラスのグルー、bindings)に限定する。手書き行数を減らすこと自体が目標。
- **生成コードは手編集しない。** `gen/` ディレクトリ配下、または `Code generated by k8flare-gen. DO NOT EDIT.` ヘッダーを持つファイルは編集禁止(hook でもブロックされる)。変更したい場合はジェネレーター(`cmd/k8flare-gen`)を直して `go generate ./...` を再実行し、生成結果をコミットする。`workers/apiserver/build/` などのビルド出力も同様に直接編集しない。
- **レイアウト**: `workers/<component>/`(各自 wrangler.jsonc を持つ独立デプロイ単位)+ `packages/`(共有 TS ライブラリのみ、デプロイ単位ではない)+ Go モジュールはルート単一(k3s-io の replace 群をモジュール間で重複させない)。
- **ブランチ**: `feat/*` | `fix/*` | `docs/*`。main への直接コミット禁止。

## docs 索引

- `docs/multi-tenancy-and-hosting.md` — マルチテナント・Facets・WatchHub のターゲットアーキテクチャ
- `docs/control-plane-architecture.md` — コントローラーと Cloudflare プリミティブの対応関係
- `docs/general-purpose-k8s-plan.md` — conformance フォーカスセットを広げる段階計画
- `docs/cloudflare-mesh-networking.md` — ノード間ネットワーキングの評価
- `docs/platform-verification.md` — 2026 Cloudflare 機能の実機検証スパイク結果(S1-S8)
- `docs/cost-model.md` — コンポーネント毎のアイドル/アクティブ単価の見積もりと実測
- `docs/k8s-version-bump.md` — go.mod の k8s.io/kubernetes pin 更新手順(regen → build → conformance CI)
- `README.md` — ユーザー向け API サポート状況・デプロイ手順
