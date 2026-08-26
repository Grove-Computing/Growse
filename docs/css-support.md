# CSS対応表

この表はGrowse v0.14.0の実装を基準とする。「部分対応」は一般的な値を扱えるが、仕様全体を実装していない機能を表す。document、stylesheet、`@import`、background resourceは最初の有効な`<base href>`から解決する。

## SelectorとCascade

| 機能 | 状態 | 制限 |
|---|---|---|
| Type、Universal、Class、ID、Compound | 対応 | Namespace Selectorは未対応 |
| Attribute Selector | 対応 | `i`、`s`などのModifierは未対応 |
| Descendant、Child、Adjacent、General Sibling | 対応 | Column Combinatorは未対応 |
| Structural Pseudo-class | 対応 | `:root`、`:empty`、child/of-type系、`an+b` |
| `:not()` | 部分対応 | Selectors Level 3のSimple Selector引数だけ |
| Link、Form State、`:hover`、`:focus` | 対応 | `:visited`は意図的に未対応 |
| `::before`、`::after` | 部分対応 | 引用文字列の`content`だけ |
| Cascade | 対応 | UA、Author、Inline、`!important`、詳細度、ソース順 |
| CSS-wide Keyword | 対応 | 実装済みPropertyの`inherit`、`initial`、`unset` |
| Custom Propertyと`var()` | 対応 | Fallbackと循環検出を含む |

## ValueとQuery

| 機能 | 状態 | 制限 |
|---|---|---|
| Absolute Length | 対応 | `px`、`in`、`cm`、`mm`、`q`、`pt`、`pc` |
| Relative Length | 対応 | `em`、`rem`、`ex`、`ch`、`vw`、`vh`、`vmin`、`vmax`、percentage |
| `calc()` | 対応 | Length/Percentageの四則演算。非互換Dimension、ゼロ除算、非有限値は無効 |
| CSS Color Level 3 | 対応 | Named Color、hex、rgb(a)、hsl(a)、`transparent`、`currentColor` |
| Media Query | 部分対応 | `all`、`screen`、width/height、orientation、resolution、color scheme、hover、pointer、`prefers-reduced-motion` |
| `@import` | 部分対応 | Stylesheet先頭のsame / cross-origin HTTP(S)。redirect、CSS MIME、mixed content、循環、深度8、32件、合計8 MiBを検証 |
| `@keyframes` | 対応 | from/to、percentage、複数selector、同一offsetのCascade。1 Stylesheetあたり256 rule |

## Property

| Property | 状態 | 制限 |
|---|---|---|
| `display` | 対応 | `none`、`inline`、`block`、`inline-block`、`flex`、`inline-flex`、`grid`、`inline-grid` |
| width/height、min/max、`box-sizing` | 対応 | Blockと主要Inline Block。Intrinsic Sizing Keywordは未対応 |
| margin、padding | 部分対応 | 1〜4値とLonghand、percentage。`auto` marginはFlex/Grid itemで対応 |
| border | 対応 | 各Shorthand/Longhandと`solid`、`dotted`、`dashed`、`double` |
| `border-radius` | 対応 | 1〜4値、slash区切りの楕円角、percentage |
| overflow | 部分対応 | `visible`、`hidden`、`auto`、`scroll`のclipとscroll extent。Scrollbar UIは未対応 |
| `background-color` | 対応 | alpha合成を含む |
| `background-image` | 部分対応 | 複数HTTP(S) PNG/JPEG/GIF、`linear-gradient()`、`radial-gradient()`。`data:`、conic gradientは未対応 |
| `background-repeat/position/size` | 部分対応 | 複数Layer、主要Keyword、1〜2値、length/percentage、cover/contain。origin/clipの独立指定は未対応 |
| `font-size`、`font-weight`、`line-height` | 対応 | 同梱Go FontのRegular/Boldを使用 |
| `white-space` | 対応 | normal、nowrap、pre、pre-wrap、pre-line |
| `color` | 対応 | CSS Color Level 3の対応範囲 |
| `text-decoration-line/color` | 対応 | underline、overline、line-through |
| `opacity` | 対応 | 0〜1。1未満はStacking Contextとoffscreen groupを生成 |
| `flex-direction`、`flex-wrap`、`flex-flow` | 対応 | horizontal writing modeのrow/column、reverse、wrap |
| `flex-grow`、`flex-shrink`、`flex-basis`、`flex` | 対応 | Length、Percentage、`auto`、`content`。indefinite sizeのPercentageはauto相当 |
| `justify-content` | 対応 | flex-start/end、center、space-between/around/evenly |
| `align-items`、`align-self`、`align-content` | 対応 | stretch、flex-start/end、center、baseline、対応する分散値 |
| `row-gap`、`column-gap`、`gap` | 対応 | LengthとPercentage。単一のrow/column gap |
| `order` | 対応 | 視覚順とPaint順だけを変更し、DOM・focus順は維持 |
| `aspect-ratio` | 部分対応 | Flex itemのdefiniteな片軸から他軸を転送。replaced element固有比率は未対応 |
| Grid track | 対応 | fixed/percentage、`auto`、min/max-content、`fr`、`minmax()`、`fit-content()`、fixed/auto `repeat()`、named line |
| Grid placement | 対応 | numbered/named line、`span`、template area、sparse/dense auto-placement、implicit track |
| Grid alignment | 対応 | `justify/align-items`、`justify/align-self`、content alignment、`place-*`、gap、auto margin、`order` |
| `position`、inset | 部分対応 | relative、absolute、fixed、sticky。absoluteはpositioned ancestor、fixedはViewport基準。stickyはdocument scrollに対するtop制約を中心に対応 |
| `z-index` | 対応 | positioned elementの`auto`またはinteger。opacity/transformと共通のStacking Context順を使用 |
| `box-shadow`、`text-shadow` | 対応 | 複数shadow、blur/spread、inset、alpha color |
| `outline`、`outline-offset` | 対応 | width/style/colorとoffset |
| `transform`、`transform-origin` | 対応 | 2D translate/scale/rotate/skew/matrixと複数function。3D、perspectiveは未対応 |
| `transition-*`、`transition` | 部分対応 | opacity、transform、主要Color。複数Transition、list matching、delay、Easing、中断・反転 |
| `animation-*`、`animation` | 部分対応 | 複数Keyframes Animation、delay、iteration、direction、fill、play-state。加算・累積合成は未対応 |
| `visibility` | 対応 | `visible`、`hidden`。`display:none`とは別にLayout geometryを保持 |
| `font-family/style`、`text-align` | 未対応 | Declarationを描画へ反映しない |
| `word-break`、`overflow-wrap`、letter/word spacing、`vertical-align` | 未対応 | Declarationを描画へ反映しない |

