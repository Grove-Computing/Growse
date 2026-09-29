package ui

import (
	"image"
	"net/url"
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

func TestSearchPanelTabResultSelectsExistingTabByKeyboardAndMouse(t *testing.T) {
	for _, useMouse := range []bool{false, true} {
		t.Run(map[bool]string{false: "keyboard", true: "mouse"}[useMouse], func(t *testing.T) {
			session := browser.NewSession(func() *browser.Browser { return browser.New(nil) })
			first, err := session.NewTab(nil)
			if err != nil {
				t.Fatal(err)
			}
			second, err := session.NewTab(nil)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := session.SelectTab(first.ID); err != nil {
				t.Fatal(err)
			}
			ui := NewBrowserUIWithTabs(nil, session, nil)
			defer ui.Close()
			router := new(input.Router)
			gtx := layout.Context{Ops: new(op.Ops), Source: router.Source(), Constraints: layout.Exact(image.Pt(1000, 800)), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}
			ui.Layout(gtx)
			router.Frame(gtx.Ops)
			router.Queue(key.Event{Name: "A", Modifiers: key.ModShortcut | key.ModShift, State: key.Press})
			gtx.Reset()
			ui.Layout(gtx)
			router.Frame(gtx.Ops)
			ui.setSearchPanelScope(omnibox.Tabs)
			if len(ui.searchPanel.candidates) != 2 {
				t.Fatalf("tab candidates = %#v", ui.searchPanel.candidates)
			}
			if useMouse {
				ui.searchPanel.rows[1].Click()
				gtx.Reset()
				ui.Layout(gtx)
			} else {
				router.Queue(key.Event{Name: key.NameEnd, State: key.Press})
				gtx.Reset()
				ui.Layout(gtx)
				router.Frame(gtx.Ops)
				router.Queue(key.Event{Name: key.NameReturn, State: key.Press})
				gtx.Reset()
				ui.Layout(gtx)
			}
			active, ok := session.ActiveTab()
			if !ok || active.ID != second.ID || len(session.Tabs()) != 2 {
				t.Fatalf("active tab = (%+v, %t), tabs=%d", active, ok, len(session.Tabs()))
			}
			if ui.searchPanel.open {
				t.Fatal("search panel remained open after Tab selection")
			}
		})
	}
}

func TestSearchPanelHistoryAndBookmarkUseRequestedDisposition(t *testing.T) {
	session := browser.NewSession(func() *browser.Browser { return browser.New(nil) })
	first, err := session.NewTab(nil)
	if err != nil {
		t.Fatal(err)
	}
	ui := NewBrowserUIWithTabs(nil, session, nil)
	defer ui.Close()
	gtx := layout.Context{Ops: new(op.Ops), Constraints: layout.Exact(image.Pt(1000, 800)), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}

	ui.searchPanel.open = true
	ui.executeSearchPanelCandidate(gtx, omnibox.Candidate{Source: omnibox.BookmarkSource, URL: "https://bookmark.example/"}, omniboxNewBackgroundTab)
	if tabs := session.Tabs(); len(tabs) != 2 || !tabs[0].Active || tabs[0].ID != first.ID || tabs[1].URL != "https://bookmark.example/" {
		t.Fatalf("background bookmark tabs = %+v", tabs)
	}

	ui.searchPanel.open = true
	ui.executeSearchPanelCandidate(gtx, omnibox.Candidate{Source: omnibox.HistorySource, URL: "https://history.example/"}, omniboxNewForegroundTab)
	tabs := session.Tabs()
	if len(tabs) != 3 || tabs[2].URL != "https://history.example/" || !tabs[2].Active {
		t.Fatalf("foreground history tabs = %+v", tabs)
	}
}

func TestSearchPanelDeletesHistoryByKeyboardAndBookmarkByMouse(t *testing.T) {
	store := searchdata.NewMemoryStore()
	when := time.Date(2026, time.September, 29, 2, 0, 0, 0, time.UTC)
	if err := store.RecordNavigation(searchdata.Navigation{URL: "https://history.example/delete", Title: "Delete history", VisitedAt: when, TopLevel: true, Success: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveBookmark("", "https://bookmark.example/delete", "Delete bookmark"); err != nil {
		t.Fatal(err)
	}
	ui := NewBrowserUI(nil, nil)
	defer ui.Close()
	ui.SetSearchDataStore(store)
	router := new(input.Router)
	gtx := layout.Context{Ops: new(op.Ops), Source: router.Source(), Constraints: layout.Exact(image.Pt(1000, 800)), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}
	ui.openSearchPanel(gtx)
	deadline := time.Now().Add(time.Second)
	for ui.searchPanel.localApplied != ui.searchPanel.localGeneration && time.Now().Before(deadline) {
		runtime.Gosched()
		ui.syncSearchPanel()
	}
	ui.Layout(gtx)
	router.Frame(gtx.Ops)

	historyIndex := -1
	for index, candidate := range ui.searchPanel.candidates {
		if candidate.Source == omnibox.HistorySource {
			historyIndex = index
		}
	}
	if historyIndex < 0 {
		t.Fatalf("history result missing: %#v", ui.searchPanel.candidates)
	}
	ui.searchPanel.selected = historyIndex
	router.Queue(key.Event{Name: key.NameDeleteForward, State: key.Press})
	gtx.Reset()
	ui.Layout(gtx)
	if len(store.History()) != 0 || len(store.SuggestionSnapshot(t.Context(), "delete").History) != 0 {
		t.Fatal("keyboard deletion left history in data or index")
	}

	deadline = time.Now().Add(time.Second)
	for ui.searchPanel.localApplied != ui.searchPanel.localGeneration && time.Now().Before(deadline) {
		runtime.Gosched()
		ui.syncSearchPanel()
	}
	bookmarkIndex := -1
	for index, candidate := range ui.searchPanel.candidates {
		if candidate.Source == omnibox.BookmarkSource {
			bookmarkIndex = index
		}
	}
	if bookmarkIndex < 0 {
		t.Fatalf("bookmark result missing: %#v", ui.searchPanel.candidates)
	}
	gtx.Reset()
	ui.Layout(gtx)
	ui.searchPanel.deleteButtons[bookmarkIndex].Click()
	gtx.Reset()
	ui.Layout(gtx)
	if len(store.Bookmarks()) != 0 || len(store.SuggestionSnapshot(t.Context(), "delete").Bookmarks) != 0 {
		t.Fatal("mouse deletion left bookmark in data or index")
	}
}

func TestSearchPanelClearHistoryRequiresConfirmationAndKeepsCurrentPage(t *testing.T) {
	pageURL, err := url.Parse("https://current.example/page")
	if err != nil {
		t.Fatal(err)
	}
	page := browser.NewPage(pageURL)
	navigator := &stubNavigator{page: page}
	store := searchdata.NewMemoryStore()
	for index, rawURL := range []string{"https://one.example/", "https://two.example/"} {
		if err := store.RecordNavigation(searchdata.Navigation{URL: rawURL, Title: "History", VisitedAt: time.Unix(int64(index+1), 0), TopLevel: true, Success: true}); err != nil {
			t.Fatal(err)
		}
	}
	ui := NewBrowserUI(navigator, nil)
	defer ui.Close()
	ui.SetSearchDataStore(store)
	ui.searchPanel.open = true
	gtx := layout.Context{Ops: new(op.Ops), Constraints: layout.Exact(image.Pt(1000, 800)), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}
	ui.layoutSearchPanel(gtx)

	ui.searchPanel.clearHistoryButton.Click()
	ui.handleSearchPanelActions(gtx)
	if !ui.searchPanel.confirmClearHistory || ui.searchPanel.clearHistoryCount != 2 || len(store.History()) != 2 {
		t.Fatalf("confirmation state=%t count=%d history=%d", ui.searchPanel.confirmClearHistory, ui.searchPanel.clearHistoryCount, len(store.History()))
	}
	gtx.Reset()
	ui.layoutSearchPanel(gtx)
	ui.searchPanel.confirmClearHistoryButton.Click()
	ui.handleSearchPanelActions(gtx)
	if ui.searchPanel.confirmClearHistory || len(store.History()) != 0 || len(store.SuggestionSnapshot(t.Context(), "").History) != 0 {
		t.Fatal("confirmed clear did not remove history from data and index")
	}
	if navigator.Page() != page || navigator.Page().URL.String() != pageURL.String() {
		t.Fatal("clearing history changed the current page")
	}
}
