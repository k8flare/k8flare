# S4: Cloudflare Mesh — desk research (2026-07-02)

Scope: `docs/platform-verification.md` の S4 検証項目 3 点(課金範囲 / flannel-over-Mesh
プロトタイプの成立性 / Cluster DNS 代替可否)を、実機検証なしの公式情報ベースで埋める。
`docs/cloudflare-mesh-networking.md`(コミット 4547fc0、本リサーチと同日追加)を前提知識とし、
そこにない情報・古くなった情報のみ扱う。二重に書かない。**コード変更・デプロイ・docs/ 編集は行っていない。**

## 結論サマリ

| # | 検証項目 | 判定 | 確信度 |
|---|---|---|---|
| 1 | Workers Paid の範囲で使えるか | ほぼ Yes — 50 node + 50 user の無料枠は Zero Trust **Free**($0)で到達でき、Workers Paid とは独立に追加コスト無しで有効化できる。ただし超過分の価格は依然未公開 | 中(推論を含む。公式が「追加費用ゼロ」と明言した一次情報は見つからず) |
| 2 | 接続モデル | 既存ドキュメントの記述を裏付け・補強。true L3、CIDR advertise、スクリプト可能な enrollment。デフォルト transport は MASQUE(QUIC/UDP)、WireGuard も選択可 | 高(公式ドキュメント多数で確認) |
| 3 | flannel-over-Mesh の成立性(机上判定) | **host-gw は不成立と判定**(公式に L2 隣接必須と明記、類似の VPN メッシュ上での破綻事例あり)。vxlan なら成立しうるが二重 UDP カプセル化になる。かつ **k3s には Mesh を使わない公式代替(`--flannel-backend=wireguard-native`)が既にある** — 論点が変わる | 中〜高 |
| 4 | Cluster DNS 代替 | **不可と判定。** Mesh 自体の hostname routing は未出荷(2026年夏予定と公式発表)、プログラム的に使える Cloudflare Internal DNS は Enterprise 限定。CoreDNS 案を fallback ではなく第一候補に格上げすべき | 高 |
| 5 | 成熟度 | GA は 2026-04-14 だが、Mesh ノードが依存する Linux クライアント自体が GA したのは 2026-06-29(本リサーチの3日前)。K8s/CNI 統合事例は依然ゼロ | 高 |

---

## 1. Workers Paid の範囲で使えるか

