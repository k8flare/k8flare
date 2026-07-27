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
ローテーションは発行済みクラスタ専用で、default では 409 になります)。

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

## 3. クラスタの発行と管理 (管理 API)

`/clusters` 配下。認証は `Authorization: Bearer <default クラスタの有効トークン>`(= K3S_TOKEN または発行済みトークン)。
「default」クラスタは発行不要(デプロイした瞬間から存在)で、作成/削除は
できませんが、トークン発行・kubeconfig 取得は他のクラスタと同じに使えます。

| 操作 | エンドポイント | 返り値 |
|---|---|---|
| クラスタ作成 | `POST /clusters` body `{"id":"team-a"}` | `{id, token, tokenId, kubeconfig}` |
| 一覧 | `GET /clusters` | `{items:[...]}` |
| 詳細 | `GET /clusters/<id>` | レコード |
| kubeconfig 取得 | `GET /clusters/<id>/kubeconfig` | YAML(利用者に渡すのはこれ) |
| トークン追加(ローテーション) | `POST /clusters/<id>/tokens` | `{tokenId, token}` |
| トークン削除 | `DELETE /clusters/<id>/tokens/<tokenId>` | 204(最後の 1 本は拒否) |
| クラスタ削除 | `DELETE /clusters/<id>` | 202(途中失敗時の再実行は継続。完了後の再実行は 404) |

例:

```sh
B=https://<your-worker>.workers.dev
curl -s -X POST -H "Authorization: Bearer $AT" $B/clusters -d '{"id":"team-a"}'
curl -s -H "Authorization: Bearer $AT" $B/clusters/team-a/kubeconfig > team-a.yaml
```

トークンのローテーション手順: 新トークンを POST → 利用者に配布 → 旧
トークンを DELETE。ゲートウェイでの検証はアイソレートごとに約 60 秒キャッシュされるため、
失効の伝播に最大 1 分かかります(Go 層の防御的キャッシュは isolate 再生成
まで残ることがありますが、門はゲートウェイ側です)。緊急失効はクラスタ削除です。

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
- **アイドル確認**: 誰も使っていないクラスタは tail に何も流れないのが
  正常です。alarm が定期的に出続けていたらバグなので報告してください
  (判定基準は cost-model.md の実測記録)。
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
