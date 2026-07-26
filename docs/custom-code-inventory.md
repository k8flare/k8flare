# 独自コード棚卸し (custom code inventory)

2026-07-25 時点 (branch `chore/cleanup`)。行数は `wc -l`(テスト・生成コード除く、別掲)。
目的: 「どこが手書きで、どこが upstream (k8s.io / k3s-io) の実物か」を一覧できるようにし、
新規コードを書く前に「実物を埋め込めないか」(CLAUDE.md 不可侵ルール 3) を判断する土台にする。

## 分類

| クラス | 意味 |
|---|---|
| **A** | 純粋な手書き (upstream に対応物がない、または使えない) |
| **B** | 手書きグルー — upstream パッケージをそのまま配線する薄い層 |
| **C** | 生成コード ([cmd/k8flare-gen](../cmd/k8flare-gen) が出力。手編集禁止) |
| **D** | upstream オーバーレイ — js/wasm でビルドできない upstream ファイルの差し替え・narrow 版 |

## 総計

| 区分 | 行数 |
|---|---|
| Go 手書き (A+B) | 約 13,900 |
| Go 生成 (C) | 2,109 |
| Go オーバーレイ (D) | 約 1,300 |
| Go テスト | 5,818 (全て [pkg/apiserver](../pkg/apiserver)) |
| TypeScript 手書き (全てプラットフォームグルー) | 約 5,400 |
| TypeScript 生成 (C) | 49 |
| 生成アセット (OpenAPI v2/v3 + discovery 文書, コミット済み) | 約 220,000 (≈10MB) |

生成コードの視点で言えば、このリポジトリの「実効的な手書き量」は Go+TS 合わせて約 19,000 行で、
残り(生成 2.2k + アセット 220k + オーバーレイ 1.3k)はジェネレーター 1,454 行
([cmd/k8flare-gen](../cmd/k8flare-gen)) と upstream ピンから機械的に導出される。

## 1. Go ツリー

| path | 行数 | 分類 | 配線する upstream / 備考 |
|---|---|---|---|
| [pkg/apiserver](../pkg/apiserver) | 7,910 | A/B | 最大の手書き領域。§2 参照 |
| [pkg/apiserver/apidef](../pkg/apiserver/apidef) | 738 | A | リソーステーブル(discovery/table/restmapper を駆動) |
| [pkg/controllers/controllermanager.go](../pkg/controllers/controllermanager.go) | 231 | B | **実 kube-controller-manager**(ワークロード系 6 コントローラー) |
| [pkg/controllers/gc](../pkg/controllers/gc) | 319 | B | **実 garbagecollector**(無改変。リンク閉包分離のため別 pkg) |
| [pkg/controllers/sched](../pkg/controllers/sched) | 176 | B | **実 kube-scheduler** |
| [pkg/controllers/restconfig](../pkg/controllers/restconfig) | 100 | B | resident WASM コントローラー用 rest.Config |
| [pkg/controllers/cmd](../pkg/controllers/cmd) (kcm/gc/sched entrypoints) | 129 | B | WASM エントリポイント |
| [pkg/apiserver/cmd/apiserver-wasm](../pkg/apiserver/cmd/apiserver-wasm) | 142 | B | apiserver WASM エントリポイント |
| [pkg/agent](../pkg/agent) | 966 | A | BYO-VM ノードエージェント(埋め込み k3s 周りの shim 群) |
| [pkg/cfruntime](../pkg/cfruntime) | 686 | A/B | Go↔Workers ランタイムブリッジ (syscall/js) |
| [pkg/leanclient](../pkg/leanclient) (root+clientset+informers) | 1,413 | B | lean client-go 表面 (informer/clientset グルー) |
| [pkg/leanclient/gen](../pkg/leanclient/gen) | 2,052 | **C** | 生成 typed clients (8 グループ) |
| [pkg/clientgo-lean-overlays](../pkg/clientgo-lean-overlays) | 1,039 | **D** | clientset/informers の narrow 版オーバーレイ |
| [pkg/k8s-js-overlays](../pkg/k8s-js-overlays) | 269 | **D** | js/wasm でビルド不能な upstream ファイルの build-tag 差し替え |
| [pkg/selectormatch](../pkg/selectormatch) | 163 | B | **実 apimachinery** labels/fields パーサ(旧 TS サブセット実装を置換, S22) |
| [cmd/k8flare-gen](../cmd/k8flare-gen) | 1,454 | A | コードジェネレーター本体 |
| [cmd/agent](../cmd/agent) / [cmd/scheduler](../cmd/scheduler) / [cmd/controller-manager](../cmd/controller-manager) | 475 | B | ホスト/BYO-VM バイナリ (k3s agent / 実 sched / 実 KCM 埋め込み) |

