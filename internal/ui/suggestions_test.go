package ui

import (
	"fmt"
	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"image"
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

func suggestionFrame(ui *BrowserUI, router *input.Router, gtx *layout.Context) {
	gtx.Reset()
	ui.Layout(*gtx)
	router.Frame(gtx.Ops)
}

func newSuggestionTestUI(t *testing.T) (*BrowserUI, *input.Router, *layout.Context) {
	t.Helper()
	ui := NewBrowserUI(nil, nil)
	t.Cleanup(ui.Close)
	router := new(input.Router)
	gtx := &layout.Context{Ops: new(op.Ops), Source: router.Source(), Constraints: layout.Exact(image.Pt(1000, 800)), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}
	suggestionFrame(ui, router, gtx)
	gtx.Execute(key.FocusCmd{Tag: ui.address})
	suggestionFrame(ui, router, gtx)
	return ui, router, gtx
}

func TestSuggestionPopupKeyboardPreviewEscapeAndTabPreserveDraft(t *testing.T) {
	for _, closeKey := range []key.Name{key.NameEscape, key.NameTab} {
		t.Run(string(closeKey), func(t *testing.T) {
			ui, router, gtx := newSuggestionTestUI(t)
			ui.address.SetText("guide")
			var history []omnibox.Candidate
			for i := 0; i < 30; i++ {
				history = append(history, omnibox.Candidate{Primary: "guide title", URL: fmt.Sprintf("https://example.com/%02d", i)})
			}
			ui.SetSuggestionSnapshot(omnibox.Snapshot{History: history})
			suggestionFrame(ui, router, gtx)
			if len(ui.suggestionPopup.candidates) != omnibox.MaxVisibleCandidates {
				t.Fatal("popup did not apply 12-item bound")
			}
			for _, tc := range []struct {
				name  key.Name
				index int
			}{
				{key.NameDownArrow, 0}, {key.NameDownArrow, 1}, {key.NamePageDown, 6}, {key.NamePageUp, 1}, {key.NameEnd, 11}, {key.NameHome, 0}, {key.NameUpArrow, 11},
			} {
				router.Queue(key.Event{Name: tc.name, State: key.Press})
				suggestionFrame(ui, router, gtx)
				if ui.suggestionPopup.selected != tc.index {
					t.Fatalf("key %s selected %d, want %d", tc.name, ui.suggestionPopup.selected, tc.index)
				}
				if ui.address.Text() != "guide" {
					t.Fatal("preview replaced draft")
				}
			}
			if ui.omniboxStates[0].preview == "" {
				t.Fatal("no omnibox preview")
			}
			router.Queue(key.Event{Name: closeKey, State: key.Press})
			suggestionFrame(ui, router, gtx)
			if ui.suggestionPopup.open || ui.omniboxStates[0].preview != "" || ui.address.Text() != "guide" {
				t.Fatal("closing popup lost draft or preview")
			}
			_, results := ui.suggestions.Results()
			if len(results) != 0 {
				t.Fatal("popup close retained generation")
			}
		})
	}
}

func TestSuggestionPopupNativeEditingAndFocusClose(t *testing.T) {
	ui, router, gtx := newSuggestionTestUI(t)
	ui.address.SetText("")
	router.Queue(key.EditEvent{Range: key.Range{Start: 0, End: 0}, Text: "日本語"})
	suggestionFrame(ui, router, gtx)
	if !ui.suggestionPopup.open || len(ui.suggestionPopup.candidates) != 1 || ui.suggestionPopup.candidates[0].Query != "日本語" {
		t.Fatalf("edit did not generate candidates: %+v", ui.suggestionPopup.candidates)
	}
	gtx.Execute(key.FocusCmd{Tag: &ui.devToolsButton})
	suggestionFrame(ui, router, gtx)
	if ui.suggestionPopup.open || ui.address.Text() != "日本語" {
		t.Fatal("focus close lost input")
	}
}

func TestSuggestionPopupMouseHoverAndOutsideClick(t *testing.T) {
	ui, router, gtx := newSuggestionTestUI(t)
	ui.address.SetText("guide")
	ui.SetSuggestionSnapshot(omnibox.Snapshot{History: []omnibox.Candidate{{Primary: "guide title", URL: "https://example.com/"}}})
	suggestionFrame(ui, router, gtx)
	position := f32.Pt(float32(ui.suggestionPopup.bounds.Min.X+20), float32(ui.suggestionPopup.bounds.Min.Y+70))
	router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: position})
	suggestionFrame(ui, router, gtx)
	suggestionFrame(ui, router, gtx)
	if ui.suggestionPopup.selected != 1 {
		t.Fatalf("hover selected %d", ui.suggestionPopup.selected)
	}
	router.Queue(key.Event{Name: key.NameHome, State: key.Press})
	suggestionFrame(ui, router, gtx)
	suggestionFrame(ui, router, gtx)
	if ui.suggestionPopup.selected != 0 {
		t.Fatal("stationary mouse overrode keyboard selection")
	}
	position = f32.Pt(float32(ui.suggestionPopup.bounds.Min.X+20), float32(ui.suggestionPopup.bounds.Max.Y+30))
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: position})
	suggestionFrame(ui, router, gtx)
	if ui.suggestionPopup.open || ui.address.Text() != "guide" {
		t.Fatal("outside click did not safely close popup")
	}
}

