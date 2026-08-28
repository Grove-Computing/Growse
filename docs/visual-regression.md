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

`internal/paint/TestPersistentAppStatesVisualRegression`は480×600 CSS px、scale 1、Go Regular 72 dpi、固定timestampでinitial、restored、routing、saving、saved、cache revalidation、errorの各状態を描画する。amd64では`internal/paint/testdata/persistent-app.golden.json`、macOS Apple SiliconではRasterizerのアーキテクチャ差を記録した`internal/paint/testdata/persistent-app-darwin-arm64.golden.json`と比較する。Layout geometryとDisplay Listは両方のReferenceで一致させる。`examples/persistent-app/TestPersistentAppRestoresNotesRoutesAndReusesHTTPCache`はAnimation Frame、Back / Forward後のScroll・Paint復元、および再起動後のStorage / Cache再利用を`examples/persistent-app/testdata/lifecycle.golden.json`と比較する。

```sh
go test ./internal/paint ./examples/persistent-app -run 'PersistentApp'
```

## v0.12.0 WebGo DevTools

`internal/ui/TestDevToolsPanelsVisualRegression`は1280×800、下部280 pxのDevTools geometryと、Console、Inspector、Networkの通常・空・error・truncated状態をsemantic snapshotとして比較する。password valueとNetwork query valueのmaskも同じReferenceに含める。Integration側は`examples/devtools/TestDevToolsShowcaseExercisesAllPanelsWithoutCredentialLeaks`がlocal fixtureのDOM / Style / Layout、redirect、cache、HTTP error、timeoutを検証する。

```sh
go test ./internal/ui ./examples/devtools -run DevTools
```

## v0.15.0 Modern Web Compatibility

`examples/modern-web-compat/TestFrameworkVisualRegression`は`viewport=1024x720 scale=1 font=goregular clock=fixed`を環境identityとして、Next.jsのSSR初期、hydration後、操作後、Tailwindのresponsive / narrow、resource failureとDevTools診断をsemantic Layout / Paint snapshotへ固定する。Node text、display、color、background、root geometry、box / paint / font / image件数、Style revision、有限diagnostic categoryを`examples/modern-web-compat/testdata/framework-visual.golden.json`と比較する。

既存の`internal/ui/TestDevToolsPanelsVisualRegression`と合わせ、次のrelease gateで実行する。

```sh
bash tests/v015-visual.sh
```

差分が出た場合はSSR Node identity、hydration state、responsive query、fallback resource、DevTools categoryのどれが変化したかを先に確認する。環境identityまたはgoldenだけを先に更新せず、意図したStyle / Layout / Paint変更を対応するUnit / Framework Testでも固定する。
