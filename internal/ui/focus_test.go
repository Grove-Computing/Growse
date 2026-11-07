package ui

import (
	"image"
	"net/url"
	"testing"

	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"github.com/Grove-Computing/Growse/internal/browser"
	"github.com/Grove-Computing/Growse/internal/searchdata"
)

func TestChromeKeyboardFocusOrderIncludesOmniboxBookmarkAndGo(t *testing.T) {
	ui, router, gtx := newSuggestionTestUI(t)
	ui.address.SetText("guide")
	ui.refreshSuggestions()
	suggestionFrame(ui, router, gtx)

	router.Queue(key.Event{Name: key.NameTab, State: key.Press})
	suggestionFrame(ui, router, gtx)
	if !gtx.Focused(&ui.bookmarkButton) || ui.suggestionPopup.open {
		t.Fatalf("Tab focus bookmark=%t popup=%t", gtx.Focused(&ui.bookmarkButton), ui.suggestionPopup.open)
	}
	router.Queue(key.Event{Name: key.NameTab, State: key.Press})
	suggestionFrame(ui, router, gtx)
	if !gtx.Focused(&ui.goButton) {
		t.Fatal("second Tab did not focus Go action")
	}
	router.Queue(key.Event{Name: key.NameTab, Modifiers: key.ModShift, State: key.Press})
	suggestionFrame(ui, router, gtx)
	if !gtx.Focused(&ui.bookmarkButton) {
		t.Fatal("Shift+Tab did not restore Bookmark focus")
	}
}

func TestBookmarkActionWorksFromKeyboard(t *testing.T) {
	pageURL, err := url.Parse("https://bookmark.example/guide")
	if err != nil {
		t.Fatal(err)
	}
	ui := NewBrowserUI(&stubNavigator{page: &browser.Page{URL: pageURL}}, nil)
	defer ui.Close()
	store := searchdata.NewMemoryStore()
	ui.SetSearchDataStore(store)
	router := new(input.Router)
	gtx := &layout.Context{Ops: new(op.Ops), Source: router.Source(), Constraints: layout.Exact(image.Pt(1000, 700)), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}
	suggestionFrame(ui, router, gtx)
	gtx.Execute(key.FocusCmd{Tag: &ui.bookmarkButton})
	suggestionFrame(ui, router, gtx)
	router.Queue(key.Event{Name: key.NameReturn, State: key.Press}, key.Event{Name: key.NameReturn, State: key.Release})
	suggestionFrame(ui, router, gtx)
	if _, ok := store.Bookmark(pageURL.String()); !ok {
		t.Fatal("keyboard Bookmark action did not save the current page")
	}
}

func TestFindBarKeyboardFocusOrder(t *testing.T) {
	ui := NewBrowserUI(&stubNavigator{}, nil)
	defer ui.Close()
	state := ui.ensureFindState()
	state.open = true
	router := new(input.Router)
	gtx := &layout.Context{Ops: new(op.Ops), Source: router.Source(), Constraints: layout.Exact(image.Pt(1000, 700)), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}
	suggestionFrame(ui, router, gtx)
	gtx.Execute(key.FocusCmd{Tag: state.editor})
	suggestionFrame(ui, router, gtx)
	router.Queue(key.Event{Name: key.NameTab, State: key.Press})
	suggestionFrame(ui, router, gtx)
	if !gtx.Focused(&state.previous) {
		t.Fatal("find Tab order did not move editor -> previous")
	}
	router.Queue(key.Event{Name: key.NameTab, Modifiers: key.ModShift, State: key.Press})
	suggestionFrame(ui, router, gtx)
	if !gtx.Focused(state.editor) {
		t.Fatal("find Shift+Tab did not restore editor focus")
	}
}

func TestSearchPanelKeyboardFocusOrderAndActivation(t *testing.T) {
	ui := NewBrowserUI(nil, nil)
	defer ui.Close()
	router := new(input.Router)
	gtx := &layout.Context{Ops: new(op.Ops), Source: router.Source(), Constraints: layout.Exact(image.Pt(1000, 700)), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}
	suggestionFrame(ui, router, gtx)
	ui.openSearchPanel(*gtx)
	suggestionFrame(ui, router, gtx)
	router.Queue(key.Event{Name: key.NameTab, State: key.Press})
	suggestionFrame(ui, router, gtx)
	if !gtx.Focused(&ui.searchPanel.allButton) {
		t.Fatal("search panel Tab order did not move editor -> All")
	}
	router.Queue(key.Event{Name: key.NameTab, State: key.Press})
	suggestionFrame(ui, router, gtx)
	if !gtx.Focused(&ui.searchPanel.tabsButton) {
		t.Fatal("search panel Tabs scope was not focused")
	}
	router.Queue(key.Event{Name: key.NameReturn, State: key.Press}, key.Event{Name: key.NameReturn, State: key.Release})
	suggestionFrame(ui, router, gtx)
	if ui.searchPanel.scope != "tabs" {
		t.Fatalf("keyboard scope = %q, want tabs", ui.searchPanel.scope)
	}
}

func TestProviderSettingsKeyboardFocusOrder(t *testing.T) {
	ui := NewBrowserUI(nil, nil)
	defer ui.Close()
	router := new(input.Router)
	gtx := &layout.Context{Ops: new(op.Ops), Source: router.Source(), Constraints: layout.Exact(image.Pt(1000, 700)), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}
	ui.providerPanel.open = true
	ui.providerPanel.focusPending = true
	suggestionFrame(ui, router, gtx)
	suggestionFrame(ui, router, gtx)
	if !gtx.Focused(&ui.providerPanel.remote) {
		t.Fatal("provider settings did not focus the first action")
	}
	router.Queue(key.Event{Name: key.NameTab, State: key.Press})
	suggestionFrame(ui, router, gtx)
	if !gtx.Focused(&ui.providerPanel.close) {
		t.Fatal("provider settings Tab order did not reach Close")
	}
}

func TestFocusIndicatorStylesAreVisible(t *testing.T) {
	unfocusedColor, unfocusedWidth := focusBorder(false)
	focusedColor, focusedWidth := focusBorder(true)
	if focusedColor == unfocusedColor || focusedWidth <= unfocusedWidth {
		t.Fatalf("focus border = %#v/%v, unfocused %#v/%v", focusedColor, focusedWidth, unfocusedColor, unfocusedWidth)
	}
	normal := unfocusedColor
	if focused := focusSurface(true, normal); focused == normal || focused.A == 0 {
		t.Fatalf("focus surface = %#v, normal %#v", focused, normal)
	}
}
