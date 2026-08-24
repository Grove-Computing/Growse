# Runtime / Web API対応表

この表はGrowse v0.13.0の実装を基準とする。GrowseはGoを既定Engineとし、Tab単位でJavaScriptへ切り替えられる。切り替えは旧Runtimeを停止する完全なPage reloadであり、両Engineを同時には実行しない。

## EngineとScript

| 項目 | Go | JavaScript |
| --- | --- | --- |
| Runtime | Yaegi | goja |
| Script type | `text/go` | type省略、空type、`text/javascript`、`application/javascript` |
| 既定選択 | 対応 | 非対応。Tabの`JS` selectorで明示的に選択する |
| inline / external | 対応 | 対応 |
| 実行順 | 選択EngineのScriptを文書順に実行 | 選択EngineのScriptを文書順に実行 |
| Engine切り替え | 旧Runtimeを停止して完全reload | 旧Runtimeを停止して完全reload |

自動実行はlocalhost、127.0.0.1、`::1`のHTTP(S) Pageに限定する。external ScriptはDocumentとsame-originで、redirect後の最終URLも信頼済みでなければならない。選択していないEngineのexternal Scriptは取得しない。

Scriptは1件2 MiB、選択Engineあたり64件、Page合計8 MiBを上限とする。Runtime、値、function、listenerはTab間またはEngine間で共有しない。

## Web API

| API | Go | JavaScript | v0.13.0の範囲 |
| --- | --- | --- | --- |
| Console | `growse/console` | `console.log`、`info`、`warn`、`error` | Engine付きrecord、1件4 KiB、Page 1,000件 |
| DOM検索・生成 | `growse/dom` | `document.getElementById`、`querySelector`、`createElement` | 対応selectorと安全なNode wrapperに限定 |
| Element | `growse/dom` | `textContent`、`value`、`getAttribute`、`setAttribute`、`appendChild`、`remove`、`classList.add` / `remove` | 同じDOM mutation経路を使用 |
| Event | `growse/dom` | Elementの`addEventListener` | click、input、change、submit、reset、focus、blur、mouseenter、mouseleave、`preventDefault` |
| Scheduler | `growse/scheduler` | `setTimeout`、`setInterval`、clear、`requestAnimationFrame`、cancel | 文字列callbackは拒否し、Page終了時に解除 |
| Fetch | `growse/fetch` callback | Promise形式`fetch`、`AbortController` | method、headers object、text body、credentials、timeout、abort、`Response.text()` / `json()` |
| Storage | `growse/storage` | `localStorage`、`sessionStorage` | get / set / remove / clear / key / length、same-origin `storage` Event |
| Navigation | `growse/navigation` | read-only `location`、`location.assign`、`history` | back / forward / go / pushState / replaceState、popstate、hashchange |

GoとJavaScriptはGrowseのDOM、Scheduler、Network、Storage、Navigation基盤を共有するため、Origin policy、quota、callback budget、History上限、Fetch上限、およびPage lifecycleも同じ規則に従う。JavaScript callbackはPageごとの単一直列queueで実行し、Page closeまたはEngine切り替え後には配送しない。

## JavaScript Fetchの範囲

`fetch(input, init)`はPromiseを返す。`input`はstring URL、`init`はmethod、headers object、text body、`omit` / `same-origin` / `include` credentials、GrowseのAbortSignal、millisecond timeoutを扱う。

Responseは`status`、`statusText`、`url`、`redirected`、`ok`、read-only headersと、一度だけ消費できる`text()` / `json()` Promiseを提供する。Same-Origin Policy、CORS、Cookie、forbidden Header、Request 1 MiB、Response 4 MiB、Header 100件 / 64 KiB、redirect 10回、Page 16件・Session 128件の同時Fetch上限をGoと共有する。

## JavaScriptで公開しない機能

- Node.js、npm、CommonJS、ECMAScript module、dynamic import、import map
- OS、filesystem、process、Go reflection、Go package import
- TypeScript、JSX、WebAssembly、Service Worker、Web Worker
- ReadableStream、WebSocket、WebRTC、Canvas、iframe、Shadow DOM、custom elements
- DevTools REPL、source debugger、breakpoint、heap profiler

gojaの言語機能全体や既存Webサイトとの互換性は保証しない。Growseが提供するJavaScript host APIはこの文書に記載した範囲だけである。

## Security Boundary

YaegiとgojaはProcess Sandboxではない。JavaScript host surfaceからOSやfilesystemを公開しないこと、trusted loopback Pageとsame-origin Scriptだけを自動実行すること、PageごとのVMとlifecycleを分離することは攻撃面を縮小するが、CPU、memory、Processを隔離しない。Go / JavaScriptのどちらでも、信頼できるローカルPageだけを開く。
