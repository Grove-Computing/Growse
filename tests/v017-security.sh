#!/usr/bin/env bash
set -euo pipefail

readonly browser_pattern='Test(ImageLimitsRejectDimensionSurfaceCountAndDecoderPanic|ImageQueueSaturationUsesPlaceholdersAndBoundsDiagnostics|ImageAllocationAndCorruptAnimationFailuresStayLocal|ImageResizeHonorsCancellationWithoutRetainingSurfaceBudget|NavigationCancelsStaleResponsiveImageCompletion|PageCloseCancelsPendingResponsiveImage|ResourceQueueBoundsConcurrencyPendingWorkAndPreservesPriority|FrameSchedulerPrioritizesInputAndChromeWithinBudget|FrameSchedulerThrottlesBackgroundAndAnimationStorm|FrameGenerationRejectsCloseNavigationAndStaleResourceCompletion|CompatibilityDiagnosticsAggregateAndBoundMetadataWithoutPayloadCopies)$'
readonly ui_pattern='Test(PageImagePaintCacheUsesBoundedLRUAndClearsOnClose|ClosedTabRejectsDelayedNavigationResultAndReleasesUIState)$'
readonly conformance_pattern='Test(CompareRejectsMissingAndNonFiniteGeometry|CompareVisualRejectsDimensionMismatchAndEmptyRegion|ComparePerformanceRequiresFixedRunnerIdentity)$'

for seed in 1700 1701 1702; do
  go test -count=1 -shuffle="${seed}" -run "${browser_pattern}" ./internal/browser
  go test -count=1 -shuffle="${seed}" -run "${ui_pattern}" ./internal/ui
  go test -count=1 -shuffle="${seed}" -run "${conformance_pattern}" ./internal/conformance
done

echo "v0.17.0決定性安全検証成功: limit, failure, cancel, backpressure, lifecycle, stale generation"
