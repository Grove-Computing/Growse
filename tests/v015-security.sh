#!/usr/bin/env bash
set -euo pipefail

readonly css_pattern='Test(ParseRejectsMalformedCombinators|RuleAndSelectorQuotasAreBounded|FunctionalSelectorLayerAndFunctionDepthQuotasAreLocal|CSSNestingDepthIsBoundedLocally)$'
readonly browser_pattern='Test(ValidateSVGRejectsExecutableAndExternalContent|ValidateSVGAppliesEncodedTreeAndPathLimits|ImageLimitsRejectDimensionSurfaceCountAndDecoderPanic|SVGAndFontLimitsRejectOnlyAffectedResource|NavigationCancelsStaleResponsiveImageCompletion)$'
readonly runtime_pattern='Test(DynamicScriptCountAndInsertionDepthAreFinite|ResourceReprepareCountAndFailureRetryLimits|ModuleGraphEnforcesCountDepthAndTotalSize|ObserverMutationLoopBecomesFiniteRuntimeError|ObserverRecordLimitBecomesFiniteRuntimeError|StoppedRuntimeDropsDynamicResourcesHydrationCallbacksAndMutations|IframeSameOriginAccessCrossOriginProxyAndStaleGeneration)$'

for seed in 1510 1511 1512; do
  go test -count=1 -shuffle="${seed}" -run "${css_pattern}" ./internal/css
  go test -count=1 -shuffle="${seed}" -run "${browser_pattern}" ./internal/browser
  go test -count=1 -shuffle="${seed}" -run "${runtime_pattern}" ./internal/runtime/javascript
done

echo "v0.15.0 Security Test成功: malformed CSS/SVG/font/image, resource chain, observer loop, mutation storm, stale generation"
