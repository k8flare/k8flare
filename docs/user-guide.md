# 利用者ガイド (k8flare クラスタを使う人向け)

管理者からクラスタを渡された人のためのガイドです。結論から言うと、
**普通の Kubernetes クラスタとして kubectl で使えます**。中身が
Cloudflare Workers で動いていることは、いくつかの制限を除けば意識する
必要はありません。

## 1. 接続する

管理者から kubeconfig(YAML)を受け取り:

```sh
export KUBECONFIG=~/k8flare.yaml
kubectl get ns
```

自分で作る場合はサーバー URL とトークンだけで足ります:

```yaml
apiVersion: v1
kind: Config
clusters:
- name: k8flare
  cluster: { server: "https://<worker>.workers.dev" }   # 共有クラスタなら /c/<id> 付き
contexts:
- name: k8flare
  context: { cluster: k8flare, user: me }
current-context: k8flare
users:
- name: me
  user: { token: "<渡されたトークン>" }
```

トークンは admin 相当(RBAC をバイパス)です。取り扱いはクラスタの
root 権限と同じと考えてください。

## 2. 普通に使えるもの

- `kubectl get/describe/apply/delete/edit/patch` — 主要リソース一式
  (Pod, Deployment, ReplicaSet, StatefulSet, DaemonSet, Job, CronJob,
  Service, ConfigMap, Secret, Namespace, ServiceAccount ほか全 41 種類)
- `kubectl get -w`(watch)、`kubectl logs` / `exec`(ノード側の対応が前提。
  `port-forward` は未対応 → 制限の表を参照)
- Deployment のローリング更新、Job/CronJob、RBAC、ServiceAccount トークン
- Service (ClusterIP) とクラスタ DNS、EndpointSlice
- `kubectl apply` のクライアント側検証(OpenAPI 配信済み。
  `--validate=false` は不要)

裏では本物の kube-controller-manager / kube-scheduler / GC が動いている
ので、コントローラーの挙動(ReplicaSet の作られ方、observedGeneration、
カスケード削除など)は普通の k8s と同じです。

## 3. 知っておくべき違い・制限

| 挙動 | 説明 |
|---|---|
| **最初の 1 リクエストが遅いことがある** | 完全アイドルから起きるとき、コントロールプレーンのロードに数秒〜数十秒かかることがあります。以降は速い(ミリ秒台)です。`kubectl create` 直後に Pod が出るまで数十秒待つのは正常 |
| **削除は eventually-consistent** | `kubectl delete` のカスケード(依存オブジェクトの削除)は非同期 GC が行います。実 k8s と同じですが、数十秒残って見えることがあります |
| **PVC は Pending のまま** | 動的プロビジョナー未実装。PV/PVC/StorageClass のオブジェクト自体は作れますが、ボリュームは供給されません |
| **`kubectl port-forward` は未対応** | 501 が返ります。`kubectl exec` / `logs` は使えます |
| **PDB / HPA / NetworkPolicy は「作れるが効かない」** | オブジェクトの CRUD はできますが、それを実行するコントローラー/エンフォーサが未稼働です |
| **Pod はノードがないと Pending** | ノード(Linux VM)の追加は管理者に依頼してください |
| **admission webhook / CRD は未対応** | MutatingWebhook・ValidatingWebhook・CustomResourceDefinition は使えません |
| **RBAC はトークンでバイパスされる** | 渡されたクラスタトークンは system:masters 相当。個別ユーザー/権限分離が必要なら管理者に相談 |

## 4. よくある「壊れた?」と答え

- **`kubectl get pods` が空 / Deployment だけある** → コントローラーが
  起きる前です。数十秒待つか、もう 1 回何か書き込む(annotate 等)と
  すぐ反応します。
- **Pod が Pending** → `kubectl get nodes` でノードがいるか確認。
  いなければそれが理由です(スケジューラは正常で、置き場所がないだけ)。
- **消したのに events が見える / リソースが一瞬残る** → 非同期 GC の
  仕様です。1 分待って残っていたら管理者へ。
- **接続が 401** → トークンのローテーション直後は反映に最大 1 分
  かかります。それ以降も 401 なら新しい kubeconfig を管理者からもらって
  ください。

## 5. お作法

- このクラスタは書き込みがなければ課金がほぼゼロになる設計です。
  **秒単位のポーリングをする独自ツール**(watch でなく list を高頻度で
  叩くもの)はコストに直結するので避け、`kubectl get -w` / informer を
  使ってください。
- 大量オブジェクトのバッチ投入は普通にできますが、1 つの巨大クラスタ
  より用途ごとにクラスタを分ける方が向いています(発行は管理者が
  数秒でできます)。