- Cloudflare Mesh は **「50 nodes and 50 users free... included with every Cloudflare account」**
  と公式ブログが明言している([Introducing Cloudflare Mesh](https://blog.cloudflare.com/mesh/))。
- ただし製品自体は Cloudflare ダッシュボードの **Zero Trust / Cloudflare One** 配下
  (`Networking > Mesh`)にあり、Workers ダッシュボードとは別の製品領域。Zero Trust には
  そもそも **Free プラン($0、50 ユーザーまで)が常設されている**([Cloudflare Zero Trust
  Pricing](https://costbench.com/software/business-vpn/cloudflare-zero-trust/) — 非公式だが
  複数の価格まとめサイトで一致)。Mesh の「50 node + 50 user」という数字は、この Zero Trust
  Free の 50 シート上限と同じ枠を指していると考えるのが自然(Mesh のノード/ユーザーも
  Zero Trust のシートを消費する設計 — [Seat management](https://developers.cloudflare.com/cloudflare-one/team-and-resources/users/seat-management/))。
- 結論: **Workers Paid だけの契約でも、Zero Trust Free($0)を追加で有効化すれば
  Mesh を 50 node/user まで追加コストなしで使える可能性が高い。** ただし「Workers Paid の
  支払いだけで自動的に付いてくる」わけではなく、別プロダクトの明示的な有効化が要る。
- **未解決のまま**: 50 を超えた場合の課金体系(ノード単価・帯域課金の有無)は、公式
  pricing ページ・changelog・コミュニティフォーラムのいずれにも見当たらなかった
  (`cloudflare.com/products/mesh/` は pricing ページへの参照のみで具体額なし)。
  既存ドキュメントの「Open questions」の指摘は依然有効 — Cloudflare に直接確認するしかない。

## 2. 接続モデル

既存ドキュメントの記述(private IP `100.96.0.0/12`、TCP/UDP/ICMP 到達性、CIDR route
advertisement、スクリプト可能な enrollment)を裏付ける一次情報が今回複数見つかった。追加で
判明した点のみ記載する。

- **Transport はデフォルト MASQUE(QUIC over UDP, HTTP/3, TLS 1.3)、WireGuard も可**
  ([Zero Trust WARP: tunneling with a MASQUE](https://blog.cloudflare.com/zero-trust-warp-with-a-masque/)、
  [Donning a MASQUE](https://blog.cloudflare.com/masque-building-a-new-protocol-into-cloudflare-warp/))。
  これは既存ドキュメントの UDP ロス懸念(セクション「Why this isn't a clean drop-in」)を
  補強する事実 — **Mesh の下り物理トランスポート自体が UDP(QUIC)なので、既存ドキュメントが
  引用する ~14% UDP ロスは、アプリケーション層が UDP かどうかに関わらず Mesh を通る
  すべてのトラフィック(TCP ペイロードを含む)に及ぶ構造的な性質**と読める。既存ドキュメントは
  「CoreDNS 等の UDP アプリケーショントラフィックが特に危険」と書いていたが、より正確には
  「Mesh 自体が UDP トンネルなので、上に何を流しても同じ輻輳特性を継承する」という理解の方が
  正しい。
- **対応 OS(サーバーノード)**: RHEL 9/10, Debian 12/13, Fedora 43/44, Ubuntu 22.04/24.04
  ([Get started with Cloudflare Mesh](https://developers.cloudflare.com/cloudflare-one/networks/connectors/cloudflare-mesh/get-started/))。
  k3s の典型デプロイ対象(Ubuntu/Debian 系 BYO VM)は問題なくカバーされる。
- **MTU は「推奨」ではなく実害あり**: [Tips and best practices](https://developers.cloudflare.com/cloudflare-one/networks/connectors/cloudflare-mesh/tips/)
  に明記—「If source devices send packets near the maximum size (1,460 bytes or more), the
  double encapsulation can push packets over 1,500 bytes, **causing them to be dropped**.」
  既存ドキュメントは MTU 1,280/MSS 1,240 を「推奨値」として触れていたが、実際は
  未調整だとサイレントにパケットが落ちる、という運用上のフェイルモードがある。k3s/flannel の
  デフォルト MTU(1,450 前後、バックエンドにより変動)は素で載せると壊れる可能性が高く、
  プロトタイプでは MTU 明示設定が前提条件になる。
- **CIDR ルート広告の実態**: [Routes](https://developers.cloudflare.com/cloudflare-one/networks/connectors/cloudflare-mesh/routes/)
  によれば、advertise した CIDR 宛てのトラフィックをノードが「ローカルネットワーク上の
  適切なホストに配信するゲートウェイ」として振る舞う、という説明。これは Mesh 非参加の
  マシンが後ろにぶら下がる場合(オンプレサブネットのゲートウェイ役)を主眼にした説明で、
  「Mesh 参加ノード同士が互いの Pod CIDR を広告し合う」という k3s のシナリオへの直接適用は
  ドキュメント化されていない(既存ドキュメントの指摘通り、依然として推論の域)。
- **HA**: アクティブ/スタンバイの replica フェイルオーバーが公式サポート
  ([High availability](https://developers.cloudflare.com/cloudflare-one/networks/connectors/cloudflare-mesh/high-availability/)、
  2026-05-28 にダッシュボード UI が追加: [changelog](https://developers.cloudflare.com/changelog/post/2026-05-28-mesh-ha-replica-ui/))。

## 3. flannel-over-Mesh の成立性(机上判定)

既存ドキュメントは「Mesh の CIDR advertise は host-gw と同じゲートウェイ役を果たせるので
技術的には Yes」という推論だったが、flannel 自体の一次情報とバックエンド別の要件を
直接確認したところ、**バックエンド選択によって結論が割れる**ことが分かった。

| flannel backend | 前提条件(公式) | Mesh 上での見立て |
|---|---|---|
| **host-gw** | 「Requires direct layer2 connectivity between hosts running flannel」— 同一 L2 が明文の必須条件([flannel backends.md](https://github.com/flannel-io/flannel/blob/master/Documentation/backends.md)) | **不成立と判定。** Mesh の `100.96.0.0/12` は Cloudflare PoP 経由の仮想アドレス空間で、ノード間に共有 L2/ブロードキャストドメインは存在しない(既存ドキュメントの「no direct peer-to-peer path」と同じ理由)。傍証として、同種の VPN メッシュ(Tailscale)+ host-gw の組み合わせで実際にパケットドロップが起きた k3s の Issue が存在する([k3s-io/k3s#8372](https://github.com/k3s-io/k3s/issues/8372) — 「Tailscale dropped packets since they're not from a Tailscale IP」)。kernel の `onlink` route flag で回避できる可能性はあるが、Cloudflare Mesh でこれが機能するという記述・実例はどこにもない。 |
| **vxlan** | UDP/8472 の L3 到達性のみ要求、L2 隣接は不要([flannel backends.md](https://github.com/flannel-io/flannel/blob/master/Documentation/backends.md)) | Mesh が提供する「任意 IP 間の UDP/TCP/ICMP 到達性」と条件がちょうど一致する。**プロトタイプするならこちらが技術的に正しい選択。** ただし vxlan 自体が UDP カプセル化であり、その上に Mesh 自身の UDP(MASQUE/QUIC)カプセル化が乗る**二重 UDP カプセル化**になる。MTU はさらに厳しくなる(vxlan オーバーヘッド ~50 byte が Mesh の 1,280 byte 制約にさらに食い込む)。 |
| **wireguard-native**(flannel 組込みの WireGuard backend) | UDP/51820、L2 隣接不要 | Mesh を経由せず**この backend 単体で同じ問題を解決できる**(下記参照)。Mesh 経由で使う意味は薄い。 |

**論点そのものが変わる指摘**: k3s には元々、複数クラウド/複数拠点にまたがるノードを
つなぐための **公式サポートされた組込み手段**がある。
[Distributed hybrid or multicloud cluster](https://docs.k3s.io/networking/distributed-multicloud)
に明記: 「K3s uses wireguard to establish a VPN mesh for cluster traffic」— サーバー/エージェント
両方に `--node-external-ip` を、サーバーに `--flannel-backend=wireguard-native` を渡すだけで、
Mesh のような第三者プロダクトなしに「ノードが同一 VPC にない」問題を解決できる(k3s 側では
GA 機能、実績あり)。したがって S4 が本当に問うべきなのは「Mesh は flannel を動かせるか」
ではなく **「Mesh は k3s 純正の wireguard-native に対して何を付加するか」** である。最も筋の
良い付加価値仮説は「BYO VM が inbound ポートを一切開けなくても(自宅回線・NAT配下でも)
参加できる」という NAT traversal 面 — wireguard-native は各ノードが他ノードから直接
inbound UDP/51820 を受けられる必要があるが、Mesh はノード側が Cloudflare への outbound
接続だけで済む(Tailscale 等と同じ相対利点)。この仮説自体はまだ検証していない。

(参考: k3s の Tailscale 統合ドキュメントには「Embedded etcd is not supported in this type
of deployment」という注記がある。k8flare は etcd を使わない構成のため直接は関係しないが、
VPN メッシュ + 分散 k3s の組み合わせ一般に既知の粗さがあることを示す傍証として記録する。)

## 4. Cluster DNS 代替

既存ドキュメントはこの論点に触れていなかった(スコープ外だった)。今回新規に調査。

- **Mesh 自身の hostname routing は未出荷。** 公式ブログに「Cloudflare is bringing hostname
  routing to Mesh **this summer**」(2026)と明記([Connect and secure any private or public
  app by hostname, not IP](https://blog.cloudflare.com/tunnel-hostname-routing/))。
  2026-06-29 の Linux クライアント GA changelog にも「Cloudflare Mesh now supports
  hostname-based routing」という記述が現れ始めているが([changelog](https://developers.cloudflare.com/changelog/post/2026-06-29-warp-linux-ga/))、
  本体ドキュメント(get-started/tips/routes)には未反映で、機能の全体像・API 可否は不明。
  ローンチされたばかりの機能に、存在しない/削除される Kubernetes Service を数十〜数百単位で
  同期させる用途を賭けるのは時期尚早。
- **アーキテクチャ的にも Kubernetes Service DNS とは相性が悪い可能性が高い。** hostname
  routing の仕組みは「DNS クエリとトラフィックが同じトンネル経由で Cloudflare に届く」ことを
  前提にしており([同上])、ダッシュボードで個々のノード/デバイスに名前を付ける
  static な運用が基本形に見える。`*.svc.cluster.local` のように Service の生成・削除の
  たびに動的にレコードが増減するワイルドカードゾーンを外部コントローラーから push する、
  という CoreDNS の kubernetes plugin 相当の使い方は、現行のドキュメントのどこにも
  示唆されていない。
- **プログラム的に叩ける唯一の候補(Cloudflare Internal DNS)は Enterprise 限定。**
  [Internal DNS](https://developers.cloudflare.com/dns/internal-dns/get-started/) は
  batch API でレコードを追加・削除できる、という点では external-dns 的な「コントローラーが
  Service を DNS レコードに同期する」パターンに理論上ハマる建付けだが、ドキュメントに
  明記: 「Make sure you have an Enterprise account with access to Gateway resolver
  policies」。Workers Paid はおろか Zero Trust の Pay-as-you-go でも届かない。ワイルドカード
  レコード対応の有無すら未確認のまま、プラン要件で脱落する。
- **Gateway の DNS 機能(Resolver policies / Local Domain Fallback)は「振り分け」機能であって
  「権威サーバー」ではない。** [Resolver policies](https://developers.cloudflare.com/cloudflare-one/traffic-policies/resolver-policies/)
  と [Local Domain Fallback](https://developers.cloudflare.com/cloudflare-one/team-and-resources/devices/cloudflare-one-client/configure/route-traffic/local-domains/)
  はいずれも「特定ドメイン宛てのクエリを指定した既存の DNS サーバーに転送する」設計で、
  Cloudflare 側が `*.svc.cluster.local` の権威応答を動的に生成・返答する仕組みではない。
  結局その転送先に CoreDNS 相当のサーバーを自分で立てる必要があり、Gateway を挟む意味がない。

**判定: Cluster DNS の Mesh/Gateway 代替は現時点で不可。** `docs/platform-verification.md`
S4 セクションが既に「不採用の場合のフォールバック」として言及している CoreDNS Deployment +
`kube-dns` Service([general-purpose-k8s-plan.md](../../docs/general-purpose-k8s-plan.md) Phase 4)
を、fallback ではなく **第一候補として確定させてよい**。Mesh の DNS 機能の進捗は年内ウォッチ
対象として残すに留める。

## 5. 成熟度

- **GA 日**: 2026-04-14、Agents Week で発表([changelog](https://developers.cloudflare.com/changelog/post/2026-04-14-cloudflare-mesh/)、
  文言は「Cloudflare Mesh is now available」)。既存ドキュメントの「GA-ish」という
  ヘッジは、一次情報で GA と確認できたので不要になった — **正式に GA、ただし ~2.5ヶ月選手**。
- ローンチ直後に **ノード上限を 10 → 50 に引き上げ**(同 changelog)。
- **2026-05-28**: HA replica 管理の UI が追加([changelog](https://developers.cloudflare.com/changelog/post/2026-05-28-mesh-ha-replica-ui/))
  — 機能自体はまだ拡張が続いている。
- **2026-06-29(本リサーチの3日前)**: Mesh ノードが実際に使う Cloudflare One Client の
  **Linux 版がベータから GA に昇格**([changelog](https://developers.cloudflare.com/changelog/post/2026-06-29-warp-linux-ga/))。
  同時に RHEL 9/10 上での Mesh サポートが明記され、TPM 2.0 によるハードウェアデバイス
  登録などのセキュリティ機能もこのリリースで初めて安定版入りしている。**Mesh は「GA」を
  名乗っているが、Linux サーバーでの土台(クライアント自体)がつい最近までベータだった**、
  という事実は運用リスクとして明記に値する。
- **コンテナ対応(Docker image)は依然「later this year」**(未出荷、既存ドキュメントの
  記述通り変化なし — [Introducing Cloudflare Mesh](https://blog.cloudflare.com/mesh/))。
- **Kubernetes/CNI との統合事例・公式サンプルは今回の調査でも一件も見つからなかった。**
  既存ドキュメントの記述を追認。

## 既存ドキュメント(`docs/cloudflare-mesh-networking.md`)からの更新点

1. **GA ステータスを確定**(「GA-ish」→ 確認済み GA、2026-04-14 起点で ~2.5ヶ月)。
2. **UDP ロス懸念を再解釈**: 既存ドキュメントは「CoreDNS 等の UDP アプリ通信が特に危険」と
   書いていたが、Mesh の transport 自体(MASQUE/QUIC)が UDP であるため、実際には
   TCP ペイロードを含む Mesh 経由の全通信が同じ輻輳特性を継承する、という構造的な理解に
   訂正・補強。
3. **flannel-over-Mesh の判定を host-gw については「不成立」に格下げ**。既存ドキュメントの
   「技術的には Yes」は vxlan backend でのみ成立しうる話として再整理。
4. **k3s 純正の `wireguard-native` backend という代替の存在を追加**。これにより、S4 の
   問いは「flannel は Mesh 上で動くか」から「Mesh は wireguard-native に対して何を
   付加するか(NAT traversal 仮説)」に変わる。
5. **Cluster DNS 代替可否を新規調査・「不可」と判定**(既存ドキュメントは対象外だった)。
6. **MTU 問題を「推奨値」から「未設定だと実際にパケットが落ちる」という運用上のフェイルモードに格上げ**。
7. 課金範囲について、「Zero Trust Free($0)経由で Workers Paid ユーザーも追加コストなしで
   到達できる可能性が高い」という一次情報に基づく推論を追加(既存ドキュメントは完全に
   オープンクエスチョンのままだった)。50 超過分の価格が不明という結論自体は変わらず。

## 未解決のまま残る問い

- 50 node/user 超過時の Mesh 課金体系(単価・帯域従量課金の有無) — 公式に非公開。
  Cloudflare へ直接問い合わせるしかない。
- vxlan-over-Mesh の実際の MTU 実効値・スループット・UDP ロス実測(第三者データの
  NetBird 発ベンチマークは Mesh の競合企業によるものであり、方向性の参考にはなるが
  数値をそのまま設計値に使うべきではない、という既存ドキュメントの留保は継続して有効)。
- Mesh の「BYO VM の inbound ポート開放が不要」という NAT traversal 上の利点仮説の実証
  (k3s 純正 wireguard-native との比較優位性の核心)。
- Mesh hostname routing が今夏出荷された後、API 経由でのプログラム的なレコード管理が
  可能になるかどうか(可能になれば Cluster DNS 代替の判定を再考する価値がある)。

## 出典

- [Introducing Cloudflare Mesh(公式ブログ)](https://blog.cloudflare.com/mesh/)
- [Cloudflare Mesh Overview](https://developers.cloudflare.com/cloudflare-one/networks/connectors/cloudflare-mesh/)
- [Get started with Cloudflare Mesh](https://developers.cloudflare.com/cloudflare-one/networks/connectors/cloudflare-mesh/get-started/)
- [Cloudflare Mesh Routes](https://developers.cloudflare.com/cloudflare-one/networks/connectors/cloudflare-mesh/routes/)
- [Cloudflare Mesh Tips and best practices](https://developers.cloudflare.com/cloudflare-one/networks/connectors/cloudflare-mesh/tips/)
- [Cloudflare Mesh High availability](https://developers.cloudflare.com/cloudflare-one/networks/connectors/cloudflare-mesh/high-availability/)
- [Changelog: Introducing Cloudflare Mesh (2026-04-14)](https://developers.cloudflare.com/changelog/post/2026-04-14-cloudflare-mesh/)
- [Changelog: HA replica management for Cloudflare Mesh (2026-05-28)](https://developers.cloudflare.com/changelog/post/2026-05-28-mesh-ha-replica-ui/)
- [Changelog: Cloudflare One Client for Linux GA (2026-06-29)](https://developers.cloudflare.com/changelog/post/2026-06-29-warp-linux-ga/)
- [Connect and secure any private or public app by hostname, not IP(公式ブログ)](https://blog.cloudflare.com/tunnel-hostname-routing/)
- [Zero Trust WARP: tunneling with a MASQUE(公式ブログ)](https://blog.cloudflare.com/zero-trust-warp-with-a-masque/)
- [Donning a MASQUE(公式ブログ)](https://blog.cloudflare.com/masque-building-a-new-protocol-into-cloudflare-warp/)
- [Cloudflare Internal DNS: Get started](https://developers.cloudflare.com/dns/internal-dns/get-started/)
- [Cloudflare Gateway Resolver policies](https://developers.cloudflare.com/cloudflare-one/traffic-policies/resolver-policies/)
- [Cloudflare One Client: Local Domain Fallback](https://developers.cloudflare.com/cloudflare-one/team-and-resources/devices/cloudflare-one-client/configure/route-traffic/local-domains/)
- [Cloudflare One Client settings(DNS search suffix)](https://developers.cloudflare.com/cloudflare-one/team-and-resources/devices/cloudflare-one-client/configure/settings/)
- [Cloudflare One: Seat management](https://developers.cloudflare.com/cloudflare-one/team-and-resources/users/seat-management/)
- [Cloudflare Mesh product page](https://www.cloudflare.com/products/mesh/)
- [Cloudflare Zero Trust Pricing 2026(非公式まとめ)](https://costbench.com/software/business-vpn/cloudflare-zero-trust/)
- [Cloudflare Zero Trust & SASE Plans & Pricing(公式)](https://www.cloudflare.com/plans/zero-trust-services/)
- [flannel: Backends(公式ドキュメント)](https://github.com/flannel-io/flannel/blob/master/Documentation/backends.md)
- [K3s: Distributed hybrid or multicloud cluster(公式ドキュメント)](https://docs.k3s.io/networking/distributed-multicloud)
- [k3s-io/k3s#8372 — Tailscale + host-gw 相当の破綻事例](https://github.com/k3s-io/k3s/issues/8372)
- [Cloudflare Mesh vs NetBird vs Tailscale: Performance Compared(NetBird、非公式・利害関係あり)](https://netbird.io/knowledge-hub/cloudflare-mesh-vs-netbird-vs-tailscale)
- [Connect Workers to Cloudflare Mesh(Workers VPC)](https://developers.cloudflare.com/workers-vpc/examples/connect-to-cloudflare-mesh/)
- [Cloudflare WAN(旧 Magic WAN)Connectivity options](https://developers.cloudflare.com/cloudflare-wan/zero-trust/connectivity-options/)
