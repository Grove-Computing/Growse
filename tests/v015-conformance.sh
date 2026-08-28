#!/usr/bin/env bash
set -euo pipefail

readonly runtime_pattern='Test(JavaScriptNodeRelationshipsMutationsFragmentsAndCloneKeepIdentity|JavaScriptElementReflectionSelectorsDatasetStyleAndHTMLMutation|DynamicClassicScriptsSnapshotFetchAndExecuteExactlyOnce|DynamicModuleAndModulePreloadShareGraphAndEvaluateOnce|CSSOMGeometryAndMediaQueriesUseBrowserSnapshots|MutationObserverDeliversBoundedRecordsAtCheckpoint|ResizeAndIntersectionObserversRunAfterFrame)$'
readonly style_pattern='Test(Level4SelectorMatchingHasScopeIsWhereAndComplexNot|CascadeLayerOrderImportantReversalAndRevertLayer)$'
readonly browser_pattern='Test(JavaScriptDynamicStylesheetAndPreloadUpdateBrowserStyleRevision|ImageCandidatesSelectPictureSourceByTypeMediaSizesAndScale|LoadWebFontsValidatesDescriptorsAndDecodesWOFF)$'

for seed in 1500 1501 1502; do
  go test -count=1 -shuffle="${seed}" -run "${runtime_pattern}" ./internal/runtime/javascript
  go test -count=1 -shuffle="${seed}" -run "${style_pattern}" ./internal/style
  go test -count=1 -shuffle="${seed}" -run "${browser_pattern}" ./internal/browser
done

echo "v0.15.0選定WPT成功: DOM, dynamic resource, selectors, layers, CSSOM, observers, image, font"
