# Web Platform Tests由来テスト

GrowseはWeb Platform Tests（WPT）をブラウザで直接実行せず、対応する範囲をGoのUnit Testへ縮約して管理する。

- Upstream: `web-platform-tests/wpt`
- Revision: `a7b5671e50ee3610ec3ad2e1278a33b2cb11339c`
- License: WPTリポジトリの`LICENSE.md`（3-Clause BSD）
- 配置: `internal/style/wpt_test.go`、`internal/layout/wpt_test.go`、`internal/forms/wpt_test.go`、`internal/browser/wpt_v*_test.go`、`internal/network/wpt_test.go`、`internal/storage/wpt_test.go`、`internal/webapi/scheduler/wpt_test.go`、`internal/runtime/javascript/wpt_v14_test.go`、`internal/serviceworker/wpt_v14_test.go`、および`tests/v015-conformance.sh`が選択するv0.15.0実装Test
- v0.14.0対象: v0.13.0までの範囲に加え、Module単一評価、WASM Module validation、iframe sandbox script gate、Service Worker default scope

## 対応表

| Growse Test | WPT Source | 適応内容 | 差分 |
|---|---|---|---|
| `TestWPTSpecificity001AttributeBeatsType` | `css/CSS2/cascade/specificity-001.xht` | 視覚的な緑色判定をComputed Colorの判定へ変換 | なし |
| `TestWPTCalcDivideByZeroIsRejectedWithoutCrash` | `css/css-values/calc-catch-divide-by-0.html` | ゼロ除算ケースをLength parserへ入力し、panicしないことを確認 | WPTの現行仕様はInfinity/NaNを直列化するが、v0.4.0は非有限値をDeclaration無効として扱う |
| `TestWPTBorderRadius001ZeroProducesSquareCorners` | `css/css-backgrounds/border-radius-001.xht` | ReftestをComputed Border Radiusのゼロ値判定へ変換 | なし |
| `TestWPTFlexGrow001DistributesPositiveFreeSpaceByFactor` | `css/css-flexbox/flex-grow-001.xht` | 240pxのmain sizeをgrow factor 0:1:2へ分配し、各itemの数値geometryを比較 | 視覚的な参照画像を数値比較へ変換 |
| `TestWPTFlexWrap002FormsTwoColumnFlexLines` | `css/css-flexbox/flex-wrap-002.html` | 100pxのcolumn main axisへ100pxのitemを2つ置き、2 line形成を比較 | fit-content cross sizeの詳細はGrowse対応範囲外 |
| `TestWPTFlexDirectionRowReverseMapsMainAxis` | `css/css-flexbox/flex-direction-row-reverse.html` | row-reverseのhorizontal/reverse axis変換を直接比較 | 視覚的な順序比較を内部axisの数値比較へ変換 |
| `TestWPTComputedGridColumnPlacesItemOnSecondColumn` | `css/css-grid/computed-grid-column.html` | row 1 / column 2のcomputed placementと最終geometryを比較 | upstreamの複雑な`calc()`式は未対応のため、同じ計算結果となる整数値へ縮約 |
| `TestWPTAbsoluteWithoutPositionedAncestorUsesInitialContainingBlock` | `css/CSS2/abspos/abspos-containing-block-initial-005b.xht` | positioned ancestorがないabsolute要素をviewport基準で配置し、数値geometryを比較 | upstreamのroot要素に対するreftestを通常要素のright/bottom解決へ縮約 |
| `TestWPTTransformOriginZeroDiffersFromDefaultCenter` | `css/css-transforms/transform-origin-001.html` | 45度回転時の`0% 0%`と既定`50% 50%`の変換行列が異なることを比較 | mismatch画像比較を変換行列の比較へ変換 |
| `TestWPTAnimationImportantWithTransitionCascade` | `css/css-animations/animation-important-with-transition.html` | Transition、Animation、`!important`が同時に存在するcomputed valueの優先順を比較 | Web Animations APIの型・列挙順はv0.7.0対象外 |
| `TestWPTAnimationFillModeNoneRestoresUnderlyingColor` | `css/css-animations/animation-fill-mode-001-manual.html` | Animation終了後にFillなしでunderlying background colorへ戻ることを比較 | 5秒の目視試験をfake timestampのcomputed value判定へ変換 |
| `TestWPTTransitionDurationShorthandUsesLastMatchingProperty` | `css/css-transitions/transition-duration-shorthand.html` | shorthand内で最後に一致する0秒のPropertyは即時反映され、別PropertyだけTransitionを開始することを比較 | Layout対象のwidth/heightをv0.7.0対応のopacity/colorへ置換 |
| `TestWPTURLEncodedPreservesDuplicateNamesAndEmptyValues` | `html/semantics/forms/form-submission-0/urlencoded2.window.js` | duplicate nameと空値をEntry順のurlencoded文字列へ縮約 | FormDataとJavaScript harnessは使用しない |
| `TestWPTURLOriginNormalizesDefaultPorts` | `url/url-origin.any.js` | HTTPSの省略portと443を同じOriginとして比較 | opaque originと非HTTP schemeはv0.8.0対象外 |
| `TestWPTCookiePathDoesNotMatchPrefixLookalike` | `cookies/attributes/resources/path-redirect-shared.js` | Cookie Path直後がslashでないprefix lookalikeを非一致として比較 | redirect harnessをJar lookupへ縮約 |
| `TestWPTCORSSafelistedContentTypes` | `cors/cors-safelisted-request-header.any.js` | safelisted Content-Type 3種のsimple request判定を比較 | byte-level unsafe value全組合せは未移植 |
| `TestWPTNegativeTimeoutRunsBeforePositiveTimeout` | `html/webappapis/timers/negative-settimeout.any.js` | 負delayを0として扱い、正のdelayより先に実行されることをfake clockで比較 | WPTの実時間10ms待機を手動deadline配送へ変換 |
| `TestWPTHistoryPushStateSetsURLAndBackRestoresIt` | `html/browsers/history/the-history-interface/history_pushstate_url.html` | fragmentをpushStateした後、Backで元URLへ復元することを比較 | Windowとtestharnessを単一Page History modelへ縮約 |
| `TestWPTStorageKeyOrderSurvivesValueReplacement` | `webstorage/storage_key.window.js` | value更新でkey順が変わらず、範囲外indexが空になることを比較 | JavaScriptのunsigned long変換はv0.9.0のGo API対象外 |
| `TestWPTHTTPCacheMaxAgeOverridesExpiresAndAgeCanMakeItStale` | `fetch/http-cache/freshness.any.js` | max-ageのExpires優先とAge超過によるstale判定をfake timestampで比較 | WPT HTTP server protocolをCache modelの直接入力へ縮約 |
| `TestWPTFailedUnsafeRequestDoesNotInvalidateFreshEntry` | `fetch/http-cache/invalidate.any.js` | 失敗したPOSTがfresh GET entryを無効化しないことを比較 | WPT harnessを`httptest.Server`へ置換 |
| `TestWPTTargetBlankCreatesDistinctTopLevelContext` | `html/semantics/links/links-created-by-a-and-area-elements/target_blank_implicit_noopener.html` | `_blank` submissionがsourceを維持して独立Top-level Contextを作ることを比較 | opener APIを提供しないためForm SubmissionとTab ID分離へ縮約 |
| `TestWPTClosedBrowsingContextRejectsFutureWork` | `html/browsers/windows/auxiliary-browsing-contexts/opener-closed.html` | close後のBrowsing Contextを参照してもworkを配送できないことを比較 | WindowProxyの`closed` propertyをSession dispatch拒否へ縮約 |
| `TestWPTLocalStorageEventCarriesCommittedOldAndNewValuesOnce` | `webstorage/event_basic.js` | 別Contextだけがcommitごとにold/new valueとURLを一度受信することを比較 | iframeとtestharnessを共有Storage Areaのsource-aware observerへ縮約 |
| `TestWPTModuleSharedDependencyEvaluatesOnce` | `html/semantics/scripting-1/the-script-element/module/single-evaluation-1.html` | 2本のimport branchが共有するModuleをfetch / evaluate各1回に固定 | HTML harnessをin-memory Module graphとConsole assertionへ縮約 |
| `TestWPTWebAssemblyModuleConstructorValidatesBytes` | `wasm/jsapi/module/constructor.any.js` | invalid binaryの`validate=false`、`CompileError`とvalid constructorを比較 | upstream fixture binaryをGrowse test生成binaryへ置換 |
| `TestWPTIframeEmptySandboxBlocksScriptExecution` | `html/semantics/embedded-content/the-iframe-element/sandbox_005.htm` | 空sandboxがscriptとsame-originを許可せず、既知tokenだけをgrantすることを比較 | postMessage harnessを実行前FramePolicy assertionへ縮約 |
| `TestWPTServiceWorkerDefaultScopeIsScriptDirectory` | `service-workers/service-worker/register-default-scope.https.html` | scope省略時にscript URLのdirectoryを採用することを比較 | HTTPS harnessとregistration jobをURL policyの直接assertionへ縮約 |
| `TestJavaScriptNodeRelationshipsMutationsFragmentsAndCloneKeepIdentity` | `dom/nodes/Node-cloneNode.html`、`dom/nodes/ParentNode-append.html` | Node identity、DocumentFragment挿入、clone、tree mutationをJavaScript fixtureで比較 | custom element reactionとshadow tree cloneは対象外 |
| `TestJavaScriptElementReflectionSelectorsDatasetStyleAndHTMLMutation` | `dom/nodes/Element-closest.html`、`dom/nodes/ParentNode-querySelector-All.html` | `matches`、`closest`、`querySelectorAll`とattribute reflectionを単一DOM fixtureへ縮約 | Shadow DOM境界とnamespace selectorは対象外 |
| `TestDynamicClassicScriptsSnapshotFetchAndExecuteExactlyOnce` | `html/semantics/scripting-1/the-script-element/execution-timing/121.html` | 動的classic scriptの挿入時snapshot、fetch、load/error、単一評価をfake loaderで比較 | parser-blocking順序と実network timingは対象外 |
| `TestJavaScriptDynamicStylesheetAndPreloadUpdateBrowserStyleRevision` | `css/cssom/stylesheet-same-origin.sub.html`、`html/semantics/document-metadata/the-link-element/link-load-event.html` | 動的`style` / `link`のload/errorとStyle revision更新をoffline responseで比較 | cross-document sheet共有とResource Timingは対象外 |
| `TestLevel4SelectorMatchingHasScopeIsWhereAndComplexNot` | `css/selectors/is-where.html`、`css/selectors/has-basic.html` | `:is()`、`:where()`、`:has()`、complex `:not()`のmatchingとspecificityをGo assertionへ縮約 | pseudo-element、Shadow DOM、live invalidation全組合せは対象外 |
| `TestCascadeLayerOrderImportantReversalAndRevertLayer` | `css/css-cascade/layer-order.html`、`css/css-cascade/revert-layer-001.html` | layer順、important反転、`revert-layer`をcomputed valueで比較 | user / UA origin layerとanimation originは対象外 |
| `TestCSSOMGeometryAndMediaQueriesUseBrowserSnapshots` | `css/cssom/CSSStyleDeclaration-cssText.html`、`css/cssom-view/elementFromPoint.html` | CSS declaration、geometry、media queryを固定Viewport snapshotへ縮約 | live DOMRect、scrolling、visual viewportは対象外 |
| `TestMutationObserverDeliversBoundedRecordsAtCheckpoint` | `dom/nodes/MutationObserver-childList.html` | child / attribute mutation recordとcheckpoint配送を決定的queueで比較 | browser全体のmicrotask orderingとShadow DOMは対象外 |
| `TestResizeAndIntersectionObserversRunAfterFrame` | `resize-observer/observe.html`、`intersection-observer/basic.html` | Resize / Intersection entryを固定FrameとViewportで比較 | device pixel box、cross-origin root、scroll marginは対象外 |
| `TestImageCandidatesSelectPictureSourceByTypeMediaSizesAndScale` | `html/semantics/embedded-content/the-img-element/update-the-image-data/select-an-image-source.html` | `picture`、`srcset`、`sizes`、DPR候補選択をoffline image metadataで比較 | animated image、client hints、network priorityは対象外 |
| `TestLoadWebFontsValidatesDescriptorsAndDecodesWOFF` | `css/css-fonts/font-face-src-local.html`、`css/css-font-loading/fontface-load.html` | `@font-face` descriptor、source fallback、WOFF decodeをbounded loaderで比較 | OS local font探索、FontFaceSet Promise、可変font axisは対象外 |

