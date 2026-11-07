package ui

import (
	"image"
	"testing"

	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"github.com/Grove-Computing/Growse/internal/browser"
	"github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/style"
)

func queueComposition(router *input.Router, start, end int) {
	router.Queue(key.CompositionEvent(key.Range{Start: start, End: end}))
}

func TestOmniboxIMECompositionDefersSearchAndCandidateKeys(t *testing.T) {
	ui, router, gtx := newSuggestionTestUI(t)
	ui.address.SetText("")
	suggestionFrame(ui, router, gtx)
	ui.closeSuggestionPopup()

	queueComposition(router, 0, 0)
	suggestionFrame(ui, router, gtx)
	if !editorCompositionActive(ui.address) {
		t.Fatal("omnibox composition was not observed")
	}
	router.Queue(key.EditEvent{Range: key.Range{Start: 0, End: 0}, Text: "日本語"})
	suggestionFrame(ui, router, gtx)
	if ui.suggestionPopup.open {
		t.Fatal("omnibox searched uncommitted composition text")
	}

	for _, name := range []key.Name{key.NameDownArrow, key.NameUpArrow, key.NameEscape, key.NameReturn} {
		router.Queue(key.Event{Name: name, State: key.Press})
		suggestionFrame(ui, router, gtx)
		if ui.suggestionPopup.open || ui.loading {
			t.Fatalf("%s acted on an active omnibox composition", name)
		}
	}

	queueComposition(router, -1, -1)
	suggestionFrame(ui, router, gtx)
	if !ui.suggestionPopup.open || len(ui.suggestionPopup.candidates) == 0 || ui.suggestionPopup.candidates[0].Query != "日本語" {
		t.Fatalf("committed omnibox text was not searched: %+v", ui.suggestionPopup.candidates)
	}
}

func TestOmniboxIMECommittedUnicodeTextIsPreserved(t *testing.T) {
	for name, query := range map[string]string{
		"latin":     "café",
		"cjk":       "検索",
		"emoji":     "go🙂",
		"combining": "e\u0301",
		"rtl":       "بحث",
	} {
		t.Run(name, func(t *testing.T) {
			ui, router, gtx := newSuggestionTestUI(t)
			ui.address.SetText("")
			suggestionFrame(ui, router, gtx)
			ui.closeSuggestionPopup()
			queueComposition(router, 0, 0)
			suggestionFrame(ui, router, gtx)
			router.Queue(key.EditEvent{Range: key.Range{Start: 0, End: 0}, Text: query})
			suggestionFrame(ui, router, gtx)
			if ui.suggestionPopup.open {
				t.Fatal("searched before composition commit")
			}
			queueComposition(router, -1, -1)
			suggestionFrame(ui, router, gtx)
			if got := ui.address.Text(); got != query {
				t.Fatalf("editor text = %q, want %q", got, query)
			}
			if len(ui.suggestionPopup.candidates) == 0 || ui.suggestionPopup.candidates[0].Query != query {
				t.Fatalf("candidate did not preserve %q: %+v", query, ui.suggestionPopup.candidates)
			}
		})
	}
}

func TestHomeIMECompositionDefersSearchAndCandidateKeys(t *testing.T) {
	ui, router, gtx, state := newHomeInputTestUI(t)
	queueComposition(router, 0, 0)
	gtx.Reset()
	ui.Layout(*gtx)
	router.Frame(gtx.Ops)
	if !editorCompositionActive(state.editor) {
		t.Fatal("home composition was not observed")
	}
	router.Queue(key.EditEvent{Range: key.Range{Start: 0, End: 0}, Text: "絵文字🙂"})
	gtx.Reset()
	ui.Layout(*gtx)
	router.Frame(gtx.Ops)
	if len(state.candidates) != 0 {
		t.Fatal("home searched uncommitted composition text")
	}

	for _, name := range []key.Name{key.NameDownArrow, key.NameUpArrow, key.NameEscape, key.NameReturn} {
		router.Queue(key.Event{Name: name, State: key.Press})
		gtx.Reset()
		ui.Layout(*gtx)
		router.Frame(gtx.Ops)
		if !ui.homeVisible() || len(state.candidates) != 0 {
			t.Fatalf("%s acted on an active home composition", name)
		}
	}

	queueComposition(router, -1, -1)
	gtx.Reset()
	ui.Layout(*gtx)
	if len(state.candidates) == 0 || state.candidates[0].Query != "絵文字🙂" {
		t.Fatalf("committed home text was not searched: %+v", state.candidates)
	}
}

