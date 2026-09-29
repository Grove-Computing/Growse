package ui

import (
	"image"
	"reflect"
	"runtime"
	"testing"
	"time"

	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/Grove-Computing/Growse/internal/browser"
	"github.com/Grove-Computing/Growse/internal/omnibox"
	"github.com/Grove-Computing/Growse/internal/searchdata"
)

func TestSearchPanelUsesOmniboxRankingForEveryScope(t *testing.T) {
	now := time.Date(2026, time.September, 29, 0, 0, 0, 0, time.UTC)
	snapshot := omnibox.Snapshot{
		Now:       now,
		Tabs:      []omnibox.Candidate{{Primary: "Guide tab", URL: "https://tab.example/guide", TabID: 7}},
		History:   []omnibox.Candidate{{Primary: "Guide history", URL: "https://history.example/guide", LastVisited: now.Add(-time.Hour)}},
		Bookmarks: []omnibox.Candidate{{Primary: "Guide bookmark", URL: "https://bookmark.example/guide", LastVisited: now.Add(-2 * time.Hour)}},
	}
	for _, scope := range []omnibox.Scope{"", omnibox.Tabs, omnibox.History, omnibox.Bookmarks} {
		got := rankSearchPanel("guide", scope, snapshot)
		input := "guide"
		if scope != "" {
			input = "@" + string(scope) + " guide"
		}
		var want []omnibox.Candidate
		for _, candidate := range omnibox.Rank(input, snapshot, nil) {
			if candidate.Source == omnibox.TabSource || candidate.Source == omnibox.HistorySource || candidate.Source == omnibox.BookmarkSource {
				want = append(want, candidate)
			}
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("scope %q results = %#v, want %#v", scope, got, want)
		}
	}
}

func TestSearchPanelShortcutAndLocalSnapshot(t *testing.T) {
	session := browser.NewSession(func() *browser.Browser { return browser.New(nil) })
	if _, err := session.NewTab(nil); err != nil {
		t.Fatal(err)
	}
	ui := NewBrowserUIWithTabs(nil, session, nil)
	defer ui.Close()
	store := searchdata.NewMemoryStore()
	when := time.Date(2026, time.September, 29, 1, 0, 0, 0, time.UTC)
	if err := store.RecordNavigation(searchdata.Navigation{URL: "https://history.example/guide", Title: "Guide history", VisitedAt: when, TopLevel: true, Success: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveBookmark("", "https://bookmark.example/guide", "Guide bookmark"); err != nil {
		t.Fatal(err)
	}
	ui.SetSearchDataStore(store)

	router := new(input.Router)
	gtx := layout.Context{Ops: new(op.Ops), Source: router.Source(), Constraints: layout.Exact(image.Pt(1000, 800)), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}
	ui.Layout(gtx)
	router.Frame(gtx.Ops)
	router.Queue(key.Event{Name: "A", Modifiers: key.ModShortcut | key.ModShift, State: key.Press})
	gtx.Reset()
	ui.Layout(gtx)
	if !ui.searchPanel.open || !gtx.Focused(ui.searchPanel.editor) {
		t.Fatalf("search panel open=%t focused=%t", ui.searchPanel.open, gtx.Focused(ui.searchPanel.editor))
	}

	ui.searchPanel.editor.SetText("guide")
	ui.refreshSearchPanel()
	deadline := time.Now().Add(time.Second)
	for ui.searchPanel.localApplied != ui.searchPanel.localGeneration && time.Now().Before(deadline) {
		runtime.Gosched()
		ui.syncSearchPanel()
	}
	if len(ui.searchPanel.candidates) != 2 {
		t.Fatalf("all candidates = %#v", ui.searchPanel.candidates)
	}
	ui.setSearchPanelScope(omnibox.Bookmarks)
	deadline = time.Now().Add(time.Second)
	for ui.searchPanel.localApplied != ui.searchPanel.localGeneration && time.Now().Before(deadline) {
		runtime.Gosched()
		ui.syncSearchPanel()
	}
	if len(ui.searchPanel.candidates) != 1 || ui.searchPanel.candidates[0].Source != omnibox.BookmarkSource {
		t.Fatalf("bookmark candidates = %#v", ui.searchPanel.candidates)
	}
}
