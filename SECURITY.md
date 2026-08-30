# Security Policy

## Supported Versions

セキュリティ修正は、最新のリリース系列を対象に提供します。

| Version | Supported |
| --- | --- |
| 0.17.x | Yes |
| 0.16.x | No |
| 0.15.x | No |
| 0.14.x | No |
| 0.13.x | No |
| 0.12.x | No |
| 0.11.x | No |
| 0.10.x | No |
| 0.9.x | No |
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
VERSION=v0.17.0
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

## Go / JavaScript Runtime Security Boundary

Growse v0.17.0はPage / FrameのYaegi・goja・WASMとService WorkerをBrowser UIとは別の専用worker processで実行します。workerはversion / length制限付きの型付きIPCを通して、BrowserがbrokerするDOM、Event、Timer、Frame、Fetch、Storage、Navigation、Console、Module、dynamic resource、CSSOM、observer、WASM操作だけを要求できます。Browser credential、任意filesystem、直接socket / DNS、subprocess、dynamic library、OS shell、Go reflection、Node.js APIをhost surfaceとして公開しません。

Browserはcodeを渡す前に、別PID、worker executable / protocol、brokered host I/O、最小environment、parent lifecycle、memory上限を検証します。Linuxでは`no-new-privileges`とparent-death signal、macOS / Windowsでは専用process group、全platformではparent IPC EOFによる終了を適用します。必須条件が欠ける、workerがtimeout / crashする、protocolに違反する場合は対象contextだけをfail closedし、停止済みgenerationのcallbackを拒否します。

ここでいうsandboxはGrowseが公開する実行機能とhost作用のdefault-deny境界です。seccomp、container、VMと同等の全system call隔離や、OS kernel、Go runtime、Yaegi、goja、wazero、decoder、Growse自身の未知の脆弱性が存在しないことまでは保証しません。Growseを権限の高いユーザーや機密profileで実行しないでください。

JavaScriptは通常のHTTP(S) Pageのinline / external classic、CORSを通過したECMAScript Module、dynamic script / stylesheet、WebAssemblyを実行できます。redirect、status、MIME、mixed content、credentials、CORS、integrity、sizeをBrowser側で再検証します。これらの取得・実行とhydration callbackは利用者がTabの`JS` selectorを明示選択した場合だけ有効です。Go sourceは明示的な`text/go`かつtrusted loopback / same-originに限定し、外部Internet Originから暗黙実行しません。選択していないEngineのScriptは取得せず、停止したJavaScript generationのresource completion、observer、DOM mutationをGo Pageへcommitしません。FetchはSame-Origin PolicyとCORSを適用し、`omit`、`same-origin`、`include`のCredentials Modeに従います。

Browser Sessionあたりworker 32件、IPC message 1 MiB、pending message 256件、転送中payload 8 MiB、既定task 5秒を上限とします。Page close、Navigation、Engine切替でlistener、Timer、Fetch、Frame / WASM callbackをcancelし、通常停止に応答しないworkerは終了します。

Navigation、Form Submission、FetchはBrowser Session単位のメモリ内Cookie Jarを共有します。各RequestでDomain、Path、Secure、HttpOnly、SameSite、Origin、Credentialsを再評価し、WebGoからHttpOnly Cookieを参照できないようにします。Request Bodyは1 MiB、Headerは100件かつ64 KiB、Response Bodyは既定4 MiB、Redirectは10回を上限とし、Page終了時は進行中のFetchをcancelします。URLを含むErrorと表示にはuserinfoを残さず、CookieとAuthorizationの値をLogへ出力しません。

SchedulerはPageあたりTimer 10,000件、Animation Frame callback 10,000件、1 turnあたりのTimer callback 1,000件、delay 365日を上限とし、Page終了時にcallbackと待機goroutineを解放します。Historyは1,024 entry、URL 8 KiB、state 1件64 KiB、Session全体4 MiBを上限とし、stateやURL credentialをLogへ出力しません。

Local StorageはOSのUser Config Directory配下へOriginごとのJSONとして保存し、directoryを0700、fileを0600に制限します。暗号化機能ではないため、OS user accountとprofile directoryを信頼境界とします。同じBrowser Sessionのsame-origin TabはLocal Storageを共有しますが、Storage Eventには値をLog出力せず、更新元、cross-origin、closed Tabへ配送しません。Session StorageはTabのPage Sessionだけに保持し、Tab終了時に破棄します。key 4 KiB、value 1 MiB、Originごと5 MiB、Profile全体50 MiB、Origin数128を上限とし、transaction失敗時は更新前の状態へ戻します。

TabはDOM、Runtime worker、History、Session Storageを分離します。1つのBrowser SessionではLocal Storage、Cookie Jar、HTTP Cache、Service Worker registration / Cache Storageを仕様の範囲で共有するため、Origin policyとscopeがsecurity boundaryになります。

