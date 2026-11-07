package ui

import (
	"encoding/json"
	"fmt"
	"image"
	"os"
	"reflect"
	"testing"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"github.com/Grove-Computing/Growse/internal/homeconfig"
)

type searchHomeVisualSnapshot struct {
	Desktop     []string `json:"desktop"`
	Narrow      []string `json:"narrow"`
	Backgrounds []string `json:"backgrounds"`
}

func TestSearchHomeVisualRegression(t *testing.T) {
	describe := func(viewport image.Point, narrow bool) []string {
		metrics := searchHomeLayoutMetrics(narrow)
		ui := NewBrowserUI(nil, nil)
		for index := 0; index < homeconfig.MaxShortcuts; index++ {
			_, _ = ui.homeSettings.Add(fmt.Sprintf("Shortcut %d", index+1), fmt.Sprintf("https://%d.example/", index+1))
		}
		state := ui.homeState(0)
		state.editor.SetText("guide")
		ui.refreshHomeSuggestions(state)
		gtx := layout.Context{Ops: new(op.Ops), Constraints: layout.Exact(viewport), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}
		dimensions := ui.layoutHome(gtx)
		return []string{
			fmt.Sprintf("viewport=%dx%d output=%dx%d", viewport.X, viewport.Y, dimensions.Size.X, dimensions.Size.Y),
			fmt.Sprintf("card=%d inset=%d,%d logo=%dx%d", int(metrics.cardWidth), int(metrics.horizontalInset), int(metrics.verticalInset), int(metrics.logoWidth), int(metrics.logoHeight)),
			fmt.Sprintf("search-candidates=%d shortcut-columns=%d shortcut-rows=%d scroll=vertical", int(metrics.candidateHeight), metrics.shortcutColumns, (homeconfig.MaxShortcuts+metrics.shortcutColumns-1)/metrics.shortcutColumns),
		}
	}
	backgrounds := make([]string, 0, len(homeconfig.BackgroundPresets()))
	for _, preset := range homeconfig.BackgroundPresets() {
		background, surface := homeBackgroundColors(preset.ID)
		backgrounds = append(backgrounds, fmt.Sprintf("%s=%02x%02x%02x/%02x%02x%02x", preset.ID, background.R, background.G, background.B, surface.R, surface.G, surface.B))
	}
	got := searchHomeVisualSnapshot{
		Desktop:     describe(image.Pt(1056, 700), false),
		Narrow:      describe(image.Pt(496, 700), true),
		Backgrounds: backgrounds,
	}
	body, err := os.ReadFile("testdata/search-home.golden.json")
	if err != nil {
		t.Fatal(err)
	}
	var want searchHomeVisualSnapshot
	if err := json.Unmarshal(body, &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		actual, _ := json.MarshalIndent(got, "", "  ")
		t.Fatalf("search home visual snapshot changed; inspect before updating golden\n--- actual ---\n%s", actual)
	}
}
