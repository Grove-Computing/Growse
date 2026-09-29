package ui

import (
	"errors"
	"image"
	"image/color"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"github.com/Grove-Computing/Growse/internal/browser"
	"github.com/Grove-Computing/Growse/internal/homeconfig"
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

func newHomeInputTestUI(t *testing.T) (*BrowserUI, *input.Router, *layout.Context, *homeTabState) {
	t.Helper()
	ui := NewBrowserUI(&stubNavigator{}, nil)
	t.Cleanup(ui.Close)
	router := new(input.Router)
	gtx := &layout.Context{Ops: new(op.Ops), Source: router.Source(), Constraints: layout.Exact(image.Pt(1000, 760)), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}
	ui.Layout(*gtx)
	router.Frame(gtx.Ops)
	gtx.Reset()
	ui.Layout(*gtx)
	router.Frame(gtx.Ops)
	return ui, router, gtx, ui.homeState(0)
}

func TestHomeNativeEditingKeyboardSelectionAndEscapePreserveQuery(t *testing.T) {
	ui, router, gtx, state := newHomeInputTestUI(t)
	router.Queue(key.EditEvent{Range: key.Range{Start: 0, End: 0}, Text: "日本語"})
	gtx.Reset()
	ui.Layout(*gtx)
	router.Frame(gtx.Ops)
	if state.editor.Text() != "日本語" || len(state.candidates) == 0 || state.candidates[0].Query != "日本語" {
		t.Fatalf("IME-compatible edit state = text %q candidates %+v", state.editor.Text(), state.candidates)
	}

	router.Queue(key.Event{Name: key.NameDownArrow, State: key.Press})
	gtx.Reset()
	ui.Layout(*gtx)
	router.Frame(gtx.Ops)
	if state.selected != 0 {
		t.Fatalf("keyboard selected = %d", state.selected)
	}
	router.Queue(key.Event{Name: key.NameEscape, State: key.Press})
	gtx.Reset()
	ui.Layout(*gtx)
	if state.editor.Text() != "日本語" || len(state.candidates) != 0 {
		t.Fatalf("escape state = text %q candidates %+v", state.editor.Text(), state.candidates)
	}
}

func TestHomeMouseCandidateExecutionPreservesQuery(t *testing.T) {
	ui := NewBrowserUI(&stubNavigator{}, nil)
	defer ui.Close()
	state := ui.homeState(0)
	state.editor.SetText("gopher")
	ui.refreshHomeSuggestions(state)
	if len(state.candidates) == 0 {
		t.Fatal("home candidate missing")
	}
	state.rows[0].Click()
	gtx := layout.Context{Ops: new(op.Ops), Constraints: layout.Exact(image.Pt(1000, 760)), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}
	ui.handleHomeActions(gtx)
	if state.editor.Text() != "gopher" {
		t.Fatalf("mouse execution changed query to %q", state.editor.Text())
	}
	if ui.homeVisible() {
		t.Fatal("mouse candidate did not execute in current tab")
	}
}

func TestHomeCandidateDispositionPreservesSourceQuery(t *testing.T) {
	for _, test := range []struct {
		name        string
		disposition omniboxDisposition
		foreground  bool
	}{
		{"foreground", omniboxNewForegroundTab, true},
		{"background", omniboxNewBackgroundTab, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			session := browser.NewSession(func() *browser.Browser {
				loader := &controlledNavigationLoader{started: make(chan struct{}, 1), release: make(chan struct{})}
				close(loader.release)
				return browser.New(loader)
			})
			first, err := session.NewTab(nil)
			if err != nil {
				t.Fatal(err)
			}
			ui := NewBrowserUIWithTabs(nil, session, nil)
			defer ui.Close()
			ui.syncActiveTabChrome()
			state := ui.homeState(first.ID)
			state.editor.SetText("keep this query")
			ui.submitHomeCandidate(state, omnibox.Candidate{Source: omnibox.HistorySource, URL: "https://example.test/"}, test.disposition)
			tabs := session.Tabs()
			if len(tabs) != 2 || tabs[1].Active != test.foreground {
				t.Fatalf("tabs after %s = %+v", test.name, tabs)
			}
			if state.editor.Text() != "keep this query" {
				t.Fatalf("source home query = %q", state.editor.Text())
			}
		})
	}
}