Upstreamのファイル全体はコピーせず、assertionの意味と最小入力だけを移植する。ケースを追加または更新するときは、Revision、Source、適応内容、および意図的な差分をこの表へ記録する。

## v0.6.0の選定範囲

- Grid Level 1はtrack sizing、placement、auto-placement、alignmentを選定対象とする。Grid Level 2のsubgridとmasonryはリリース対象外のため移植しない。
- Positioned Layoutはrelative、absolute、fixed、stickyと包含ブロックを選定対象とする。Anchor Positioningとtop layerはプロパティモデル自体が対象外のため移植しない。
- Transforms Level 1は2D transformとtransform-originを選定対象とする。3D transform、perspective、preserve-3dはv0.6.0の明示的な対象外のため移植しない。
- WPT harness、WebDriver、Font依存のassertionは直接実行せず、Growseの責務に対応するcomputed value、Layout geometry、Display List、Hit Testの決定的なassertionへ縮約する。

## v0.7.0の選定範囲

- CSS Transitions Level 2、CSS Animations Level 1、CSS Easing Functions Level 1から、v0.7.0が対応する構文、Timing、補間、Cascade、Lifecycleを選定対象とする。
- Web Animations API、Animation Event、3D Transform、およびLayoutを変更するPropertyのAnimationは対象外とし、同じ仕様上のassertionをpaint-only Propertyとfake timestampへ縮約できる場合だけ移植する。
- 追加時は固定Revisionに対象Sourceが存在することを確認し、Goテストの直前コメントと対応表の両方へSource pathを記録する。

