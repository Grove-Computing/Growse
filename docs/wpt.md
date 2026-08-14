# Web Platform Tests由来テスト

GrowseはWeb Platform Tests（WPT）をブラウザで直接実行せず、対応する範囲をGoのUnit Testへ縮約して管理する。

- Upstream: `web-platform-tests/wpt`
- Revision: `816bbf3ebae17dc6866deb65b2286b1a1c162819`
- License: WPTリポジトリの`LICENSE.md`（3-Clause BSD）
- 配置: `internal/style/wpt_test.go`、`internal/layout/wpt_test.go`

## 対応表

| Growse Test | WPT Source | 適応内容 | 差分 |
|---|---|---|---|
| `TestWPTSpecificity001AttributeBeatsType` | `css/CSS2/cascade/specificity-001.xht` | 視覚的な緑色判定をComputed Colorの判定へ変換 | なし |
| `TestWPTCalcDivideByZeroIsRejectedWithoutCrash` | `css/css-values/calc-catch-divide-by-0.html` | ゼロ除算ケースをLength parserへ入力し、panicしないことを確認 | WPTの現行仕様はInfinity/NaNを直列化するが、v0.4.0は非有限値をDeclaration無効として扱う |
| `TestWPTBorderRadius001ZeroProducesSquareCorners` | `css/css-backgrounds/border-radius-001.xht` | ReftestをComputed Border Radiusのゼロ値判定へ変換 | なし |
| `TestWPTFlexGrow001DistributesPositiveFreeSpaceByFactor` | `css/css-flexbox/flex-grow-001.xht` | 240pxのmain sizeをgrow factor 0:1:2へ分配し、各itemの数値geometryを比較 | 視覚的な参照画像を数値比較へ変換 |
| `TestWPTFlexWrap002FormsTwoColumnFlexLines` | `css/css-flexbox/flex-wrap-002.html` | 100pxのcolumn main axisへ100pxのitemを2つ置き、2 line形成を比較 | fit-content cross sizeの詳細はGrowse対応範囲外 |
| `TestWPTFlexDirectionRowReverseMapsMainAxis` | `css/css-flexbox/flex-direction-row-reverse.html` | row-reverseのhorizontal/reverse axis変換を直接比較 | 視覚的な順序比較を内部axisの数値比較へ変換 |

Upstreamのファイル全体はコピーせず、assertionの意味と最小入力だけを移植する。ケースを追加または更新するときは、Revision、Source、適応内容、および意図的な差分をこの表へ記録する。
