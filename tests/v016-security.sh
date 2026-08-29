#!/usr/bin/env bash
set -euo pipefail

readonly browser_pattern='Test(ImageResourceCacheEvictsLeastRecentlyUsedEntryWithinLimits|ImageResourceCacheIsReleasedOnEngineSwitch|ImageLimitsRejectDimensionSurfaceCountAndDecoderPanic|NavigationCancelsStaleResponsiveImageCompletion|PageCloseCancelsPendingResponsiveImage|AnimationInvalidationStopsForOffscreenHiddenCanceledAndStalePage|NavigationAndReloadDiscardPreviousPageAnimations|BackgroundTabSuppressesFrameCallbacksUntilSelected)$'
readonly ui_pattern='Test(PageImagePaintCacheUsesBoundedLRUAndClearsOnClose|PageImagePaintCacheDropsPreviousGeneration|PaintOnlyAnimationFramesReuseLayoutTree|LayoutAnimationFramesRebuildLayoutAndDisplayList)$'
readonly style_pattern='Test(AnimationSafetyLimits|ClassifyAnimationDamageSeparatesCompositePaintAndLayout)$'
readonly scheduler_pattern='Test(SchedulerBoundsPendingTimerAndFrameCallbacks|SchedulerBudgetsAnimationFrameCallbacksAcrossTicks|AnimationFrameRegistersCancelsAndDefersNestedCallback)$'

for seed in 1600 1601 1602; do
  go test -count=1 -shuffle="${seed}" -run "${browser_pattern}" ./internal/browser
  go test -count=1 -shuffle="${seed}" -run "${ui_pattern}" ./internal/ui
  go test -count=1 -shuffle="${seed}" -run "${style_pattern}" ./internal/style
  go test -count=1 -shuffle="${seed}" -run "${scheduler_pattern}" ./internal/webapi/scheduler
done

echo "v0.16.0安全境界検証成功: image cache上限/失敗/cancel、animation budget/Lifecycle/stale generation"