func TestHomeShortcutEditorAddsEditsReordersAndDeletes(t *testing.T) {
	ui := NewBrowserUI(nil, nil)
	panel := &ui.homePanel
	panel.open = true
	gtx := layout.Context{Ops: new(op.Ops), Constraints: layout.Exact(image.Pt(1000, 760)), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}

	panel.title.SetText("Docs")
	panel.rawURL.SetText("https://Example.com:443/docs/#section")
	panel.save.Click()
	ui.handleHomeSettingsActions(gtx)
	panel.add.Click()
	ui.handleHomeSettingsActions(gtx)
	panel.title.SetText("News")
	panel.rawURL.SetText("https://news.example/")
	panel.save.Click()
	ui.handleHomeSettingsActions(gtx)
	if got := ui.homeSettings.Shortcuts; len(got) != 2 || got[0].URL != "https://Example.com:443/docs/" {
		t.Fatalf("added shortcuts = %#v", got)
	}

	panel.rows[1].up.Click()
	ui.handleHomeSettingsActions(gtx)
	if ui.homeSettings.Shortcuts[0].Title != "News" {
		t.Fatalf("reordered shortcuts = %#v", ui.homeSettings.Shortcuts)
	}
	panel.rows[0].edit.Click()
	ui.handleHomeSettingsActions(gtx)
	panel.title.SetText("Docs merged")
	panel.rawURL.SetText("https://example.com/docs")
	panel.save.Click()
	ui.handleHomeSettingsActions(gtx)
	if len(ui.homeSettings.Shortcuts) != 1 || ui.homeSettings.Shortcuts[0].Title != "Docs merged" {
		t.Fatalf("merged shortcuts = %#v", ui.homeSettings.Shortcuts)
	}
	panel.rows[0].remove.Click()
	ui.handleHomeSettingsActions(gtx)
	if len(ui.homeSettings.Shortcuts) != 0 {
		t.Fatalf("deleted shortcuts = %#v", ui.homeSettings.Shortcuts)
	}
}

func TestHomeShortcutValidationErrorPreservesSettings(t *testing.T) {
	ui := NewBrowserUI(nil, nil)
	_, _ = ui.homeSettings.Add("Safe", "https://safe.example/")
	before := ui.homeSettings.Clone()
	ui.homePanel.open = true
	ui.homePanel.title.SetText("Credential")
	ui.homePanel.rawURL.SetText("https://user:secret@example.com/")
	ui.homePanel.save.Click()
	gtx := layout.Context{Ops: new(op.Ops), Constraints: layout.Exact(image.Pt(1000, 760)), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}
	ui.handleHomeSettingsActions(gtx)
	if ui.homePanel.errorMessage == "" || !reflect.DeepEqual(ui.homeSettings, before) {
		t.Fatalf("invalid edit = error %q settings %#v", ui.homePanel.errorMessage, ui.homeSettings)
	}
}

func TestHomeShortcutDisplayDoesNotStartNavigation(t *testing.T) {
	ui := NewBrowserUI(&stubNavigator{}, nil)
	_, _ = ui.homeSettings.Add("No favicon request", "https://resource.example/")
	gtx := layout.Context{Ops: new(op.Ops), Constraints: layout.Exact(image.Pt(600, 300)), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}
	ui.layoutHomeShortcuts(gtx)
	if len(ui.navigations) != 0 {
		t.Fatalf("shortcut display started navigation: %#v", ui.navigations)
	}
	if got := shortcutLetter(" 日本語 "); got != "日" {
		t.Fatalf("fallback letter = %q", got)
	}
}

