# Visual Regression Test

v0.6.0のVisual Regressionは`internal/paint/TestDashboardVisualRegression`で実行する。

- Viewportは320×240 CSS px、scaleは1に固定する。
- FontはGo moduleに埋め込まれたGo Regularを72 dpi、hintingなしで使用する。
- Referenceは`internal/paint/testdata/dashboard.golden.json`に保存する。
- ReferenceにはRGBA rasterのSHA-256、Layout geometry、Display List順、代表座標のHit Test結果を含める。

実行方法:

```sh
go test ./internal/paint -run TestDashboardVisualRegression
```

差分が出た場合は失敗ログの`actual`を確認し、意図したLayoutまたはPaint変更であることを個別Testでも確認してからReferenceを更新する。pixel hashだけを先に更新しない。

## v0.9.0 Persistent App

`internal/paint/TestPersistentAppStatesVisualRegression`は480×600 CSS px、scale 1、Go Regular 72 dpi、固定timestampでinitial、restored、routing、saving、saved、cache revalidation、errorの各状態を描画し、`internal/paint/testdata/persistent-app.golden.json`と比較する。`examples/persistent-app/TestPersistentAppRestoresNotesRoutesAndReusesHTTPCache`はAnimation Frame、Back / Forward後のScroll・Paint復元、および再起動後のStorage / Cache再利用を`examples/persistent-app/testdata/lifecycle.golden.json`と比較する。

```sh
go test ./internal/paint ./examples/persistent-app -run 'PersistentApp'
```
