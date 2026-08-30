#!/usr/bin/env bash
set -euo pipefail

go test ./internal/conformance
go test ./examples/browser-grade-compat
go test ./examples/modern-web-compat -run 'Test(NextJSSSRFixtureHydratesWithoutReplacingDOM|SvelteKitSSRFixtureHydratesAndEnhancesForm|RealSiteFixtureLoadsGeneratedCSSImagesAnimationAndHydration|RealSiteVisualRegression)$' -count=1
go test ./internal/browser -run 'Test(ImageGalleryWarmAccessDoesNotDecodeOrResizeAgain|CompatibilityDiagnostics|FrameScheduler)' -count=1
go test ./internal/ui -run 'Test(PaintOnlyAnimationFramesReuseLayoutTree|ScrollFrameDoesNotRebuildLayoutOrDisplayList)' -count=1

echo "v0.17.0 Browser differential conformance成功: Unit, Integration, Differential, Visual, Performance"
