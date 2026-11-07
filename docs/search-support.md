# Search対応表 (v0.21.0)

Growse v0.21.0のOmnibox、検索ホーム、Find in Page、Tab / History / Bookmark検索の対応範囲です。検索provider固有の設定は[Search Provider](search-providers.md)、offline検証手順は[Search Showcase](../examples/search-showcase/README.md)を参照してください。

## 検索surface

| Surface | 開き方 | Source / 対象 | 主な動作 |
| --- | --- | --- | --- |
| Omnibox | `Ctrl+L` / `Cmd+L` | 入力action、開いているTab、History、Bookmark、opt-inしたremote suggestion | URL / query / command分類、preview、最大12件表示、current / foreground / background Tabで実行 |
| 検索ホーム | 空Tab、新規Tab、Home action | Omniboxと同じsnapshot、分類、provider、ranking | 最大10件の固定shortcut、単色と4種類以内の同梱背景preset、Tab別query / selection |
| Find in Page | `Ctrl+F` / `Cmd+F` | active Tabの可視text node | Unicode substring、case sensitive、whole word、次 / 前 / wrap、全matchとactive matchのhighlight |
| Search panel | `Ctrl+Shift+A` / `Cmd+Shift+A` | All、Tabs、History、Bookmarks | 既存Tabへ切替、History / Bookmarkを選択したTabへ表示、個別削除、確認付き全History削除 |

Omniboxの先頭に`@tabs`、`@history`、`@bookmarks`、`@search`を置くとsourceを限定できます。通常query内の`@`はcommandとして扱いません。URL、localhost、IPv4 / IPv6、明示schemeはNavigationとして分類し、それ以外の自然言語、CJK、emojiは検索queryとして扱います。曖昧または危険なURL形状は可視errorにし、外部providerへfallbackしません。

## Keyboard shortcut

`Ctrl`はLinux / Windows、`Cmd`はmacOSです。

| 操作 | Shortcut |
| --- | --- |
| Omniboxへfocus | `Ctrl+L` / `Cmd+L` |
| 新規Tab / active Tabを閉じる | `Ctrl+T` / `Cmd+T`、`Ctrl+W` / `Cmd+W` |
| Tabを前後へ切替 | `Ctrl+Tab`、`Ctrl+Shift+Tab`（macOSは`Cmd`） |
| Bookmarkを追加 / 削除 | `Ctrl+D` / `Cmd+D` |
| Find in Page | `Ctrl+F` / `Cmd+F` |
| Findの次 / 前 | `Enter`または`F3`、`Shift+Enter`または`Shift+F3` |
| 横断Search panel | `Ctrl+Shift+A` / `Cmd+Shift+A` |
| 通常 / cache再検証reload | `Ctrl+R`、`Ctrl+Shift+R`（macOSは`Cmd`） |
| 候補を移動 | `ArrowUp` / `ArrowDown`、`PageUp` / `PageDown`、`Home` / `End` |
| 候補を閉じる | `Escape`（Omniboxでは`Tab`でも入力を保持して閉じる） |

Omnibox、検索ホーム、Search panelでは`Enter`が現在Tab、`Shift+Enter`が新規foreground Tab、`Alt+Enter`が新規background Tabです。Tab候補は新規Tabを作らず既存Tabへ切り替えます。IME composition中のEnter、矢印、Escapeは検索操作に使わず、確定後のtextだけを検索します。

## 検索ホーム、shortcut、背景

検索ホームはBrowser所有の内部viewであり、表示だけでは外部Page、favicon、previewを取得しません。shortcutはHTTP(S) URLだけを保存し、title / URL編集、削除、並び順変更、正規化URLの重複統合に対応します。上限は10件、title 128 Unicode scalar value、URL 4,096 UTF-8 bytesです。

背景は単色と同梱presetだけを使用し、外部背景URL、画像upload、動画には対応しません。shortcutの内容・順序と背景選択は`home-settings.json`へatomic保存し、直前の有効backupまたは同梱defaultへ復旧します。検索query、候補response、Page本文はホーム設定へ保存しません。

## Privacyと保存範囲

- remote suggestionは既定で無効です。明示的に有効化したproviderへだけ、2,048 UTF-8 bytes以内のqueryを送ります。
- 空入力、command、URL / credentialらしい入力、localhost / IP、制御文字を含む入力はsuggestion endpointへ送りません。localのTab、History、Bookmark候補も送信しません。
- suggestionは150 ms以上debounceし、active generation 1件、timeout 2秒、redirect 3回、response 256 KiBまでです。無効化、入力変更、Tab切替、popup / window closeでcancelします。
- Historyは最大50,000件、Bookmarkは最大10,000件です。派生indexは再構築し、queryやPage本文を永続化しません。
- provider設定、History / Bookmark、ホーム設定はOS標準のGrowse profileへ保存します。query、suggestion response、local検索dataをapplication log、crash message、telemetryへ複製しません。v0.21.0はtelemetryを導入しません。

## Find in Pageの境界

可視text nodeをDOM順に検索し、CJKとcombining sequenceを扱います。`script`、`style`、form control内部、`display:none`、`visibility:hidden`は対象外です。最大10,000 match、検索対象16 MiBで、DOM mutation、font完了、layout、resize後にrangeを再構築します。Shadow DOM、generated content、canvas、画像OCR、PDF、正規表現、fuzzy検索は対象外です。

## 検証

```sh
GOTOOLCHAIN=go1.26.10 make v021-search-regression-test
go run ./examples/search-showcase
```

自動suiteはURL / Unicode differential、keyboard / IME、privacy、storage recovery、50,000件History rankingを検証します。Showcaseはlocalhost TLS serverと一時profileだけを使用し、Public Internet、実credential、利用者の実profileへ接続しません。
