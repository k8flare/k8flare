# component-base の GOOS=js オーバーレイ

`.build/component-base-mirror` を作り、`tracing/utils.go` だけを差し替える。
`go.wasm.mod` の `k8s.io/component-base` はこのミラーを指す。

## なぜ必要か

`k8s.io/apiserver/pkg/storage/cacher` は `tracing.Start` と
`tracing.SpanFromContext` を使う。どちらも `tracing/tracing.go` にあり、
エクスポータとは無関係である。

ところが同じパッケージの `tracing/utils.go` が
`go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc` を
**パッケージスコープで import している**。Go はパッケージ単位でリンクするので、
`tracing` を import した時点で OTLP エクスポータ一式、そして
`google.golang.org/grpc` と protobuf ランタイムが入る。

k8flare はトレースを一切設定しない。`utils.go` が提供する関数の呼び出し元
(`pkg/server/options`、`pkg/server`、`pkg/util/webhook`、
`pkg/endpoints/filters`)は**どれもリンクグラフに無い**——`go list -deps` で
確認済み。

## 効果(実測)

apiserver チャンク:

```
64,220,107 バイト  ->  43,895,613 バイト   (-20,324,494、-32%)
Loader cap への余裕: 2,821 KiB -> 22,669 KiB
```

リンクされるパッケージ数: grpc 63 -> 9、otlptrace 10 -> 0。

見積もり(grpc 1.37MB + protobuf 2.44MB + otel 0.66MB = 4.48MB)を大きく
上回った。OTLP エクスポータを外すと、その推移的依存——protobuf のリフレクション
機構、grpc-gateway、otel SDK——がまとめて落ちるためである。**リンクグラフの
推論は削減量を予測できない。実測すること。**

## 差し替えた内容

`tracing_utils.go` は `TracerProvider` インターフェース、
`NewNoopTracerProvider`、`Propagators` だけを残す。前者二つはリンク済みコードが
名前で参照する(`pkg/server/config.go`)。

`NewProvider` と `WrapperFor` は**意図的に置いていない**。実エクスポータを
求める呼び出し元が現れたら、黙って no-op を掴むのではなくコンパイルエラーに
なるべきだからである。

## upstream 更新時

`upstream-tracing-utils.go.sha256` が upstream の `tracing/utils.go` を
ピン留めしている。k8s のバージョンを上げて中身が変わると
`gen-component-base-mirror.ts` が停止する。`tracing_utils.go` を新しい
upstream と突き合わせてからピンを更新すること。
