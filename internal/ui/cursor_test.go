package ui

import (
	"image"
	"testing"

	"gioui.org/f32"
	"gioui.org/gpu/headless"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/Grove-Computing/Growse/internal/browser"
	"github.com/Grove-Computing/Growse/internal/dom"
	layoutengine "github.com/Grove-Computing/Growse/internal/layout"
	paintmodel "github.com/Grove-Computing/Growse/internal/paint"
	"github.com/Grove-Computing/Growse/internal/style"
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

func TestBrowserUITracksMouseAndShowsGopherCursor(t *testing.T) {
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
	if !ui.gopherCursorVisible() {
		t.Fatal("Gopher cursor is not visible for the default cursor")
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

func TestGopherCursorRendersAtPointerPosition(t *testing.T) {
	const width, height = 96, 96
	window, err := headless.NewWindow(width, height)
	if err != nil {
		t.Skipf("headless raster backend unavailable: %v", err)
	}
	defer window.Release()

	ui := NewBrowserUI(nil, nil)
	ui.pointer = pointerState{position: f32.Pt(40, 40), inside: true}
	ops := new(op.Ops)
	gtx := layout.Context{
		Ops:         ops,
		Constraints: layout.Exact(image.Pt(width, height)),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
	}
	ui.layoutGopherCursor(gtx)
	if err := window.Frame(ops); err != nil {
		t.Fatal(err)
	}
	raster := image.NewRGBA(image.Rect(0, 0, width, height))
	if err := window.Screenshot(raster); err != nil {
		t.Fatal(err)
	}

	visible := 0
	for y := 35; y < 76; y++ {
		for x := 30; x < 70; x++ {
			pixel := raster.RGBAAt(x, y)
			if pixel.A != 0 && (pixel.R != 0 || pixel.G != 0 || pixel.B != 0) {
				visible++
			}
		}
	}
	if visible == 0 {
		t.Fatal("Gopher cursor produced no visible pixels near the pointer")
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

func TestGopherCursorTrackingDoesNotStealInputFocus(t *testing.T) {
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
		Position: f32.Pt(float32(tabRailWidth)+40, float32(toolbarHeight)+40),
	})
	gtx.Reset()
	ui.Layout(gtx)

	editor := ui.inputEditors[inputNode.ID]
	if editor == nil || !gtx.Focused(editor) {
		t.Fatal("Gopher cursor tracking prevented input focus")
	}
}

func TestExplicitPageCursorUsesPlatformCursor(t *testing.T) {
	page := &browser.Page{StyleRevision: 1}
	ui := NewBrowserUI(&stubNavigator{page: page}, nil)
	ui.pointer = pointerState{
		position: f32.Pt(float32(tabRailWidth)+20, float32(toolbarHeight)+20),
		inside:   true,
	}
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Constraints: layout.Exact(image.Pt(576, 500)),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
	}
	displayList := &paintmodel.DisplayList{
		Revision: 1,
		Commands: []paintmodel.Command{
			paintmodel.DrawBox{NodeID: 1, X: 0, Y: 0, Width: 100, Height: 100, Cursor: style.CursorPointer},
		},
	}

	ui.updateViewportHover(gtx, page, &layoutengine.Tree{Revision: 1}, displayList)

	if !ui.pointer.platformCursor {
		t.Fatal("explicit CSS cursor did not select the platform cursor")
	}
	if ui.gopherCursorVisible() {
		t.Fatal("Gopher cursor remained visible over an explicit CSS cursor")
	}
}

func TestAutoCursorOverInputUsesPlatformTextCursor(t *testing.T) {
	displayList := &paintmodel.DisplayList{Commands: []paintmodel.Command{
		paintmodel.DrawInput{NodeID: 1, X: 0, Y: 0, Width: 100, Height: 40, Cursor: style.CursorAuto},
	}}

	hit, ok := hitTestPaintedDisplayList(displayList, layout.Position{}, image.Pt(20, 20), 1)
	if !ok {
		t.Fatal("input was not hit")
	}
	if got, want := hit.Cursor, style.CursorText; got != want {
		t.Fatalf("input cursor = %v, want %v", got, want)
	}
}

func TestCSSCursorMapsToPlatformCursor(t *testing.T) {
	for name, test := range map[string]struct {
		value style.Cursor
		want  pointer.Cursor
	}{
		"default": {value: style.CursorAuto, want: pointer.CursorDefault},
		"link":    {value: style.CursorPointer, want: pointer.CursorPointer},
		"text":    {value: style.CursorText, want: pointer.CursorText},
		"wait":    {value: style.CursorWait, want: pointer.CursorWait},
	} {
		t.Run(name, func(t *testing.T) {
			if got := cssCursor(test.value); got != test.want {
				t.Fatalf("cssCursor(%v) = %v, want %v", test.value, got, test.want)
			}
		})
	}
}
