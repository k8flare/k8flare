# 設計: クラスタ管理の k8s ネイティブ化 (Cluster リソース + operator)

Status: **設計のみ・未実装** (2026-07-27 ユーザー決定: mgmt は default
クラスタに相乗り、トークンは Secret に集約)。実装フェーズは末尾。

## ゴール / 非ゴール

- ゴール: 独自の Admin API (`/clusters`, clusters/api.ts) を廃止し、
  クラスタの発行・トークン管理・削除を **kubectl + RBAC + controller**
  という k8s の標準的な仕組みに載せ替える。
- 非ゴール: 汎用 CRD (apiextensions.k8s.io、ユーザー定義型の動的登録)。
  それは discovery/OpenAPI の動的化を要する別プロジェクト。本設計は
  k8flare 自身の管理面だけを対象にする。

## 方式: 「CRD 風」built-in リソース

k8flare の API 面は apidef.Table + k8flare-gen が単一情報源なので、
`k8flare.com/v1alpha1` を **コンパイル時に組み込むグループ**として追加
する (k3s が独自型を embed するのと同じ)。テーブルに 1 エントリ足せば
S25 の upstream generic registry / discovery / OpenAPI / kubectl Table /
watch / RBAC / graceful deletion がすべて自動で付いてくる。
apiextensions 機構は一切不要で、利用者からは CRD と区別がつかない。

## API 型 (pkg/apis/k8flare/v1alpha1)

```yaml
apiVersion: k8flare.com/v1alpha1
kind: Cluster              # cluster-scoped (namespace なし)
metadata:
  name: team-a             # = 公開 URL の /c/team-a
  finalizers: [k8flare.com/cluster-teardown]
spec:
  displayName: "Team A"    # 任意
  suspended: false          # true でゲートウェイが 503 (将来用・任意)
status:
  phase: Provisioning | Ready | Terminating
  doName: "team-a@<uid>"   # DO ツリーの実名 (uid で再作成を区別)
  endpoint: "https://<worker>/c/team-a"
  tokenSecretRef: { namespace: k8flare-system, name: cluster-team-a }
  conditions: [...]
```

- トークンは **core Secret に集約** (ユーザー決定):
  `k8flare-system/cluster-team-a` に `token` と `kubeconfig` キー。
  実体は従来どおりテナント側 Cluster DO の vault
  (/ca/cluster-tokens) で、Secret はその配布用ミラー。
- ローテーション: `k8flare.com/rotate-token: "<任意の新しい値>"` を
  Cluster に annotate → controller が新トークンを vault に追加し
  Secret を更新、旧トークンを削除して annotation を消す。
- `default` クラスタ自身も `Cluster` オブジェクト "default" として
  表現する (controller が起動時に seed)。削除は admission で拒否。

## cluster-operator (4 つ目の resident 動的ワーカー)

kcm/gc/sched と同型: Go + leanclient の小さな controller を
`pkg/controllers/clusterop` に置き、Controllers DO の poke ポンプに
乗せる。storage の pingControllers トリガーに `/registry/clusters/` を
追加するだけで event-armed になる (アイドルでパーク、不変条件維持)。

**プラットフォーム操作ブリッジ (S2/S14 対応の必須要素)**: Loader で
動く Go からは DO namespace に触れない (S2) ため、vault 書き込み・
teardown・解決キャッシュ更新はシェル Worker に生やす最小の内部 API
(`/internal/clusters/*`、実装は現 clusters/api.ts のロジックの
呼び出し口替え) を GATEWAY Fetcher 経由で叩く。Go 側は k8s オブジェクト
操作のみを直接行う。

**poke フィードバック防止**: operator 自身の status/Secret 書き込みが
ポンプを再駆動して無限 reconcile にならないよう、(a) status 更新は
差分がある時のみ、(b) `status.observedGeneration` で世代管理、
(c) Secret は CONTROLLER_RELEVANT_PREFIXES に入れない (現状入って
いないことを維持)。

reconcile:

1. **作成** (phase 空 → Ready):
   doName = `<name>@<uid>` を採番 → テナント vault 初期化 + 初回
   トークン mint → CA は初回リクエスト時生成 (現状どおり) →
   Secret (token/kubeconfig) を default クラスタに書く →
   status.phase=Ready, tokenSecretRef, endpoint を更新 →
   解決キャッシュ (下記) を更新。
