# Developer Supply Chain Security

## 目的とThreat Model

この文書は、悪意あるEditor ExtensionなどがDeveloper workstationからGitHub credentialを窃取し、正規AccountとしてSource、tag、Release、GHCR imageを変更する攻撃へ備えるための運用基準です。Growseは当面一人で開発するため、二者承認ではなく、署名、GitHub Ruleset、自動検査、Release cool-down、短命Credential、監査を独立した防御として重ねます。

不可視Unicodeの検出だけで、暗号化JavaScript、WASM、native malware、正規に見えるSourceへ埋め込まれたすべてのPayloadを検出できるわけではありません。Editor Extension自体の実行をGitHub側で止めることもできないため、端末側の管理とIncident Responseを省略しないでください。

## Repositoryの自動検査

`cmd/securityscan`は外部Scanner Actionを追加せず、Go標準Libraryだけで構成したRepository所有のScannerです。`git ls-files`を基準に、全tracked fileとPull Request差分を検査します。

- bidi control、zero-width／format control、variation selector、Private Use Areaを検出する
- 正当な日本語、accent付き文字、通常のEmoji、Emoji間のZWJ、Emoji variation selectorは許可する
- `.vsix`、`.wasm`、ELF、PE、Mach-O、Archive、未知のBinary blobを検出する
- 必要な画像などはpathとSHA-256が一致する場合だけ許可する
- `.vscode/extensions.json`のrecommendationをPublisher／Version policyと照合する
- GitHub Actions上ではFile、Line、Column、code pointをannotationとして表示する

ローカルでは次を実行します。

```sh
make securityscan
go run ./cmd/securityscan -diff origin/main
```

例外は[security-policy.json](../.security/security-policy.json)だけで管理します。Unicode例外にはFile、code point、理由、失効日が必須です。Binary例外にはFile、SHA-256、理由が必須です。期限のないUnicode例外やhashの一致しないBinaryは拒否されます。

### 同じPRによるScanner無効化への対策

通常CIは変更後のScannerを検査するbootstrapです。恒久的な`unicode-security`checkは`pull_request_target`でdefault branchのScannerとpolicyを先にbuildし、書き込み権限やSecretを持たない状態でPRのFileを読み取ります。PRに含まれるGo、Workflow、Scriptは実行しません。そのため、Scannerやpolicyを同じPRで無効化してPayloadを通すことはできません。Scanner policyの変更はmerge後の次のPRから有効になります。

導入PRだけはdefault branchにtrusted workflowがまだ存在しないため、通常CIのbootstrapを利用します。導入PRをmergeした後に`unicode-security`をmain Rulesetの必須Status checkへ追加し、Ruleset auditを再実行してください。

```sh
bash scripts/audit-github-security.sh
```

## GitHub／Organization設定

次の状態を維持します。

| 対象 | 必須設定 |
| --- | --- |
| `main` Ruleset | Pull Request必須、force push／delete禁止、verified signature必須、必須CI未完了のmerge禁止、bypassなし |
| `v*` tag Ruleset | `refs/tags/v*`を対象にupdate／deleteを禁止、verified signature必須、bypassなし |
| Organization 2FA | 必須。有効な方法はpasskeyまたはhardware security keyを優先し、recovery codeをoffline保管 |
| Classic PAT | Organization accessを禁止 |
| Fine-grained PAT | `Growse`だけ、必要Permissionだけ、最大90日。不要になった時点で失効 |
| Release Environment | `release`に10分のwait timer、Protection ruleのadmin bypass無効、`v*` tagだけを許可 |
| Releases | Immutable Releasesを有効にし、公開後のtag／Asset差し替えを禁止 |

一人開発ではPR作成者本人のmergeを許可しますが、必須CI完了前のmergeとRuleset bypassは許可しません。Collaboratorが増えたらrequired reviewerを1人以上にし、last push approvalとself-review禁止を追加します。

GitHub ActionsのReleaseはRepository標準の短命`GITHUB_TOKEN`とOIDCだけを使用します。Release SecretへPAT、SSH private key、Registry passwordなどの長期Credentialを追加しないでください。

## commit署名とtag署名

GPG keyをGitHub Accountへ登録し、commitとtagの署名を既定にします。

```sh
git config --global user.signingkey <GPG-key-ID>
git config --global commit.gpgsign true
git config --global tag.gpgsign true
git commit -S -m "変更内容"
git tag -s v0.0.0 -m "Growse v0.0.0"
git verify-commit HEAD
git verify-tag v0.0.0
```

GitHub上でcommitとrelease tagの両方に`Verified`が表示されることを確認します。可能なら署名Keyをhardware-backed keyへ移し、秘密Keyのexportを端末へ常置しません。

