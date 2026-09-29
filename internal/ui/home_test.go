package ui

import (
	"image"
	"net/url"
	"reflect"
	"testing"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"github.com/Grove-Computing/Growse/internal/browser"
	"github.com/Grove-Computing/Growse/internal/omnibox"
)

func TestEmptyAndCreatedTabsOpenInternalHome(t *testing.T) {
	session := browser.NewSession()
	first, err := session.NewTab(nil)
	if err != nil {
		t.Fatal(err)
	}
	ui := NewBrowserUIWithTabs(nil, session, nil)
	ui.syncActiveTabChrome()
	if !ui.homeVisible() {
		t.Fatal("initial empty tab did not open internal home")
	}

	gtx := layout.Context{Ops: new(op.Ops), Constraints: layout.Exact(image.Pt(1280, 800)), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}
	ui.createTab(gtx)
	active, ok := session.ActiveTab()
	if !ok || active.ID == first.ID || !ui.homeVisible() {
		t.Fatalf("created tab = (%+v, %v), home = %v", active, ok, ui.homeVisible())
	}
}

func TestTabWithExplicitURLDoesNotOpenInternalHome(t *testing.T) {
	session := browser.NewSession()
	target, err := url.Parse("https://example.test/path")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := session.NewTab(target); err != nil {
		t.Fatal(err)
	}
	ui := NewBrowserUIWithTabs(nil, session, nil)
	ui.syncActiveTabChrome()
	if ui.homeVisible() {
		t.Fatal("URL-initialized tab opened internal home")
	}
}

func TestHomeActionShowsBrowserOwnedViewWithoutReplacingPage(t *testing.T) {
	target, err := url.Parse("https://example.test/")
	if err != nil {
		t.Fatal(err)
	}
	page := browser.NewPage(target)
	navigator := &stubNavigator{page: page}
	ui := NewBrowserUI(navigator, nil)
	ui.setHomeVisible(0, false)

	ui.showHome()

	if !ui.homeVisible() {
		t.Fatal("Home action did not show internal home")
	}
	if navigator.Page() != page {
		t.Fatal("Home action replaced the underlying web page")
	}
	if ui.address.Text() != "" {
		t.Fatalf("Home omnibox = %q, want blank", ui.address.Text())
	}
}

func TestInternalHomeFillsViewport(t *testing.T) {
	ui := NewBrowserUI(nil, nil)
	gtx := layout.Context{Ops: new(op.Ops), Constraints: layout.Exact(image.Pt(900, 600)), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}
	if dims := ui.layoutViewport(gtx); dims.Size != image.Pt(900, 600) {
		t.Fatalf("home dimensions = %v", dims.Size)
	}
}

func TestHomeShowsDefaultProviderSearchAction(t *testing.T) {
	ui := NewBrowserUI(nil, nil)
	if got, want := ui.homeSearchLabel(), "DuckDuckGo で検索"; got != want {
		t.Fatalf("home search action = %q, want %q", got, want)
	}
}

func TestHomeSearchKeepsItsQueryWhenNavigationStarts(t *testing.T) {
	ui := NewBrowserUI(&stubNavigator{}, nil)
	state := ui.homeState(0)
	state.editor.SetText("gopher browser")

	ui.submitHomeSearch(state)

	if state.editor.Text() != "gopher browser" {
		t.Fatalf("home query = %q", state.editor.Text())
	}
	if ui.homeVisible() {
		t.Fatal("successful search dispatch left home visible")
	}
	ui.cancelTabNavigation(0)
}

func TestHomeAndOmniboxUseIdenticalClassificationSnapshotAndRanking(t *testing.T) {
	ui := NewBrowserUI(nil, nil)
	ui.SetSuggestionSnapshot(omnibox.Snapshot{
		Bookmarks: []omnibox.Candidate{{Primary: "Guide bookmark", URL: "https://bookmark.example/guide", VisitCount: 4}},
		History:   []omnibox.Candidate{{Primary: "Guide history", URL: "https://history.example/guide", TypedCount: 2}},
	})
	state := ui.homeState(0)
	state.editor.SetText("@bookmarks guide")
	ui.refreshHomeSuggestions(state)

	request := ui.buildSuggestionRequest(state.editor.Text(), ui.suggestionSnapshot)
	want := omnibox.Rank(request.input, request.snapshot, nil)
	if !reflect.DeepEqual(state.candidates, want) {
		t.Fatalf("home candidates = %#v, want shared rank %#v", state.candidates, want)
	}
	for _, candidate := range state.candidates {
		if candidate.Source != omnibox.BookmarkSource {
			t.Fatalf("scoped home candidate source = %v", candidate.Source)
		}
	}
}

func TestHomeAndOmniboxResolveProviderKeywordIdentically(t *testing.T) {
	ui := NewBrowserUI(nil, nil)
	state := ui.homeState(0)
	state.editor.SetText("ddg gopher browser")
	ui.refreshHomeSuggestions(state)
	request := ui.buildSuggestionRequest("ddg gopher browser", ui.suggestionSnapshot)

	if state.providerKeyword != request.providerKeyword || state.providerKeyword != "ddg" {
		t.Fatalf("home provider keyword = %q, shared = %q", state.providerKeyword, request.providerKeyword)
	}
	want := omnibox.Rank(request.input, request.snapshot, nil)
	if !reflect.DeepEqual(state.candidates, want) {
		t.Fatalf("provider candidates = %#v, want %#v", state.candidates, want)
	}
	gotURL, err := ui.providerSearchURL(request.input)
	if err != nil {
		t.Fatal(err)
	}
	wantURL, err := ui.providerSearchURL("gopher browser")
	if err != nil || gotURL != wantURL {
		t.Fatalf("search target = %q, want %q (err %v)", gotURL, wantURL, err)
	}
}
