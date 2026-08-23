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
