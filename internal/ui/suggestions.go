package ui

import "github.com/Grove-Computing/Growse/internal/omnibox"

// SetSuggestionSnapshot supplies local source snapshots on the UI thread.
// History and bookmark persistence are owned by their respective data sources.
func (ui *BrowserUI) SetSuggestionSnapshot(snapshot omnibox.Snapshot) {
	ui.suggestionSnapshot = omnibox.Snapshot{
		History:   append([]omnibox.Candidate(nil), snapshot.History...),
		Bookmarks: append([]omnibox.Candidate(nil), snapshot.Bookmarks...),
	}
	ui.refreshSuggestions()
}

// SetSuggestionProvider cancels the previous provider before replacing it.
// External suggestions stay disabled unless explicitly enabled by the caller.
func (ui *BrowserUI) SetSuggestionProvider(fetch omnibox.RemoteFetcher, enabled bool) {
	ui.suggestions.Cancel()
	ui.suggestionFetcher, ui.remoteSuggestions = fetch, enabled
	ui.refreshSuggestions()
}

func (ui *BrowserUI) refreshSuggestions() {
	snapshot := ui.suggestionSnapshot
	for _, tab := range ui.tabSnapshots() {
		snapshot.Tabs = append(snapshot.Tabs, omnibox.Candidate{Primary: tabDisplayTitle(tab), URL: tab.URL, TabID: uint64(tab.ID)})
	}
	ui.suggestions.Update(ui.address.Text(), snapshot, ui.suggestionFetcher, ui.remoteSuggestions)
}
