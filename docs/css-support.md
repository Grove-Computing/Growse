# CSS対応表

この表はGrowse v0.19.0の実装を基準とする。「部分対応」は一般的な値を扱えるが、仕様全体を実装していない機能を表す。document、initial / dynamic stylesheet、`@import`、image、font resourceは最初の有効な`<base href>`から解決する。

## SelectorとCascade

| 機能 | 状態 | 制限 |
|---|---|---|
| Type、Universal、Class、ID、Compound | 対応 | CSS escape付きTailwind utility classとforgiving selector listを含む。Namespace Selectorは未対応 |
| Attribute Selector | 対応 | `i`、`s`などのModifierは未対応 |
| Descendant、Child、Adjacent、General Sibling | 対応 | Column Combinatorは未対応 |
| Structural Pseudo-class | 対応 | `:root`、`:empty`、child/of-type系、`an+b` |
| `:is()`、`:where()`、`:not()` | 対応 | forgiving list、complex selector、`:where()`のゼロ詳細度を扱う |
| `:has()`、`:scope` | 部分対応 | relative selectorのfixture範囲。Shadow DOMとpseudo-elementは未対応 |
| Link、Form State、`:hover`、`:focus` | 対応 | `:defined`、`:placeholder-shown`、read / write、required / optional、focus visible / withinを含む。`:visited`は意図的に未対応 |
| `::before`、`::after` | 部分対応 | 引用文字列の`content`だけ |
| Cascade | 対応 | UA、Author、Inline、`!important`、詳細度、ソース順 |
| CSS-wide Keyword | 対応 | 実装済みPropertyの`inherit`、`initial`、`unset` |
| Custom Propertyと`var()` | 対応 | Fallbackと循環検出を含む |
| Cascade Layer | 対応 | `@layer`、nested / anonymous layer、`@import layer()`、important反転、`revert-layer` |
| CSS Nesting | 部分対応 | `&`とnested group rule。深さ32、展開後selector 1,024件まで |

## ValueとQuery

| 機能 | 状態 | 制限 |
|---|---|---|
| Absolute Length | 対応 | `px`、`in`、`cm`、`mm`、`q`、`pt`、`pc` |
| Relative Length | 対応 | `em`、`rem`、`ex`、`ch`、`vw` / `vh` / `vmin` / `vmax`、small / large / dynamic viewport、container query unit、percentage |
| `calc()`、`min()`、`max()`、`clamp()` | 対応 | Length/Percentageの四則演算とnest。非互換Dimension、ゼロ除算、非有限値は無効 |
| CSS Color Level 4 subset | 対応 | Named / hex alpha、space区切りrgb / hsl、`hwb()`、Lab / LCH / OKLab / OKLCH、`color-mix()`をsRGBへ変換 |
| Media Query | 部分対応 | `all`、`screen`、width/height、orientation、resolution、color scheme、hover、pointer、`prefers-reduced-motion` |
| `@import` | 部分対応 | Stylesheet先頭のsame / cross-origin HTTP(S)。redirect、CSS MIME、mixed content、循環、深度8、32件、合計8 MiBを検証 |
| `@keyframes` | 対応 | from/to、percentage、複数selector、同一offsetのCascade。1 Stylesheetあたり256 rule |
| `@supports` | 部分対応 | property / value、`selector()`、not / and / or。未実装Featureはfalse |
| `@container` | 部分対応 | inline-size、named container、`container-type` / `container-name`。style / scroll-state queryは未対応 |
| `@property` | 部分対応 | syntax、initial value、inheritsと登録Custom Property |

## Property

