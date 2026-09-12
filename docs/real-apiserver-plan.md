# 実物の apiserver を patch で動かす

CLAUDE.md の不可侵ルール #3「upstream の実物を再実装より優先する」に対して、
現状どこが実物でどこが手書きかを整理し、手書きを実物に寄せる道筋を示す。

目標の形:

- **ロジックは Go 側の upstream 実物**。差異は `pkg/k8s-js-overlays/` の
  パッチで吸収する
- **TypeScript はストレージ層を Worker のプリミティブに繋ぎ直す stub だけ**。
  ロジックを持たない

## 現状(2026-09-12 実測)

手書き: Go 20,873 行 / TypeScript 7,665 行。うち `pkg/apiserver` が 8,962 行で
Go の 43%。

| 層 | 現状 | 実物か |
|---|---|---|
| ストレージ(REST の CRUD 実体) | `k8s.io/apiserver/pkg/registry/generic/registry.Store` を Durable Object の上でそのまま使う(S25) | **実物** |
| ストレージバックエンド | `pkg/apiserver/upstreamstorage.go` が `storage.Interface` を DO 向けに実装 | 繋ぎ込み(あるべき姿) |
| **HTTP ルーティング / verb dispatch** | **`pkg/apiserver/handler.go` 806 行 + `apidef/table.go` 693 行の手書き** | **再実装** |
| Table 変換(kubectl の列) | `pkg/apiserver/table.go` 399 行 | 再実装 |
| subresource | `pkg/apiserver/subresource.go` 571 行 | 再実装 |
| 認証 / 認可 | `pkg/apiserver/auth.go` / `rbac.go` | 再実装 |
| 証明書 | `pkg/apiserver/certmanager.go` 467 行 | 再実装 |
| graceful delete のガード 4 つ | `pkg/apiserver/gracefuldelete.go` 481 行 | **upstream に無い独自 admission** |

## 実物の API installer は GOOS=js でビルドできる(実測)

`k8s.io/apiserver/pkg/endpoints` の `APIGroupVersion.InstallREST` は、
`genericregistry.Store` を受け取って go-restful のコンテナにルートを張る。
これが使えれば `handler.go` / `apidef/table.go` / `table.go` /
`subresource.go` の大半が不要になる。

**ビルドできる。必要なパッチは 1 件だけだった。**

```
probe: k8s.io/apiserver/pkg/endpoints を import するだけの main
結果  : ビルド成功 22.67 MB
```

唯一の障害は `k8s.io/apiserver/pkg/storageversion` が
`clientset.InternalV1alpha1()` を呼ぶことだった。lean clientset は
`leanwidth` タグでそのグループを刈っている。**この機能(StorageVersion API
への公開)は、apiserver が 1 台で移行コントローラーも居ない k8flare には
そもそも意味が無い**ので、publish 部分だけを no-op にするオーバーレイを
置いた(`pkg/k8s-js-overlays/apiserver/storageversion_manager.go`)。

サイズの余地もある。`component-base/tracing` から OTLP エクスポータを外した
ことで apiserver チャンクは 64.2 MB → **43.9 MB** になり、Loader cap への
余裕は **22,669 KiB**(docs/wasm-size.md)。installer 単体が 22.67 MB なので、
既に入っている分と重複する部分を考えれば収まる見込みが立つ。

## 段階

各段階は独立に出荷でき、前段の成果を壊さない。

### 段階 1: API installer を実物にする

`endpoints.APIGroupVersion` を、既に実物の `genericregistry.Store` 群に対して
組み立て、`restful.Container` を `pkg/cfruntime` の fetch ハンドラから
`ServeHTTP` で叩く。Worker には listener が無いが、**installer が返すのは
`http.Handler` なので listener は要らない**。

消えるはずのもの: `handler.go` のルーティング、`apidef/table.go`、
`table.go`、`subresource.go` の大半。

確かめること:
- go-restful がどれだけ増やすか(未計測)
- content negotiation が JSON だけで閉じるか
- watch のストリーミングが pump window と両立するか(既存の watch 実装は
  シェル Worker 側にある。ここは stub のままで良いか要判断)

### 段階 2: 認証・認可を実物にする

`k8s.io/apiserver/pkg/authentication` / `authorization` の実物を使い、
`auth.go` / `rbac.go` を置き換える。RBAC は
`plugin/pkg/auth/authorizer/rbac` が既にリンクされている。

このとき Codex レビューの P0(node join の秘密と管理者トークンが同一)も
同時に解消できる可能性がある——upstream の認証器は複数の資格情報源を
前提にしているため。

### 段階 3: TypeScript を stub に落とす

現在の TS 7,665 行のうち、ロジックを持つものを Go 側へ移すか削る。

| 現在 | 行数 | 目標 |
|---|---|---|
| `storage/` | 1,760 | **残す**(DO の kine 相当。これが「繋ぎ直す stub」の本体) |
| `gateway/` | 966 | 認証は段階 2 で Go へ。ルーティングは段階 1 で Go へ。残るのは DO への振り分けだけ |
| `nodes/` | 1,049 | Containers / NodeVM の操作。Loader から DO を触れない制約(S2)があるため stub として残る |
| `clusters/` | 858 | マルチテナント。Cluster CRD の reconcile は既に Go(clusterop)。TS 側は DO 操作のブリッジだけに絞れるはず |
| `controllers/` | 682 | dynamic worker のロードと poke。プラットフォーム操作なので stub |
| `k8s/` | 706 | watch のファンアウト。段階 1 の結論次第 |
| `loader/` | 266 | チャンク組み立て。stub |

### 段階 4: 独自 admission の撤去

`gracefuldelete.go` の 4 ガードは、pump window を跨ぐ controller の不連続を
補償している。P0-4 の Stage 1 が入るまで外せない
(`docs/pump-window-design.md`、TODO P2-1)。段階 1〜3 とは独立。

## やらないこと

**1 から作り直さない。** 実測した制約は作り直しても消えない:

- Worker Loader の 64MiB 上限
- `k8s.io/kubernetes/pkg/scheduler` が `syscall.SIGUSR2` に無条件依存し
  GOOS=js でビルドできない
- Workers にファイルシステムもプロセスも無く、k3s の実バイナリは起動できない
- Go + k8s の型グラフの下限は約 15.8 MB(Pod を 1 種類登録するだけで
  6.76 MB、全 API グループを足しても +1.06 MB。docs/wasm-size.md 実験 B)

作り直して減るのは手書きコード 0.68 MB——**バイナリの 1.6%** である。
減らすべきは手書きコードの「量」ではなく「役割」で、それはこの段階計画で
実物に置き換えることで達成される。
