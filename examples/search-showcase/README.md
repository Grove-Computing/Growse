# Growse Search Showcase

v0.21.0の検索機能を、Public Internet・実credential・利用者の実profileなしで確認するnative Showcaseです。起動ごとにlocalhost TLS serverと一時profileを作成し、window終了時に破棄します。

## 起動

```sh
go run ./examples/search-showcase
```

windowは1280×800で開きます。desktop evidenceはこのsize、narrow evidenceは720×640（Browser viewport 496 px）を基準にしています。windowをresizeしてpopup、Search panel、Home card、shortcut gridがviewport内に残ることを確認できます。

## Omnibox・Home・open disposition

1. `Ctrl+L` / `Cmd+L`を押して`guide`を入力します。
2. Input、Tab、History、Bookmark、fake remote suggestionの全sourceと選択表示を確認します。
3. `Enter`、`Shift+Enter`、`Alt+Enter`でcurrent、new foreground、new background Tabを開きます。
4. Home actionで内部Homeへ戻り、中央検索欄へ同じ`guide`を入力して共通rankingを確認します。
5. `@tabs guide`、`@history guide`、`@bookmarks guide`、`demo gopher browser`でscopeとoffline providerを確認します。

## Find in Page

1. `Find mutation` Tabを選択し、`Ctrl+F` / `Cmd+F`を開きます。
2. `interoperability`、`検索対象`、`Café`、`nested needle`を順に検索します。
3. `nested needle`で内側のscroll containerがactive matchまで移動することを確認します。
4. `DOM mutationを追加`を押し、`mutated needle`が再検索で現れることを確認します。

## failure・削除・profile復旧

1. 検索設定でremote suggestionをoff / onし、off時に進行中requestがcancelされることを確認します。
2. `timeout`と`oversize`を入力し、2秒timeoutまたは256 KiB response上限後も入力とlocal候補が残ることを確認します。
3. `Ctrl+Shift+A` / `Cmd+Shift+A`でSearch panelを開き、History・Bookmarkの個別削除と確認付き全History削除を試します。
4. Homeの`ショートカットを編集`でshortcut・順序・背景を変更します。変更はShowcase専用一時profileへatomic保存されます。

自動testはprovider opt-in/off、timeout、oversize、削除とindex同期、検索profileがderived indexを永続化しないこと、Home設定のbackup復旧、desktop / narrow visual evidenceを固定します。
