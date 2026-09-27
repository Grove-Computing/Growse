package ui

import (
	"testing"

	"github.com/Grove-Computing/Growse/internal/browser"
	"github.com/Grove-Computing/Growse/internal/omnibox"
)

func TestSuggestionPipelineConnectedToTabsAndLocalSnapshots(t *testing.T) {
	session := browser.NewSession(func() *browser.Browser { return browser.New(nil) })
	first, _ := session.NewTab(nil)
	second, _ := session.NewTab(nil)
	ui := NewBrowserUIWithTabs(nil, session, nil)
	defer ui.Close()
	ui.syncActiveTabChrome()
	ui.address.SetText("@tabs")
	ui.refreshSuggestions()
	generation, results := ui.suggestions.Results()
	if len(results) != 2 || results[0].TabID != uint64(first.ID) || results[1].TabID != uint64(second.ID) {
		t.Fatalf("tabs = %+v", results)
	}
	session.SelectTab(second.ID)
	ui.syncActiveTabChrome()
	next, results := ui.suggestions.Results()
	if next <= generation || len(results) != 0 {
		t.Fatal("tab switch retained stale results")
	}
	ui.address.SetText("local")
	ui.SetSuggestionSnapshot(omnibox.Snapshot{History: []omnibox.Candidate{{Primary: "local history", URL: "https://local.example/"}}})
	_, results = ui.suggestions.Results()
	if len(results) != 2 || results[1].Source != omnibox.HistorySource {
		t.Fatalf("history = %+v", results)
	}
	ui.SetSuggestionProvider(nil, false)
	next, _ = ui.suggestions.Results()
	if next <= generation {
		t.Fatal("provider change did not invalidate generation")
	}
}
