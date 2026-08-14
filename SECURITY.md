# Security Policy

## Supported Versions

セキュリティ修正は、最新のリリース系列を対象に提供します。

| Version | Supported |
| --- | --- |
| 0.6.x | Yes |
| 0.5.x | No |
| 0.4.x | No |
| 0.3.x | No |
| 0.2.x | No |

## Reporting a Vulnerability

脆弱性を発見した場合は、公開Issueへ詳細を投稿せず、GitHubの
[Private vulnerability reporting](https://github.com/Grove-Computing/Growse/security/advisories/new)
から報告してください。

報告には、可能な範囲で次の情報を含めてください。

- 影響を受けるGrowseのバージョンとOS
- 再現手順または最小の再現ページ
- 想定される影響
- 既知の回避策

受領後7日以内に初回回答を行い、影響範囲と修正方針が確定するまでは報告内容を非公開で扱います。

## WebGo Security Boundary

Growse v0.6.0のYaegi Runtimeは、信頼できないGoコードを安全に実行するSandboxではありません。
プロセス分離、CPU時間制限、メモリ制限、およびGo標準ライブラリ全体に対する完全な制限は提供していません。

WebGoの自動実行は`localhost`、`127.0.0.1`、`::1`のページと、同じく信頼済みOriginから取得したGoスクリプトに限定されます。ただし、ローカルで配信されるページやスクリプトを信頼できることは利用者自身が確認してください。

信頼できないWebGoソースを開いたり、Growseを権限の高いユーザーで実行したりしないでください。

## Hoverとカーソル表示

リンク先プレビューはURLの表示だけを行い、hoverを理由とするDNS問い合わせ、HTTPリクエスト、先読み、WebGo実行は行いません。URLにパスワードが含まれる場合は伏字にしますが、表示されたリンク先と接続先を利用者自身でも確認してください。

Gopherカーソルには`internal/ui/assets/blue.svg`から生成してビルドへ埋め込んだ`gopher-blue.png`だけを使用します。閲覧ページから取得したSVGをカーソルとして実行時に読み込む機能はありません。

## CSS Resource

外部Stylesheetの`@import`は同一Originに限定し、循環を検出した上で最大深度8、最大32 Stylesheet、合計8 MiBに制限します。各Background ImageはHTTP(S)のPNG、JPEG、GIFだけを受け入れ、応答を4 MiB、Decode後の画像を1600万画素までに制限します。複数Backgroundでも各URLへ同じ検証を適用し、MIME TypeやDecodeの検証に失敗したLayerは描画せず、ページ本体の表示は継続します。

Gradient、Shadow、Transform、Clip、Opacityは取得したコードを実行せず、型付きのStyle値からLayout TreeとDisplay Listを生成します。極端に大きいGridや深いStacking Contextを含む信頼できないページはCPU・メモリを消費し得るため、WebGoと同様に高い権限で実行しないでください。