## LayoutとPaint

Block Flow、隣接Blockのmargin collapsing、Inline Text Run、Atomic Inline Block、実フォント計測、line-height、baseline、折り返し、Overflow Clipを実装する。

Flexboxはhorizontal writing modeで、単一・複数line、grow/shrinkのfreeze、min/maxとautomatic minimum size、alignment、auto margin、gap、order、nested flex、inline-flex baselineを実装する。Text、Input、Button、Blockをitemとして扱い、最終geometryをPaint、Overflow、Scroll extent、Hit Testingへ共有する。Intrinsic contributionはTextと対応済みInputを中心とする。Vertical Writing ModeとFragmentationは未対応である。

Gridはexplicit/implicit track、intrinsic/flexible sizing、line/span/area placement、sparse/dense auto-placement、alignment、auto-fill/auto-fit、Grid/Flex相互nestを実装する。Subgrid、masonry、Vertical Writing Mode、Fragmentationは未対応である。

PaintとHit Testingは同じStacking Context順、nested rounded clip、group opacity、2D transformを参照する。Hit Testingは逆Paint順とTransform逆行列を使い、`visibility:hidden`とclip外を除外する。通常alphaのsource-overを扱い、Blend Mode、Filter、Backdrop Filter、3D Transformは未対応である。

TransitionとKeyframes AnimationはOpacity、Color、Background Color、Border Color、Outline Color、2D Transformを補間する。Duration、正負のDelay、cubic-bezier/stepsを含むEasing、Iteration、Direction、Fill Mode、Pauseを扱い、active Animationがある間だけFrameを更新する。Layoutを変更するProperty、Discrete Animation、Animation Event、Web Animations API、Scroll-driven Animationは未対応である。

Float、Multi-columnは未対応である。

WPTから適応した回帰テストと出典は[Web Platform Tests由来テスト](wpt.md)に記録する。

## 外部Pageと動的更新

top-level documentとiframeはsame / cross-origin stylesheetを取得できる。最終URLのscheme、mixed content、HTTP status、`text/css` MIME、sizeを検証し、失敗したstylesheet / importだけを無効にして残りのDocumentを描画する。CSS resource取得はCookieやAuthorizationをStyle modelへ渡さず、Network policyが選んだcredentials modeに従う。

JavaScriptによるattribute、class、tree、`innerHTML` mutation後はStyle revisionを増やし、Computed Style、Layout Tree、Display List、Hit Test、Inspector snapshotを同じrevisionから再生成する。iframeは親Layoutの置換要素としてborder box、clip、scrollを持ち、子DocumentのPaintを親のclip内へ合成する。

v0.14.0の「通常サイト描画」は[External Web Platform Showcase](../examples/external-web-platform)と選定WPTで固定した範囲を指す。未知の公開サイトとのpixel完全一致、Web Font、SVG、Canvas、video、全CSS仕様への適合は保証しない。