| Property | 状態 | 制限 |
|---|---|---|
| `display` | 対応 | `none`、`contents`、`inline`、`block`、`flow-root`、`inline-block`、`flex`、`inline-flex`、`grid`、`inline-grid`、Table内部display |
| width/height、min/max、`box-sizing` | 対応 | `auto`、length / percentage、min-content / max-content / fit-content、content-box / border-boxを主要layout modeで解決 |
| margin、padding | 対応 | 1〜4値とphysical / logical Longhand、percentage、Block margin collapse、主要layout modeの`auto` margin |
| border | 対応 | 各Shorthand/Longhandと`solid`、`dotted`、`dashed`、`double` |
| `border-radius` | 対応 | 1〜4値、slash区切りの楕円角、percentage |
| overflow | 対応 | x / yの`visible`、`hidden`、`clip`、`auto`、`scroll`をclip / extent / offset / Paint / Hit Testingへ共有。Scrollbar UIはplatform widget範囲 |
| `background-color` | 対応 | alpha合成を含む |
| `background-image` | 部分対応 | 複数HTTP(S) PNG/JPEG/GIF、`linear-gradient()`、`radial-gradient()`。`data:`、conic gradientは未対応 |
| `background-repeat/position/size` | 部分対応 | 複数Layer、主要Keyword、1〜2値、length/percentage、cover/contain。origin/clipの独立指定は未対応 |
| `font`、family / size / style / weight / stretch、`line-height` | 対応 | CORSを通過したWOFF / WOFF2、同梱Go Font、JS Page専用system font fallback。日本語・Latin・記号をglyph coverageでfallback。可変font axisは未対応 |
| `white-space` | 対応 | normal、nowrap、pre、pre-wrap、pre-line |
| `color` | 対応 | 上記CSS Color Level 4 subset |
| `text-decoration-line/color` | 対応 | underline、overline、line-through |
| `opacity` | 対応 | 0〜1。1未満はStacking Contextとoffscreen groupを生成 |
| `flex-direction`、`flex-wrap`、`flex-flow` | 対応 | Writing Modeに従うrow / column、reverse、single / multi-line |
| `flex-grow`、`flex-shrink`、`flex-basis`、`flex` | 対応 | Length、Percentage、`auto`、`content`。indefinite sizeのPercentageはauto相当 |
| `justify-content` | 対応 | flex-start/end、center、space-between/around/evenly |
| `align-items`、`align-self`、`align-content` | 対応 | stretch、flex-start/end、center、baseline、対応する分散値 |
| `row-gap`、`column-gap`、`gap` | 対応 | LengthとPercentage。単一のrow/column gap |
| `order` | 対応 | 視覚順とPaint順だけを変更し、DOM・focus順は維持 |
| `aspect-ratio` | 対応 | Block、Flex / Grid item、image、iframe、form controlでintrinsic ratioとdefiniteな片軸を転送 |
| Grid track | 対応 | fixed/percentage、`auto`、min/max-content、`fr`、`minmax()`、`fit-content()`、fixed/auto `repeat()`、named line |
| Grid placement | 対応 | numbered/named line、`span`、template area、sparse/dense auto-placement、implicit track |
| Grid alignment | 対応 | `justify/align-items`、`justify/align-self`、content alignment、`place-*`、gap、auto margin、`order`、safe / unsafe |
| `subgrid` | 対応 | row / columnの親track、gap、named lineを継承し、nested contributionとplacementを共有。Masonryは未対応 |
| `position`、inset | 対応 | relative、absolute、fixed、sticky。opposing / percentage inset、両軸Sticky、nested scroll container、container終端を共有geometryで解決 |
| `z-index` | 対応 | positioned elementの`auto`またはinteger。opacity/transformと共通のStacking Context順を使用 |
| `box-shadow`、`text-shadow` | 対応 | 複数shadow、blur/spread、inset、alpha color |
| `outline`、`outline-offset` | 対応 | width/style/colorとoffset |
| `transform`、`transform-origin` | 対応 | 2D translate/scale/rotate/skew/matrixと複数function。3D、perspectiveは未対応 |
| `transition-*`、`transition` | 部分対応 | opacity、transform、width / height、主要Color。複数Transition、list matching、delay、Easing、中断・反転 |
| `animation-*`、`animation` | 部分対応 | 複数Keyframes Animation、delay、iteration、direction、fill、play-state。加算・累積合成は未対応 |
| `visibility` | 対応 | `visible`、`hidden`。`display:none`とは別にLayout geometryを保持 |
| `text-align/transform/indent`、letter / word spacing | 対応 | horizontal-tb / vertical-rl / vertical-lrのText layout / paintへ反映 |
| `word-break`、`overflow-wrap`、`vertical-align`、`text-overflow` | 部分対応 | fixtureで使う主要値。完全なUnicode bidi / line breaking / hyphenationは未対応 |
| `writing-mode`、`direction`、logical property | 対応 | horizontal-tb / vertical-rl / vertical-lr、ltr / rtlとlogical size / edge / inset / float / clear / alignment。sideways、Ruby、縦中横は未対応 |
| Table layout | 対応 | anonymous wrapper、caption / row / column group、auto / fixed、colspan / rowspan、border spacing / collapse、vertical alignment |
| Multi-column / fragmentation | 対応 | count / width / columns / gap / rule、auto / balance、span:all、主要break control、widow / orphanの安全なsubset。Print / Paged Mediaは未対応 |
| `object-fit`、`object-position` | 対応 | replaced imageのcontain / cover / fill / none / scale-downと主要position |
| `list-style`、`appearance`、`accent-color`、`cursor` | 部分対応 | fixtureで使うmarker、form state、標準cursor subset |
| `filter`、`backdrop-filter`、`mix-blend-mode` | 部分対応 | bounded offscreen / kernelの主要functionとblend。未対応functionは局所無効 |

## LayoutとPaint

Block / Inline Formatting Context、親子・隣接Blockの正負margin collapsing、BFC、左右float / clear / shrink-to-fit、Inline / Atomic Inline / replaced element、line-height、baseline、折り返しを実装する。Layout Tree、Paint、Hit Testing、Scroll extentは同じgeometryとrevisionを参照する。