func TestHomeSettingsPersistAcrossWindows(t *testing.T) {
	root := t.TempDir()
	first := NewBrowserUI(nil, nil)
	if err := first.OpenSearchProfile(root); err != nil {
		t.Fatal(err)
	}
	settings := first.homeSettings.Clone()
	settings.Background = homeconfig.BackgroundForest
	if _, err := settings.Add("Growse", "https://growse.example/"); err != nil {
		t.Fatal(err)
	}
	if !first.applyHomeSettings(settings) {
		t.Fatal("first window did not save home settings")
	}

	second := NewBrowserUI(nil, nil)
	if err := second.OpenSearchProfile(root); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(second.homeSettings, settings) {
		t.Fatalf("second window settings = %#v, want %#v", second.homeSettings, settings)
	}
}

func TestHomeSettingsWriteFailurePreservesVisibleState(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "profile")
	ui := NewBrowserUI(nil, nil)
	if err := ui.OpenSearchProfile(root); err != nil {
		t.Fatal(err)
	}
	before := ui.homeSettings.Clone()
	if err := os.Rename(root, root+"-moved"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(root, []byte("profile path blocked"), 0600); err != nil {
		t.Fatal(err)
	}
	next := before.Clone()
	next.Background = homeconfig.BackgroundDusk
	if ui.applyHomeSettings(next) {
		t.Fatal("write unexpectedly succeeded")
	}
	if !reflect.DeepEqual(ui.homeSettings, before) {
		t.Fatalf("failed write changed visible settings: %#v", ui.homeSettings)
	}
}

func TestBundledHomeBackgroundPresetsAreOpaqueAndDistinct(t *testing.T) {
	seen := map[color.NRGBA]bool{}
	for _, preset := range homeconfig.BackgroundPresets() {
		background, surface := homeBackgroundColors(preset.ID)
		if background.A != 255 || surface.A < 240 {
			t.Fatalf("preset %q colors = %#v %#v", preset.ID, background, surface)
		}
		seen[background] = true
	}
	if len(seen) != len(homeconfig.BackgroundPresets()) {
		t.Fatalf("background colors are not distinct: %#v", seen)
	}
}

func TestHomeBackForwardRestoresInternalViewWithoutNetworkTraversal(t *testing.T) {
	target, _ := url.Parse("https://example.test/page")
	navigator := &stubNavigator{}
	ui := NewBrowserUI(navigator, nil)
	state := ui.homeState(0)
	ui.beginHomeNavigation(0)
	navigator.page = browser.NewPage(target)
	ui.setCommittedOmniboxURL(0, target.String(), true)
	delete(ui.homeRollbacks, 0)
	if state.visible || state.viewIndex != 1 {
		t.Fatalf("page view state = %+v", state)
	}

	if !ui.traverseHomeHistory(-1) || !state.visible || ui.address.Text() != "" {
		t.Fatalf("back to home = visible %v address %q", state.visible, ui.address.Text())
	}
	if !ui.traverseHomeHistory(1) || state.visible || ui.address.Text() != target.String() {
		t.Fatalf("forward to page = visible %v address %q", state.visible, ui.address.Text())
	}
}

func TestHomeActionBackRestoresExistingPage(t *testing.T) {
	target, _ := url.Parse("https://existing.example/")
	navigator := &stubNavigator{page: browser.NewPage(target)}
	ui := NewBrowserUI(navigator, nil)
	ui.setCommittedOmniboxURL(0, target.String(), true)
	ui.showHome()
	if !ui.homeVisible() || !ui.canTraverseHomeHistory(0, -1) {
		t.Fatal("Home action did not append internal history view")
	}
	if !ui.traverseHomeHistory(-1) || ui.homeVisible() || ui.address.Text() != target.String() {
		t.Fatalf("back restored visible=%v address=%q", ui.homeVisible(), ui.address.Text())
	}
}

