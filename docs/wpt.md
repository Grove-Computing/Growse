# Web Platform Tests由来テスト

GrowseはWeb Platform Tests（WPT）をブラウザで直接実行せず、v0.4.0で対応する範囲をGoのUnit Testへ縮約して管理する。

- Upstream: `web-platform-tests/wpt`
- Revision: `816bbf3ebae17dc6866deb65b2286b1a1c162819`
- License: WPTリポジトリの`LICENSE.md`（3-Clause BSD）
- 配置: `internal/style/wpt_test.go`

## 対応表

| Growse Test | WPT Source | 適応内容 | 差分 |
|---|---|---|---|
| `TestWPTSpecificity001AttributeBeatsType` | `css/CSS2/cascade/specificity-001.xht` | 視覚的な緑色判定をComputed Colorの判定へ変換 | なし |
| `TestWPTCalcDivideByZeroIsRejectedWithoutCrash` | `css/css-values/calc-catch-divide-by-0.html` | ゼロ除算ケースをLength parserへ入力し、panicしないことを確認 | WPTの現行仕様はInfinity/NaNを直列化するが、v0.4.0は非有限値をDeclaration無効として扱う |
| `TestWPTBorderRadius001ZeroProducesSquareCorners` | `css/css-backgrounds/border-radius-001.xht` | ReftestをComputed Border Radiusのゼロ値判定へ変換 | なし |

Upstreamのファイル全体はコピーせず、assertionの意味と最小入力だけを移植する。ケースを追加または更新するときは、Revision、Source、適応内容、および意図的な差分をこの表へ記録する。
