package main

import (
	"encoding/json"
	"image"
	"image/color"
	"os"
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/conformance"
	"github.com/Grove-Computing/Growse/internal/css"
	htmlparser "github.com/Grove-Computing/Growse/internal/html"
	"github.com/Grove-Computing/Growse/internal/layout"
	"github.com/Grove-Computing/Growse/internal/style"
)

type differentialReferenceFile struct {
	SchemaVersion int                              `json:"schemaVersion"`
	Fixture       string                           `json:"fixture"`
	CapturedAt    string                           `json:"capturedAt"`
	Browsers      map[string]differentialReference `json:"browsers"`
}

type differentialReference struct {
	Version  string               `json:"version"`
	Snapshot conformance.Snapshot `json:"snapshot"`
}

var differentialLandmarks = []string{
	"page", "header", "sidebar", "workspace", "card-a", "card-b", "card-c", "card-d",
	"footer", "scroller", "scroll-content", "flex-probe", "flex-a", "flex-b",
}

func TestCSSLayoutDifferentialMatchesChromiumAndFirefoxGeometryScrollAndVisualThreshold(t *testing.T) {
	encoded, err := os.ReadFile("testdata/differential-v019.json")
	if err != nil {
		t.Fatalf("%v\nactual=%s", err, mustJSON(growseDifferentialSnapshot(t)))
	}
	var references differentialReferenceFile
	if err := json.Unmarshal(encoded, &references); err != nil {
		t.Fatal(err)
	}
	if references.SchemaVersion != 1 || references.Fixture != "differential.html" || references.CapturedAt == "" || len(references.Browsers) != 2 {
		t.Fatalf("differential reference header = %#v", references)
	}
	actual := growseDifferentialSnapshot(t)
	for _, browserName := range []string{"chromium", "firefox"} {
		reference, exists := references.Browsers[browserName]
		if !exists || reference.Version == "" {
			t.Fatalf("missing pinned %s reference", browserName)
		}
		if report := conformance.Compare(reference.Snapshot, actual); !report.Passed() {
			t.Fatalf("%s differential failed: %+v\nactual=%s", browserName, report.Differences, mustJSON(actual))
		}
		visual := conformance.CompareVisual(
			differentialRaster(reference.Snapshot), differentialRaster(actual),
			[]conformance.VisualRegion{
				{Name: "page", Bounds: image.Rect(0, 0, 720, 620)},
				{Name: "workspace", Bounds: image.Rect(200, 120, 700, 400)},
				{Name: "footer", Bounds: image.Rect(20, 400, 700, 600)},
			}, nil,
		)
		if !visual.Passed() {
			t.Fatalf("%s visual threshold failed: %+v", browserName, visual)
		}
	}
}

