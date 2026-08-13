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

Counter Demoは別のターミナルで次のように配信します。

```sh
python3 -m http.server 8080 --directory examples/counter
```

Growseで`http://localhost:8080`を開き、`+`ボタンを押すとWebGoのクリックハンドラーがカウントを更新します。WebGoソースはGoツールによる通常ビルドの対象外にするため、`_app.go`として配置しています。

起動すると、戻る・進む・再読込・URL入力欄・Gopher ボタンを備えたブラウザウィンドウが表示されます。リンクのクリック、戻る・進む、履歴を増やさない再読込に対応しています。URLを入力してGopherボタンを押すとHTMLと同一オリジンのCSSを取得し、Growse独自DOM・Computed Style・Layout Tree・Display Listを経由してViewportへ描画します。現在はタグ・`.class`・`#id`・`tag.class`セレクタと、文字色・背景色・フォントサイズ・太さ・`display`・margin・paddingに対応しています。インライン要素はスタイル付きText Runとして行内へ配置し、簡易的に折り返します。`<script type="text/go">` のインラインソースと外部`.go`ファイルはPageへ読み込み、localhost・127.0.0.1・`::1` のページではYaegi Runtimeで`main()`を実行します。WebGoスクリプトは`growse/console`の`Log`でログを出力でき、`growse/dom`の`GetElementByID`・`Text`・`SetText`でDOMテキストを操作できます。`growse/strconv.Itoa`による整数変換と、`OnClick`で登録したGoハンドラーへのクリック配信にも対応し、DOM変更後はComputed Styleを再計算して画面を更新します。

## 品質チェック

GitHub Actionsでは、Linuxのraceテストと70%の最低カバレッジ、`go vet`、Staticcheck、actionlint、govulncheckを実行します。Go ModulesとGitHub ActionsはDependabotで週次確認します。

ローカルでは必要な[GioのLinux依存パッケージ](https://gioui.org/doc/install/linux)を導入した上で、次のコマンドから同等の検査を実行できます。

```sh
make ci
```

## セキュリティ

WebGo Runtimeは、信頼できないGoコードを安全に実行するSandboxではありません。信頼できるローカルページだけを開いてください。脆弱性の非公開報告方法とサポート対象は[SECURITY.md](SECURITY.md)を参照してください。

## リリース

`v0.1.0`のようなバージョンタグをpushすると、GitHub ActionsがLinux amd64、macOS Intel、macOS Apple Silicon、Windows amd64向けのアーカイブとSHA-256チェックサムをGitHub Releaseへ公開します。

Linux、macOS、Git Bashを利用できるWindowsでは、次のコマンドで最新版を`~/.local/bin`へインストールできます。ダウンロードしたアーカイブはインストール前にSHA-256チェックサムを検証します。

```sh
wget -qO- https://github.com/saku0512/growse/releases/latest/download/install.sh | bash
```

特定バージョンやインストール先を指定する場合は環境変数を利用します。

```sh
wget -qO- https://github.com/saku0512/growse/releases/latest/download/install.sh | GROWSE_VERSION=v0.1.0 GROWSE_INSTALL_DIR=/usr/local/bin bash
```

同時にLinux amd64のDockerイメージをGitHub Container Registryへ、バージョンタグと`latest`タグで公開します。

```sh
docker pull ghcr.io/saku0512/growse:v0.1.0
```

GrowseはGUIアプリケーションのため、コンテナから起動する場合はホストのディスプレイサーバーとGPUデバイスをコンテナへ接続する必要があります。

## Go Gopher のクレジット

The Go Gopher was designed by Renée French.
The Go Gopher is licensed under the Creative Commons Attribution 4.0 License.
