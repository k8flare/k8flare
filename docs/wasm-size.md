# WASM のサイズ — 何が 64MiB を占めているか

Worker Loader の上限は **1 dynamic worker あたり 67,108,864 バイト**で、
apiserver チャンクは現在 64,679,214 バイト、**残り 2,372 KiB** しかない。
この文書は「何を削れば効くか」を実測で示す。推測は書かない。

測定日 2026-09-12、対象は `pkg/apiserver/cmd/apiserver-wasm`。

## 測り方

出荷版は `-ldflags="-s -w"` でシンボルを落としているので、解析用に
シンボル付きで別途ビルドし、`wasm-objdump` の name section から関数ごとの
サイズを取って集計した。

```sh
GOFLAGS=-modfile=go.wasm.mod GOOS=js GOARCH=wasm \
  go build -tags leanwidth -trimpath -o /tmp/apiserver-symbols.wasm ./pkg/apiserver/cmd/apiserver-wasm
wasm-objdump -h /tmp/apiserver-symbols.wasm              # セクション内訳
wasm-objdump -j Code -x /tmp/apiserver-symbols.wasm      # 関数ごとのサイズ
```

`go tool nm` は wasm を `unrecognized object file` として拒否するので使えない。

## セクション内訳(シンボル付き 75.2 MB)

| セクション | サイズ | 割合 |
|---|---|---|
| **Code** | **42.6 MB** | 56.7% |
| **Data** | **29.8 MB** | 39.6% |
| Custom(name 等) | 2.6 MB | 3.5% |

出荷版(`-s -w` + `wasm-opt`)は 61.7 MB。Custom が落ち、Code が縮む。
**Data の 29.8 MB はほぼそのまま残る**——型記述子・文字列・インターフェース
テーブルなど、リフレクションと型情報の塊である。

## Code セクションの内訳(42.6 MB / 46,422 関数)

| owner | MB | % |
|---|---|---|
| **`k8s.io/api`(外部 API 型)** | **10.71** | **25.2** |
| Go 標準ライブラリ + ランタイム | 5.15 | 12.1 |
| **grpc / protobuf ランタイム** | **3.85** | 9.1 |
| crypto / TLS | 2.76 | 6.5 |
| **gnostic(OpenAPI モデル)** | **2.42** | 5.7 |
| `k8s.io/kubernetes` 内部型 + 変換 | 2.38 | 5.6 |
| apimachinery | 2.20 | 5.2 |
| net / http | 1.63 | 3.8 |
| `k8s.io/apiserver` | 1.10 | 2.6 |
| sigs.k8s.io | 0.73 | 1.7 |
| **k8flare の手書きコード** | **0.68** | **1.6** |

**手書きコードはバイナリの 1.6% しかない。** 削るべきは自分たちのコードでは
なく、**リンクされている upstream のうち使っていない部分**である。

単体で大きい関数(上位):

```
213,333  k8s.io/kubernetes/pkg/features.init                      <- feature gate 定義
196,139  .../rbac/bootstrappolicy.buildControllerRoles            <- 既定 ClusterRole
191,579  k8s.io/api/core/v1.init
118,822  .../rbac/bootstrappolicy.ClusterRoles
105,873  k8s.io/kubernetes/pkg/apis/core/v1.RegisterConversions
 72,093  gnostic/openapiv3.NewSchema
 69,230  k8s.io/api/core/v1.(*PodSpec).Unmarshal                  <- protobuf
```

## 効く順に並べた削減候補

### 1. protobuf のマーシャラ — **8.04 MB(Code の 18.9%)**

`k8s.io/api` の 10.71 MB のうち **75% (8.04 MB)** が
`Marshal` / `MarshalTo` / `MarshalToSizedBuffer` / `Unmarshal` / `Size` /
`Descriptor` — つまり **protobuf のための生成コード**である
(`generated.pb.go`)。

**k8flare は protobuf を一切扱っていない。** `vnd.kubernetes.protobuf` を
処理する箇所はハンドラにもゲートウェイにも存在しない。つまり現在動いている
クライアント(kubectl、client-go、k3s agent)は**すべて JSON で話している**。

したがってこの 8 MB は**今この瞬間、完全な死荷重**である。

- **やり方**: `.build/k8s-js-mirror` のミラー変換(既存の仕組み)で、
  `k8s.io/api/**/generated.pb.go` の protobuf メソッドを落とす。
  型定義(`types.go`)は残す。
- **トレードオフ**: 将来 protobuf を話すクライアントを受け入れられなくなる。
  kubelet は既定で protobuf を要求しうるが、**現在 JSON で動いている以上、
  すでにそのトレードオフは取られている**。明示的に文書化して固定する。
- **Data セクションにも効く**はず(protobuf のフィールドタグ・記述子)。未測定。

### 2. grpc ランタイム — **3.85 MB(9.1%)**

引き込み経路は一本道である:

```
k8s.io/apiserver/pkg/storage/storagebackend
  -> k8s.io/apiserver/pkg/server/egressselector
    -> google.golang.org/grpc
```

**egress selector は konnectivity プロキシ経由で etcd に繋ぐための機能**で、
k8flare は etcd を使わない(永続層は Durable Object + SQLite)。
`storagebackend` の設定構造体がフィールドとして参照しているだけで、
実行時には一度も使われない。

- **やり方**: `pkg/k8s-js-overlays/` の既存のオーバーレイ機構で
  `storagebackend` の egressselector 参照を落とす。
- **トレードオフ**: 無し(使っていない機能)。

### 3. gnostic(OpenAPI モデル)— **2.42 MB(5.7%)**

```
k8s.io/kube-openapi/pkg/util/proto -> github.com/google/gnostic-models/openapiv2, openapiv3
```

k8flare は **OpenAPI 文書を Static Assets から配信**している(CLAUDE.md の
アーキテクチャ 3.)。実行時に OpenAPI スキーマを構築する必要はない。

- **やり方**: 同じくオーバーレイで `kube-openapi/pkg/util/proto` を落とす。
- **トレードオフ**: 実行時の OpenAPI スキーマ検証ができなくなる。現在
  サーバ側の strict field validation は `pkg/apiserver/fieldvalidation.go`
  が別実装で行っているので、影響の有無は要確認。

### 4. RBAC bootstrap policy + feature gates — **合計約 0.8 MB**

`bootstrappolicy.buildControllerRoles` (196 KB) と `ClusterRoles` (119 KB) は
**kube-controller-manager 用の既定 ClusterRole 定義**、
`pkg/features.init` (213 KB) は**全 feature gate の定義表**である。
apiserver チャンクがこれらを必要としているかは未確認。

### 合計の見込み

候補 1〜3 だけで **Code から約 14.3 MB(34%)**。Data への波及を含めれば、
**61.7 MB の出荷サイズが 45 MB 前後まで落ちる可能性がある**(未検証の見積もり)。

## 検証していないこと

- Data セクション 29.8 MB の内訳。Code と同じ手法では取れない。
  型記述子とリフレクションのメタデータが主と推測しているが**未確認**。
- 上の削減候補を実際に適用したときの実測値。**この文書は測定であって、
  実装の結果ではない。**
- 候補 1 が kubelet / k3s agent の動作を壊さないこと。現在 JSON で動いている
  という観測からの推論であり、protobuf を明示的に要求された場合の挙動は
  確認していない。
