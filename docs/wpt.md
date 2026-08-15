# Web Platform Tests由来テスト

GrowseはWeb Platform Tests（WPT）をブラウザで直接実行せず、対応する範囲をGoのUnit Testへ縮約して管理する。

- Upstream: `web-platform-tests/wpt`
- Revision: `816bbf3ebae17dc6866deb65b2286b1a1c162819`
- License: WPTリポジトリの`LICENSE.md`（3-Clause BSD）
- 配置: `internal/style/wpt_test.go`、`internal/layout/wpt_test.go`、`internal/forms/wpt_test.go`、`internal/browser/wpt_v09_test.go`、`internal/browser/wpt_v10_test.go`、`internal/network/wpt_test.go`、`internal/storage/wpt_test.go`、`internal/webapi/scheduler/wpt_test.go`
- v0.10.0対象: v0.9.0までの範囲に加え、Top-level Browsing Context、close Lifecycle、Storage Event

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