func TestHomeStateAndSelectionAreTabOwnedAndRequestsCancelOnSwitch(t *testing.T) {
	session := browser.NewSession()
	first, _ := session.NewTab(nil)
	second, _ := session.NewTab(nil)
	ui := NewBrowserUIWithTabs(nil, session, nil)
	defer ui.Close()
	ui.syncActiveTabChrome()
	ui.SetSuggestionSnapshot(omnibox.Snapshot{History: []omnibox.Candidate{
		{Primary: "guide one", URL: "https://one.example/"},
		{Primary: "guide two", URL: "https://two.example/"},
	}})
	firstState := ui.homeState(first.ID)
	firstState.editor.SetText("guide")
	ui.refreshHomeSuggestions(firstState)
	firstState.selected = 1

	if _, err := session.SelectTab(second.ID); err != nil {
		t.Fatal(err)
	}
	ui.syncActiveTabChrome()
	if len(firstState.candidates) != 0 || !firstState.needsRefresh {
		t.Fatalf("switched state retained request results: %+v", firstState.candidates)
	}
	secondState := ui.homeState(second.ID)
	secondState.editor.SetText("other")
	if _, err := session.SelectTab(first.ID); err != nil {
		t.Fatal(err)
	}
	ui.syncActiveTabChrome()
	gtx := layout.Context{Ops: new(op.Ops), Constraints: layout.Exact(image.Pt(1000, 760)), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}
	ui.layoutHome(gtx)
	if firstState.editor.Text() != "guide" || firstState.selected != 1 {
		t.Fatalf("restored first state = query %q selected %d", firstState.editor.Text(), firstState.selected)
	}
	if secondState.editor.Text() != "other" {
		t.Fatalf("second query changed to %q", secondState.editor.Text())
	}
}

func TestFailedNavigationRestoresHomeViewAndQuery(t *testing.T) {
	ui := NewBrowserUI(&stubNavigator{}, nil)
	state := ui.homeState(0)
	state.editor.SetText("keep failed query")
	ui.beginHomeNavigation(0)
	before := ui.homeRollbacks[0]
	ui.navigations[0] = tabNavigation{id: 7, cancel: func() {}, homeBefore: &before}
	ui.results <- navigationResult{id: 7, tabID: 0, err: errors.New("failed")}
	ui.consumeNavigationResult()
	if !ui.homeVisible() || state.editor.Text() != "keep failed query" {
		t.Fatalf("failed navigation state = visible %v query %q", ui.homeVisible(), state.editor.Text())
	}
}

func flattenSemanticNodes(nodes []input.SemanticNode) []input.SemanticNode {
	// Router.AppendSemantics already returns every node in tree order while also
	// retaining child links for assistive-technology consumers.
	return nodes
}

func TestHomeAccessibleNamesRolesStatesAndErrors(t *testing.T) {
	ui := NewBrowserUI(nil, nil)
	_, _ = ui.homeSettings.Add("Docs", "https://docs.example/")
	state := ui.homeState(0)
	state.editor.SetText("guide")
	ui.SetSuggestionSnapshot(omnibox.Snapshot{History: []omnibox.Candidate{{Primary: "Guide history", URL: "https://history.example/"}}})
	ui.refreshHomeSuggestions(state)
	state.selected = min(1, len(state.candidates)-1)
	state.errorMessage = "検索できません"
	router := new(input.Router)
	gtx := layout.Context{Ops: new(op.Ops), Source: router.Source(), Constraints: layout.Exact(image.Pt(1056, 700)), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}
	ui.layoutHome(gtx)
	router.Frame(gtx.Ops)
	nodes := flattenSemanticNodes(router.AppendSemantics(nil))

	var editor, candidate, shortcut, errorNode bool
	for _, node := range nodes {
		switch {
		case node.Desc.Class == semantic.Editor && strings.Contains(node.Desc.Description, "ホーム検索"):
			editor = true
		case node.Desc.Class == semantic.Button && strings.Contains(node.Desc.Description, "History: Guide history") && node.Desc.Selected:
			candidate = true
		case node.Desc.Class == semantic.Button && strings.Contains(node.Desc.Description, "ショートカット: Docs"):
			shortcut = true
		case strings.Contains(node.Desc.Description, "入力エラー: 検索できません"):
			errorNode = true
		}
	}
	if !editor || !candidate || !shortcut || !errorNode {
		t.Fatalf("semantic states editor=%v candidate=%v shortcut=%v error=%v nodes=%+v", editor, candidate, shortcut, errorNode, nodes)
	}
}