## 2. pkg/apiserver ファイル別 (手書きの本丸)

doc comment を確認した結果、多くは「upstream の実物を配線する B」で、ゼロから再実装した A は
リクエストディスパッチ/Table/サブリソース機構と DO 固有トランスポートに集中している。(2026-07-26: ストア層は upstream genericregistry.Store + KineStorage への移行が完了し、A から B に転じた — S25 参照。)

| file | 行数 | upstream との関係 |
|---|---|---|
| [handler.go](../pkg/apiserver/handler.go) | 729 | **A**: 汎用 REST verb ディスパッチ。upstream generic registry+endpoints 相当の再実装 |
| [store.go](../pkg/apiserver/store.go) | 405 | **B** (2026-07-26〜): CRUD は upstream genericregistry.Store に委譲。残りはキー構築とコレクション生パス |
| [subresource.go](../pkg/apiserver/subresource.go) | 555 | **A**: status/binding/scale サブリソースルーティング |
| [certmanager.go](../pkg/apiserver/certmanager.go) | 467 | **A**: CA 2 系統 + kubelet 証明書署名 (crypto/x509、DO 永続化) |
| [endpoints.go](../pkg/apiserver/endpoints.go) | 456 | A: Endpoints/EndpointSlice reconcile。upstream の定数を手コピー(import 不可の internal) |
| [supervisor.go](../pkg/apiserver/supervisor.go) | 347 | A: k3s supervisor プロトコル互換面 |
| [storage.go](../pkg/apiserver/storage.go) | 344 | **A**: Cluster DO への kine 風 k/v トランスポート (プラットフォーム固有) |
| [table.go](../pkg/apiserver/table.go) | 376 | A: kubectl 用 Table 変換。upstream printers/tableconvertor 相当 |
| [gracefuldelete.go](../pkg/apiserver/gracefuldelete.go) | 289 | B: スタンプ自体も upstream Store.Delete に委任 (2026-07-26)。カスケードは**実 GC に委任**。残りは orphan straggler sweep 等の k8flare 固有分 |
| [clusterip.go](../pkg/apiserver/clusterip.go) | 328 | B: IP↔offset 計算は **upstream ServiceIPAllocator**、DO 永続化のみ独自 |
| [nodelifecycle.go](../pkg/apiserver/nodelifecycle.go) | 312 | A: node-lifecycle-controller のサブセット |
| [serviceaccounttoken.go](../pkg/apiserver/serviceaccounttoken.go) | 261 | B: **実 upstream JWT authenticator/token generator** |
| [rbac.go](../pkg/apiserver/rbac.go) | 259 | B: **実 upstream RBACAuthorizer** をローカル getter で配線 |
| [nodecidr.go](../pkg/apiserver/nodecidr.go) | 234 | B: **upstream CIDRSet** のビット割当、永続化のみ独自 |
| [computeclass.go](../pkg/apiserver/computeclass.go) | 222 | A: Pod-on-Containers 用 admission(k8flare 固有機能) |
| [discovery.go](../pkg/apiserver/discovery.go) | 216 | A: apidef.Table から discovery 文書を計算 |
| [scheme.go](../pkg/apiserver/scheme.go) | 214 | B: 実型登録 + upstream defaulters/codec の設定 |
| [defaults.go](../pkg/apiserver/defaults.go) | 172 | B: **実 upstream versioned defaulters** の適用 + 少数の上書き |
| [server.go](../pkg/apiserver/server.go) | 146 | B: サーバー組み立て |
| その他 15 ファイル | ≈1,300 | A/B 混在: auth/vkubeproxy/ssar/priority/fieldvalidation/sa/tokenreview/namespacedelete/nodepassword/bootstrap/tokenvault/stores/casretry/watch |

## 3. TypeScript ([packages/k8flare-worker/src](../packages/k8flare-worker/src)) — 全て手書きプラットフォームグルー(設計上 TS にビジネスロジックを置かない)

