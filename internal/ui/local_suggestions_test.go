package ui

import (
	"testing"
	"time"

	"github.com/Grove-Computing/Growse/internal/omnibox"
	"github.com/Grove-Computing/Growse/internal/searchdata"
)

func TestSearchDataIndexFeedsScopedOmniboxWithoutBlockingUI(t *testing.T) {
	store := searchdata.NewMemoryStore()
	if err := store.RecordNavigation(searchdata.Navigation{
		URL: "https://example.com/history", Title: "日本語の履歴", TopLevel: true, Success: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveBookmark("", "https://example.com/bookmark", "日本語のBookmark"); err != nil {
		t.Fatal(err)
	}
	ui := NewBrowserUI(&stubNavigator{}, nil)
	defer ui.Close()
	ui.SetSearchDataStore(store)
	ui.refreshSuggestionsFor("@history 日本語")
	wantGeneration := ui.suggestionPopup.localGeneration
	deadline := time.Now().Add(time.Second)
	for {
		generation, _ := ui.localSuggestions.Results()
		if generation == wantGeneration {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("local suggestion generation did not complete")
		}
		time.Sleep(time.Millisecond)
	}
	ui.syncSuggestions()
	if len(ui.suggestionPopup.candidates) != 1 || ui.suggestionPopup.candidates[0].Source != omnibox.HistorySource || ui.suggestionPopup.candidates[0].Primary != "日本語の履歴" {
		t.Fatalf("scoped local candidates = %#v", ui.suggestionPopup.candidates)
	}
}