## v0.8.0の選定範囲

- HTML Formsはsuccessful controls、改行正規化、urlencoded、validation、submit cancellationを選定し、File uploadとmultipart body生成は対象外とする。
- URLはHTTP(S) Originとpercent-encodingを選定し、opaque origin、IDNA全fixture、URL API全体は対象外とする。
- CookiesはDomain、Path、Secure、HttpOnly、SameSite、expirationを選定し、partitioned Cookieは対象外とする。
- Fetch/CORSはsimple request、preflight、credentials、公開Response Headerを選定し、JavaScript Promise/Stream APIは対象外とする。

## v0.9.0の選定範囲

- Timersは負delay、同一deadline順序、nested clamp、解除を選定し、Worker/Window realm差と文字列Callbackは対象外とする。
- Session HistoryはpushState / replaceState、同一Document traversal、state / URL復元を選定し、iframe joint session historyとBFCacheは対象外とする。
- Web Storageはkey順、更新、削除、quota、Origin分離を選定し、複数Window間Storage Eventは対象外とする。
- HTTP CacheはFreshness、Age、Vary、Validation、304 merge、unsafe MethodによるInvalidationを選定し、Range、shared cache、`stale-if-error`は対象外とする。
- RFC 9111 §4.2.1（Freshness Lifetime）、§4.2.3（Age Calculations）、§4.3.4（304 merge）、§4.4（Invalidation）の例は`internal/network/rfc9111_test.go`およびHTTP Cache testへtable-drivenで縮約し、WPTと同じfake timestamp / fake serverで実行する。

