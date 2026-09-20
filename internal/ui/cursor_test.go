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

	"github.com/Grove-Computing/Growse/internal/browser"
	"github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/style"
)

func TestBrowserUITracksMouseAndKeepsNativeCursorVisible(t *testing.T) {
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
	if got, want := router.Cursor(), pointer.CursorDefault; got != want {
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

func TestPlatformCursorTrackingDoesNotStealInputFocus(t *testing.T) {
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
		t.Fatal("platform cursor tracking prevented input focus")
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
