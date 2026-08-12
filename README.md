# Growse

Go をクライアントサイド言語として実行する、実験的な Web ブラウザです。

## 開発環境のセットアップ

Go 1.26 以降と、Gio が利用する OS のグラフィックス開発ライブラリを用意します。Ubuntu / Debian 系では少なくとも Vulkan 開発ヘッダが必要です。

```sh
sudo apt-get update
sudo apt-get install -y libvulkan-dev
```

ほかの Linux ディストリビューションを使う場合は、[Gio の Linux セットアップ手順](https://gioui.org/doc/install/linux) を参照してください。

依存関係を取得して起動します。

```sh
go mod download
go run ./cmd/growse
```

起動すると、戻る・進む・再読込・URL入力欄・Gopher ボタンを備えたブラウザウィンドウが表示されます。URLを入力してGopherボタンを押すとHTMLと同一オリジンのCSSを取得し、Growse独自DOM・Computed Style・Layout Tree・Display Listを経由してViewportへ描画します。現在はタグ・`.class`・`#id`・`tag.class`セレクタと、文字色・背景色・フォントサイズ・太さ・`display`・margin・paddingに対応しています。

## Go Gopher のクレジット

The Go Gopher was designed by Renée French.
The Go Gopher is licensed under the Creative Commons Attribution 4.0 License.