func growseDifferentialSnapshot(t *testing.T) conformance.Snapshot {
	t.Helper()
	source, err := cssLayoutAssets.ReadFile("differential.html")
	if err != nil {
		t.Fatal(err)
	}
	document, err := htmlparser.Parse(strings.NewReader(string(source)))
	if err != nil {
		t.Fatal(err)
	}
	cssStart := strings.Index(string(source), "<style>")
	cssEnd := strings.Index(string(source), "</style>")
	if cssStart < 0 || cssEnd <= cssStart {
		t.Fatal("differential stylesheet is missing")
	}
	stylesheet, err := css.Parse(strings.NewReader(string(source)[cssStart+len("<style>") : cssEnd]))
	if err != nil {
		t.Fatal(err)
	}
	computed := style.ComputeWithEnvironment(document, stylesheet, style.InteractionState{}, style.Environment{
		ViewportWidth: 720, ViewportHeight: 520, RootFontSize: 16, ResolutionDPI: 96, BrowserDefaults: true,
	})
	tree := layout.BuildWithViewport(document, computed, 720, 520)
	snapshot := conformance.Snapshot{
		Scenario: "css-layout-2026-desktop", Viewport: conformance.Extent{Width: 720, Height: 520},
		DOMLandmarks: append([]string(nil), differentialLandmarks...),
		Computed:     make(map[string]map[string]string, len(differentialLandmarks)),
		Geometry:     make(map[string]conformance.Rect, len(differentialLandmarks)),
		ScrollExtent: conformance.Extent{Width: float64(tree.ScrollWidth), Height: float64(tree.ScrollHeight)},
		Focus:        "", Resources: map[string]string{"fixture": "complete"},
	}
	for _, id := range differentialLandmarks {
		node, exists := document.GetElementByID(id)
		if !exists {
			t.Fatalf("missing differential landmark %q", id)
		}
		bounds, exists := tree.Bounds[node.ID]
		if !exists {
			t.Fatalf("missing differential geometry %q", id)
		}
		computedStyle, exists := computed.For(node)
		if !exists {
			t.Fatalf("missing differential style %q", id)
		}
		snapshot.Geometry[id] = conformance.Rect{X: float64(bounds.X), Y: float64(bounds.Y), Width: float64(bounds.Width), Height: float64(bounds.Height)}
		snapshot.Computed[id] = map[string]string{
			"display": displayName(computedStyle.Display), "position": positionName(computedStyle.Position),
			"overflowX": overflowName(computedStyle.OverflowX), "overflowY": overflowName(computedStyle.OverflowY),
		}
	}
	return snapshot
}

func differentialRaster(snapshot conformance.Snapshot) image.Image {
	canvas := image.NewNRGBA(image.Rect(0, 0, 720, 620))
	fillRect(canvas, canvas.Bounds(), color.NRGBA{R: 255, G: 255, B: 255, A: 255})
	palette := map[string]color.NRGBA{
		"page": {R: 248, G: 250, B: 252, A: 255}, "header": {R: 15, G: 118, B: 110, A: 255},
		"sidebar": {R: 29, G: 78, B: 216, A: 255}, "workspace": {R: 226, G: 232, B: 240, A: 255},
		"card-a": {R: 245, G: 158, B: 11, A: 255}, "card-b": {R: 139, G: 92, B: 246, A: 255},
		"card-c": {R: 236, G: 72, B: 153, A: 255}, "card-d": {R: 34, G: 197, B: 94, A: 255},
		"footer": {R: 203, G: 213, B: 225, A: 255}, "scroller": {R: 255, G: 255, B: 255, A: 255},
		"scroll-content": {R: 239, G: 68, B: 68, A: 255}, "flex-probe": {R: 51, G: 65, B: 85, A: 255},
		"flex-a": {R: 56, G: 189, B: 248, A: 255}, "flex-b": {R: 163, G: 230, B: 53, A: 255},
	}
	for _, id := range differentialLandmarks {
		rect := snapshot.Geometry[id]
		fillRect(canvas, image.Rect(int(rect.X+.5), int(rect.Y+.5), int(rect.X+rect.Width+.5), int(rect.Y+rect.Height+.5)), palette[id])
	}
	return canvas
}

func fillRect(target *image.NRGBA, bounds image.Rectangle, value color.Color) {
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if image.Pt(x, y).In(target.Bounds()) {
				target.Set(x, y, value)
			}
		}
	}
}

func displayName(value style.Display) string {
	names := map[style.Display]string{style.DisplayBlock: "block", style.DisplayFlex: "flex", style.DisplayGrid: "grid"}
	return names[value]
}

func positionName(value style.Position) string {
	names := map[style.Position]string{style.PositionStatic: "static", style.PositionRelative: "relative", style.PositionAbsolute: "absolute", style.PositionFixed: "fixed", style.PositionSticky: "sticky"}
	return names[value]
}

func overflowName(value style.Overflow) string {
	names := map[style.Overflow]string{style.OverflowVisible: "visible", style.OverflowHidden: "hidden", style.OverflowClip: "clip", style.OverflowAuto: "auto", style.OverflowScroll: "scroll"}
	return names[value]
}

func mustJSON(value any) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}
