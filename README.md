# Growse

Growseは、GoまたはJavaScriptをクライアントサイド言語として実行する実験的なWebブラウザです。

HTMLとCSSで画面を構築し、Tab単位の`Go` / `JS` selectorでWebGoまたはJavaScriptから同じDOM、Event、Scheduler、Fetch、Storage、Navigationを操作できます。Goが既定です。

## Growseでできること

| 項目 | 主な機能 |
| --- | --- |
| ブラウジング | 左側Vertical Tab、URL入力、リンク遷移、same-document Routing、戻る、進む、再読込、Scroll復元 |
| HTML / DOM | 要素の検索・生成・追加・削除、属性・class・Form値の操作 |
| CSS Layout | Box Model、Flexbox、Grid、Position、Overflow、logical property、container query |
| CSS Paint | Color Level 4、Gradient、複数Background、Shadow、Opacity、2D Transform、bounded Filter / Blend |
| Animation | CSS Transition、`@keyframes`、Easing、Iteration、Direction、Fill Mode、Pause、`prefers-reduced-motion` |
| Form | Form Controls、Focus、Constraint Validation、GET / POST Submission |
| Scheduler | timeout、interval、Animation Frame、Page終了時の自動解除 |
| Storage | Tab間で共有する永続Local Storage、Storage Event、Tab単位のSession Storage |
| HTTP | WebGo Fetch、Cookie、Same-Origin Policy、CORS、Freshness・再検証・Disk対応のHTTP Cache |
| Dual Runtime | Go既定、Tab単位Go / JavaScript切替、完全reload、Engine間分離 |
| External JavaScript | 通常HTTP(S) Pageのclassic / async / defer、ES Modules、dynamic import、import map |
| Modern Web Compatibility | 明示的にJSを選んだTabでdynamic script / stylesheet、hydration DOM、CSSOM、observer、Next.js / SvelteKit / Tailwind縮約fixture |
| WebAssembly | JavaScript API、streaming compile / instantiate、Memory / Table / Instance quota |
| Browsing Context | same / cross-origin iframe、sandbox、`postMessage`、独立Runtime |
| Offline | Service Worker registration / lifecycle / fetch、Cache Storage、idle restart |
| Web API | Go / JavaScript共通のDOM Event、Fetch、Scheduler、History、Navigation、Storage、Console API |
| DevTools | Console、Inspector、Network、秘密情報を持たないPage / Frame / Service Worker Runtime診断 |

詳しい対応範囲と制限は、[CSS対応表](docs/css-support.md)、[Form / Fetch / Cookie対応表](docs/form-fetch-cookie-support.md)、[Storage / Cache対応表](docs/storage-cache-support.md)を参照してください。

## インストール

### Installerを使う

Linux、macOS、Git Bashを利用できるWindowsでは、最新版を`~/.local/bin`へインストールできます。InstallerはダウンロードしたArchiveのSHA-256 checksumを検証します。

Installerはcommandに加えて、OSごとのGUI Applicationも登録します。

- Linux: Desktop EntryとIconを`$XDG_DATA_HOME`へ配置します。未設定時は`~/.local/share`を使用します。UbuntuのApplication一覧から起動してDockへ登録できます。
- macOS: `Growse.app`を`~/Applications`へ配置し、`growse` commandから同じApplication Bundleを起動します。
- Windows: 専用Iconを配置し、ユーザーのStart Menu ProgramsへGrowse Shortcutを登録します。

実行中は、リリース確認、環境判定、検証、インストールの進捗を段階ごとに表示します。対話端末ではColor表示になり、CIやFileへのRedirectでは自動的にColorを無効化します。明示的に切り替える場合は`GROWSE_COLOR=always|never`を、Colorを使わない一般的な設定には`NO_COLOR=1`を指定できます。

```sh
wget -qO- https://github.com/Grove-Computing/Growse/releases/latest/download/install.sh | bash
```

Versionとインストール先を指定する場合は、環境変数を利用します。

```sh
wget -qO- https://github.com/Grove-Computing/Growse/releases/latest/download/install.sh | GROWSE_VERSION=v0.15.0 GROWSE_INSTALL_DIR=/usr/local/bin bash
```

GUI Applicationの配置先は、`GROWSE_DATA_HOME`、`GROWSE_APPLICATIONS_DIR`、`GROWSE_WINDOWS_PROGRAMS_DIR`で変更できます。

### Docker imageを使う

Linux amd64のDocker imageを、GitHub Container Registryから取得できます。

```sh
docker pull ghcr.io/grove-computing/growse:v0.15.0
```

GrowseはGUI applicationのため、Containerから起動する場合はホストのDisplay ServerとGPU deviceを接続する必要があります。

## クイックスタート

### 必要なもの

