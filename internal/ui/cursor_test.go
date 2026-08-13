package ui

import (
	"image"
	"testing"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/saku0512/growse/internal/browser"
	"github.com/saku0512/growse/internal/dom"
	"github.com/saku0512/growse/internal/style"
)

func TestEmbeddedGopherCursorSVGCanBeRasterized(t *testing.T) {
	imageValue, err := rasterizeGopherCursor(gopherCursorSVG)
	if err != nil {
		t.Fatalf("rasterizeGopherCursor() error = %v", err)
	}
	if got, want := imageValue.Bounds().Size(), image.Pt(336, 457); got != want {
		t.Fatalf("cursor image size = %v, want %v", got, want)
	}

	visible := false
	for y := imageValue.Bounds().Min.Y; y < imageValue.Bounds().Max.Y && !visible; y++ {
		for x := imageValue.Bounds().Min.X; x < imageValue.Bounds().Max.X; x++ {
			_, _, _, alpha := imageValue.At(x, y).RGBA()
			if alpha != 0 {
				visible = true
				break
			}
		}
	}
	if !visible {
		t.Fatal("rasterized cursor image is fully transparent")
	}
}

func TestRasterizeGopherCursorRejectsInvalidSVG(t *testing.T) {
	for name, source := range map[string][]byte{
		"empty":   nil,
		"invalid": []byte("<svg>"),
		"viewBox": []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 0 0"></svg>`),
	} {
		t.Run(name, func(t *testing.T) {
			if imageValue, err := rasterizeGopherCursor(source); err == nil || imageValue != nil {
				t.Fatalf("rasterizeGopherCursor() = (%v, %v), want nil error", imageValue, err)
			}
		})
	}
}

func TestCursorGeometryPreservesAspectRatioAndDPI(t *testing.T) {
	source := image.Pt(336, 457)
	position := f32.Pt(100, 80)

	size, origin := cursorGeometry(source, unit.Metric{PxPerDp: 1}, position)
	if got, want := size, image.Pt(24, 32); got != want {
		t.Fatalf("1x cursor size = %v, want %v", got, want)
	}
	if got, want := origin, image.Pt(98, 78); got != want {
		t.Fatalf("1x cursor origin = %v, want %v", got, want)
	}

	size, origin = cursorGeometry(source, unit.Metric{PxPerDp: 2}, position)
	if got, want := size, image.Pt(47, 64); got != want {
		t.Fatalf("2x cursor size = %v, want %v", got, want)
	}
	if got, want := origin, image.Pt(96, 76); got != want {
		t.Fatalf("2x cursor origin = %v, want %v", got, want)
	}
}

func TestBrowserUITracksMouseAndHidesNativeCursor(t *testing.T) {
	invalidations := 0
	ui := NewBrowserUI(nil, func() { invalidations++ })
	router := new(input.Router)
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Source:      router.Source(),
		Constraints: layout.Exact(image.Pt(800, 600)),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
	}

	ui.Layout(gtx)
	router.Frame(gtx.Ops)
	router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: f32.Pt(120, 80)})
	gtx.Reset()
	ui.Layout(gtx)
	router.Frame(gtx.Ops)

	if !ui.pointer.inside || ui.pointer.position != f32.Pt(120, 80) {
		t.Fatalf("pointer state = %#v, want inside at (120,80)", ui.pointer)
	}
	if got, want := router.Cursor(), pointer.CursorNone; got != want {
		t.Fatalf("native cursor = %v, want %v", got, want)
	}
	if invalidations == 0 {
		t.Fatal("pointer movement did not request redraw")
	}

	router.Queue(pointer.Event{Kind: pointer.Leave, Source: pointer.Mouse, Position: f32.Pt(120, 80)})
	gtx.Reset()
	ui.Layout(gtx)
	if ui.pointer.inside {
		t.Fatal("pointer remains inside after Leave")
	}
}

func TestPointerTrackerIgnoresTouchInput(t *testing.T) {
	ui := NewBrowserUI(nil, nil)
	router := new(input.Router)
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Source:      router.Source(),
		Constraints: layout.Exact(image.Pt(800, 600)),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
	}

	ui.Layout(gtx)
	router.Frame(gtx.Ops)
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Touch, Position: f32.Pt(10, 10)})
	gtx.Reset()
	ui.Layout(gtx)
	if ui.pointer.inside {
		t.Fatal("touch input enabled the mouse cursor overlay")
	}
}

func TestGopherCursorOverlayDoesNotStealInputFocus(t *testing.T) {
	document := dom.NewDocument()
	inputNode := document.CreateElement("input", map[string]string{"type": "text"})
	if err := document.AppendChild(document.Root, inputNode); err != nil {
		t.Fatal(err)
	}
	page := &browser.Page{Document: document, ComputedStyles: style.Compute(document, nil)}
	ui := NewBrowserUI(&stubNavigator{page: page}, nil)
	router := new(input.Router)
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Source:      router.Source(),
		Constraints: layout.Exact(image.Pt(800, 600)),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
	}

	ui.Layout(gtx)
	router.Frame(gtx.Ops)
	router.Queue(pointer.Event{
		Buttons:  pointer.ButtonPrimary,
		Kind:     pointer.Press,
		Source:   pointer.Mouse,
		Position: f32.Pt(40, float32(toolbarHeight)+40),
	})
	gtx.Reset()
	ui.Layout(gtx)

	editor := ui.inputEditors[inputNode.ID]
	if editor == nil || !gtx.Focused(editor) {
		t.Fatal("Gopher cursor overlay prevented input focus")
	}
}
