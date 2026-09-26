# CLAUDE.md

A Kubernetes control plane compiled to `GOOS=js GOARCH=wasm` and run as
Cloudflare Workers. Merge with the global guidelines.

## 正典を先に読む

既存のプロトコル・API・定数を扱うときは、**推測や提案で名前を決めない**。

このリポジトリは upstream Kubernetes を `.build/*-mirror` に展開し、
`go.mod` で置き換えている。**正典はローカルにある。**

- 定数名・サブプロトコル名・ヘッダ名・パス・エラー文字列は上流のソースで確認する。
  もっともらしい名前は実在しない名前であることが多い。
  実例: `base64.k8s.io` は存在しない。正しくは `base64.binary.k8s.io`
  (`apimachinery/pkg/util/httpstream/wsstream/stream.go`)。
- 探す順序: (1) `.build/*-mirror` と `go env GOMODCACHE` のソース、
  (2) 上流の GitHub、(3) ライブラリ。自前で書く前に、同じことをしている実装が
  このリポジトリ内に既にないか見る (pod ログの HTTP 経路は
  `apiserver-core/podlog.go` に最初からあった)。
- **通すべき conformance spec があるなら、その spec 本体を読む。**
  `.build/kubernetes-mirror/test/e2e/` にある。期待される入出力と比較方法が
  書いてあり、それが仕様。失敗メッセージはその要約にすぎない。
- サブエージェントや他の AI の提案は**情報源ではなく仮説**。実装前に正典で裏を取る。

## ビルドと検証

- **ホストの `go test` / `go build` は `CGO_ENABLED=0` が必要。** この Mac の
  SDK の `.tbd` が壊れていて cgo リンクが通らない。リポジトリの問題ではない。
- **`make wasm` は `wrangler dev` を殺す。** ウォッチャが書き換え中のチャンクを
  stat して ENOENT で落ち、誰も再起動しない。ノードの lease 書き込みが全部失敗し、
  クラスタが無言で死ぬ。**再ビルド前に dev サーバを止める。**
- **コードを直したら `make wasm` するまでランタイムには届かない。** アセットが
  ソースより数時間古い状態は起こりうるし、実際に起きた。診断ログを足したのに
  ビルドし忘れて1回分の計測を捨てたことがある。
- `make sizes` でワーカーのサイズと**リンクされた関数数**を出す。上限は関数数の
  予算 (約45,000、1関数あたり1.25〜1.65KB) と考えるとよい。
- 計測は**再現手段そのものを先に検証する**。`kubectl` が書けない状態で
  「名前空間に ServiceAccount が作られない」と誤診したことがある。
  自分が作った負荷の残骸を測定対象に混ぜたこともある。

## 制約

- **Worker のコードサイズ上限 64 MiB はランタイムが厳密に強制する。**
  超えると `Dynamic Worker code size (N bytes) exceeds the maximum allowed size`
  でロードされず、キューのメッセージは dead letter に落ちる。機能が
  「静かに存在しない」状態になるので、まずサイズを疑う。
- `wasm-opt` は Code だけを書き換える。Data は全バイナリの 45〜53% を占め、
  最適化しても**バイト単位で不変**。
- クライアントは **ContentType をネゴシエートしない。** `rest.Config` で
  明示しないと protobuf ボディを送る。サーバが protobuf を落とすと kubectl の
  書き込みが全滅する (読み取りは negotiate して動き続けるので気づきにくい)。

## 詳細

- `plans/remaining.md` — 残作業、既知の穴、**再試行すべきでない手**とその理由
- `plans/podlogs-websocket.md` — pod ログ websocket の引き継ぎ
