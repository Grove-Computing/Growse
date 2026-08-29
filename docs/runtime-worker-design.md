# Runtime worker / Web Platform設計

本書はGrowse v0.16.0の実行・閲覧context設計を示す。v0.1.0時点の歴史的な詳細設計は[旧MVP詳細設計](details-design.md)に残すが、Runtime、JavaScript、Frame、Service Worker、dynamic resource、hydrationについては本書を現行仕様とする。

## 所有境界

Browser processはNetwork、Cookie、Cache、DOM、Style、Layout、Paint、History、Storage、UIを所有する。Page / FrameのGo・JavaScript・WASMとService Workerは専用worker processで評価し、Browser objectやpointerを共有しない。

```text
Browser process
  ├─ Navigation / Network / Origin policy
  ├─ DOM → Style → Layout → Paint
  ├─ Storage / HTTP Cache / Service Worker registration
  └─ typed IPC broker
       ├─ Page runtime worker (Go または JavaScript + WASM)
       ├─ Frame runtime worker (Go または JavaScript + WASM)
       └─ Service Worker process (JavaScript)
```

## worker protocol

IPC messageはprotocol version、request ID、method、長さ制限付きJSON payloadを持つ。1 messageは1 MiB、pending requestは256件、Page全体の転送中payloadは8 MiBまでとする。DOM mutation、Event、Timer、Fetch、Storage、Navigation、Console、Module / dynamic resource取得、CSSOM snapshot、observer配送、Frame通信だけを登録済みmethodとして受理し、未知method、oversized payload、panic、transport切断をcontext単位のRuntime errorへ変換する。

workerへ生のCookie、Authorization、Header集合、filesystem path、Browser environment、Go pointerを渡さない。Network requestはworker入力を信用せず、Browser側でURL、Origin、kind、CORS、credentials、mixed content、sizeを再構築・再検証する。

## lifecycleとgeneration

Page、Frame、Service Workerの起動ごとにgenerationを増やす。Navigation、Frame navigation、Engine切り替え、close、timeout、crash後は旧generationのcallback、message、DOM snapshot、resource completion、observer recordを拒否する。通常停止はIPC stopと1秒の猶予を使い、応答しないprocessを終了する。Browser Session全体でworkerは32件までとし、上限超過は新しいRuntimeだけをfail closedする。

Goは新規Tabの既定Engineであり、JavaScriptのinitial / dynamic script、Module、modulepreload、hydration callbackは利用者が`JS`を明示選択したgenerationだけへbrokerする。Goへ戻す場合はJavaScript worker、Frame、Module graph、dynamic resource、observer、Event listenerを停止して完全reloadし、Browser所有のDOM / Style / Layout / Paintへ両Engineのobjectを混在させない。

Service Worker registrationとCache StorageはOrigin profile stateとして残すが、worker VMはidle 30秒で停止する。次のeventは保存済みscriptから新generationを起動し、Pageのcancelとは分離した期限付きevent contextで完了させる。

## sandbox検証

Browserはcodeを渡す前に、別PID、protocol / profile version、brokered host I/O、最小environment、parent lifecycle、worker memory上限、platform constraint reportを照合する。必須条件が欠ける場合はcodeを実行しない。

Linuxは`PR_SET_NO_NEW_PRIVS`とparent-death signal、macOS / Windowsは専用process group、全platformはparent IPC EOFを終了条件にする。platform constraintは適用できた項目だけをDevToolsへ報告する。seccomp、container、VMと同等のOS隔離や未知のnative/runtime脆弱性への耐性は宣言しない。

## browsing context

top-level Pageと各iframeはDocument、Runtime、generation、Event queue、Session Storageを分ける。same-origin Frameは制限付きDocument snapshotを共有できる。cross-origin Frameとopaque sandbox OriginはID、generation、Origin、URLの最小proxyだけを公開する。`postMessage`は送信時と配送時に`targetOrigin`を検査し、循環なし・1 MiB以下のstructured clone subsetを渡す。

Service WorkerはOrigin / scopeで最長一致するactive registrationを選び、fetch eventの同期`respondWith()`だけをinterceptionとして採用する。worker内部fetchとupdate script fetchは再interceptせず、network fallbackもBrowser policyを通る。

## 診断境界

DevTools Runtime panelはPage / Frame / Service Workerのcontext ID、browsing / worker generation、Engine、state、script kind / schedule、redacted URL、有限error category、sandbox capabilityだけを表示する。source、response body、Header、Cookie、Authorization、IPC payload、raw error本文は記録しない。

## 非対象

既存browser engineの埋め込み、Node.js互換、npm resolution、WASI、Web Worker / Shared Worker、Shadow DOM、custom elements、Service Worker Push / Background Sync、完全なPermissions Policy、BFCache、全WPTおよび任意frameworkへの完全適合はv0.16.0の対象外である。
