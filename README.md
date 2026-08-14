# Growse

Go をクライアントサイド言語として実行する、実験的な Web ブラウザです。

## 開発環境のセットアップ

Go 1.26 以降と、Gio が利用する OS のグラフィックス開発ライブラリを用意します。Ubuntu / Debian 系では少なくとも Vulkan 開発ヘッダが必要です。

```sh
sudo apt-get update
sudo apt-get install -y libvulkan-dev gcc pkg-config libwayland-dev libx11-dev libx11-xcb-dev libxkbcommon-x11-dev libgles2-mesa-dev libegl1-mesa-dev libffi-dev libxcursor-dev libvulkan-dev
```

ほかの Linux ディストリビューションを使う場合は、[Gio の Linux セットアップ手順](https://gioui.org/doc/install/linux) を参照してください。

依存関係を取得して起動します。

```sh
go mod download
go run ./cmd/growse
```

Demoは、別のターミナルでいずれか1つを次のように配信します。

```sh
python3 -m http.server 8080 --directory examples/counter
python3 -m http.server 8080 --directory examples/todo
python3 -m http.server 8080 --directory examples/css3-core
python3 -m http.server 8080 --directory examples/flexbox
python3 -m http.server 8080 --directory examples/dashboard
python3 -m http.server 8080 --directory examples/animation
python3 -m http.server 8080 --directory examples/data-app
```

Growseで`http://localhost:8080`を開くと、Counterではクリックによるカウント更新を、Todoではテキスト入力、フォーム送信、完了切替、削除を確認できます。CSS3 Core Showcaseでは、Custom Property、`calc()`、Media Query、Gradient、Box Model、角丸、Opacity、Text Decoration、Overflowを確認できます。Flexbox Showcaseでは、grow/shrink、wrap、alignment、gap、auto margin、inline-flex、nested flexを確認できます。Dashboardでは、Grid、named area、Position、複数Background、Gradient、Shadow、Transform、Opacityを組み合わせた画面を確認できます。Animation Showcaseでは、Transitionのhover反転と、Opacity・Color・2D Transformを使う複数Keyframes Animationを確認できます。Data App Showcaseでは、Form送信、WebGo Fetch、Session Cookie、通信結果によるDOM更新とAnimationを一つの画面で確認できます。WebGoソースはGoツールによる通常ビルドの対象外にするため、`_app.go`として配置しています。

起動すると、戻る・進む・再読込・URL入力欄・Gopherボタン・状態表示を備えたブラウザウィンドウが表示され、ウィンドウ内のマウスカーソルは青いGopherになります。リンクへカーソルを重ねると、認証情報を伏せた解決済み遷移先URLを状態表示で確認できます。リンクのクリック、戻る・進む、履歴を増やさない再読込に対応しています。

URLを入力してEnterを押すかGopherボタンを押すとHTMLと同一オリジンのCSSを取得し、Growse独自DOM・Computed Style・Layout Tree・Display Listを経由してViewportへ描画します。v0.7.0ではCSS Transition、`@keyframes` Animation、Easing、繰り返し、Direction、Fill Mode、Pause、`prefers-reduced-motion`を扱います。v0.8.0ではForm Controls、Constraint Validation、GET/POST Submission、WebGo FetchとHTTP Lifecycle、Cookie、Same-Origin Policy、CORSを追加しました。詳細な対応範囲と制限は[CSS対応表](docs/css-support.md)と[Form / Fetch / Cookie対応表](docs/form-fetch-cookie-support.md)を参照してください。

`<script type="text/go">`のインラインソースと外部`.go`ファイルはPageへ読み込み、localhost・127.0.0.1・`::1`のページではYaegi Runtimeで`main()`を実行します。WebGoスクリプトは`growse/dom`からDOMとFormを操作し、`growse/fetch`から非同期HTTPリクエストを開始できます。FetchのcallbackはPageのイベントキューで実行され、Page終了時には通信をcancelします。DOM変更後はComputed Styleを再計算して画面を更新します。

## 品質チェック

GitHub Actionsでは、Linuxのraceテストと70%の最低カバレッジ、`go vet`、Staticcheck、actionlint、govulncheckを実行します。Go ModulesとGitHub ActionsはDependabotで週次確認します。

固定viewport/font/scaleのVisual Regressionと、Layout・Paint benchmarkの実行方法は[Visual Regression Test](docs/visual-regression.md)と[Performance Baseline](docs/performance.md)に記録しています。

ローカルでは必要な[GioのLinux依存パッケージ](https://gioui.org/doc/install/linux)を導入した上で、次のコマンドから同等の検査を実行できます。

```sh
make ci
```

## セキュリティ

WebGo Runtimeは、信頼できないGoコードを安全に実行するSandboxではありません。信頼できるローカルページだけを開いてください。脆弱性の非公開報告方法とサポート対象は[SECURITY.md](SECURITY.md)を参照してください。

## リリース

`v0.8.0`のようなバージョンタグをpushすると、GitHub Actionsが各OSでテストを実行し、Linux amd64、macOS Intel、macOS Apple Silicon、Windows amd64向けのアーカイブとSHA-256チェックサムをGitHub Releaseへ公開します。Counter、Todo、CSS3 Core Showcase、Flexbox Showcase、Dashboard、Animation Showcase、Data App Showcaseも成果物へ同梱します。カーソル画像は実行ファイルへ埋め込まれるため、別途アセットを配置する必要はありません。

Linux、macOS、Git Bashを利用できるWindowsでは、次のコマンドで最新版を`~/.local/bin`へインストールできます。ダウンロードしたアーカイブはインストール前にSHA-256チェックサムを検証します。

```sh
wget -qO- https://github.com/Grove-Computing/Growse/releases/latest/download/install.sh | bash
```

特定バージョンやインストール先を指定する場合は環境変数を利用します。

```sh
wget -qO- https://github.com/Grove-Computing/Growse/releases/latest/download/install.sh | GROWSE_VERSION=v0.8.0 GROWSE_INSTALL_DIR=/usr/local/bin bash
```

同時にLinux amd64のDockerイメージをGitHub Container Registryへ、バージョンタグと`latest`タグで公開します。

```sh
docker pull ghcr.io/grove-computing/growse:v0.8.0
```

GrowseはGUIアプリケーションのため、コンテナから起動する場合はホストのディスプレイサーバーとGPUデバイスをコンテナへ接続する必要があります。

## Go Gopher のクレジット

The Go Gopher was designed by Renée French.
The Go Gopher is licensed under the Creative Commons Attribution 4.0 License.