func TestSearchPanelIMECompositionDefersQueryAndActions(t *testing.T) {
	ui := NewBrowserUI(nil, nil)
	defer ui.Close()
	router := new(input.Router)
	gtx := &layout.Context{Ops: new(op.Ops), Source: router.Source(), Constraints: layout.Exact(image.Pt(1000, 800)), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}
	ui.openSearchPanel(*gtx)
	ui.Layout(*gtx)
	router.Frame(gtx.Ops)

	queueComposition(router, 0, 0)
	gtx.Reset()
	ui.Layout(*gtx)
	router.Frame(gtx.Ops)
	router.Queue(key.EditEvent{Range: key.Range{Start: 0, End: 0}, Text: "履歴"})
	gtx.Reset()
	ui.Layout(*gtx)
	router.Frame(gtx.Ops)
	if ui.searchPanel.query != "" {
		t.Fatalf("search panel queried uncommitted text %q", ui.searchPanel.query)
	}

	for _, name := range []key.Name{key.NameDownArrow, key.NameUpArrow, key.NameEscape, key.NameReturn} {
		router.Queue(key.Event{Name: name, State: key.Press})
		gtx.Reset()
		ui.Layout(*gtx)
		router.Frame(gtx.Ops)
		if !ui.searchPanel.open || ui.searchPanel.query != "" {
			t.Fatalf("%s acted on an active search panel composition", name)
		}
	}

	queueComposition(router, -1, -1)
	gtx.Reset()
	ui.Layout(*gtx)
	if ui.searchPanel.query != "履歴" {
		t.Fatalf("committed search panel query = %q", ui.searchPanel.query)
	}
}

func TestFindIMECompositionDefersSearchEnterAndEscape(t *testing.T) {
	document := dom.NewDocument()
	body := document.CreateElement("body", nil)
	text := document.CreateText("検索 target")
	if err := document.AppendChild(document.Root, body); err != nil {
		t.Fatal(err)
	}
	if err := document.AppendChild(body, text); err != nil {
		t.Fatal(err)
	}
	ui := NewBrowserUI(&stubNavigator{page: &browser.Page{Document: document, ComputedStyles: style.Compute(document, nil), StyleRevision: 1}}, nil)
	defer ui.Close()
	state := ui.ensureFindState()
	state.open = true
	router := new(input.Router)
	gtx := &layout.Context{Ops: new(op.Ops), Source: router.Source(), Constraints: layout.Exact(image.Pt(1000, 800)), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}
	ui.Layout(*gtx)
	router.Frame(gtx.Ops)
	gtx.Execute(key.FocusCmd{Tag: state.editor})
	gtx.Reset()
	ui.Layout(*gtx)
	router.Frame(gtx.Ops)

	queueComposition(router, 0, 0)
	gtx.Reset()
	ui.Layout(*gtx)
	router.Frame(gtx.Ops)
	if !editorCompositionActive(state.editor) {
		t.Fatal("find composition was not observed")
	}
	router.Queue(key.EditEvent{Range: key.Range{Start: 0, End: 0}, Text: "検索"})
	gtx.Reset()
	ui.Layout(*gtx)
	router.Frame(gtx.Ops)
	if state.pendingSearch || state.searching {
		t.Fatal("find searched uncommitted composition text")
	}
	for _, name := range []key.Name{key.NameEscape, key.NameReturn} {
		router.Queue(key.Event{Name: name, State: key.Press})
		gtx.Reset()
		ui.Layout(*gtx)
		router.Frame(gtx.Ops)
		if !state.open || state.pendingSearch || state.searching {
			t.Fatalf("%s acted on an active find composition", name)
		}
	}

	queueComposition(router, -1, -1)
	gtx.Reset()
	ui.Layout(*gtx)
	router.Frame(gtx.Ops)
	gtx.Reset()
	ui.Layout(*gtx)
	if !state.pendingSearch && !state.searching && len(state.result.Matches) == 0 {
		t.Fatalf("find did not search the committed composition text: pending=%t searching=%t matches=%d", state.pendingSearch, state.searching, len(state.result.Matches))
	}
}