2. **削除** (deletionTimestamp あり):
   phase=Terminating → プラットフォーム操作ブリッジ (下記) 経由で
   既存 teardownCluster の手順化された TS カスケード
   (Scheduler destroy → Controllers → WatchHub → Cluster DO → registry)
   を実行 → Secret 削除 → finalizer 除去 (ここで実際に消える)。
   途中失敗は次の reconcile が再開 (冪等)。finalizer 機構は S25 の
   upstream graceful deletion に乗るが、**teardown 本体は ownerRef
   ベースの実 GC ではなく手順化されたカスケード**である (Codex
   レビュー指摘 #7 の訂正: DO やコンテナは k8s オブジェクトではない
   ので GC の管轄外)。
3. **ローテーション annotation**: 上記。

## ゲートウェイの /c/<id> 解決

現 ClusterRegistry DO は「controller が維持する解決キャッシュ」に降格
して温存する (リクエスト毎に default クラスタの storage を読むより
安い O(1) 索引 + 既存の 60 秒 isolate キャッシュ)。書き込み手は
Admin API から controller に変わるだけで、gateway 側は無変更。

## 認証・RBAC・ブートストラップ

- 管理操作の認可は **default クラスタの RBAC** に移る。クラスタ管理
  だけ許す ClusterRole を同梱する
  (`apiGroups: ["k8flare.com"], resources: ["clusters"]` +
  `k8flare-system` の Secret read)。**注意**: クラスタトークンは
  system:masters バイパスのままなので、RBAC が意味を持つのは SA
  ベースの管理者アカウントに移行してから — それまで「RBAC で権限
  分離」は謳わない (Codex 指摘 #10)。
  ※前提: 現状クラスタトークンは system:masters バイパスなので、
  RBAC を意味のあるものにするには SA ベースの管理者アカウント
  (ServiceAccount + TokenRequest は実装済み) を使う。
- **ブートストラップ**: `K3S_TOKEN` シークレット = default クラスタの
  常時有効なルートトークン (2026-07-27 決定: 不活性化などの状態遷移は
  入れず直感的な体系を優先。ADMIN_TOKENS は廃止済みで、管理操作は
  default クラスタのトークン所持 = 管理者)。トークン細分化や失効は
  RBAC + Cloudflare Access 連携を設計するときに再検討する。

## Admin API の廃止手順

1. Cluster 型 + operator を実装、Admin API と並走 (両者とも vault を
   正とするので競合しない)
2. docs を kubectl 手順に切替え、Admin API を deprecated 化
3. `/clusters` ルートを bootstrap 専用 (`/bootstrap/kubeconfig`) に
   縮退させ、clusters/api.ts の CRUD を削除

## コスト

- operator は poke 駆動 resident: アイドル増分ゼロ、Loader
  +$0.002/unique/day、reconcile 時の DO 読み書きのみ。
- Cluster オブジェクトは低頻度書き込みなので watch/poke 負荷は無視可。

## サイズ/リンク見積もり (実装前に実測すること)

- 型追加による apiserver チャンク増は小 (型 + 生成 client)。ただし
  現 headroom は ~1.9MB なので、apidef 追加後に必ず `make wasm` の
  ゲートで確認する。
- operator バイナリは leanclient ベースの新チャンク (~40MB 級 ×1 追加)
  になるか、gc チャンクへの同居が可能かを比較する (gc は informer が
  全型に張られる特殊事情があるため、まず同居のリンク幅を実測)。

## 実装フェーズ

- P1: apidef に k8flare.com/v1alpha1 Cluster を追加 (types/gen/table、
  make gen)。kubectl で CRUD できるだけの状態。サイズ実測。
  **完了 (2026-07-27)** — 下記「P1 の実測結果」参照。
- P2: cluster-operator (作成/削除/ローテーション reconcile) + 解決
  キャッシュ書き込み。suite に Cluster ライフサイクルの統合テスト追加。
- P3: RBAC ロール同梱 + bootstrap 縮退、Admin API 削除、docs 更新。
  **完了 (2026-07-27)** — 末尾「P3 実装結果」参照。

### P1 の実測結果 (2026-07-27)

実装: `pkg/apis/k8flare/v1alpha1`(Cluster/ClusterList、DeepCopy は手書き —
このリポジトリに deepcopy-gen が無いため。型が増えたらジェネレーター化する)、
apidef.Table への 1 エントリ(cluster-scoped + status subresource)、
`pkg/apiserver/clusterprotect.go`(default の削除を 403)、
`pkg/apiserver/cluster_test.go`(dynamic client で CRUD/watch/status/削除保護、
および DeepCopy の独立性)。

- **サイズ**: apiserver チャンク 65,193,745 バイト(追加前 65,142,479 から
  +51,266 バイト = 約 50KiB)。64MiB cap まで残り 1,915,119 バイト
  (1,870KiB)。ゲート通過。
- **OpenAPI は生成しない**: upstream に k8flare.com のスキーマ文書は存在
  しないので、`cmd/k8flare-gen/openapi.go` の `noUpstreamOpenAPI` でこの
  GroupVersion を v3 文書・v3 discovery index の両方から除外した。結果と
  して **kubectl はこのグループをクライアント側 OpenAPI 検証しない**
  (サーバーは strict field validation が opt-in なので受理する)。実スキーマ
  が欲しくなったら upstream の openapi-gen を k8flare-gen に組み込む。
- **訂正 1 (発見: この作業中)**: `cmd/k8flare-gen/discovery.go` が
  2026-07-08 のレイアウト変更で取り残された `workers/k8flare/assets` に
  書いていた。`make gen` のたびに未追跡の `workers/` を作り、実際に配信
  される `packages/k8flare-worker/assets/apis/` は一切更新されていなかった
  (git diff は未追跡ファイルを見ないので ci.yml の regen-diff も検出でき
  なかった)。openapi.go と同じ `assetsDir` を使うよう修正。この修正が無い
  と clusters は discovery に出ない。
- **訂正 2 (発見: この作業中、実機で)**: `packages/k8flare-worker/src/storage/
  keyspace.ts` の `CLUSTER_SCOPED_RESOURCES` に `clusters` が無く、
  `/registry/clusters/team-a` が「namespace team-a」と解釈されて ns/team-a
  facet に書かれていた。GET は同じ誤分類で一貫するので成功する一方、
  LIST は実在 namespace への fan-out になるため、名前がたまたま既存
  namespace と一致するオブジェクトしか返らなかった(実測: GET 成功・LIST に
  出ない)。`clusters` を追加して解消。この denylist は手書きなので、
  cluster-scoped な型を足すたびに同じ罠がある — apidef.Table の
  `Namespaced` から生成するのが本来の直し方(P2 以降で検討)。
- **既知の隙間**: 削除保護は名前指定 DELETE のみ。collection delete
  (`DELETE .../clusters`)は default も消せる。実際に infra を壊すのは
  operator が入る P2 からなので、そこで generic な DeleteCollection に
  保護名スキップを入れる。
- 検証: `make test`(全 suite、クリーン state から)/ `make test-kcm` /
  `make check` / `npx tsc --noEmit` すべて green。`make vet` は js/wasm 側で
  失敗するが、これは変更前の main でも同一に失敗する(手元の Go 1.26.4 と
  systemd/etcd 依存の組み合わせ)ローカル環境の既存問題。

## 将来拡張: WasmController — 動的ワーカーの k8s ネイティブ宣言 + OCI pull

(2026-07-27 追記。設計のみ。cluster-operator P2 の後継フェーズ)

現在ホストするコントローラーは Controllers DO の
`COMPONENTS = ["kcm","sched","gc"]` にハードコードされている。これを
リソース宣言 + **OCI レジストリからの pull** に置き換える。

### フォーマット: CNCF Wasm OCI Artifact (独自形式は作らない)

CNCF TAG Runtime Wasm WG の標準 layout を採用する:
- config: `application/vnd.wasm.config.v0+json`
- layer: `application/wasm` (1 レイヤー = 1 モジュール)
- カスタム media type を扱えないレジストリ向けに compat 変種
  (標準 media type) も受理する
- push 側は ORAS / 各社レジストリの既存ツールがそのまま使える

参照: https://tag-runtime.cncf.io/wgs/wasm/deliverables/wasm-oci-artifact/

### API 型

```yaml
apiVersion: k8flare.com/v1alpha1
kind: WasmController
metadata:
  name: cluster-operator
spec:
  image: ghcr.io/k2wanko/k8flare-clusterop:v1@sha256:...  # digest ピン必須 (tag のみは admission で拒否)
  imagePullSecrets: [{ name: ghcr-cred }]   # kubernetes.io/dockerconfigjson
  imagePullPolicy: IfNotPresent             # digest 前提なら実質キャッシュ制御
  triggers:                                  # poke プレフィックス (event-armed 宣言)
    - /registry/clusters/
  serviceAccountName: cluster-operator       # 将来: 焼き込みトークンを SA 化し RBAC で権限を絞る
status:
  phase: Pulling | Ready | Failed
  resolvedDigest: sha256:...
  moduleBytes: 41234567                      # 64MiB cap 検査の記録
```

### pull パイプライン (コンテナの pull と同じ流れ)

1. **puller (TS グルー)**: OCI distribution API を素の fetch で実装
   (token 認証 → manifest GET → media type 検証 → layer blob GET)。
   S2 制約により Go 動的ワーカーからは不可なので、Controllers DO /
   シェル Worker 側の仕事。
2. **検証**: manifest の layer size を**ダウンロード前に** 64MiB cap と
   照合して超過は即 Failed (悪意あるレジストリにメモリ/転送を浪費
   させない)。blob はストリームで sha256 検証しつつ R2 へ書き、
   不一致なら破棄。
3. **保存**: R2 に content-addressed で置く (`wasm/sha256:<digest>`)。
   R2 は 25MiB/ファイルの Assets 制約がないため、現行のチャンク分割
   +manifest 機構が丸ごと不要になる。digest アドレスなので不変・
   再 pull は digest 一致確認のみ。
4. **ロード**: Loader factory が R2 から読み
   `wasmctl:<name>@<digest>#<tokenTag>` で LOADER.get。ポンプ/パーク等
   の実行形態は既存 COMPONENTS と完全に同一。

### コスト (不変条件 5: 実装前見積もり)

- pull はイベント時のみ (spec 変更時)。R2: 保存 $0.015/GB-月 +
  Class A 書き込み数回。読み出しは Worker からで egress 無料。
- 実行コストは既存 dynamic worker と同じ (Loader $0.002/unique/day +
  CPU 時間)。アイドル増分ゼロ (poke 駆動のまま)。
- 新規に R2 バケット binding が 1 つ増える (wrangler.jsonc +
  cost-model.md に追記してから実装)。

### 段階

- WC-P1: 型 + puller + R2 キャッシュ。まず cluster-operator 自身を
  OCI 配布で動かす (dogfooding)。**このフェーズでは受理する image を
  同梱 allowlist (既知の名前 + digest) にハード制限する** — SA/RBAC の
  信頼境界 (WC-P3) より前に任意 WASM が root 相当の動的ワーカーに
  なる窓を作らない (Codex 指摘 #12)。
- WC-P2: kcm/gc/sched も WasmController 表現に移行。Static Assets の
  wasm チャンク機構を退役し、**Worker のデプロイとコントロールプレーン
  の版数を分離**する (k8s バージョンアップが wrangler deploy 不要に)。
- WC-P3: SA トークン焼き込み + RBAC で各コントローラーの権限を最小化。
  任意 WASM の持ち込み (マルチテナント) はこの信頼境界が済んでから。

### Pod + RuntimeClass 案を採らない理由 (記録)

Pod として表現すると kubelet 意味論 (probe/exec/restartPolicy) を
約束することになるが、DW は isolate の生死がプラットフォーム任せの
イベント駆動であり嘘になる。専用 CRD の方が誠実 (Knative が
Deployment を使わず Service/Revision を切ったのと同じ判断)。


## 2026-07-27 Codex 設計レビューの反映 (No-Go → 条件付き Go)

上記本文は指摘 #1 (プラットフォーム操作ブリッジ)・#7 (teardown の実像)・
#8 (poke ガード)・#10 (RBAC 表記と時期)・#11 (digest 必須 + cap 先行
検査)・#12 (WC-P1 allowlist) を反映済み。残りは実装フェーズの入口条件
として記録する:

- **#2/#3 生成系の実作業 (P1 の実体)**: 「apidef に 1 行」では済まない。
  必要なのは (a) pkg/apis/k8flare/v1alpha1 の型 + deepcopy + Scheme 登録、
  (b) k8flare-gen の OpenAPI/discovery 生成が upstream スペックのコピー
  前提なので k8flare.com グループを自前生成できるよう拡張、
  (c) leanclient に k8flare.com クライアント生成 + Secrets() の実装
  (現状 panic) + Cluster の status サブリソース対応。P1 はこの 3 点を
  含む見積もりに改める。
- **#4 識別子の一本化**: 移行中に registry 由来の doName と Cluster
  オブジェクト由来の doName が分岐しないよう、「doName の採番者は
  常に 1 人」ルールを決める (P2 では operator が唯一の採番者になり、
  admin API は既存 registry のレコードを読むだけに落とす)。
- **#5 解決キャッシュの整合プロトコル**: Cluster オブジェクトを真実と
  し、registry は「operator が全量再構築できる純キャッシュ」と明文化。
  operator 起動時に全 Cluster を list して registry を突き合わせ、
  差分は Cluster 側に合わせて修復する。
- **#6 default 削除保護の実装**: 現 genericStrategy は Validate が
  no-op。P1 で Cluster 用に「default の delete/update 制限」を持つ
  strategy を追加する (設計上の主張を実装で裏取りしてから謳う)。
- **#9 サイズゲート**: P1 完了時と operator チャンク追加時に
  `make wasm` の cap ゲートで実測し、超過なら先に削減 (fieldmanager 等)
  を行う。これを各フェーズの完了条件に含める。

### P1 実装結果 (2026-07-27)

- `clusters.k8flare.com/v1alpha1` は計画どおり apidef.Table 1 エントリ +
  手書き型 (pkg/apis/k8flare/v1alpha1) で全経路 (scheme/store/route/
  discovery/TS RESOURCE_KINDS/status subresource) が開通。dynamic client
  での CRUD/watch/status/default 削除保護のテストを suite に追加、
  フルスイート + test-kcm green。
- genOpenAPI は noUpstreamOpenAPI スキップ方式: **この group には
  kubectl クライアント側検証が効かない** (typo フィールドは黙って通る)。
  自前 OpenAPI 生成は必要になったときの拡張ポイント。
- サイズ実測: apiserver チャンク 65.19MB (P1 増分 +42KB)。cap 67.11MB
  に対し残 1.83MB。
- 副産物の修正: Makefile の js 側 `make vet` が go.wasm.mod を使って
  おらず S25 以降壊れていたのを発見、ビルドマトリクスどおり
  leanwidth/schedwidth の 2 行に分割して修復。

## P2 実装結果 (2026-07-27)

cluster-operator (作成/削除/ローテーション reconcile + 解決キャッシュ書き込み)
と、その統合テストを実装。5 番目の resident 動的ワーカーとして
Controllers DO に載る (**default クラスタのみ** — テナントには Cluster
オブジェクトが無いので `loadComponent` が null を返す)。

実装:

- `pkg/controllers/clusterop` — client.go (leanclient verb generics 上の
  手書き Clusters クライアント。**意図的にジェネレーターを迂回**: k8flare.com は
  自前グループで、満たすべき upstream インターフェースが無いため
  Apply/ApplyStatus と applyconfigurations 依存を生成する意味がない)、
  clusterop.go (SharedIndexInformer + ListWatch + typed workqueue。
  finalizer `k8flare.com/cluster-teardown`、doName = `<name>@<uid>`、
  Secret `k8flare-system/cluster-<name>`)、bridge.go (GATEWAY Fetcher 経由の
  `/internal/clusters/*` タイプ付きラッパー)。
- `pkg/controllers/cmd/clusterop-wasm` — ResidentService "clusterOperator"。
- `packages/k8flare-worker/src/clusters/internalapi.ts` + `teardown.ts`
  (api.ts から抽出、operator 側は await する)。registry DO に PUT
  (uid を operator が採番、uid 不一致は 409) を追加。
- Controllers DO に "clusterop" コンポーネント + `CLUSTEROP_DISABLED`、
  storage の `CONTROLLER_RELEVANT_PREFIXES` に `/registry/clusters/`
  (Secret は**入れない**)、`hasUnconvergedWork` に Cluster の 1 list を追加。

### サイズ実測 (全 5 チャンク、64MiB cap = 67,108,864 バイト)

| チャンク  | バイト     | 残 headroom |
| --------- | ---------- | ----------- |
| apiserver | 65,188,375 | 1,875KiB    |
| kcm       | 42,212,290 | 24,313KiB   |
| gc        | 41,277,283 | 25,226KiB   |
| sched     | 45,194,291 | 21,400KiB   |
| clusterop | 32,235,159 | 34,056KiB   |

clusterop は gc への同居ではなく**独立チャンク**にした (設計時の比較項目):
gc のバイナリは garbagecollector の依存グラフツリー全体を既に抱えており、
同居させると呼ばれないそのツリーが clusterop 側にもリンクされる
(pkg/controllers/restconfig の ~4MB 実測と同じ理屈)。32MB 単独なら
cap に対して十分な余裕がある。apiserver は下記の getTokens 修正で
65,193,745 → 65,188,375 バイト (-5,370)。

### 実機で見つかった 2 件 (どちらも実装前の想定に無かった)

1. **default クラスタに vault トークンを mint してはならない。** 最初の実装は
   全 Cluster に一律 `EnsureVault` を掛けたが、`clusters/tokens.ts` の
   `clusterSecrets` は **default の vault が空のときだけ** K3S_TOKEN /
   dev トークンを候補に加える。つまり default に 1 本 mint した瞬間、
   ルート資格情報が黙って無効になる — operator が自分自身の最初の
   reconcile で 401 になって発覚した (`create k8flare-system namespace:
   the server has asked for the client to provide credentials`)。
   default の資格情報は K3S_TOKEN ルートシークレットである、という
   ブートストラップ決定 (本ドキュメント上部) の当然の帰結で、
   `internalapi.ts` の `handleVault` が default には endpoint だけを返し、
   operator は Secret も tokenSecretRef も作らないようにした。
   rotate も default では 409 (明示的な非機能)。
2. **apiserver の `getTokens` が `sync.OnceValue` でトークン一覧を
   isolate 寿命の間ずっと凍結していた。** apiserver の loader id は
   `apiserver:<doName>@<sha>` でトークン指紋を含まない (controllers 側は
   `#tokenTag` を含むので回転で isolate が変わる)ため、プロビジョン済み
   クラスタのローテーション後の新トークンが**永久に 401** になる。
   ゲートウェイのドアは新トークンを通すのに内側で落ちる、という切り分けの
   しにくい形で出た。これは multi-cluster 作業時点から
   `main.go` に「KNOWN GAP … provisioned-cluster token rotation needs …
   follow-up」と自己申告されていた既知の穴で、**P2 のローテーションが
   それに最初に依存した機能**だった。60 秒 TTL キャッシュに置き換え
   (TS 側ドアの `TOKEN_CACHE_TTL_MS` と同じ時間軸)。文脈なし
   `storageDo` が global STORAGE binding にフォールバックする件は未解決の
   まま (頻度が上がるだけで性質は変わらない)。

### 検証

- `make test` (フルスイート、`rm -rf .wrangler/state` から) green
- `make test-kcm` green (36s)
- `make test-clusterop` green (70s) — 新規 `TestClusterOperatorLifecycle`:
  Cluster 作成 → phase Ready + doName `<name>@<uid>` + Secret (token/
  kubeconfig) → 発行トークンで `/c/<id>/api/v1/namespaces` 200・偽トークン
  401・`/c/<id>/version` 200 → **差分ガード検証** (Ready 到達後 45 秒の
  resourceVersion 不変を要求。無限 reconcile の回帰ゲート) → annotation で
  ローテーション → 新トークン 200 / 旧トークン 401 / annotation 消える →
  削除 → finalizer 完了・オブジェクト消滅・`/c/<id>` 404・Secret 消滅。
  併せて default Cluster が seed され doName が `default` のままであることも
  確認する。
- `make vet` / `make check` / `npx tsc --noEmit` green、`make gen` 差分なし。

### P3 への申し送り

- Admin API (`clusters/api.ts`) は温存され、legacy として並走している。
  ただし **doName の採番者は operator ただ 1 人**という規則 (#4) は、
  admin API の POST が今も registry に uid を採番させる経路を残している
  ため、まだ完全には守られていない。P3 の Admin API 削除で解消する。
- collection delete (`DELETE .../clusters`) の default 保護は P1 の
  「既知の隙間」のまま未実装。operator が入った今は実際に infra を壊せる
  ので、P3 の入口条件にする。

補足 (2026-07-27): `make test` → `make test-kcm` → `make test-clusterop` を
1 シェルで連鎖実行した際に test-clusterop が 15 分タイムアウトした事例が
1 回ある (単体・kcm→clusterop の 2 連鎖では 2/2 PASS、再現せず)。前段
スイートの wrangler 残留プロセスとの競合を疑っている。再発したら
flaky ルール (不可侵 5) に従いここを起点に掘ること。

## P3 実装結果 (2026-07-27)

Admin API を bootstrap 1 本に縮退し、クラスタ運用を完全に kubectl に寄せた。

- **`clusters/api.ts`**: 164 行 → 54 行。残したのは
  `GET /clusters/default/kubeconfig` だけ(kubeconfig を持たない管理者の
  最初の 1 回のための経路。認証は従来どおり default クラスタトークン)。
  他の `/clusters` ルートは **410 Gone + kubectl 相当の手順**を JSON で返す。
  - **実装中に判明**: この経路を旧実装のまま `readClusterTokens` にすると
    **常に 409 になる**。P2 の決定で default の vault は空のままだからで、
    `clusterSecrets`(vault → `K3S_TOKEN` → dev トークンの順)に切り替えた。
    P2 の「default に mint してはならない」の当然の帰結だが、bootstrap 経路が
    それに依存していることは実装するまで気付かなかった。
- **識別子の一本化 (レビュー指摘 #4) が完了**: admin API の POST が
  ClusterRegistry DO の uid 採番 POST の最後の呼び出し元だったので、
  両方まとめて削除。registry は operator が全量再構築できる純キャッシュに
  なり、doName の採番者は operator ただ 1 人になった。
- **collection delete の default 保護 (P1 の「既知の隙間」/ P3 入口条件)**:
  `ResourceStore.DeleteCollection` に `keepName` を追加し、handler が
  `ProtectedClusterCollectionKeep` から埋める。コレクション DELETE 自体を
  403 にすると「全テナントクラスタを消す」が不可能になるため、default だけ
  残して他は消す方式にした。
- **RBAC ロール同梱**: `k8flare:cluster-admin` (ClusterRole: clusters +
  clusters/status に全 verb) と `k8flare:cluster-secret-reader`
  (`k8flare-system` の Role: secrets get/list/watch)。upstream の
  `system:*` と同じ read-time union 方式(rbac.go の deviation #1)なので
  storage には書かれず、`kubectl get clusterroles` には出ないが roleRef で
  参照できる。secrets を cluster-wide にせず namespaced Role にしたのは、
  operator が資格情報を `k8flare-system` にしか置かないため。
  **バインドは同梱しない**(誰に与えるかは運用者の判断)。
  指摘 #10 のとおり、クラスタトークンは system:masters バイパスのままなので
  **これは今日の時点で権限分離ではない**。テストも「何を許可するか」を
  検証するもので、境界の検証ではない。docs にもその旨だけを書いた。
- **追加 (計画外)**: `kubectl get clusters` が NAME+AGE しか出さなかった
  (k8flare.com には継承できる upstream printer が無く fallback だった)。
  admin-guide が PHASE を読ませる手順になったので、PHASE/ENDPOINT の
  2 列を table.go に足し、列セットを固定するテストを付けた。

### サイズ実測

apiserver チャンク 65,202,156 バイト(P2 の 65,188,375 から +13,781 =
RBAC ロール +5,349、Cluster printer +8,432)。64MiB cap まで残り
1,906,708 バイト(1,862KiB)。ゲート通過。

### テスト

`pkg/apiserver` の常設スイート(= `make test`、opt-in ではない)に 4 本追加:

- `TestRetiredClusterManagementAPI` — bootstrap kubeconfig が 200 で
  使えるトークンを含むこと、廃止した 5 ルートが 410 + kubectl 案内を返すこと
- `TestClusterProtectedCollectionDelete` — コレクション DELETE でテナントが
  消えて default が残ること
- `TestClusterAdminBootstrapRoles` — 未バインドの derived identity が 403、
  2 ロールをバインドすると clusters の CRUD と `k8flare-system` の secrets
  読みだけが通り、default namespace の secrets・pods・secrets の delete は
  403 のままであること
- `TestClusterTableColumns` — `kubectl get clusters` の列が
  NAME/PHASE/ENDPOINT/AGE であること(admin-guide の記述と同期)

`make test` フル(クリーン state から、52s)・`make test-kcm` (35s)・
`make test-clusterop` (70s)・`make check` / `npx tsc --noEmit` /
`make gen` 差分なし、いずれも green。**`make vet` も js/wasm 側を含めて
green** — P1 で記録した「js 側 vet が失敗する」既存問題は、P1 の
Makefile 修正 (go.wasm.mod を使う 2 行への分割) で解消済みだった。

### 残件

- Admin API 廃止で `docs/multi-tenancy-and-hosting.md` の
  「Provisioning は `POST /clusters`」という記述が古くなったが、当時の記録
  なので書き換えていない(不可侵ルール 4: 訂正は消さずに足す)。参照する
  ときはこの節を優先すること。
- `adminauth.ts` の Cloudflare Access 経路は bootstrap 1 本のためだけに
  残っている。SA + RBAC 移行時に「Access で管理者を認証し SA トークンを
  発行する」形に作り替えるのが自然な次の一手。

## cluster operator の可観測化 (2026-07-28)

本番で Cluster の create/delete reconcile が間欠的に停止する問題
(別ブランチの「P3 実機検証で発見した未解決問題」節)の調査で、まず
「operator のログが tail に一切出ない」という前提そのものを実機で検証した。

### 確定: ログ配管は壊れていない。単に何も出力していなかった

Go の stdout/stderr への書き込みは、`wasm_exec.js` の `globalThis.fs`
shim (`writeSync` が改行ごとに `console.log` する)を通って dynamic worker
の console に出る。**実機確認 (wrangler dev, 2026-07-28)**: 計装後の
`cluster-operator: ...` 行と、同居する GC の klog 行 (`I0728 ...
garbagecollector.go:141] "Starting controller"`) が、いずれも同じ
wrangler 出力に出た。つまり従来 operator が無言だったのは配管の問題では
なく、**成功パスにログが 1 行も無かった**ためである。

(検証したのは `wrangler dev` の出力までで、本番 `wrangler tail` に同じ行が
出ることはまだ実機で確認していない。同居する GC の klog は本番 tail でも
観測済みなので同経路のはずだが、次の本番再現時に確認すること。)

```
controllers: clusterop load queued/starting
2026/07/28 07:31:30 clusterOperator: run starting
cluster-operator: starting (build go1.26.4/d5360aa...)
cluster-operator: waiting for informer cache sync
cluster-operator: informer WATCH established from rv=
cluster-operator: informer cache synced
cluster-operator: startup complete, entering work loop
cluster-operator: reconcile obs-test: begin (attempt 1, queue depth 1)
cluster-operator: reconcile obs-test: vault ok (doName=obs-test@1b1b1e42-... tokenId=5fc57520 superseded=0)
cluster-operator: reconcile obs-test: status written (phase=Ready observed=1)
cluster-operator: reconcile obs-test: ok in 76ms
```

### 落とし穴: `wrangler --log-level` が console 出力を落とす

wrangler のログレベルは **debug > log > info > warn > error** の順で、
Worker の `console.log` は **`log` レベル**に出る。したがって:

- `--log-level error` (`make test-clusterop` のハーネスが従来使っていた値)
  では console 出力が**一切出ない**。
- `--log-level info` でも駄目 — リクエストログ (`[wrangler:info] GET ...`)
  だけが出て console 行はゼロになる。これは「配管が壊れている」ように
  見えるので特に紛らわしい(実測 2026-07-28: 34 行すべてリクエストログ、
  `cluster-operator:` は 0 行)。
- 必要なのは `--log-level log` (= 既定値)。

`pkg/apiserver/clusterop_test.go` は `K8FLARE_DEV_LOG=<path>` を渡すと
wrangler の出力をそのファイルに保存し、同時にレベルを `log` に上げる。
既定は従来どおり無音。実測: 有効化すると `cluster-operator:` 行が 57 行
出た。

### 付随して分かったこと: 初回 LIST は起きない (watch-list)

`informer LIST ok:` の行は成功パスで一度も出ず、代わりに
`informer WATCH established from rv=` (rv 空)が出る。client-go の
WatchList (streaming list) が有効なため、reflector は初期同期を LIST
ではなく `sendInitialEvents` 付き WATCH で行っている。**「pump window が
informer の初回 LIST を殺している」という仮説を立てるなら、見るべきは
LIST ではなくこの WATCH である。** 上記の実測では WATCH 確立から
cache synced まで 250ms 弱で、その間の大量のログは同居する GC の klog
であって停止ではなかった。

### 入れた計装

- `pkg/controllers/clusterop/clusterop.go`: `logf` (prefix
  `cluster-operator: `) と `buildTag()`。起動 / informer LIST・WATCH の
  成否 / cache sync / reconcile の begin・end (経過時間・attempt・queue
  depth) / 各エラーパス (bridge は HTTP ステータス込み) / workqueue の
  再キュー回数 / status update の conflict / `Run` の return。
- `pkg/cfruntime/residentservice.go`: run の開始と、**return したこと**
  (`RUN RETURNED (controller is no longer running)`)。resident が return
  するのは外から見ると reconcile 停止と区別が付かないため。
- status condition `ReconcileError` (message は 256 文字で切る)。
  kubectl だけで停止が診断できる。実機確認: registry に uid 違いの
  エントリを先置きして 409 を強制すると、条件が載り backoff 再試行が
  ログに出た。同一エラーの再試行では status を書き直さない
  (`statusEqual` が効くので resourceVersion は増えない)。

### バグは見つかっていない

計装は入れたが、ローカルでは create/delete とも正常に収束し
(`make test-clusterop` green)、停止は再現しなかった。本番で再現した
ときに上記のログと condition で切り分ける。