- Go 1.26以降
- Gioが利用するOSのグラフィックス開発ライブラリ

Ubuntu / Debian系では、次のパッケージをインストールします。

```sh
sudo apt-get update
sudo apt-get install -y libvulkan-dev gcc pkg-config libwayland-dev libx11-dev libx11-xcb-dev libxkbcommon-x11-dev libgles2-mesa-dev libegl1-mesa-dev libffi-dev libxcursor-dev
```

ほかのLinuxディストリビューションについては、[GioのLinuxセットアップ手順](https://gioui.org/doc/install/linux)を参照してください。

### 起動する

```sh
go mod download
go run ./cmd/growse
```

起動すると、左側Vertical Tab Rail、戻る・進む・再読込・DevTools・URL入力欄・Gopherボタン・状態表示を備えたブラウザウィンドウが開きます。

## Demoを試す

別のターミナルでDemoを1つ配信し、Growseで`http://localhost:8080`を開きます。

```sh
python3 -m http.server 8080 --directory examples/data-app
```

ほかのDemoへ切り替える場合は、配信するディレクトリを変更します。

| Demo | 配信ディレクトリ | 確認できる機能 |
| --- | --- | --- |
| Counter | `examples/counter` | click Eventによるカウント更新 |
| Todo | `examples/todo` | テキスト入力、Form送信、完了切替、削除 |
| CSS3 Core Showcase | `examples/css3-core` | Custom Property、`calc()`、Media Query、Gradient、Box Model |
| Flexbox Showcase | `examples/flexbox` | grow / shrink、wrap、alignment、gap、auto margin、nested flex |
| Dashboard | `examples/dashboard` | Grid、Position、複数Background、Shadow、Transform、Opacity |
| Animation Showcase | `examples/animation` | hover Transition、複数Keyframes Animation |
| Data App Showcase | `examples/data-app` | Form、WebGo Fetch、structured Headers、Session Cookie、DOM更新、Animation |
| Persistent App Showcase | `examples/persistent-app` | Scheduler、same-document Routing、Local / Session Storage、Fetch、HTTP Cache、offline状態 |
| Multi-Tab Workspace | `go run ./examples/multi-tab-workspace` | Vertical Tab、Storage Event、共有Cookie / Cache、Tab別Session / Scheduler |
| DevTools Showcase | `go run ./examples/devtools` | Console 4 level、DOM / Style / Layout、成功Fetch、redirect、cache、HTTP error、timeout |
| Dual Runtime Showcase | `go run ./examples/dual-runtime` | 同じUIのGo / JavaScript切替、DOM、Event、Timer、Fetch、Storage、History、Runtime error |
| External Web Platform | `go run ./examples/external-web-platform` | 外部classic / Module、dynamic import、WASM、same / cross-origin iframe、Service Worker offline、cross-origin CSS、sandbox |
| Modern Web Compatibility | `go run ./examples/modern-web-compat` | Next.js / SvelteKit SSR、Tailwind CSS、hydration、dynamic chunk、responsive、font / image / SVG、失敗診断 |

WebGoソースは通常のGo build対象から除外するため、各Demoでは`_app.go`として配置しています。

Multi-Tab Workspaceは専用のlocal fixture serverを起動し、Growseで`http://localhost:8080`を開きます。Notes画面のリンクからTasksとActivityを新しいVertical Tabへ開けます。外部ServiceやAPI keyは不要です。

DevTools Showcaseも専用のlocal fixture serverだけを使用します。外部通信や実Credentialなしで、Console、Inspector、Networkの通常・error・timeout・cache状態を再現できます。

Dual Runtime Showcaseもlocalhost内だけで完結します。ツールバーの`Go` / `JS`を押すとPageを完全reloadし、同じHTML/CSS上のCounterとWeb API処理を選択Engineだけで実行します。

External Web Platform Showcaseはtop-level、CDN、Frameを別のlocal Originで配信し、Internet、DNS、Credentialなしで外部サイト経路を再現します。起動後に表示されるURLをJavaScript Engineで開くと、Module / WASM / iframe / Service Worker / sandbox状態を確認できます。

Modern Web Compatibility ShowcaseはNext.js、SvelteKit、Tailwindの生成済み縮約artifactをlocalhostだけで配信します。Go EngineではSSR HTMLとEngine非依存のCSS / image / fontだけを表示し、JavaScriptを取得・実行しません。ツールバーで`JS`を明示選択すると完全reload後にhydration、dynamic chunk、操作、client-side Navigation、responsive表示、DevToolsのfallback診断を確認できます。

## ブラウザの仕組み

### Rendering Pipeline

Growseは取得したHTMLとCSSを、独自のPipelineで画面へ描画します。

```text
HTML / CSS
  → DOM
  → Computed Style
  → Layout Tree
  → Display List
  → Viewport
```

Animation中のPaintとHit Testingは、同じFrameの値を参照します。DOMを変更するとComputed Style以降を再計算し、画面を更新します。

### Navigation

- 左側のVertical Tab RailからTabを作成、選択、終了できます。TabごとにNavigation、History、Scroll、Focus、WebGo Runtime、Scheduler、Session Storageを保持します。
- URL入力欄でEnterを押すか、Gopherボタンを押すと移動します。
- リンクのclick、戻る、進む、履歴を増やさない再読込に対応しています。
- `Ctrl+T`（macOSでは`Command+T`）でTabを作成し、`Ctrl+W`（macOSでは`Command+W`）でactive Tabを終了します。`Ctrl+Tab`と`Ctrl+Shift+Tab`で前後のTabへ切り替えます。
- `Ctrl+R`（macOSでは`Command+R`）で通常の再読込、`Ctrl+Shift+R`（macOSでは`Command+Shift+R`）でHTTP Cacheへ再検証を要求する強制再読込を実行します。
- リンクへカーソルを重ねると、認証情報を除去した遷移先URLを状態表示に示します。
- ウィンドウ内では、青いGopherをマウスカーソルとして表示します。

### Go / JavaScript Dual Runtime

Goでは`<script type="text/go">`をYaegiで、JavaScriptではtype省略、`text/javascript`、`application/javascript`と`type="module"`をgojaで実行します。Go sourceはtrusted loopbackかつsame-originに限定します。JavaScriptは通常のHTTP(S) Pageでinline / external classicとES Modulesを実行し、redirect、MIME、mixed content、CORS、credentials、integrity、sizeをBrowser側で検証します。選択していないEngineのScriptは取得しません。

EngineはTabごとに保持し、切り替え時は旧Runtime、Event、Timer、Fetch、Storage callbackを停止して完全reloadします。Go RuntimeとJavaScript Runtimeを同時実行せず、値やfunctionを共有しません。

- `growse/dom`: DOM、Form、Eventを操作
- `growse/fetch`: Headers、JSON / text / binary / FormData body、AbortController、timeoutを備えた非同期HTTP Requestを実行
- `growse/navigation`: 現在URL、pushState / replaceState、History traversalとEventを操作
- `growse/scheduler`: timeout、interval、Animation Frameを登録・解除
- `growse/storage`: Origin単位のLocal / Session Storageを操作し、`OnChange`で別TabのLocal Storage更新を受信
- `growse/console`: `Log`、`Info`、`Warn`、`Error`をPage単位のDevTools Consoleへ記録
- Fetch callback: PageのEvent Queueで実行
- Page終了時: Timer、Frame callback、実行中Fetchをcancelし、Runtime参照を解放

JavaScriptは`console`、DOM / Event、Timer / Animation Frame、Promise形式`fetch`、Storage、Navigationに加え、ES Modules、dynamic resource、hydration向けDOM / CSSOM / observer、WebAssembly、iframe messaging、Service Worker / Cache Storageの検証済みsubsetを提供します。これらは利用者が`JS`を明示選択したTabだけで有効になり、Go Engineへhost objectやcallbackを公開しません。Node.js、npm、CommonJS、WASI、OS、filesystem、process、Go reflectionは公開しません。詳細は[Runtime / Web API対応表](docs/runtime-support.md)を参照してください。

Go / JavaScript / WASMとService WorkerはBrowser UIとは別のresource-bounded worker processで実行し、version / size制限付きIPCとBrowserが再検証するbrokered APIだけを使用します。必須sandbox状態を検証できない場合はcodeを実行しません。この境界は未知のOS kernel、Go runtime、goja、wazero、decoderの脆弱性まで排除するものではありません。詳細は[Runtime worker / Web Platform設計](docs/runtime-worker-design.md)と[SECURITY.md](SECURITY.md)を参照してください。

### WebGo DevTools

ツールバーの`DevTools`ボタンまたは`F12`で、active Tabの下部へDevToolsを開閉できます。選択panelとfilterはTabごとに保持されます。

- Console: Go / JavaScript Engine、4 level、script / runtime errorを表示し、level filterとclearを提供
- Inspector: active PageのDOM snapshotを選択し、公開attribute、主要Computed Style、Layout Boxをread-only表示
- Network: Navigation、resource、Form、Fetchのmethod、外部Script Engine、redacted URL、timing、status、redirect、cache、size、error categoryを表示
- Runtime: Page / Frame / Service Worker context、script kind / schedule、Engine、generation、有限error category、sandbox capabilityを表示

DevToolsはRequest / Response body、Header、Cookie、Authorizationを保持しません。URL userinfoとquery valueをマスクし、password input valueもInspectorへ公開しません。安全上限と詳細は[WebGo DevTools設計](docs/devtools.md)を参照してください。

## ドキュメント

| 文書 | 内容 |
| --- | --- |
| [CSS対応表](docs/css-support.md) | CSS Property、Layout、Animationの対応状況と制限 |
| [Form / Fetch / Cookie対応表](docs/form-fetch-cookie-support.md) | Form、HTTP、Cookie、CORSの対応状況と制限 |
| [Storage / Cache対応表](docs/storage-cache-support.md) | Web Storage、HTTP Cache、永続化、quotaの対応状況と制限 |
| [Visual Regression Test](docs/visual-regression.md) | 固定Viewport、Font、ScaleによるDashboard、Persistent App、DevTools、framework状態遷移の回帰テスト |
| [Performance Baseline](docs/performance.md) | Layout、Paint、Form、Scheduler、History、Storage、CacheのBenchmark基準値 |
| [WPT由来テスト](docs/wpt.md) | Web Platform Testsから移植したTestと出典 |
| [Developer Supply Chain Security](docs/developer-security.md) | 不可視Code検査、Extension管理、署名、Credential、Incident Response |
| [WebGo DevTools設計](docs/devtools.md) | Console、Inspector、Network、Runtimeのデータ境界、redaction、安全上限 |
| [Runtime / Web API対応表](docs/runtime-support.md) | Go / JavaScript Engine、Script type、API対応、制約、非対応範囲 |
| [Runtime worker / Web Platform設計](docs/runtime-worker-design.md) | worker process、IPC、sandbox、generation、Frame / Service Worker所有境界 |
| [v0.9.0リリース定義](docs/v0.9.0.md) | v0.9.0のTheme、Scope、完了条件 |
| [v0.10.0リリース定義](docs/v0.10.0.md) | v0.10.0のTheme、Scope、完了条件 |
| [v0.11.0リリース定義](docs/v0.11.0.md) | v0.11.0のTheme、Scope、完了条件 |
| [v0.12.0リリース定義](docs/v0.12.0.md) | v0.12.0 WebGo DevToolsのTheme、Scope、完了条件 |
| [v0.13.0リリース定義](docs/v0.13.0.md) | v0.13.0 Go / JavaScript Dual RuntimeのTheme、Scope、完了条件 |
| [v0.14.0リリース定義](docs/v0.14.0.md) | v0.14.0 External JavaScript / Module / WASM / iframe / Service Worker / sandboxのTheme、Scope、完了条件 |
| [v0.15.0リリース定義](docs/v0.15.0.md) | v0.15.0 Modern Web Compatibility、framework CSS / hydration、image / SVG / fontのTheme、Scope、完了条件 |

## 品質チェック

ローカルで、CIと同等の検査を実行できます。

```sh
make ci
```

主な検査項目は次のとおりです。

- `go test -race ./...`
- 最低Test Coverage 70%
- `go vet`
- Staticcheck
- actionlint
- govulncheck
- 不可視Unicode、未承認Binary、Editor Extension recommendationの検査
- Installer、Release成果物、Docker、文書の検証

Go Modules、GitHub Actions、Docker base imageの依存関係は、Dependabotで週次確認します。Pull Requestでは追加されたHigh/Criticalの既知脆弱性をDependency Reviewで遮断します。

## セキュリティ

v0.15.0のRuntime workerは外部JavaScriptとhydration resourceのhost作用をdefault denyにしますが、実験的browserとして未知のnative/runtime/decoder脆弱性への完全耐性は保証しません。外部Go sourceは引き続きtrusted loopbackだけに限定されます。Growseを権限の高いユーザーや機密profileで実行しないでください。

脆弱性の非公開報告方法、サポート対象、通信とResource LoadingのSecurity Boundaryは[SECURITY.md](SECURITY.md)を参照してください。

## リリース成果物

`v0.15.0`のようなVersion tagをpushすると、GitHub Actionsが次の成果物、SHA-256 checksum、SPDX JSON SBOMをGitHub Releaseへ公開します。ArchiveとSBOMにはGitHub Artifact Attestation、Docker imageにはBuildKitのSBOMとSLSA Provenanceを付与します。

- Linux amd64
- macOS Intel
- macOS Apple Silicon
- Windows amd64
- Linux amd64 Docker imageのVersion tagと`latest` tag

Archiveには、Growse本体、Runtime workerとして再起動する同一検証済み実行ファイル、すべてのDemoを同梱します。Gopher cursor imageは実行ファイルへ埋め込まれるため、別途assetを配置する必要はありません。

checksum、SBOM、Provenanceを使った成果物の検証手順は、[SECURITY.mdのSupply Chain Security](SECURITY.md#supply-chain-security)を参照してください。

## Go Gopherのクレジット

The Go Gopher was designed by Renée French.

The Go Gopher is licensed under the Creative Commons Attribution 4.0 License.
