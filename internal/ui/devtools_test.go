package ui

import (
	"image"
	"testing"

	devtoolsmodel "github.com/Grove-Computing/Growse/internal/devtools"
)

func TestBrowserChromeGeometryWithDevToolsSplitsViewport(t *testing.T) {
	geometry := calculateBrowserChromeGeometryWithDevTools(image.Pt(1280, 800), 224, 92, 280)
	if got, want := geometry.viewport, image.Rect(224, 92, 1280, 520); got != want {
		t.Fatalf("viewport = %v, want %v", got, want)
	}
	if got, want := geometry.devTools, image.Rect(224, 520, 1280, 800); got != want {
		t.Fatalf("devtools = %v, want %v", got, want)
	}
	if geometry.viewport.Overlaps(geometry.devTools) || geometry.viewport.Max.Y != geometry.devTools.Min.Y {
		t.Fatalf("Issue #78 regression: viewport %v overlaps DevTools chrome %v", geometry.viewport, geometry.devTools)
	}
}

func TestDevToolsChromeRemainsDisjointForConstrainedWindows(t *testing.T) {
	for _, size := range []image.Point{{}, image.Pt(1, 1), image.Pt(224, 92), image.Pt(320, 180), image.Pt(1280, 800)} {
		geometry := calculateBrowserChromeGeometryWithDevTools(size, 224, 92, 280)
		window := image.Rectangle{Max: size}
		for name, region := range map[string]image.Rectangle{
			"rail": geometry.tabRail, "toolbar": geometry.toolbar, "viewport": geometry.viewport, "devtools": geometry.devTools,
		} {
			if region.Dx() < 0 || region.Dy() < 0 || !region.In(window) {
				t.Fatalf("%s region %v is invalid for window %v", name, region, window)
			}
		}
		if geometry.viewport.Overlaps(geometry.devTools) || geometry.toolbar.Overlaps(geometry.devTools) || geometry.tabRail.Overlaps(geometry.devTools) {
			t.Fatalf("Issue #78 regression for window %v: %+v", size, geometry)
		}
	}
}

func TestConsoleFilterCyclesAllLevels(t *testing.T) {
	levels := []devtoolsmodel.ConsoleLevel{
		devtoolsmodel.ConsoleLog,
		devtoolsmodel.ConsoleInfo,
		devtoolsmodel.ConsoleWarn,
		devtoolsmodel.ConsoleError,
		"",
	}
	current := devtoolsmodel.ConsoleLevel("")
	for _, want := range levels {
		current = nextConsoleFilter(current)
		if current != want {
			t.Fatalf("next filter = %q, want %q", current, want)
		}
	}
}
