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
	"github.com/saku0512/growse/internal/dom"
	paintmodel "github.com/saku0512/growse/internal/paint"
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

func (navigator *stubNavigator) Back(context.Context) (*browser.Page, error) {
	return navigator.page, navigator.err
}

func (navigator *stubNavigator) Forward(context.Context) (*browser.Page, error) {
	return navigator.page, navigator.err
}

func (navigator *stubNavigator) Reload(context.Context) (*browser.Page, error) {
	return navigator.page, navigator.err
}

func (navigator *stubNavigator) CanBack() bool    { return true }
func (navigator *stubNavigator) CanForward() bool { return true }
func (navigator *stubNavigator) DispatchClick(dom.NodeID, float32, float32) bool {
	return false
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

	dims := ui.layoutToolbarButton(gtx, &ui.backButton, ui.backIcon, "戻る", true)
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
		Document:    testDocument(t),
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

func TestDocumentViewportFillsAvailableArea(t *testing.T) {
	pageURL, err := url.Parse("https://example.com")
	if err != nil {
		t.Fatal(err)
	}
	navigator := &stubNavigator{page: &browser.Page{URL: pageURL, Document: testDocument(t)}}
	ui := NewBrowserUI(navigator, nil)
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Constraints: layout.Exact(image.Pt(1000, 700)),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
	}

	dims := ui.layoutViewport(gtx)
	if got, want := dims.Size, image.Pt(1000, 700); got != want {
		t.Fatalf("document viewport size = %v, want %v", got, want)
	}
}

func TestDocumentPointIncludesListScrollOffset(t *testing.T) {
	ui := NewBrowserUI(nil, nil)
	ui.pageList.Position.First = 1
	ui.pageList.Position.Offset = 12
	displayList := &paintmodel.DisplayList{Commands: []paintmodel.Command{
		paintmodel.DrawText{Y: 32, Top: 32},
		paintmodel.DrawText{Y: 70, Top: 10},
	}}

	x, y, ok := ui.documentPoint(image.Pt(40, 18), displayList, 2)
	if !ok || x != 20 || y != 75 {
		t.Fatalf("documentPoint() = (%v, %v, %v), want (20, 75, true)", x, y, ok)
	}
}

func testDocument(t *testing.T) *dom.Document {
	t.Helper()
	document := dom.NewDocument()
	title := document.CreateElement("title", nil)
	text := document.CreateText("Loaded page")
	if err := document.AppendChild(document.Root, title); err != nil {
		t.Fatal(err)
	}
	if err := document.AppendChild(title, text); err != nil {
		t.Fatal(err)
	}
	return document
}
