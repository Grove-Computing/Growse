# Security Policy

## Supported Versions

セキュリティ修正は、最新のリリース系列を対象に提供します。

| Version | Supported |
| --- | --- |
| 0.9.x | Yes |
| 0.8.x | No |
| 0.7.x | No |
| 0.6.x | No |
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

## Supply Chain Security

GitHub Releaseの各Archiveには、同名の`.sha256`と、`growse_<version>_<platform>_<arch>.spdx.json`形式のSPDX JSON SBOMを公開します。SHA-256 checksumは取得時の破損や改変を検出し、SBOMはArchiveへ含まれるGo moduleとpackage componentsを確認するために使います。ArchiveとSBOMにはGitHub Artifact Attestationによる署名付きProvenanceを付与します。

たとえばLinux amd64のArchiveを検証する場合は、次を実行します。

```sh
VERSION=v0.9.0
ASSET="growse_${VERSION}_linux_amd64.tar.gz"
gh release download "$VERSION" --repo Grove-Computing/Growse \
  --pattern "$ASSET" --pattern "$ASSET.sha256" \
  --pattern "growse_${VERSION}_linux_amd64.spdx.json"
sha256sum -c "$ASSET.sha256"
gh attestation verify "$ASSET" --repo Grove-Computing/Growse
jq -e '.spdxVersion and (.packages | length > 0)' \
  "growse_${VERSION}_linux_amd64.spdx.json"
```

GHCRのDocker imageは、可変tagではなくRelease workflowが出力するimmutable digestで検証します。BuildKitのSPDX SBOMとSLSA ProvenanceはOCI attestationとしてimageへ付与され、GitHub Artifact Attestationもdigestへ結び付けられます。

```sh
IMAGE=ghcr.io/grove-computing/growse
DIGEST=sha256:<release-workflowが出力したdigest>
docker buildx imagetools inspect "$IMAGE@$DIGEST" \
  --format '{{ json .SBOM.SPDX }}' | jq -e '.spdxVersion and (.packages | length > 0)'
docker buildx imagetools inspect "$IMAGE@$DIGEST" \
  --format '{{ json .Provenance.SLSA }}' | jq -e '.buildType and .materials'
gh attestation verify "oci://$IMAGE@$DIGEST" --repo Grove-Computing/Growse
```

Release時は最終imageをdigest指定でscanし、修正の有無にかかわらずHigh/Criticalの既知脆弱性があれば公開を停止します。Supply Chain Securityの例外が必要な場合は、対象、理由、影響評価、代替策、失効日をIssueへ記録し、期限付きの変更としてreviewします。理由や失効日のない恒久的なignoreは追加しません。

Developer workstationからのCredential窃取、不可視Unicode、未承認Binary／Editor Extension、署名、Release cool-down、感染時の対応は[Developer Supply Chain Security](docs/developer-security.md)を参照してください。

## WebGo Security Boundary

Growse v0.9.0のYaegi Runtimeは、信頼できないGoコードを安全に実行するSandboxではありません。
プロセス分離、CPU時間制限、メモリ制限、およびGo標準ライブラリ全体に対する完全な制限は提供していません。

WebGoの自動実行は`localhost`、`127.0.0.1`、`::1`のページと、同じく信頼済みOriginから取得したGoスクリプトに限定されます。ただし、ローカルで配信されるページやスクリプトを信頼できることは利用者自身が確認してください。WebGo FetchはSame-Origin PolicyとCORSを適用し、`omit`、`same-origin`、`include`のCredentials Modeに従います。禁止Header、CORS Response Headerの非公開化、preflightとそのcacheを実装していますが、Runtime自体をSandboxにはしません。

Navigation、Form Submission、FetchはPage単位のメモリ内Cookie Jarを共有します。Domain、Path、Secure、HttpOnly、SameSite、expirationを検証し、WebGoからHttpOnly Cookieを参照できないようにします。Request Bodyは1 MiB、Headerは100件かつ64 KiB、Response Bodyは既定4 MiB、Redirectは10回を上限とし、Page終了時は進行中のFetchをcancelします。URLを含むErrorと表示にはuserinfoを残さず、CookieとAuthorizationの値をLogへ出力しません。

SchedulerはPageあたりTimer 10,000件、Animation Frame callback 10,000件、1 turnあたりのTimer callback 1,000件、delay 365日を上限とし、Page終了時にcallbackと待機goroutineを解放します。Historyは1,024 entry、URL 8 KiB、state 1件64 KiB、Session全体4 MiBを上限とし、stateやURL credentialをLogへ出力しません。

Local StorageはOSのUser Config Directory配下へOriginごとのJSONとして保存し、directoryを0700、fileを0600に制限します。暗号化機能ではないため、OS user accountとprofile directoryを信頼境界とします。Session StorageはBrowser Window内だけに保持します。key 4 KiB、value 1 MiB、Originごと5 MiB、Profile全体50 MiB、Origin数128を上限とし、transaction失敗時は更新前の状態へ戻します。

HTTP CacheはOSのUser Cache Directory配下にprivate cacheとして保存します。Authorization、Cookie、Set-Cookieを含むentryは保存せず、Cache hit時もMIME、Origin、CORS、Credentials Policyを再適用します。memoryは1,024 entryかつCache keyごと32 variant、diskは1 entry 4 MiB、Originごと32 MiB、全体128 MiBを上限とし、schema versionとBody SHA-256が一致しないentryを破棄します。

信頼できないWebGoソースを開いたり、Growseを権限の高いユーザーで実行したりしないでください。

## Hoverとカーソル表示

リンク先プレビューはURLの表示だけを行い、hoverを理由とするDNS問い合わせ、HTTPリクエスト、先読み、WebGo実行は行いません。URLにuserinfoが含まれる場合は認証情報全体を除去しますが、表示されたリンク先と接続先を利用者自身でも確認してください。

Gopherカーソルには`internal/ui/assets/blue.svg`から生成してビルドへ埋め込んだ`gopher-blue.png`だけを使用します。閲覧ページから取得したSVGをカーソルとして実行時に読み込む機能はありません。

## CSS Resource

外部Stylesheetの`@import`は同一Originに限定し、循環を検出した上で最大深度8、最大32 Stylesheet、合計8 MiBに制限します。各Background ImageはHTTP(S)のPNG、JPEG、GIFだけを受け入れ、応答を4 MiB、Decode後の画像を1600万画素までに制限します。複数Backgroundでも各URLへ同じ検証を適用し、MIME TypeやDecodeの検証に失敗したLayerは描画せず、ページ本体の表示は継続します。

Gradient、Shadow、Transform、Clip、Opacityは取得したコードを実行せず、型付きのStyle値からLayout TreeとDisplay Listを生成します。極端に大きいGridや深いStacking Contextを含む信頼できないページはCPU・メモリを消費し得るため、WebGoと同様に高い権限で実行しないでください。

CSS Animationは、1要素あたり32件、Page全体で4096件、Stylesheetあたり256個の`@keyframes`に制限します。各`@keyframes`のFrame数、Declaration数、Selector数にも上限を設け、極端なDuration、Iteration、Easingを非有限値やbusy loopへ発展させないよう検証します。