## v0.10.0の選定範囲

- Top-level Browsing Contextは`target="_blank"`による独立Context作成を選定し、iframe、named target再利用、opener API、およびBrowsing Context Groupは対象外とする。
- Lifecycleはclose後のContextへworkを配送せずlive siblingを維持する範囲を選定し、`beforeunload` prompt、Page Visibility Event、およびBFCacheは対象外とする。
- Storage Eventはsame-originの更新元以外へcommit順にkey、old/new value、URLを配送する範囲を選定し、複数Window Process間同期とSession Storage Eventは対象外とする。

## v0.11.0の選定範囲

- Fetch は request body の排他指定、forbidden request header、abort、timeout、Response body の一回消費を選定し、Promise、Stream、WebSocket は対象外とする。
- URLSearchParams と FormData は ordered duplicate field、空値、application/x-www-form-urlencoded serialize を選定し、File / Blob / multipart は対象外とする。
- Abort と timeout は fake clock / context cancellation へ縮約し、WPT の Window、AbortSignal Event、実時間待機は直接実行しない。

## v0.13.0の選定範囲

- Fetchは既存Network policyの結果をJavaScript Promiseへ接続し、resolve / reject、Response bodyの一回消費、abort、timeout、Page closeを選定する。ReadableStreamとbrowser全体のmicrotask timingは対象外とする。
- Timersはfunction callback、引数、登録順、clear、Page closeを選定し、文字列callbackは安全上の意図的な非対応として拒否する。
- Web StorageはJavaScriptの同期Storage操作と更新元以外へのsame-origin `storage` Eventを選定し、複数Process間EventとSession Storage Eventは対象外とする。
- HistoryはJSONへ変換可能なstate、same-origin URL、size上限、`popstate`、`hashchange`を選定し、Window / iframe joint session historyとBFCacheは対象外とする。
- WPTのJavaScript harnessは直接実行せず、Growseのgoja adapter、Page queue、fake scheduler、fake network、共有Storage Areaへ最小fixtureを縮約する。

