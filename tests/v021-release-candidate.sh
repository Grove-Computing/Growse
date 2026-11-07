#!/usr/bin/env bash
set -euo pipefail

artifact_dir="${GROWSE_V021_RC_ARTIFACT_DIR:-$(pwd)/artifacts/v0.21.0-rc}"

GROWSE_V021_RC_ARTIFACT_DIR="$artifact_dir" go test ./internal/ui \
  -run '^TestV021ReleaseCandidateRasterEvidence$' -count=1 -v

go test ./internal/ui -run 'Test(OmniboxIME|HomeIME|SearchPanelIME|FindIME|HomeNativeEditing|HomeMouseCandidate|HomeCandidateDisposition|HomeStateAndSelectionAreTabOwnedAndRequestsCancelOnSwitch|FailedNavigationRestoresHomeViewAndQuery|HomeBackForward|KeyboardReloadShortcuts|TabSelectionPreservesIndependentOmniboxEditingAndPreviewState|FindBarShortcutSearchStatusAndWrap|FindStateIsIsolatedPerTab|SearchPanelUsesOmniboxRankingForEveryScope|SearchPanelTabResultSelectsExistingTabByKeyboardAndMouse|SearchPanelHistoryAndBookmarkUseRequestedDisposition|SearchPanelDesktopAndNarrowLayoutsKeepLongMetadata|SuggestionPopupLongMetadataStaysInsideNarrowViewport)' -count=1

go test ./internal/homeconfig -run '^TestStoreRestartsAndRecoversBackupOrDefaults$' -count=1
go test ./internal/searchdata -run '^TestProfileRestartRebuildsIndexAndRecoversBackup$' -count=1
go test ./internal/searchprovider -run '^TestAtomicSettingsRecovery$' -count=1

for artifact in \
  desktop-home.png narrow-home.png \
  desktop-search-panel.png narrow-search-panel.png
do
  test -s "$artifact_dir/$artifact"
done

echo "v0.21.0 RC検証成功: desktop / narrow raster、IME、Web検索、Find、Tab / History / Bookmark、reload、Tab切替、再起動"
echo "visual evidence: $artifact_dir"