## Editor Extension Policy

Repositoryが使用を推奨するExtensionは次の3つだけです。Version更新は自動承認せず、Publisher、Marketplace URL、Release notes、Repository、権限変更を確認してから`security-policy.json`を更新します。

| Extension ID | Publisher | 許可Version |
| --- | --- | --- |
| `golang.go` | Go Team at Google | `0.56.0` |
| `github.vscode-github-actions` | GitHub | `0.32.3` |
| `redhat.vscode-yaml` | Red Hat | `1.24.0` |

現在のinventoryを取得し、許可表との差分を確認します。

```sh
code --list-extensions --show-versions | sort
```

Growseの開発に不要なExtensionは無効化または削除します。新しいMarketplace／OpenVSX Extensionを導入する前に、Publisher identity、公開期間、download推移、source repository、署名／hash、install script、native binary／WASM、network通信、過去のownership変更を確認します。自動更新は無効化し、検証したVersionへ手動更新します。

## Credential Inventory

月次、およびExtension追加後に次を棚卸しします。

```sh
gh auth status
gh api user/keys --jq '.[] | [.id, .title, .created_at] | @tsv'
gh api repos/Grove-Computing/Growse/keys --jq '.[] | [.id, .title, .read_only] | @tsv'
gh api user/installations --jq '.installations[] | [.id, .app_slug, .repository_selection] | @tsv'
```

不要なPAT、OAuth App、GitHub App、SSH key、Deploy keyは即時失効します。開発端末へclassic PATや平文Credentialを置かず、通常操作はGitHub CLIのOAuth credentialまたはhardware-backed SSH key、Actionsは`GITHUB_TOKEN`／OIDCを使います。

## Release前確認

tagをpushする前に署名と差分を確認します。

```sh
git verify-commit HEAD
git diff --check main...HEAD
make ci
git tag -s v0.0.0 -m "Growse v0.0.0"
git verify-tag v0.0.0
git push origin v0.0.0
```

Release workflowはSource treeのUnicode／Binary／Extension検査とGo dependency検査を再実行し、commit SHAをLogへ記録します。Build後は`release`Environmentのwait timerで停止するため、GitHub Security log、Organization Audit log、tag差分を確認します。公開時はArchive checksumとContainer digestをLogへ残し、SBOM／Provenanceを生成します。

## Incident Response Runbook

### 1. 隔離

感染が疑われる端末をWi-Fi、Ethernet、VPNから切断し、Networkから隔離します。疑わしい端末からGitHubへloginしたり、credentialを変更したりしません。揮発性情報が必要な場合を除き、Extensionやprocessを不用意に再実行しません。

### 2. cleanな端末からCredentialを失効

既知のcleanな端末とpasskey／hardware security keyを使い、GitHub Passwordと2FA recovery状態を確認します。その後、PAT、OAuth authorization、GitHub App token、SSH key、Deploy key、active sessionを棚卸しし、不明または感染端末で使用したものをすべて失効・再発行します。Release／Actions Secretが存在する場合もrotationします。

### 3. Audit

GitHub個人Security logとOrganization Audit logで、Repository、commit、tag、Release、Workflow、Secret、Ruleset、Deploy key、Packageに関する操作を確認します。

```sh
gh api '/user/security-log?phrase=repo:Grove-Computing/Growse' --paginate
gh api '/orgs/Grove-Computing/audit-log?phrase=repo:Grove-Computing/Growse' --paginate
```

確認した時間範囲、Actor、IP、User-Agent、操作、commit SHA、tag、Artifact digestをIncident記録へ保存します。

### 4. 不正成果物を隔離

不正commitをmergeせず、疑わしいtag、Release、GHCR imageを新規利用しないようSecurity AdvisoryとRelease noteで告知します。Immutable Releaseは差し替えられないため、証拠を保存した上でcompromised Versionを明示し、clean commitから新しいVersionをbuildします。履歴を書き換えて痕跡を消しません。

### 5. clean buildとArtifact再検証

clean checkoutで`make ci`を実行し、署名済みcommitから再buildします。公開済みArtifactはIssue #27で導入したAttestationとdigestを使って再検証します。

```sh
gh attestation verify growse_<version>_<platform>_<arch>.tar.gz \
  --repo Grove-Computing/Growse
gh attestation verify 'oci://ghcr.io/grove-computing/growse@sha256:<digest>' \
  --repo Grove-Computing/Growse
```

検証後、侵入経路、影響範囲、失効したCredential、再発防止策をSecurity Advisoryへ記録します。
