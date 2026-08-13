# CSS対応表

この表はGrowse v0.4.0の実装を基準とする。「部分対応」は一般的な値を扱えるが、仕様全体を実装していない機能を表す。

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
| Media Query | 部分対応 | `all`、`screen`、width/height、orientation、resolution、color scheme、hover、pointer |
| `@import` | 部分対応 | Stylesheet先頭の同一Origin HTTP(S)だけ。循環・深度・件数・サイズ制限あり |

## Property

| Property | 状態 | 制限 |
|---|---|---|
| `display` | 対応 | `none`、`inline`、`block`、`inline-block` |
| width/height、min/max、`box-sizing` | 対応 | Blockと主要Inline Block。Intrinsic Sizing Keywordは未対応 |
| margin、padding | 部分対応 | 1〜4値とLonghand。percentage対応、`auto` marginは未対応 |
| border | 対応 | 各Shorthand/Longhandと`solid`、`dotted`、`dashed`、`double` |
| `border-radius` | 対応 | 1〜4値、slash区切りの楕円角、percentage |
| overflow | 部分対応 | `visible`、`hidden`、`auto`、`scroll`のclipとscroll extent。Scrollbar UIは未対応 |
| `background-color` | 対応 | alpha合成を含む |
| `background-image` | 部分対応 | 単一HTTP(S) PNG/JPEG/GIFまたは単一`linear-gradient()`。複数Layerと`data:`は未対応 |
| `background-repeat/position/size` | 部分対応 | 単一Layer、主要Keyword、1〜2値、length/percentage、cover/contain |
| `font-size`、`font-weight`、`line-height` | 対応 | 同梱Go FontのRegular/Boldを使用 |
| `white-space` | 対応 | normal、nowrap、pre、pre-wrap、pre-line |
| `color` | 対応 | CSS Color Level 3の対応範囲 |
| `text-decoration-line/color` | 対応 | underline、overline、line-through |
| `opacity` | 対応 | 0〜1。要素Subtreeの実効値を描画へ反映 |
| `visibility`、`font-family/style`、`text-align/transform` | 未対応 | v0.4.0ではDeclarationを描画へ反映しない |
| `word-break`、`overflow-wrap`、letter/word spacing、`vertical-align` | 未対応 | v0.4.0ではDeclarationを描画へ反映しない |

## LayoutとPaint

Block Flow、隣接Blockのmargin collapsing、Inline Text Run、Atomic Inline Block、実フォント計測、line-height、baseline、折り返し、Overflow Clipを実装する。Flexbox、Grid、Float、Positioned Layout、Multi-column、Vertical Writing Mode、Animation、Transitionは未対応である。

WPTから適応した回帰テストと出典は[Web Platform Tests由来テスト](wpt.md)に記録する。