HTTP CacheはOSのUser Cache Directory配下にprivate cacheとして保存します。Authorization、Cookie、Set-Cookieを含むentryは保存せず、Cache hit時もMIME、Origin、CORS、Credentials Policyを再適用します。memoryは1,024 entryかつCache keyごと32 variant、diskは1 entry 4 MiB、Originごと32 MiB、全体128 MiBを上限とし、schema versionとBody SHA-256が一致しないentryを破棄します。

Service Worker registrationとCache StorageはOrigin profileへ保存します。registration 64件、Originあたりactive worker 1件、Cache 32件、entry 4,096件、1 response 4 MiB、Origin合計128 MiBを上限とします。scriptはsame-origin Secure Contextに限定し、scope、update redirect / MIME、fetch interceptionをBrowser側で検証します。Cacheへcredentialを永続化せず、破損dataはOrigin単位で隔離します。

DevToolsはPageごとのread-only診断境界です。Consoleは1件4 KiB・Page 1,000件、DOM snapshotは2,000 node・深さ128・attribute 64件・文字列4 KiB、NetworkはPage 2,000件・Browser Session 4,000件を上限とします。Runtime panelはPage / Frame / Service WorkerのID、generation、Engine、state、script種別に加え、CSS局所無視、font fallback chain、image cache counter、frame rebuild理由をbody-free metadataとして表示します。同一診断はcountへ集約し、1 Page 2,000種類・1 field 4 KiB・count 1,000,000で飽和します。Request / Response body、Header、Cookie、Authorization、Service Worker Cache body、decoded pixel、font bytes、IPC payload、raw error本文を保持せず、diagnostic URLからuserinfo、query、fragmentを除去します。Inspectorはpassword inputのvalue、WebGo callback、Runtime objectを公開しません。

外部Go sourceを信頼しないでください。外部JavaScriptはv0.17.0 sandbox boundary内で扱いますが、未知の実装脆弱性を想定し、機密情報を持つ高権限環境では実行しないでください。

## Hoverとカーソル表示

リンク先プレビューはURLの表示だけを行い、hoverを理由とするDNS問い合わせ、HTTPリクエスト、先読み、WebGo実行は行いません。URLにuserinfoが含まれる場合は認証情報全体を除去しますが、表示されたリンク先と接続先を利用者自身でも確認してください。

Gopherカーソルには`internal/ui/assets/blue.svg`から生成してビルドへ埋め込んだ`gopher-blue.png`だけを使用します。閲覧ページから取得したSVGをカーソルとして実行時に読み込む機能はありません。

## CSS Resource

外部Stylesheet、dynamic stylesheet、`@import`はsame-origin / cross-origin HTTP(S)を扱い、redirect後URL、CSS MIME、mixed content、integrity、循環を検証した上で`@import`最大深度8、1件4 MiB、Page合計16 MiB、dynamic stylesheet 128件に制限します。ImageはPNG、JPEG、GIFの静止Frame、WebP、安全な静的SVG subsetを受け入れ、1 resource 16 MiB、1辺16,384 pixel、100 megapixel、Page decode surface 256 MiBへ制限します。Web FontはCORSとMIMEを通過したWOFF / WOFF2だけをdecodeし、1件8 MiB、Page 64件・合計64 MiB、table 128件、glyph 50,000件を上限とします。MIME、decode、上限検証に失敗したresourceはplaceholderまたは同梱fontへfallbackし、ページ本体の表示を継続します。

inline / external SVGはpath、basic shape、text、gradient、clip、2D transformの静的subsetだけをrasterizeします。SVG内script、event handler、external resource、`foreignObject`、animation、filter、font load、Navigationは実行しません。XML entity、node 20,000件、path command 200,000件、4 MiB source、64 MiB raster surfaceを上限とします。

Gradient、Shadow、Transform、Clip、Opacityは取得したコードを実行せず、型付きのStyle値からLayout TreeとDisplay Listを生成します。極端に大きいGridや深いStacking Contextを含む信頼できないページはCPU・メモリを消費し得るため、WebGoと同様に高い権限で実行しないでください。

CSS Animationは、1要素あたり32件、Page全体で4096件、Stylesheetあたり256個の`@keyframes`に制限します。各`@keyframes`のFrame数、Declaration数、Selector数にも上限を設け、極端なDuration、Iteration、Easingを非有限値やbusy loopへ発展させないよう検証します。`requestAnimationFrame()`はPage 10,000件、1 frame 256 callbackのFIFO budgetを持ち、残りを次frameへ送って入力やNavigationへ制御を戻します。

JavaScript compatibility profileの画像は、URLごとのfetch bodyとdecode結果をPage generation内だけで共有し、256 MiB・512 resourceのLRU上限を適用します。target raster、background、gradient、filter cacheもbyte / entry上限を持ち、Page close、Navigation、Engine切替で破棄します。stale generationやcancel後のresource completionを新Pageへcommitせず、診断にはbody、decoded pixel、rasterを複製しません。
