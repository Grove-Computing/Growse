# Growse DevTools

Growse v0.15.0は、active TabのPageを観測するread-only DevToolsを提供する。ツールバーの`DevTools`または`F12`で開閉し、Console、Inspector、Network、Runtimeを切り替える。panel、Console filter、Inspector選択はTabごとに分離する。Headerには選択中のGo / JavaScript EngineとRuntimeのidle / running / stopped / error状態を表示する。

## Console

Goの`growse/console`とJavaScriptの`console`はLog、Info、Warn、Errorを提供する。各recordはPage内sequence、level、Engine、source、messageを持つ。external Script load、compile、runtime start、Event callbackの失敗もsource付きerrorとして同じConsoleへ記録する。

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

Network recordはNavigation、stylesheet、image、font、external Go / JavaScript Script、Module、Form submission、Fetchをresource kind付きで記録する。外部Scriptは`script/go`と`script/javascript`を区別し、requested / final URL、initiator、initial / dynamic schedule、method、開始時刻、duration、status、redirect、cache status、response byte数、error categoryを表示する。

Request / Response bodyとHeaderはObservation型にもNetworkRecord型にも存在しない。Cookie、Authorization、API key、raw error本文を保存しない。URL userinfoを除去し、queryはkeyだけを残して全valueを`[REDACTED]`へ置換する。CORS、timeout、cancel、redirect loop / limit、request / response limit、network failureを限定されたcategoryへ変換する。

- retention: Page 500件
- Browser Session: 4,000件
- lifecycle: Page close後の追加を拒否
- clear: 表示中Pageのrecordだけを削除し、sequenceとSession budgetは巻き戻さない

## Runtime

Runtime panelはtop-level Page、再帰的なFrame、same-originのService Worker registrationをcontextごとに表示する。各rowはcontext kind / ID / parent ID、browsing generation、Engine、state、worker generation、sandbox ready / process / failure、適用constraint数、script kind / schedule / location、有限error categoryを持つ。

Module / chunk / hydration / observer / stale generation / host API / WASM / sandbox / runtime / frame errorはcategoryだけを表示し、raw messageやsourceを保持しない。URLはuserinfoとquery value、fragmentを除去する。inline source、module namespace、WASM binary / memory、Service Worker Cache body、IPC payload、environment、filesystem pathは診断modelへ入れない。

Runtime contextのcompatibility diagnosticsはdynamic resourceのinitiator / schedule、Styles ruleのlayerと適用状態、selector / media / supports / containerによる無視理由、font / image fallbackの有限categoryを表示する。診断はrule番号やresource metadataだけを参照し、CSS source body、font bytes、decoded image、raw exceptionを複製しない。

worker generationはPage reload、Frame navigation、Service Worker restartを識別するための単調増加IDであり、process IDや秘密値ではない。sandbox constraintはworkerが適用・報告しBrowserが検証した件数とready状態を示し、適用できなかったOS機能を成功として表示しない。

## 再現と検証

`go run ./examples/devtools`はlocalhostだけでConsole 4 level、DOM mutation、Computed Style、Layout、成功Fetch、redirect、cache hit、HTTP 503、timeoutを再現する。`go run ./examples/external-web-platform`はPage / Frame / Service Worker、classic / Module / WASM、複数worker generation、sandbox状態を再現する。fixtureには意図的なquery / Header / password credentialが含まれ、Integration TestはConsole、Inspector、Network、Runtime snapshotに値が残らないことを確認する。

通常・空・error・truncated状態のsemantic visual snapshotは`internal/ui/testdata/devtools-panels.golden.json`で固定する。安全上限、並行access、Tab / Page lifecycleは`internal/devtools`と`internal/browser`のUnit / Integration Testで検証する。
