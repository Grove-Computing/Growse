package ui

import (
	"encoding/json"
	"fmt"
	"image"
	"os"
	"reflect"
	"strings"
	"testing"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/Grove-Computing/Growse/internal/omnibox"
)

type searchShowcaseVisualSnapshot struct {
	Desktop []string `json:"desktop"`
	Narrow  []string `json:"narrow"`
}

func TestSearchShowcaseDesktopAndNarrowVisualEvidence(t *testing.T) {
	describe := func(window image.Point) []string {
		ui := NewBrowserUI(nil, nil)
		defer ui.Close()
		metric := unit.Metric{PxPerDp: 1, PxPerSp: 1}
		geometry := calculateBrowserChromeGeometry(window, int(tabRailWidth), int(toolbarHeight))
		candidates := []omnibox.Candidate{
			{Source: omnibox.InputSource, Primary: "guide", Query: "guide"},
			{Source: omnibox.TabSource, Primary: "Guide tab", URL: "https://showcase.test/tab-guide", TabID: 2},
			{Source: omnibox.HistorySource, Primary: "Guide history", URL: "https://showcase.test/history-guide"},
			{Source: omnibox.BookmarkSource, Primary: "Guide bookmark", URL: "https://showcase.test/bookmark-guide"},
			{Source: omnibox.RemoteSource, Primary: "guide remote", Query: "guide remote"},
		}
		ui.suggestionPopup.open = true
		ui.suggestionPopup.selected = 2
		ui.suggestionPopup.candidates = candidates
		popupContext := layout.Context{Ops: new(op.Ops), Constraints: layout.Exact(window), Metric: metric}
		ui.layoutSuggestions(popupContext, geometry.viewport)

		ui.searchPanel.open = true
		ui.searchPanel.selected = 1
		ui.searchPanel.candidates = candidates[1:4]
		panelContext := layout.Context{Ops: new(op.Ops), Constraints: layout.Exact(geometry.viewport.Size()), Metric: metric}
		panelDimensions := ui.layoutSearchPanel(panelContext)

		homeContext := layout.Context{Ops: new(op.Ops), Constraints: layout.Exact(geometry.viewport.Size()), Metric: metric}
		homeDimensions := ui.layoutHome(homeContext)
		homeMetrics := searchHomeLayoutMetrics(geometry.viewport.Dx() < 600)
		sources := make([]string, 0, len(candidates))
		for _, candidate := range candidates {
			sources = append(sources, candidate.Source.Label())
		}
		return []string{
			fmt.Sprintf("window=%dx%d viewport=%s", window.X, window.Y, geometry.viewport),
			fmt.Sprintf("popup=%s rows=%d selected=%d sources=%s", ui.suggestionPopup.bounds, len(candidates), ui.suggestionPopup.selected, strings.Join(sources, "/")),
			fmt.Sprintf("search-panel=%dx%d rows=%d selected=%d", panelDimensions.Size.X, panelDimensions.Size.Y, len(ui.searchPanel.candidates), ui.searchPanel.selected),
			fmt.Sprintf("home=%dx%d card=%d columns=%d candidate-height=%d", homeDimensions.Size.X, homeDimensions.Size.Y, int(homeMetrics.cardWidth), homeMetrics.shortcutColumns, int(homeMetrics.candidateHeight)),
		}
	}
	got := searchShowcaseVisualSnapshot{
		Desktop: describe(image.Pt(1280, 800)),
		Narrow:  describe(image.Pt(720, 640)),
	}
	body, err := os.ReadFile("testdata/search-showcase.golden.json")
	if err != nil {
		t.Fatal(err)
	}
	var want searchShowcaseVisualSnapshot
	if err := json.Unmarshal(body, &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		actual, _ := json.MarshalIndent(got, "", "  ")
		t.Fatalf("Search Showcase visual evidence changed; inspect before updating golden\n--- actual ---\n%s", actual)
	}
}
