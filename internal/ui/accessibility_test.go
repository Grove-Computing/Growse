package ui

import (
	"image"
	"strings"
	"testing"

	"gioui.org/io/input"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"github.com/Grove-Computing/Growse/internal/findpage"
	"github.com/Grove-Computing/Growse/internal/omnibox"
)

func hasSemanticNode(nodes []input.SemanticNode, match func(input.SemanticNode) bool) bool {
	for _, node := range nodes {
		if match(node) {
			return true
		}
	}
	return false
}

func TestOmniboxPopupFindAndErrorExposeAccessibleState(t *testing.T) {
	ui, router, gtx := newSuggestionTestUI(t)
	ui.address.SetText("guide")
	ui.SetSuggestionSnapshot(omnibox.Snapshot{History: []omnibox.Candidate{{Primary: "Guide history", URL: "https://example.com/guide"}}})
	ui.refreshSuggestions()
	ui.selectSuggestion(1)
	find := ui.ensureFindState()
	find.open = true
	find.editor.SetText("guide")
	suggestionFrame(ui, router, gtx)
	ui.refreshSuggestions()
	ui.selectSuggestion(1)
	find.result = findpage.Result{Matches: []findpage.Match{{NodeID: 1}, {NodeID: 2}}}
	find.current = 0
	find.searching = false
	find.pendingSearch = false
	find.options.CaseSensitive = true
	ui.status = "Navigationを利用できません"
	ui.statusHasError = true
	suggestionFrame(ui, router, gtx)
	nodes := router.AppendSemantics(nil)

	checks := map[string]func(input.SemanticNode) bool{
		"omnibox name and role": func(node input.SemanticNode) bool {
			return node.Desc.Class == semantic.Editor && strings.Contains(node.Desc.Description, "Omnibox")
		},
		"popup count and selection": func(node input.SemanticNode) bool {
			return strings.Contains(node.Desc.Description, "検索候補") && strings.Contains(node.Desc.Description, "選択 2 / 2")
		},
		"selected popup option": func(node input.SemanticNode) bool {
			return node.Desc.Class == semantic.Button && node.Desc.Selected && strings.Contains(node.Desc.Description, "Guide history")
		},
		"find match count": func(node input.SemanticNode) bool {
			return strings.Contains(node.Desc.Description, "ページ内検索: 1 / 2")
		},
		"find toggle state": func(node input.SemanticNode) bool {
			return node.Desc.Class == semantic.Button && node.Desc.Selected && strings.Contains(node.Desc.Description, "大文字と小文字")
		},
		"error": func(node input.SemanticNode) bool {
			return strings.Contains(node.Desc.Description, "エラー: Navigationを利用できません")
		},
		"bookmark action": func(node input.SemanticNode) bool {
			return node.Desc.Class == semantic.Button && strings.Contains(node.Desc.Description, "Bookmarkへ追加")
		},
	}
	for name, match := range checks {
		if !hasSemanticNode(nodes, match) {
			t.Errorf("missing %s in semantic tree: %+v", name, nodes)
		}
	}
}

func TestSearchPanelExposesEditorResultCountAndSelection(t *testing.T) {
	ui := NewBrowserUI(nil, nil)
	defer ui.Close()
	ui.searchPanel.open = true
	ui.searchPanel.candidates = []omnibox.Candidate{
		{Source: omnibox.TabSource, Primary: "Guide tab", TabID: 1},
		{Source: omnibox.HistorySource, Primary: "Guide history", URL: "https://example.com/guide"},
	}
	ui.searchPanel.selected = 1
	router := new(input.Router)
	gtx := layout.Context{Ops: new(op.Ops), Source: router.Source(), Constraints: layout.Exact(image.Pt(900, 700)), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}
	ui.layoutSearchPanel(gtx)
	router.Frame(gtx.Ops)
	nodes := router.AppendSemantics(nil)
	if !hasSemanticNode(nodes, func(node input.SemanticNode) bool {
		return node.Desc.Class == semantic.Editor && strings.Contains(node.Desc.Description, "Tabs、History、Bookmarks")
	}) {
		t.Fatalf("search editor accessible name/role missing: %+v", nodes)
	}
	if !hasSemanticNode(nodes, func(node input.SemanticNode) bool {
		return strings.Contains(node.Desc.Description, "検索結果 2件、選択 2 / 2")
	}) {
		t.Fatalf("search result count/selection missing: %+v", nodes)
	}
	if !hasSemanticNode(nodes, func(node input.SemanticNode) bool {
		return node.Desc.Class == semantic.Button && node.Desc.Selected && strings.Contains(node.Desc.Description, "Guide history")
	}) {
		t.Fatalf("selected search result role/state missing: %+v", nodes)
	}
}

func TestSelectionSummaryDistinguishesEmptyUnselectedAndSelected(t *testing.T) {
	for _, test := range []struct {
		total, selected int
		want            string
	}{{0, -1, "候補 0件"}, {3, -1, "候補 3件、未選択"}, {3, 1, "候補 3件、選択 2 / 3"}} {
		if got := selectionSummary("候補", test.total, test.selected); got != test.want {
			t.Errorf("selectionSummary(%d, %d) = %q, want %q", test.total, test.selected, got, test.want)
		}
	}
}
