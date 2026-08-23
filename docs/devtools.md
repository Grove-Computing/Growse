# WebGo DevTools

Growse v0.12.0は、active TabのPageを観測するread-only DevToolsを提供する。ツールバーの`DevTools`または`F12`で開閉し、Console、Inspector、Networkを切り替える。panel、Console filter、Inspector選択はTabごとに分離する。

## Console

WebGoの`growse/console`は`Log`、`Info`、`Warn`、`Error`を提供する。各recordはPage内sequence、level、source、messageだけを持つ。external script load、compile、runtime startの失敗もsource付きerrorとして同じConsoleへ記録する。

- message: 1件4 KiB
- retention: Page 1,000件
- lifecycle: Page close時に破棄し、遅延callbackからの追記を拒否

## Inspector

InspectorはBrowser RuntimeのPage event queue上でDOMからsnapshotを生成し、生のNodeやRuntime objectをUIへ渡さない。親子順のNode一覧、公開attribute、主要Computed Style、Layout Treeのdocument-coordinate Boundsを表示する。DOM mutation、style revision、viewport変更後は次のframeで再生成し、切断された選択Nodeを解除する。

- Node: 2,000件
- depth: 128
- attribute: 1 element 64件
- name / value / text: 4 KiB
- password input: `value`を`[REDACTED]`へ置換し、live control valueを取得しない

## Network

Network recordはNavigation、stylesheet、image、external WebGo script、Form submission、WebGo Fetchをresource kind付きで記録する。method、開始時刻、duration、status、redirect、cache status、response byte数、error categoryを表示する。

Request / Response bodyとHeaderはObservation型にもNetworkRecord型にも存在しない。Cookie、Authorization、API key、raw error本文を保存しない。URL userinfoを除去し、queryはkeyだけを残して全valueを`[REDACTED]`へ置換する。CORS、timeout、cancel、redirect loop / limit、request / response limit、network failureを限定されたcategoryへ変換する。

- retention: Page 500件
- Browser Session: 4,000件
- lifecycle: Page close後の追加を拒否
- clear: 表示中Pageのrecordだけを削除し、sequenceとSession budgetは巻き戻さない

## 再現と検証

`go run ./examples/devtools`はlocalhostだけでConsole 4 level、DOM mutation、Computed Style、Layout、成功Fetch、redirect、cache hit、HTTP 503、timeoutを再現する。fixtureには意図的なquery / Header / password credentialが含まれ、Integration TestはConsole、Inspector、Network snapshotに値が残らないことを確認する。

通常・空・error・truncated状態のsemantic visual snapshotは`internal/ui/testdata/devtools-panels.golden.json`で固定する。安全上限、並行access、Tab / Page lifecycleは`internal/devtools`と`internal/browser`のUnit / Integration Testで検証する。
