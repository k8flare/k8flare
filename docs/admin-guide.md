# 管理者ガイド (k8flare を運用する人向け)

k8flare を自分の Cloudflare アカウントにデプロイし、クラスタを発行・運用・
撤収するまでの手引きです。クラスタを「使う」側は
[user-guide.md](user-guide.md) を読んでください。

## 1. 全体像

デプロイするのは Worker 1 つ (`packages/k8flare-worker`) だけです。その中に:

- **apiserver / controller-manager / scheduler / GC** — 実物の upstream
  Kubernetes コード (WASM)。リクエストが来たときだけ動きます
- **Cluster DO / WatchHub DO** — クラスタの状態と watch 配信。データは
  ここに永続化されます
- **Static Assets** — WASM チャンクと OpenAPI 文書

アイドル時は何も動きません(実測記録: [cost-model.md](cost-model.md) 末尾)。
課金が発生するのは「リクエスト処理中の CPU 時間」「DO の読み書き」、そして
使う場合のみ「Pod 用コンテナの稼働時間(壁時計)」です。

## 2. 初回デプロイ

```sh
pnpm install
make wasm                                     # WASM チャンク生成 (~2分)
npx wrangler deploy -c packages/k8flare-worker/wrangler.jsonc
```

**初回デプロイ前に `packages/k8flare-worker/wrangler.jsonc` の
デプロイ固有値を必ず変更してください**:

| 値 | 理由 |
|---|---|
| `vars.GATEWAY_URL` | ノードが接続する公開 URL であり、operator が発行する kubeconfig に焼き込まれる origin。他人のデプロイを指したままだと**自分のノードが他人のコントロールプレーンに参加します** |
| `name` | アカウント内で確保される Worker 名 |
| `containers[].authorized_keys` | 既定は空。NodeVM に SSH デバッグしたい場合のみ自分の鍵を追加 |

シークレットは **`K3S_TOKEN` の 1 つだけ**です — default(管理)クラスタの
常時有効なルートトークンで、これを持っていることが「管理者である」ことと
同義です。公開運用する前に必ず設定してください:

```sh
npx wrangler secret put K3S_TOKEN --name k8flare < .secrets/k8flare-admin-token
```

次に、そのトークンで **ブートストラップ用の kubeconfig** を 1 回だけ取得
します。以降のクラスタ運用はすべてこの kubeconfig 経由の kubectl で行います
(HTTP API はこの 1 本だけが残っています):

```sh
B=https://<your-worker>.workers.dev
AT=$(cat .secrets/k8flare-admin-token)
curl -s -H "Authorization: Bearer $AT" $B/clusters/default/kubeconfig > default.yaml
export KUBECONFIG=$PWD/default.yaml
kubectl get nodes
```

`K3S_TOKEN` 未設定のデプロイは**開発ポスチャ**(既定トークン
`k8flare-dev-token` を受理)です。default クラスタのトークンは
`K3S_TOKEN` そのものなので、ローテーションは
`wrangler secret put K3S_TOKEN` で行います(後述の annotate による
ローテーションは発行済みクラスタ専用です。default に annotate した場合、
operator が annotation を取り除き RotateUnsupported condition を記録します
(default のルートトークンは K3S_TOKEN シークレットの差し替えで更新))。

このブートストラップ経路は Cloudflare Access でも追加保護できます
(`ACCESS_TEAM_DOMAIN` + `ACCESS_AUD` を設定。実装:
`packages/k8flare-worker/src/clusters/adminauth.ts`)。

### コスト上の注意 (重要)

- `wrangler.jsonc` の **`containers` セクションは Pod-on-Containers
  (NodeVM) 用**で、コンテナアプリを作成すると**壁時計課金**が発生し得ます。
  使わないならこのセクションを除いた構成でデプロイしてください。
  稼働状況は `wrangler containers list` で確認、不要なアプリは
  `wrangler containers delete <ID>` で削除できます。
- deploy / secret 操作は実アカウントに影響します。検証は基本
  `make dev`(ローカル) で行い、本番デプロイは意図したときだけ。

## 3. クラスタの発行と管理 (kubectl)

クラスタは **`k8flare.com/v1alpha1` の `Cluster` オブジェクト**です。発行も
削除もローテーションも kubectl で行い、default クラスタ上の
cluster-operator が reconcile します(旧 `/clusters` 管理 API は廃止済み
— 叩くと 410 Gone と kubectl 相当の手順が返ります)。

「default」クラスタは発行不要(デプロイした瞬間から存在)で、`Cluster`
オブジェクトとしても自動で seed されますが、削除はできません
(単体 DELETE は 403、コレクション DELETE でも 1 つだけ残ります)。

### 発行

```yaml
# cluster.yaml
apiVersion: k8flare.com/v1alpha1
kind: Cluster
metadata:
  name: team-a          # = 公開 URL の /c/team-a
spec:
  displayName: "Team A"
```

```sh
kubectl apply -f cluster.yaml
kubectl get clusters
# NAME      PHASE   ENDPOINT                                       AGE
# default   Ready   https://<your-worker>.workers.dev              3d
# team-a    Ready   https://<your-worker>.workers.dev/c/team-a     5s
```

`PHASE` が `Ready` になると、operator が資格情報を **Secret** として
`k8flare-system` に発行します。利用者に渡すのは `kubeconfig` キーです:

```sh
kubectl get secret cluster-team-a -n k8flare-system \
  -o jsonpath='{.data.kubeconfig}' | base64 -d > team-a.yaml
```