## v0.14.0の選定範囲

- ECMAScript Moduleは共有dependencyのsingle fetch / evaluationを選定し、modulepreload、import attributes、JSON / CSS Moduleは対象外とする。
- WebAssembly JavaScript APIはbinary validation、`Module` constructor、`CompileError`を選定し、GC、exception handling、JSPI、WASIは対象外とする。
- iframeは空sandboxのscript / Origin gateと既知token grantを選定し、plugin、Permissions Policy、credentialless Frameは対象外とする。
- Service Workerはscript directory由来のdefault scopeを選定し、module worker、navigation preload、Push、Background Syncは対象外とする。
- `tests/v014-conformance.sh`は固定shuffle seedでRuntime、Browser、Service Worker packageを新規processから3回実行し、WPTとsecurity / lifecycle / quota / crash / cancel回帰の順序依存を検出する。公開DNS、実時間resource、外部APIを合否条件に使わない。

## v0.15.0の選定範囲

- DOMはNode identity、DocumentFragment、clone、tree mutation、selector APIを選定し、Shadow DOM、custom element reaction、namespace selectorは対象外とする。
- HTML dynamic resourceはclassic / module script、`style`、stylesheet `link`、preloadのsnapshot、単一評価、load/errorを選定し、parser-blocking timing、Resource Timing、公開networkは対象外とする。
- Selectors Level 4とCascade Level 5は`:is()`、`:where()`、`:has()`、complex `:not()`、layer順、important反転、`revert-layer`を選定し、Shadow DOM selector、pseudo-element全体、user origin layerは対象外とする。
- CSSOMとobserverは固定Viewportのdeclaration / geometry / media query、およびMutation / Resize / Intersectionのbounded配送を選定し、live DOMRect、visual viewport、browser全体のmicrotask orderingは対象外とする。
- imageとfontは`picture` / `srcset` / `sizes` / DPR選択、`@font-face` descriptor、WOFF decode、source fallbackを選定し、animated image、OS local font探索、FontFaceSet Promise、可変font axisは対象外とする。
- `tests/v015-conformance.sh`は上表のv0.15.0対象Testを固定shuffle seedで3回、新規processかつofflineで実行する。WPT harnessとupstream file全体は取り込まず、固定RevisionのassertionをGrowseのDOM、resource queue、computed style、layout / paint snapshotへ縮約する。
