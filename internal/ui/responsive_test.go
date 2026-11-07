package ui

import (
	"image"
	"strings"
	"testing"

	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"github.com/Grove-Computing/Growse/internal/omnibox"
)

func TestCandidateMetadataKeepsSourceOriginAndAction(t *testing.T) {
	for _, test := range []struct {
		candidate omnibox.Candidate
		want      []string
	}{
		{omnibox.Candidate{Source: omnibox.HistorySource, URL: "https://user:secret@example.com:8443/a/very/long/path", Primary: strings.Repeat("Title", 80)}, []string{"Source: History", "Origin: example.com", "Action: Open"}},
		{omnibox.Candidate{Source: omnibox.TabSource, TabID: 7, Primary: "Open tab"}, []string{"Source: Tab", "Origin: Open tabs", "Action: Switch"}},
		{omnibox.Candidate{Source: omnibox.RemoteSource, Query: "gopher"}, []string{"Source: Suggestion", "Origin: Web", "Action: Search"}},
	} {
		metadata := candidateMetadata(test.candidate)
		for _, want := range test.want {
			if !strings.Contains(metadata, want) {
				t.Errorf("metadata %q missing %q", metadata, want)
			}
		}
		if strings.Contains(metadata, "secret") {
			t.Errorf("metadata exposed URL credential: %q", metadata)
		}
	}
}

func TestFindOverlayBoundsStayInsideDesktopAndNarrowViewports(t *testing.T) {
	for _, viewport := range []image.Rectangle{
		image.Rect(224, 100, 1280, 800),
		image.Rect(120, 72, 320, 240),
		image.Rect(0, 0, 24, 32),
	} {
		bounds := findOverlayBounds(viewport, 560, 54, 12)
		if bounds.Empty() || !bounds.In(viewport) {
			t.Errorf("find bounds %v outside viewport %v", bounds, viewport)
		}
	}
}

func TestSearchPanelDesktopAndNarrowLayoutsKeepLongMetadata(t *testing.T) {
	for _, size := range []image.Point{image.Pt(1280, 720), image.Pt(320, 480), image.Pt(240, 360)} {
		t.Run(size.String(), func(t *testing.T) {
			ui := NewBrowserUI(nil, nil)
			defer ui.Close()
			ui.searchPanel.open = true
			ui.searchPanel.candidates = []omnibox.Candidate{{
				Source:  omnibox.BookmarkSource,
				Primary: strings.Repeat("Very long title ", 30),
				URL:     "https://example.com/" + strings.Repeat("long/", 80),
			}}
			router := new(input.Router)
			gtx := layout.Context{Ops: new(op.Ops), Source: router.Source(), Constraints: layout.Exact(size), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}
			dimensions := ui.layoutSearchPanel(gtx)
			if dimensions.Size.X > size.X || dimensions.Size.Y > size.Y || dimensions.Size.X < 0 || dimensions.Size.Y < 0 {
				t.Fatalf("panel dimensions %v outside %v", dimensions.Size, size)
			}
			router.Frame(gtx.Ops)
			nodes := router.AppendSemantics(nil)
			if !hasSemanticNode(nodes, func(node input.SemanticNode) bool {
				description := node.Desc.Description
				return strings.Contains(description, "Source: Bookmark") && strings.Contains(description, "Origin: example.com") && strings.Contains(description, "Action: Open")
			}) {
				t.Fatalf("long result lost source/origin/action at %v: %+v", size, nodes)
			}
		})
	}
}

func TestSuggestionPopupLongMetadataStaysInsideNarrowViewport(t *testing.T) {
	ui := NewBrowserUI(nil, nil)
	defer ui.Close()
	ui.suggestionPopup.open = true
	ui.suggestionPopup.candidates = []omnibox.Candidate{{
		Source:  omnibox.HistorySource,
		Primary: strings.Repeat("Long title ", 40),
		URL:     "https://history.example/" + strings.Repeat("path/", 60),
	}}
	viewport := image.Rect(80, 64, 280, 180)
	router := new(input.Router)
	gtx := layout.Context{Ops: new(op.Ops), Source: router.Source(), Constraints: layout.Exact(image.Pt(280, 180)), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}
	ui.layoutSuggestions(gtx, viewport)
	if !ui.suggestionPopup.bounds.In(viewport) {
		t.Fatalf("popup %v outside viewport %v", ui.suggestionPopup.bounds, viewport)
	}
	router.Frame(gtx.Ops)
	if !hasSemanticNode(router.AppendSemantics(nil), func(node input.SemanticNode) bool {
		return strings.Contains(node.Desc.Description, "Source: History") && strings.Contains(node.Desc.Description, "Origin: history.example") && strings.Contains(node.Desc.Description, "Action: Open")
	}) {
		t.Fatal("narrow popup lost source/origin/action metadata")
	}
}
