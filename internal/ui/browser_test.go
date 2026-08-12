package ui

import (
	"image"
	"testing"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
)

func TestToolbarHasFixedHeight(t *testing.T) {
	ui := NewBrowserUI()
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Constraints: layout.Exact(image.Pt(1280, 800)),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
	}

	dims := ui.layoutToolbar(gtx)
	if got, want := dims.Size.Y, 72; got != want {
		t.Fatalf("toolbar height = %d, want %d", got, want)
	}
}

func TestBrowserUILayoutFillsViewport(t *testing.T) {
	ui := NewBrowserUI()
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Constraints: layout.Exact(image.Pt(1280, 800)),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
	}

	dims := ui.Layout(gtx)
	if got, want := dims.Size, image.Pt(1280, 800); got != want {
		t.Fatalf("browser UI size = %v, want %v", got, want)
	}
}

func TestToolbarButtonHasVisibleControlSize(t *testing.T) {
	ui := NewBrowserUI()
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Constraints: layout.Constraints{Max: image.Pt(1280, 52)},
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
	}

	dims := ui.layoutToolbarButton(gtx, &ui.backButton, ui.backIcon, "戻る")
	if got, want := dims.Size, image.Pt(44, 44); got != want {
		t.Fatalf("toolbar button size = %v, want %v", got, want)
	}
}
