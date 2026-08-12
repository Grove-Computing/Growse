package ui

import (
	"context"
	"image"
	"net/url"
	"strings"
	"testing"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"github.com/saku0512/growse/internal/browser"
)

type stubNavigator struct {
	page *browser.Page
	err  error
}

func (navigator *stubNavigator) Navigate(context.Context, string) (*browser.Page, error) {
	return navigator.page, navigator.err
}

func (navigator *stubNavigator) Page() *browser.Page {
	return navigator.page
}

func TestToolbarHasFixedHeight(t *testing.T) {
	ui := NewBrowserUI(nil, nil)
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
	ui := NewBrowserUI(nil, nil)
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
	ui := NewBrowserUI(nil, nil)
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

func TestNavigationResultUpdatesAddressAndStatus(t *testing.T) {
	pageURL, err := url.Parse("https://example.com/final")
	if err != nil {
		t.Fatal(err)
	}
	navigator := &stubNavigator{page: &browser.Page{
		URL:         pageURL,
		StatusCode:  200,
		ContentType: "text/html; charset=utf-8",
		Source:      []byte("<h1>Hello</h1>"),
	}}
	invalidated := make(chan struct{}, 1)
	ui := NewBrowserUI(navigator, func() { invalidated <- struct{}{} })

	ui.startNavigation("example.com")
	select {
	case <-invalidated:
	case <-time.After(time.Second):
		t.Fatal("navigation did not invalidate the window")
	}
	ui.consumeNavigationResult()

	if got, want := ui.address.Text(), pageURL.String(); got != want {
		t.Fatalf("address = %q, want %q", got, want)
	}
	if !strings.Contains(ui.status, "取得完了") || !strings.Contains(ui.status, "14 bytes") {
		t.Fatalf("status = %q, want successful load summary", ui.status)
	}
}