| subtree | 行数 | 役割 |
|---|---|---|
| [storage/](../packages/k8flare-worker/src/storage) | 1,678 | Cluster DO + WatchHub DO (kine ストア・facets・watch fan-out) |
| [nodes/](../packages/k8flare-worker/src/nodes) | 961 | Containers スケジューラ + NodeVM DO |
| [clusters/](../packages/k8flare-worker/src/clusters) | 676 | クラスタレジストリ/トークン/管理 API 認証 |
| [k8s/](../packages/k8flare-worker/src/k8s) | 671 | watch ストリーミング・URL マッピング・selector-wasm ブリッジ |
| [gateway/](../packages/k8flare-worker/src/gateway) | 660 | シェルルーティング/認証/kubelet・exec プロキシ |
| [controllers/](../packages/k8flare-worker/src/controllers) | 436 | resident KCM/GC/sched の poke ポンプ DO |
| [loader/](../packages/k8flare-worker/src/loader) | 229 | Dynamic Worker (LOADER) 呼び出し |
| src 直下 | 151 | エントリポイント/env |

## 4. 生成パイプライン

単一の情報源は **[pkg/apiserver/apidef/table.go](../pkg/apiserver/apidef/table.go)(リソーステーブル)と
go.mod の k8s.io/kubernetes ピン**。`make gen`(= `go run ./cmd/k8flare-gen`)が 6 ステップで
以下を再生成し、CI は `git diff --exit-code` でドリフトを検出する
(手編集は [.claude/hooks/gen-guard.sh](../.claude/hooks/gen-guard.sh) でもブロック)。

| ステップ | 出力 | 行数 | 内容 |
|---|---|---|---|
| defaulters | [pkg/apiserver/zz_generated_defaulters.go](../pkg/apiserver/zz_generated_defaulters.go) | 33 | 全 API グループの**実 upstream versioned defaulters** を Scheme に登録 |
| version | [pkg/apiserver/zz_generated_version.go](../pkg/apiserver/zz_generated_version.go) | 14 | `/version` が返す値を k8s ピンから導出(手書きリテラル排除) |
| resource-kinds | [packages/k8flare-worker/src/k8s/gen/resource-kinds.gen.ts](../packages/k8flare-worker/src/k8s/gen/resource-kinds.gen.ts) | 49 | plural→Kind 表(gateway/watch 層用) |
| openapi | [packages/k8flare-worker/assets/openapi/](../packages/k8flare-worker/assets/openapi) | ≈10MB | **実 upstream OpenAPI v2/v3 文書**。Static Assets 配信で `kubectl apply` がクライアント検証込みで動く |
| discovery-assets | [packages/k8flare-worker/assets/api/](../packages/k8flare-worker/assets/api) / [apis/](../packages/k8flare-worker/assets/apis) | 小 | discovery 文書の静的配信版 |
| leanclient | [pkg/leanclient/gen/](../pkg/leanclient/gen) | 2,052 | WASM コントローラー用の typed client 8 グループ(client-go 全部をリンクしないための narrow 版) |

このほかに**ビルド時生成(コミットしない)**が 2 層ある:

- `.build/` ミラー([packages/wasm-build](../packages/wasm-build) の gen-k8s-js-mirror / gen-clientgo-lean-mirror):
  upstream ソースを js/wasm ビルド可能な形にミラーし、[pkg/k8s-js-overlays](../pkg/k8s-js-overlays) /
  [pkg/clientgo-lean-overlays](../pkg/clientgo-lean-overlays) のオーバーレイを重ねる。ビルド入力なので
  再生成前の退避が必要(CLAUDE.md「ローカル開発の落とし穴」参照)。
- `packages/k8flare-worker/assets/wasm/` チャンク(`make wasm`): apiserver/kcm/gc/sched の Go バイナリを
  wasm-opt → 24MiB 分割 + sha256 manifest 化した Loader 供給物。

k8s バージョンを上げる手順は [docs/k8s-version-bump.md](k8s-version-bump.md)
(ピン更新 → `make gen` → `make wasm` → conformance CI)。

## 5. 「upstream をそのまま使わない」判断の記録

不可侵ルール 3 (実物優先) に対し、手書きが残っている理由は 3 パターンに分類できる:

1. **プラットフォーム制約で実物がそのまま動かない** — 例: upstream generic registry は
   etcd3 ストレージ/informer/aggregator 前提で、DO ストレージ + per-request/resident WASM には
   載らない。[handler.go](../pkg/apiserver/handler.go)/[store.go](../pkg/apiserver/store.go) は
   その置き換え。これが k8flare の価値の中核 (「k8s の機能が使えない場所を埋める」)。
