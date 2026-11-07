package ui

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"gioui.org/gpu/headless"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/Grove-Computing/Growse/internal/homeconfig"
	"github.com/Grove-Computing/Growse/internal/omnibox"
)

func TestV021ReleaseCandidateRasterEvidence(t *testing.T) {
	artifactDir := os.Getenv("GROWSE_V021_RC_ARTIFACT_DIR")
	tests := []struct {
		name string
		size image.Point
		view func(*BrowserUI, layout.Context)
	}{
		{
			name: "desktop-home",
			size: image.Pt(1056, 700),
			view: func(ui *BrowserUI, gtx layout.Context) {
				ui.layoutHome(gtx)
			},
		},
		{
			name: "narrow-home",
			size: image.Pt(496, 700),
			view: func(ui *BrowserUI, gtx layout.Context) {
				ui.layoutHome(gtx)
			},
		},
		{
			name: "desktop-search-panel",
			size: image.Pt(1056, 700),
			view: func(ui *BrowserUI, gtx layout.Context) {
				ui.layoutSearchPanel(gtx)
			},
		},
		{
			name: "narrow-search-panel",
			size: image.Pt(496, 700),
			view: func(ui *BrowserUI, gtx layout.Context) {
				ui.layoutSearchPanel(gtx)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ui := releaseCandidateVisualUI(t)
			defer ui.Close()
			result := rasterReleaseCandidateView(t, test.size, artifactDir != "", func(gtx layout.Context) {
				test.view(ui, gtx)
			})
			assertRasterHasVisibleContent(t, result)
			if artifactDir != "" {
				writeReleaseCandidatePNG(t, artifactDir, test.name+".png", result)
			}
		})
	}
}

func releaseCandidateVisualUI(t *testing.T) *BrowserUI {
	t.Helper()
	ui := NewBrowserUI(nil, nil)
	ui.homeSettings.Background = homeconfig.BackgroundDusk
	for index := 0; index < homeconfig.MaxShortcuts; index++ {
		if _, err := ui.homeSettings.Add(
			fmt.Sprintf("Site %d", index+1),
			fmt.Sprintf("https://site-%d.example/guide", index+1),
		); err != nil {
			t.Fatal(err)
		}
	}
	home := ui.homeState(0)
	home.editor.SetText("guide")
	ui.refreshHomeSuggestions(home)

	ui.searchPanel.open = true
	ui.searchPanel.query = "guide"
	ui.searchPanel.editor.SetText("guide")
	ui.searchPanel.selected = 1
	ui.searchPanel.candidates = []omnibox.Candidate{
		{Source: omnibox.TabSource, Primary: "Guide tab", URL: "https://showcase.test/tab-guide", TabID: 2},
		{Source: omnibox.HistorySource, Primary: "Guide history", URL: "https://showcase.test/history-guide"},
		{Source: omnibox.BookmarkSource, Primary: "Guide bookmark", URL: "https://showcase.test/bookmark-guide"},
	}
	return ui
}

func rasterReleaseCandidateView(t *testing.T, size image.Point, required bool, draw func(layout.Context)) *image.RGBA {
	t.Helper()
	window, err := headless.NewWindow(size.X, size.Y)
	if err != nil {
		if required {
			t.Fatalf("RC visual evidence requires the headless GPU backend: %v", err)
		}
		t.Skipf("headless raster backend unavailable: %v", err)
	}
	defer window.Release()
	ops := new(op.Ops)
	gtx := layout.Context{
		Ops: ops, Constraints: layout.Exact(size),
		Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
	}
	draw(gtx)
	if err := window.Frame(ops); err != nil {
		t.Fatal(err)
	}
	result := image.NewRGBA(image.Rectangle{Max: size})
	if err := window.Screenshot(result); err != nil {
		t.Fatal(err)
	}
	return result
}

func assertRasterHasVisibleContent(t *testing.T, result *image.RGBA) {
	t.Helper()
	first := result.RGBAAt(0, 0)
	different := 0
	for y := 0; y < result.Bounds().Dy(); y += 8 {
		for x := 0; x < result.Bounds().Dx(); x += 8 {
			if result.RGBAAt(x, y) != first {
				different++
			}
		}
	}
	if different < 100 {
		t.Fatalf("raster appears blank or incomplete: only %d sampled pixels differ", different)
	}
}

func writeReleaseCandidatePNG(t *testing.T, artifactDir, name string, result *image.RGBA) {
	t.Helper()
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(artifactDir, name)
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(file, result); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	t.Logf("RC visual evidence: %s", path)
}
