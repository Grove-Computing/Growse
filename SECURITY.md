# Security Policy

## Supported Versions

セキュリティ修正は、最新のリリース系列を対象に提供します。

| Version | Supported |
| --- | --- |
| 0.2.x | Yes |
| 0.1.x | No |

## Reporting a Vulnerability

脆弱性を発見した場合は、公開Issueへ詳細を投稿せず、GitHubの
[Private vulnerability reporting](https://github.com/saku0512/growse/security/advisories/new)
から報告してください。

報告には、可能な範囲で次の情報を含めてください。

- 影響を受けるGrowseのバージョンとOS
- 再現手順または最小の再現ページ
- 想定される影響
- 既知の回避策

受領後7日以内に初回回答を行い、影響範囲と修正方針が確定するまでは報告内容を非公開で扱います。

## WebGo Security Boundary

Growse v0.2.0のYaegi Runtimeは、信頼できないGoコードを安全に実行するSandboxではありません。
プロセス分離、CPU時間制限、メモリ制限、およびGo標準ライブラリ全体に対する完全な制限は提供していません。

WebGoの自動実行は`localhost`、`127.0.0.1`、`::1`のページと、同じく信頼済みOriginから取得したGoスクリプトに限定されます。ただし、ローカルで配信されるページやスクリプトを信頼できることは利用者自身が確認してください。

信頼できないWebGoソースを開いたり、Growseを権限の高いユーザーで実行したりしないでください。