2. **実物は使うが、周辺の持ち込みが過大** — 例: NamespaceLifecycle admission (下記ケーススタディ)。
3. **k8flare 固有機能** — 例: [computeclass.go](../pkg/apiserver/computeclass.go)、
   [supervisor.go](../pkg/apiserver/supervisor.go)、TS 全域。

### ケーススタディ: NamespaceLifecycle admission (commit `9dfbb35`)

「存在しない namespace への create を 404 にする」チェックは
[handler.go](../pkg/apiserver/handler.go) に約 15 行の手書きで入れた。upstream の実物
`k8s.io/apiserver/pkg/admission/plugin/namespace/lifecycle` (233 行) を埋め込まなかった理由:

- プラグイン本体の大半は **informer キャッシュの陳腐化対策**である: cache miss 時に 50ms 待って
  再参照、namespace 削除直後の stale-cache を疑う forceLiveLookup LRU、`WaitForReady()` による
  informer 同期待ち。
- k8flare の apiserver は informer キャッシュを持たず、毎回ストレージ DO を直接読む。つまり
  手書きチェックの 1 回の `Get` は、upstream プラグインが最終防衛線とする
  「live lookup (`client.CoreV1().Namespaces().Get`)」経路**そのもの**であり、セマンティクスは同等。
- 実物を埋め込むには admission.Attributes の組み立て、偽 SharedInformerFactory、
  kubernetes.Interface アダプタが必要で、**手書きグルーがむしろ増える**(パターン 2)。

upstream プラグとの既知の差分 (未実装、必要になったら追加):

- `immortalNamespaces` (default/kube-system の削除保護) は未実装。
- Terminating phase 中の create 拒否は未実装 — k8flare の namespace 削除は同期 sweep で
  Terminating 状態を持たない ([namespacedelete.go](../pkg/apiserver/namespacedelete.go))。
  代わりに削除後の events 再 sweep で race を閉じている (commit `a012a01`)。


### 追記 (2026-07-27, Codex レビュー指摘への回答)

- [priority.go](../pkg/apiserver/priority.go) / LimitRange 系
  ([defaults.go](../pkg/apiserver/defaults.go)) は upstream admission
  プラグイン (`plugin/pkg/admission/priority` / `limitranger`) の部分
  ミラー。実物は internal 型 + admission.Attributes + informer 前提で、
  NamespaceLifecycle と同じパターン 2 (持ち込みが過大) に該当する。
  リンク実測はしていないため、apiserver チャンクの headroom (現在
  ~1.9MB) に余裕ができたら再評価する。
- [storage/index.ts](../packages/k8flare-worker/src/storage/index.ts) の
  node-lifecycle 安全網 alarm は「live Node が存在する間は 60 秒間隔」
  で、厳密には固定間隔。ノードが居る間は kubelet ハートビート
  (10s) がどのみち走るため実害は小さいが、Lease 期限からの
  deadline-armed 化が正しい形 — 縮小候補として記録。

## 6. ホストバイナリの退役方針 (2026-07-25 ユーザー決定)

`cmd/scheduler` / `cmd/controller-manager` は WASM 版 (sched/kcm 動的ワーカー) を
conformance CI の required に昇格させた上で段階的に退役する方針。手順:
(1) GitHub Actions 枠の回復後、dw モードの e2e を回して green を確認
(2) dw モードを required 昇格 (required set は増やすのみ、host モードは当面併記)
(3) 安定後に release.yml の配布と host モードを落とし、両 cmd を削除。
それまでは import グラフ上「現役」のまま維持する。

## 7. 縮小候補 (今後 upstream 置換を検討する価値がある順)

(2026-07-26 追記: 本丸の generic registry 置換はフェーズ 0 スパイクで
**GO 判定** — js リンク可・wasm-opt 後 64.99MB で cap 内。実測と
オーバーレイの詳細は docs/platform-verification.md S25。)

1. [table.go](../pkg/apiserver/table.go) (376 行) — upstream の
   `printers/internalversion` テーブルジェネレーターは internal 型前提だが、
   `k8s.io/apiserver` の tableconvertor (external 型対応) は載る可能性がある。
2. [nodelifecycle.go](../pkg/apiserver/nodelifecycle.go) (312 行) — 実 KCM の
   node-lifecycle-controller を kcm-wasm に追加リンクできれば削除できる(リンクサイズ要計測)。
3. [endpoints.go](../pkg/apiserver/endpoints.go) (456 行) — 実 KCM の
   endpoints/endpointslice コントローラーへの委任 (同上)。