Secret には `token`(生のトークン)と `kubeconfig`(そのトークン入りの
kubeconfig)が入っています。

### トークンのローテーション

annotation を書き換えるだけです。operator が新トークンを発行し、Secret を
書き換え、旧トークンを失効させ、annotation を消します:

```sh
kubectl annotate cluster team-a k8flare.com/rotate-token="$(date +%s)" --overwrite
```

同じ値を再度書いても再ローテーションはしません(値からトークン ID を
決定的に導くため、途中で失敗した際の再実行が安全)。ゲートウェイでの検証は
アイソレートごとに約 60 秒キャッシュされるため、旧トークンの失効伝播には
最大 1 分かかります。緊急失効はクラスタ削除です。

default クラスタのローテーションはこの経路では**できません**
(annotation は消えて `RotateUnsupported` condition が付きます)。default の
資格情報は `K3S_TOKEN` シークレットそのものなので、
`wrangler secret put K3S_TOKEN` で入れ替えます。

### 削除

```sh
kubectl delete cluster team-a
```

finalizer (`k8flare.com/cluster-teardown`) が付いているので、オブジェクトは
すぐには消えず `Terminating` のまま残ります。operator が NodeVM の破棄 →
Controllers/WatchHub/Cluster DO の破棄 → 解決キャッシュと Secret の削除まで
完了して初めて finalizer が外れ、オブジェクトが消えます。途中で失敗した
場合は次の reconcile が同じ手順を再開します(各ステップは冪等)。消えるまで
待つには:

```sh
kubectl wait --for=delete cluster/team-a --timeout=5m
```

### 権限

現状、**default クラスタのトークン(= `K3S_TOKEN`)を持っていることが
管理者であること**と同義です。このトークンは `system:masters` として認証
されるため RBAC を素通りします。

より細かく絞りたい場合のために、バンドル済みのロールが 2 つあります
(ServiceAccount + TokenRequest は実装済みなので、SA アカウントを作れば
RBAC で絞れます):

| ロール | 種別 | 権限 |
|---|---|---|
| `k8flare:cluster-admin` | ClusterRole | `k8flare.com` の `clusters` / `clusters/status` に全 verb |
| `k8flare:cluster-secret-reader` | Role (`k8flare-system`) | `secrets` の get/list/watch |

これらは upstream の `system:*` ロールと同じくブートストラップ扱いなので
`kubectl get clusterroles` には出てきませんが、`roleRef` で参照できます。
**注意**: クラスタトークンを配ったままでは RBAC は意味を持ちません
(上記のバイパスが効くため)。RBAC で分離するなら SA ベースの管理者
アカウントに移行してください。

## 4. ノードの提供 (BYO VM)

Linux VM に agent バイナリを置いて起動するだけです:

```sh
make nodes-agent    # packages/k8flare-worker/images/node/k8flare-agent (linux/amd64)
./k8flare-agent --server https://<your-worker>.workers.dev --token <クラスタのトークン>
```

クロスネットワークのノード間通信は Cloudflare Mesh か flannel
wireguard を使います([cloudflare-mesh-networking.md](cloudflare-mesh-networking.md))。

## 5. 日常運用

- **観測**: `npx wrangler tail k8flare` でライブログ。observability は
  有効化済み(サンプリング 100%)なのでダッシュボードでも追えます。
- **アイドル確認**: **オブジェクトが何も無い**クラスタは tail に何も
  流れないのが正常です。alarm が定期的に出続けていたらバグなので報告して
  ください(判定基準は cost-model.md の実測記録)。
  ただし「未収束の仕事があるクラスタ」は別です。例えばノードが無いのに
  Deployment があると、コントローラーは収束を試み続けるためアラームは
  鳴り続けます。ここでは指数バックオフが効き、間隔は
  15→30→60→120→240 秒と 10 分上限に向かって伸びていきます
  (2026-07-30 実測、docs/platform-verification.md の「S26 訂正」)。
  ワークロードを削除するとパークしますが、その時点でバックオフが
  伸びているため即座ではありません — パークの判定は次のアラーム発火時に
  行われるので、実測では t+180 秒時点ではまだ武装しており、約 t+300 秒で
  パークしました。数分かかるのは正常です。
- **データの全消去**(クラスタは残す): migration の
  delete+recreate サイクルで行います(手順と前例: `wrangler.jsonc` の
  migrations コメントと git 履歴の v3/v4)。
- **完全撤収**: `npx wrangler delete --name k8flare`。DO データも消えます。

## 6. アップグレード

```sh
git pull
make wasm && make test && make test-kcm   # ローカル検証
npx wrangler deploy -c packages/k8flare-worker/wrangler.jsonc
```

デプロイは無停止です(進行中のリクエストは旧バージョンで完走)。k8s
バージョン自体の更新は [k8s-version-bump.md](k8s-version-bump.md) 参照。

## 7. トラブルシューティング

| 症状 | まず見るもの |
|---|---|
| kubectl が 401 | トークンが正しいか。トークンのローテーション直後は伝播に最大 1 分 |
| Deployment を作っても Pod が生えない | `wrangler tail` で controllers のロードログ。書き込みが 1 件でもあれば KCM が起きます |
| Pod が Pending のまま | ノードが居るか (`kubectl get nodes`)。ノードなしなら正常な Pending です |
| 削除したはずのオブジェクトが残る | GC は非同期(実 k8s と同じ eventually-consistent)。数十秒待つ |
| ローカル dev が変な状態 | `rm -rf .wrangler/state` してやり直し(CLAUDE.md の落とし穴集参照) |
