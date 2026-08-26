# Runtime / Web API対応表

この表はGrowse v0.14.0の実装を基準とする。GrowseはGoを既定Engineとし、Tab単位でJavaScriptへ切り替えられる。切り替えはtop-level、Frame、WASM、pending callbackを停止する完全なPage reloadであり、両Engineのobjectを共有しない。

## EngineとScript

| 項目 | Go | JavaScript |
| --- | --- | --- |
| Runtime | Yaegi worker | goja worker |
| Script type | `text/go` | type省略、空type、`text/javascript`、`application/javascript`、`module` |
| 既定選択 | 対応 | Tabの`JS` selectorで明示的に選択 |
| inline / external | 対応 | 対応 |
| external Origin | trusted loopbackかつsame-originだけ | HTTP(S)のsame-origin / cross-origin classic、CORS必須Module |
| 実行順 | 文書順 | parser-blocking、`defer` / Module、`async`を区別 |
| 実行境界 | Page / Frameごとの別process | Page / Frame / Service Workerごとの別process |

選択していないEngineのScriptは取得しない。Go sourceは明示的な`text/go`に限定し、redirect後もtrusted loopbackかつsame-originでなければ実行しない。JavaScriptは通常のHTTP(S) Pageで実行できるが、redirect、status、MIME、mixed content、credentials、CORS、integrity、sizeを実行前に再検査する。

classic / module sourceは1件2 MiB、選択Engineあたり64件、Page合計8 MiBを上限とする。Module graphは256 module、合計16 MiB、深さ64、import mapは1件・1,024 mapping・256 KiBを上限とする。

## Web API

| API | Go | JavaScript | v0.14.0の範囲 |
| --- | --- | --- | --- |
| Console | `growse/console` | `console.log`、`info`、`warn`、`error` | Engine / context付きrecord、1件4 KiB、Page 1,000件 |
| DOM検索・生成 | `growse/dom` | `getElementById`、`querySelector(All)`、`getElementsBy*`、`createElement`、`createTextNode` | 対応selectorとBrowser所有Node wrapperに限定 |
| Element | `growse/dom` | text / value / attribute、append / prepend / remove / replace、children / parent / metadata、`innerHTML`、`classList` | 同じDOM mutation経路でStyle、Layout、Paint、Hit Testを更新 |
| Event | `growse/dom` | add / remove listener、target / phase / cancel / propagation | click、input、change、submit、reset、focus、blur、mouseenter、mouseleave、message、lifecycle |
| Scheduler | `growse/scheduler` | timeout、interval、Animation Frame、microtask | 文字列callbackを拒否し、Page終了時に解除 |
| Fetch | `growse/fetch` callback | Promise形式`fetch`、`AbortController` | credentials、CORS、timeout、abort、buffered Response |
| Storage | `growse/storage` | `localStorage`、`sessionStorage` | Origin分離、quota、same-origin `storage` Event |
| Navigation | `growse/navigation` | `location`、`history` | assign、back / forward / go、push / replace state、event |
| Frame | 非公開 | iframe、parent / top / frames、same-origin access、`postMessage` | sandbox token、opaque cross-origin proxy、structured clone subset |
| Service Worker | 非公開 | register / update / unregister / ready / controller | install / waiting / activate、fetch、Cache Storage、idle restart |

GoとJavaScriptはBrowser所有のDOM、Scheduler、Network、Storage、Navigation基盤をbroker経由で共有する。workerへCookie、Authorization Header、filesystem path、environment variable、Browser objectを渡さない。callbackはPageごとの直列queueで実行し、Page close、Navigation、Engine切り替え後には配送しない。

## ECMAScript Modules

static import / export、re-export、namespace、cycle、dynamic `import()`、top-level await、最初の有効なimport mapを扱う。Module URLはdocumentまたはreferrer moduleから解決し、graph内の正規化URLは一度だけfetch / link / evaluateする。bare specifierはimport mapで解決できる場合だけ受理し、Node.js / npm / CommonJS探索へfallbackしない。

Module fetchはCORSとJavaScript MIMEを必須とし、missing export、parse / link / evaluate error、dynamic import rejection、timeoutを対象Pageへ閉じ込める。

## WebAssembly

JavaScriptへ`Module`、`Instance`、`Memory`、`Table`、`Global`、`validate`、`compile`、`instantiate`、streaming compile / instantiateの検証済みsubsetを提供する。`CompileError`、`LinkError`、`RuntimeError`を区別する。

WASMは同じJavaScript worker内で実行し、WASI、filesystem、socket、clock、random、process environmentを暗黙提供しない。1 binary 8 MiB、Page合計32 MiB、Module 32件、Instance 64件、初期Memory 64 MiB、最大Memory 256 MiB、Table 65,536 elementとし、callをtimeout / cancel対象にする。

## iframeとService Worker

iframeは独立Document、Runtime worker、generation、Event queue、Session Storage namespaceを持つ。same-originだけが制限付きDOMへアクセスでき、cross-originまたは`allow-same-origin`のないsandbox Frameはopaque proxyになる。Frame深さ8、Page 32件、document合計32 MiB、message 1 MiBを上限とする。

Service WorkerはSecure Context（HTTPSとloopback）でOrigin / scopeごとに登録する。active workerはscope内のNavigation / Resource / Fetchをinterceptでき、`caches`へbuffered Request / Responseを保存できる。registration 64件、Cache 32件、entry 4,096件、1 response 4 MiB、Origin合計128 MiBを上限とする。Page終了後のpending eventだけを完了させ、idle 30秒で停止して次のevent時に新generationへ復元する。

## JavaScript Fetchの範囲

`fetch(input, init)`はPromiseを返す。string URL、method、headers object、text body、`omit` / `same-origin` / `include` credentials、AbortSignal、millisecond timeoutを扱う。Responseはstatus、URL、redirect、read-only headersと、一度だけ消費できる`text()` / `json()`を提供する。

Request 1 MiB、Response 4 MiB、Header 100件 / 64 KiB、redirect 10回、Page 16件・Session 128件の同時Fetch上限をGoと共有する。ReadableStreamと任意body streamingは対象外である。

## 公開しない機能

- Node.js、npm package resolution、CommonJS、TypeScript、JSX
- Web Worker、Shared Worker、Worklet、Shadow DOM、custom elements
- Canvas、WebGL、WebGPU、WebSocket、WebRTC、Media Source、DRM
- iframe Permissions Policy全般、BFCache、完全なjoint session history
- Service Worker Background Sync、Push、Notification、navigation preload、module worker
- DevTools REPL、source debugger、breakpoint、heap profiler

goja、WASM、HTML、CSS、Web APIの仕様全体や任意公開サイトの完全互換は保証しない。対応範囲は本書、v0.14.0定義、Showcase、選定WPTで観測できるsubsetである。

## Security Boundary

Page / FrameのGo・JavaScript・WASMとService WorkerはBrowser UI processの外にある専用worker processで実行する。通信はversion / size制限付きIPCとBrowserが検証するbrokered host APIだけに限定し、workerのtimeout、crash、protocol違反、stale generationを対象contextの失敗として回収する。worker executable、protocol、process boundary、minimal environment、parent lifecycle、memory上限を起動時に検証し、不足時はexternal codeをfail closedする。

Linuxは`no-new-privileges`とparent-death signal、macOS / Windowsは専用process groupとparent IPC lifecycleを追加する。これはGrowseが公開するhost作用をdefault denyにする境界であり、OS kernel、Go runtime、goja、wazero、decoder、Growse自身の未知の脆弱性が存在しないという保証ではない。高い権限や機密profileで実験的な外部codeを実行しない。