Flexboxは単一・複数line、grow / shrink freeze、automatic minimum size、baseline、percentage、absolute childのstatic position、gap、order、nested Flex / GridとWriting Mode axisを実装する。Text、Input、Button、Blockをitemとして扱い、最終geometryをPaint、Overflow、Scroll extent、Hit Testingへ共有する。

Gridはexplicit / implicit track、intrinsic / flexible sizing、cyclic percentageの有限fallback、line / span / area placement、sparse / dense auto-placement、alignment、auto-fill / auto-fit、Grid / Flex相互nestを実装する。Subgridは親track、gap、named lineとnested contributionを共有する。Masonryは無効な宣言として局所化する。

Writing Modeはhorizontal-tb、vertical-rl、vertical-lrとltr / rtlをlogical axisからphysical geometryへ変換する。Tableはauto / fixed sizingとspan / border collapseを扱う。Multi-columnはfragment identityを保ったままbalance / auto fill、span、break controlをPaint / Hit Testing / Inspectorへ共有する。

PaintとHit Testingは同じStacking Context順、nested rounded clip、group opacity、2D transformを参照する。Hit Testingは逆Paint順とTransform逆行列を使い、`visibility:hidden`とclip外を除外する。通常alphaのsource-overを扱い、Blend Mode、Filter、Backdrop Filter、3D Transformは未対応である。

TransitionとKeyframes AnimationはOpacity、Color、Background Color、Border Color、Outline Color、2D Transform、width / heightを補間する。Duration、正負のDelay、cubic-bezier/stepsを含むEasing、Iteration、Direction、Fill Mode、Pauseを扱う。transform / opacityだけのframeは基準Layout Treeと静的Display Listを再利用し、width / heightなどlayoutへ影響するsampleだけを再構築する。終了、非表示、offscreen、cancel、stale Pageではinvalidationを停止する。Discrete Animation、Animation Event、Web Animations API、Scroll-driven Animationは未対応である。

Layoutは1 pass 2秒、visual box 32,768件、全fragment 65,536件、recursion 192段、line box 16,384件、float 4,096件、column fragmentainer 32件、balancing 64 iterationを上限とし、超過subtreeを有限fallbackへ変換する。

WPTから適応した回帰テストと出典は[Web Platform Tests由来テスト](wpt.md)に記録する。

## 外部Pageと動的更新

top-level documentとiframeはsame / cross-origin stylesheetを取得できる。最終URLのscheme、mixed content、HTTP status、`text/css` MIME、sizeを検証し、失敗したstylesheet / importだけを無効にして残りのDocumentを描画する。CSS resource取得はCookieやAuthorizationをStyle modelへ渡さず、Network policyが選んだcredentials modeに従う。

JavaScriptによるattribute、class、tree、`innerHTML` mutation後はStyle revisionを増やし、Computed Style、Layout Tree、Display List、Hit Test、Inspector snapshotを同じrevisionから再生成する。iframeは親Layoutの置換要素としてborder box、clip、scrollを持ち、子DocumentのPaintを親のclip内へ合成する。

Imageは`picture` / `source` / `srcset` / `sizes`、PNG / JPEG / GIF静止Frame / WebP、安全な静的SVG subset、load / error、alt fallback、late relayoutを扱う。v0.18.0では`fetchpriority`、preload、`loading=lazy`、viewport proximityを共通priorityへ写像し、初期Page commitと画像fetch / decodeを分離する。同じURLのfetch body / decodeをPage generation内でcoalesceし、target size、DPR、object-fit、filterごとのrasterとbackground / gradient / filter resultをbounded LRUで再利用する。image mutationは対象resourceだけを後続commitで更新し、Navigation、close、Engine切替でqueue、in-flight fetch、cacheを破棄する。Web FontはCORSを通過したWOFF / WOFF2をdecodeし、完了時に影響Textを再計測する。JS PageのshaperはBrowser chromeから分離し、system font discoveryでCJK glyphへfallbackする。SVG内script、event handler、external resource、`foreignObject`、animation、filter、font load、Navigationは実行しない。

v0.19.0の「CSS Layout 2026 Baseline」は[CSS Layout 2026 Showcase](../examples/css-layout-2026)、[Browser-grade Compatibility Showcase](../examples/browser-grade-compat)、[Modern Web Compatibility Showcase](../examples/modern-web-compat)、Tailwind CSS v4.1.12実build artifact、固定Next.js / SvelteKit build、選定WPT、Chromium / Firefox differential、Visual / Performance Regressionで固定したscreen向け範囲を指す。未知の公開サイトとのpixel完全一致、framework / React全API、Shadow DOM、Canvas、video、sideways / Ruby / 完全bidi、Print / Paged Media、全CSS仕様への適合は保証しない。system font discovery、hydration、JavaScript animation state、image lifecycleは`JS`を明示選択したTabだけで有効にし、Go Runtimeの実行経路とobjectを共有しない。
