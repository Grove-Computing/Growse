#!/usr/bin/env bash
set -euo pipefail

go test ./internal/omnibox -run 'Test(URLDifferential|Classify|SearchURL|UnicodeMatchKinds|Ranking)' -count=1
go test ./internal/findpage -run 'Test(UnicodeDifferential|SearchNormalizesCaseCJKAndCombiningSequences|SearchWholeWordUsesUnicodeBoundaries)' -count=1

go test ./internal/ui -run 'Test(OmniboxIME|HomeIME|SearchPanelIME|FindIME|ChromeKeyboardFocusOrder|FindBarKeyboardFocusOrder|SearchPanelKeyboardFocusOrder|ProviderSettingsKeyboardFocusOrder|HomeSearchFocusIndicator)' -count=1

go test ./internal/searchprovider -run 'Test(TransportBoundsPrivacyAndEncoding|AtomicSettingsRecovery)' -count=1
go test ./internal/integration -run 'TestStorageHistoryAndCredentialsAreAbsentFromLogsAndErrors' -count=1
go test ./internal/searchdata -run 'Test(ProfileRestartRebuildsIndexAndRecoversBackup|ProfileNeverPersistsIndexQueryOrPageText)' -count=1
go test ./internal/homeconfig -run 'TestStoreRestartsAndRecoversBackupOrDefaults' -count=1

go test ./internal/omnibox -run '^TestRankingBoundsAfterMatchingAndScoring$' -bench '^BenchmarkRankHistorySnapshot$' -benchtime=1x -benchmem -count=1

echo "v0.21.0 Search回帰検証成功: URL / Unicode Differential, Keyboard / IME, Privacy, Storage Recovery, Performance"
