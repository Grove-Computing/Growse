#!/usr/bin/env bash
set -euo pipefail

bash tests/examples.sh

go test -count=1 -shuffle=1520 ./internal/runtime/yaegi
go test -count=1 -shuffle=1520 -run 'Test(CounterDemoIncrementsThroughWebGoClick|DOMMutationPublishesOneRenderRevision|ClassStyleHoverAndWebGoMutationReconcileAnimations|WebGoMutationRecomputesStyles|GoEngineDoesNotFetchTextGoFromArbitraryInternetOrigin|EngineSelectionFetchesOnlySelectedExternalSource|WebGoNavigationUsesBrowserLifecycleAfterPageActivation|NavigationAndReloadStopPreviousPageRuntime|BrowserCloseReleasesPageClientStorageAndRuntimeReferences)$' ./internal/browser

echo "v0.15.0 Go回帰成功: WebGo Showcase, DOM, Event, CSS, Fetch, lifecycle"
