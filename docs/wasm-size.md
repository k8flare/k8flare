# WASM のサイズ — 何が 64MiB を占めているか

Worker Loader の上限は **1 dynamic worker あたり 67,108,864 バイト**で、
apiserver チャンクは測定時点で 64,679,214 バイト、**残り 2,372 KiB** しか
なかった。この文書は「何を削れば効くか」を実測で示す。推測は書かない。

測定日 2026-09-12、対象は `pkg/apiserver/cmd/apiserver-wasm`。

> **追記 (2026-09-13): 冒頭の 64,679,214 バイトはもう現在値ではない。**
> 本文が記録した 2 つの削減(egressselector で -459,107 バイト、
> `component-base/tracing` の OTLP エクスポータで -20.3MB)が入ったあと、
> それ以降の変更が約 4.9MB を足し戻している。**内訳は未計測** — real API
> installer そのものの実測値は +1,033,606 バイト(44,929,219、
> docs/real-apiserver-plan.md)なので、残る約 3.9MB は
> それ以降のマージであり、installer に帰することはできない。
> 現在値は `make wasm` が印字する値(2026-09-13 のローカルビルド)で、
> **48,784,426 バイト・cap まで 18,324,438 バイト(17,895 KiB)**。
> `packages/k8flare-worker/assets/wasm/` は .gitignore にあるので、
> マニフェストは commit されておらず、参照先はビルド出力である。推移は
> 64,679,214 → 64,220,107(egressselector)→ 43,895,613(OTLP)→
> 44,929,219(installer)→ **48,784,426**(以降のマージ、未分解)。
> 以下の本文は測定当時の数字のまま残す
> (不可侵ルール 4: 訂正は追記し、歴史を書き換えない)。

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

### 実験して分かったこと(先に読むこと)

以下の候補 1〜3 は**リンクグラフからの推論**で書いた。**実際に測ったら、
1 は成立しなかった。** 順序も入れ替わる。測定は
「候補を実装する前に、小さなプローブで効果を確かめる」形で行った。

#### 実験 A: protobuf シリアライザを外せば marshaler が落ちるか → **落ちない**

`k8s.io/api` の marshaler が到達可能になっている根は
`apimachinery/pkg/runtime/serializer`(CodecFactory)が protobuf
シリアライザを import していることだと考えた。同じスキーマを、JSON
シリアライザだけ使う版と CodecFactory を使う版で作って比べた:

```
probe-json : 15.83 MB
probe-pb   : 15.83 MB     <- 完全に同一
```

**差が無い。** 保持している主体はシリアライザではない。

#### 実験 B: 何が型を保持しているか → **スキーマへの登録そのもの**

```
runtime.NewScheme() だけ              8.01 MB
 + corev1.Pod / PodList を登録        14.77 MB   (+6.76 MB)
 + client-go の全グループ            15.83 MB   (+1.06 MB)
```

**Pod を 1 種類登録するだけで 6.76 MB。** そこから先、他の全 API グループを
足しても **1.06 MB しか増えない。**

つまり:

- **API グループを絞る戦略は効かない**(全部足しても 1 MB)
- Pod の型グラフ(PodSpec → Container → VolumeSource → 各ボリューム
  プラグイン型…)が単体で 6.76 MB を持ち込む。**Kubernetes の制御プレーンで
  ある以上、これは避けられない**
- スキーマ登録は `reflect` 経由で全メソッドを保持させる。protobuf の
  marshaler もそれで残る。**シリアライザを外しても消えない**

**Go/WASM で Kubernetes の制御プレーンを書く限り、下限は約 15.8 MB** である。
apiserver チャンクの 64.2 MB のうち、残る約 48 MB が削減の対象になる。

#### 実験 C: egressselector の削除 → **459 KB 削減(実測)**

候補 2 として挙げた経路のうち、`storagebackend` の未使用フィールド
`EgressLookup` を落とした。

```
64,679,214 バイト  ->  64,220,107 バイト   (-459,107、余裕 2,372 -> 2,821 KiB)
```

ただし **grpc 自体は残った**(63 パッケージ)。経路が複数あったためである:

```
k8s.io/apiserver/pkg/storage/value        -> grpc/codes, grpc/status
k8s.io/apiserver/pkg/storage/cacher/progress -> grpc/metadata
k8s.io/component-base/tracing             -> otlptracegrpc -> grpc 本体一式
```

シンボル版での実測: grpc 本体 1.37 MB / protobuf ランタイム 2.44 MB /
opentelemetry 0.66 MB = **計 4.48 MB**。最大の経路は
`k8s.io/apiserver/pkg/storage/cacher` が `component-base/tracing` を
import していることで、k8flare はトレースを一切設定していない。

### ~~1. protobuf のマーシャラ~~ — **実験 A/B により却下**

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

### 合計の見込み(実験後に改訂)

~~候補 1〜3 だけで Code から約 14.3 MB~~ — **却下**。実験 A/B により、
protobuf marshaler の 8.04 MB は**シリアライザを外しても落ちない**
(スキーマ登録が reflect 経由で保持する)。

実際に取れる見込み:

| 候補 | 見込み | 状態 |
|---|---|---|
| egressselector の未使用フィールド | **459 KB** | **実施済み・実測** |
| **`component-base/tracing` → OTLP gRPC exporter** | **20.3 MB(-32%)** | **実施済み・実測** |
| gnostic(OpenAPI モデル) | ~2.4 MB | 未着手 |
| crypto / TLS 2.76 MB | 要調査(何に使っているか未確認) | 未着手 |

**API 型そのものは削れない。** 削れるのは「使っていない周辺機能」だけ——
**ただしその見積もりは外れた。**

`component-base/tracing` の OTLP エクスポータを外したところ、見積もり 3 MB に
対して **20.3 MB(-32%)** 減った。apiserver チャンクは 64,220,107 →
**43,895,613 バイト**、Loader cap への余裕は 2,821 KiB → **22,669 KiB**。

理由は推移的依存である。OTLP エクスポータを外すと、grpc 本体だけでなく
**protobuf のリフレクション機構・grpc-gateway・otel SDK** がまとめて落ちる。
リンクされるパッケージ数は grpc 63 → 9、otlptrace 10 → 0。

**教訓: リンクグラフからの推論は削減量を予測できない。** シンボル版での
関数サイズ合計(4.48 MB)も、実際の削減(20.3 MB)を 4 分の 1 に見誤った。
候補を挙げるのに推論は使えるが、**効果は必ず実測する**。

## 検証していないこと

- Data セクション 29.8 MB の内訳。Code と同じ手法では取れない。
  型記述子とリフレクションのメタデータが主と推測しているが**未確認**。
- 上の削減候補を実際に適用したときの実測値。**この文書は測定であって、
  実装の結果ではない。**
- 候補 1 が kubelet / k3s agent の動作を壊さないこと。現在 JSON で動いている
  という観測からの推論であり、protobuf を明示的に要求された場合の挙動は
  確認していない。
