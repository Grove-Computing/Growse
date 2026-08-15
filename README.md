# Growse

Growseは、Goをクライアントサイド言語として実行する実験的なWebブラウザです。

HTMLとCSSで画面を構築し、`<script type="text/go">`に書いたWebGoからDOM、Form、HTTP通信を操作できます。

## Growseでできること

| 項目 | 主な機能 |
| --- | --- |
| ブラウジング | URL入力、リンク遷移、same-document Routing、戻る、進む、再読込、Scroll復元 |
| HTML / DOM | 要素の検索・生成・追加・削除、属性・class・Form値の操作 |
| CSS Layout | Box Model、Flexbox、Grid、Position、Overflow |
| CSS Paint | Color、Gradient、複数Background、Shadow、Opacity、2D Transform |
| Animation | CSS Transition、`@keyframes`、Easing、Iteration、Direction、Fill Mode、Pause、`prefers-reduced-motion` |
| Form | Form Controls、Focus、Constraint Validation、GET / POST Submission |
| Scheduler | timeout、interval、Animation Frame、Page終了時の自動解除 |
| Storage | Origin分離された永続Local StorageとPage Session単位のSession Storage |
| HTTP | WebGo Fetch、Cookie、Same-Origin Policy、CORS、Freshness・再検証・Disk対応のHTTP Cache |
| WebGo | DOM Event、非同期Fetch、Scheduler、History、Navigation、Storage API |

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
wget -qO- https://github.com/Grove-Computing/Growse/releases/latest/download/install.sh | GROWSE_VERSION=v0.9.0 GROWSE_INSTALL_DIR=/usr/local/bin bash
```

GUI Applicationの配置先は、`GROWSE_DATA_HOME`、`GROWSE_APPLICATIONS_DIR`、`GROWSE_WINDOWS_PROGRAMS_DIR`で変更できます。

### Docker imageを使う

Linux amd64のDocker imageを、GitHub Container Registryから取得できます。

```sh
docker pull ghcr.io/grove-computing/growse:v0.9.0
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

起動すると、戻る・進む・再読込・URL入力欄・Gopherボタン・状態表示を備えたブラウザウィンドウが開きます。

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
| Data App Showcase | `examples/data-app` | Form、WebGo Fetch、Session Cookie、DOM更新、Animation |
| Persistent App Showcase | `examples/persistent-app` | Scheduler、same-document Routing、Local / Session Storage、Fetch、HTTP Cache、offline状態 |
| Multi-Tab Workspace | `go run ./examples/multi-tab-workspace` | Vertical Tab、Storage Event、共有Cookie / Cache、Tab別Session / Scheduler |

WebGoソースは通常のGo build対象から除外するため、各Demoでは`_app.go`として配置しています。

Multi-Tab Workspaceは専用のlocal fixture serverを起動し、Growseで`http://localhost:8080`を開きます。Notes画面のリンクからTasksとActivityを新しいVertical Tabへ開けます。外部ServiceやAPI keyは不要です。

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

- URL入力欄でEnterを押すか、Gopherボタンを押すと移動します。
- リンクのclick、戻る、進む、履歴を増やさない再読込に対応しています。
- `Ctrl+R`（macOSでは`Command+R`）で通常の再読込、`Ctrl+Shift+R`（macOSでは`Command+Shift+R`）でHTTP Cacheへ再検証を要求する強制再読込を実行します。
- リンクへカーソルを重ねると、認証情報を除去した遷移先URLを状態表示に示します。
- ウィンドウ内では、青いGopherをマウスカーソルとして表示します。

### WebGo Runtime

インラインの`<script type="text/go">`と外部`.go`ファイルをPageへ読み込み、localhost、127.0.0.1、`::1`のページでYaegi Runtimeの`main()`を実行します。

- `growse/dom`: DOM、Form、Eventを操作
- `growse/fetch`: 非同期HTTP Requestを実行
- `growse/navigation`: 現在URL、pushState / replaceState、History traversalとEventを操作
- `growse/scheduler`: timeout、interval、Animation Frameを登録・解除
- `growse/storage`: Origin単位のLocal / Session Storageを操作
- Fetch callback: PageのEvent Queueで実行
- Page終了時: Timer、Frame callback、実行中Fetchをcancelし、Runtime参照を解放

WebGo RuntimeはSandboxではありません。信頼できるローカルページだけを開いてください。

## ドキュメント

| 文書 | 内容 |
| --- | --- |
| [CSS対応表](docs/css-support.md) | CSS Property、Layout、Animationの対応状況と制限 |
| [Form / Fetch / Cookie対応表](docs/form-fetch-cookie-support.md) | Form、HTTP、Cookie、CORSの対応状況と制限 |
| [Storage / Cache対応表](docs/storage-cache-support.md) | Web Storage、HTTP Cache、永続化、quotaの対応状況と制限 |
| [Visual Regression Test](docs/visual-regression.md) | 固定Viewport、Font、ScaleによるDashboardとPersistent Appの画像回帰テスト |
| [Performance Baseline](docs/performance.md) | Layout、Paint、Form、Scheduler、History、Storage、CacheのBenchmark基準値 |
| [WPT由来テスト](docs/wpt.md) | Web Platform Testsから移植したTestと出典 |
| [Developer Supply Chain Security](docs/developer-security.md) | 不可視Code検査、Extension管理、署名、Credential、Incident Response |
| [v0.9.0リリース定義](docs/v0.9.0.md) | v0.9.0のTheme、Scope、完了条件 |

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

GrowseとWebGo Runtimeは、信頼できないGoコードを安全に実行するSandboxではありません。信頼できないWebGoソースを開いたり、権限の高いユーザーで実行したりしないでください。

脆弱性の非公開報告方法、サポート対象、通信とResource LoadingのSecurity Boundaryは[SECURITY.md](SECURITY.md)を参照してください。

## リリース成果物

`v0.9.0`のようなVersion tagをpushすると、GitHub Actionsが次の成果物、SHA-256 checksum、SPDX JSON SBOMをGitHub Releaseへ公開します。ArchiveとSBOMにはGitHub Artifact Attestation、Docker imageにはBuildKitのSBOMとSLSA Provenanceを付与します。

- Linux amd64
- macOS Intel
- macOS Apple Silicon
- Windows amd64
- Linux amd64 Docker imageのVersion tagと`latest` tag

Archiveには、Growse本体とすべてのDemoを同梱します。Gopher cursor imageは実行ファイルへ埋め込まれるため、別途assetを配置する必要はありません。

checksum、SBOM、Provenanceを使った成果物の検証手順は、[SECURITY.mdのSupply Chain Security](SECURITY.md#supply-chain-security)を参照してください。

## Go Gopherのクレジット

The Go Gopher was designed by Renée French.

The Go Gopher is licensed under the Creative Commons Attribution 4.0 License.