func TestHomeBackgroundPresetExposesRadioSelection(t *testing.T) {
	ui := NewBrowserUI(nil, nil)
	ui.homePanel.open = true
	ui.homeSettings.Background = homeconfig.BackgroundDusk
	router := new(input.Router)
	gtx := layout.Context{Ops: new(op.Ops), Source: router.Source(), Constraints: layout.Exact(image.Pt(720, 700)), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}
	ui.layoutHomeSettings(gtx)
	router.Frame(gtx.Ops)
	selected := 0
	for _, node := range flattenSemanticNodes(router.AppendSemantics(nil)) {
		if node.Desc.Class == semantic.RadioButton && strings.HasPrefix(node.Desc.Description, "ホーム背景:") && node.Desc.Selected {
			selected++
		}
	}
	if selected != 1 {
		t.Fatalf("selected background radio count = %d", selected)
	}
}

func TestHomeSearchFocusIndicatorAndKeyboardTabOrder(t *testing.T) {
	unfocusedColor, unfocusedWidth := homeSearchBorder(false)
	focusedColor, focusedWidth := homeSearchBorder(true)
	if focusedColor == unfocusedColor || focusedWidth <= unfocusedWidth {
		t.Fatalf("focus indicator = color %#v/%#v width %v/%v", unfocusedColor, focusedColor, unfocusedWidth, focusedWidth)
	}
	ui, router, gtx, state := newHomeInputTestUI(t)
	router.Queue(key.Event{Name: key.NameTab, State: key.Press})
	gtx.Reset()
	ui.Layout(*gtx)
	router.Frame(gtx.Ops)
	gtx.Reset()
	ui.Layout(*gtx)
	if !gtx.Focused(&state.search) {
		t.Fatal("Tab did not move focus from home search to search action")
	}
}

func TestHomeSettingsKeyboardMovesFromTitleToURL(t *testing.T) {
	ui := NewBrowserUI(nil, nil)
	ui.homePanel.open = true
	ui.homePanel.focusPending = true
	router := new(input.Router)
	gtx := &layout.Context{Ops: new(op.Ops), Source: router.Source(), Constraints: layout.Exact(image.Pt(900, 700)), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}
	homeFrame := func() {
		gtx.Reset()
		ui.Layout(*gtx)
		router.Frame(gtx.Ops)
	}
	homeFrame()
	homeFrame()
	if !gtx.Focused(&ui.homePanel.title) {
		t.Fatal("home settings did not focus title editor")
	}
	router.Queue(key.EditEvent{Range: key.Range{Start: 0, End: 0}, Text: "Docs"})
	homeFrame()
	router.Queue(key.Event{Name: key.NameTab, State: key.Press})
	homeFrame()
	homeFrame()
	if !gtx.Focused(&ui.homePanel.rawURL) {
		t.Fatal("Tab did not move from title to URL editor")
	}
	router.Queue(key.EditEvent{Range: key.Range{Start: 0, End: 0}, Text: "https://example.com/"})
	homeFrame()
	if ui.homePanel.title.Text() != "Docs" || ui.homePanel.rawURL.Text() != "https://example.com/" {
		t.Fatalf("keyboard fields = %q / %q", ui.homePanel.title.Text(), ui.homePanel.rawURL.Text())
	}
}
