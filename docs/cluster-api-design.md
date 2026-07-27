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

## 実装フェーズ (未着手)

- P1: apidef に k8flare.com/v1alpha1 Cluster を追加 (types/gen/table、
  make gen)。kubectl で CRUD できるだけの状態。サイズ実測。
- P2: cluster-operator (作成/削除/ローテーション reconcile) + 解決
  キャッシュ書き込み。suite に Cluster ライフサイクルの統合テスト追加。
- P3: RBAC ロール同梱 + bootstrap 縮退、Admin API 削除、docs 更新。

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
