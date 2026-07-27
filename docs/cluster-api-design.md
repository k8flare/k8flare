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

reconcile:

1. **作成** (phase 空 → Ready):
   doName = `<name>@<uid>` を採番 → テナント vault 初期化 + 初回
   トークン mint → CA は初回リクエスト時生成 (現状どおり) →
   Secret (token/kubeconfig) を default クラスタに書く →
   status.phase=Ready, tokenSecretRef, endpoint を更新 →
   解決キャッシュ (下記) を更新。
2. **削除** (deletionTimestamp あり):
   phase=Terminating → 既存 teardownCluster 相当
   (Scheduler destroy → Controllers → WatchHub → Cluster DO) →
   Secret 削除 → finalizer 除去 (ここで実際に消える)。
   途中失敗は次の reconcile が再開 (冪等) — 既存の graceful deletion
   と実 GC の設計にそのまま乗る。
3. **ローテーション annotation**: 上記。

## ゲートウェイの /c/<id> 解決

現 ClusterRegistry DO は「controller が維持する解決キャッシュ」に降格
して温存する (リクエスト毎に default クラスタの storage を読むより
安い O(1) 索引 + 既存の 60 秒 isolate キャッシュ)。書き込み手は
Admin API から controller に変わるだけで、gateway 側は無変更。

## 認証・RBAC・ブートストラップ

- 管理操作の認可は **default クラスタの RBAC** に移る。クラスタ管理
  だけ許す ClusterRole (`clusters.k8flare.com` の CRUD +
  `k8flare-system` namespace の Secret read) を同梱し、管理者ごとの
  権限分離を可能にする (現 ADMIN_TOKENS の全権共有より改善)。
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
  image: ghcr.io/k2wanko/k8flare-clusterop:v1@sha256:...  # digest ピン推奨
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
2. **検証**: blob の sha256 を manifest と照合。64MiB (Loader cap) 超は
   その場で Failed。
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
  OCI 配布で動かす (dogfooding)。
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