func TestSuggestionPopupTabActionKeyboardAndMouse(t *testing.T) {
	for _, mouse := range []bool{false, true} {
		t.Run(fmt.Sprint(mouse), func(t *testing.T) {
			session := browser.NewSession(func() *browser.Browser { return browser.New(nil) })
			first, _ := session.NewTab(nil)
			second, _ := session.NewTab(nil)
			ui := NewBrowserUIWithTabs(nil, session, nil)
			defer ui.Close()
			router := new(input.Router)
			gtx := &layout.Context{Ops: new(op.Ops), Source: router.Source(), Constraints: layout.Exact(image.Pt(1000, 800)), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}
			suggestionFrame(ui, router, gtx)
			gtx.Execute(key.FocusCmd{Tag: ui.address})
			suggestionFrame(ui, router, gtx)
			ui.address.SetText("@tabs")
			ui.refreshSuggestions()
			suggestionFrame(ui, router, gtx)
			if mouse {
				position := f32.Pt(float32(ui.suggestionPopup.bounds.Min.X+20), float32(ui.suggestionPopup.bounds.Min.Y+70))
				router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: position})
				suggestionFrame(ui, router, gtx)
				router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: position})
				suggestionFrame(ui, router, gtx)
			} else {
				router.Queue(key.Event{Name: key.NameEnd, State: key.Press})
				suggestionFrame(ui, router, gtx)
				router.Queue(key.Event{Name: key.NameReturn, State: key.Press})
				suggestionFrame(ui, router, gtx)
			}
			active, _ := session.ActiveTab()
			if active.ID != second.ID || len(session.Tabs()) != 2 {
				t.Fatalf("action failed to switch existing Tab: %+v", active)
			}
			if ui.omniboxStates[first.ID].editor.Text() != "@tabs" {
				t.Fatal("Tab action lost source draft")
			}
		})
	}
}

func TestSuggestionPopupFitsViewport(t *testing.T) {
	ui := NewBrowserUI(nil, nil)
	defer ui.Close()
	ui.address.SetText("guide")
	ui.SetSuggestionSnapshot(omnibox.Snapshot{History: []omnibox.Candidate{{Primary: "guide title", URL: "https://example.com/"}}})
	for _, size := range []image.Point{image.Pt(1280, 800), image.Pt(320, 200), image.Pt(250, 100)} {
		gtx := layout.Context{Ops: new(op.Ops), Constraints: layout.Exact(size), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}
		viewport := calculateBrowserChromeGeometry(size, 224, 92).viewport
		ui.layoutSuggestions(gtx, viewport)
		if !ui.suggestionPopup.bounds.In(viewport) {
			t.Fatalf("popup %v outside %v", ui.suggestionPopup.bounds, viewport)
		}
	}
}
