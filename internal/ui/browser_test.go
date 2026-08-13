package ui

import (
	"context"
	"image"
	"net/url"
	"strings"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/saku0512/growse/internal/browser"
	"github.com/saku0512/growse/internal/dom"
	paintmodel "github.com/saku0512/growse/internal/paint"
	"github.com/saku0512/growse/internal/style"
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
func (navigator *stubNavigator) SetInputValue(nodeID dom.NodeID, value string) bool {
	if navigator.page == nil || navigator.page.Document == nil {
		return false
	}
	return navigator.page.Document.SetAttribute(nodeID, "value", value)
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

type recordingNavigator struct {
	stubNavigator
	navigated chan string
}

func (navigator *recordingNavigator) Navigate(_ context.Context, rawURL string) (*browser.Page, error) {
	navigator.navigated <- rawURL
	return navigator.page, navigator.err
}

func TestAddressEnterStartsNavigation(t *testing.T) {
	pageURL, err := url.Parse("https://example.com/search")
	if err != nil {
		t.Fatal(err)
	}
	navigator := &recordingNavigator{
		stubNavigator: stubNavigator{page: &browser.Page{URL: pageURL}},
		navigated:     make(chan string, 1),
	}
	ui := NewBrowserUI(navigator, nil)
	ui.address.SetText("example.com/search")

	router := new(input.Router)
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Source:      router.Source(),
		Constraints: layout.Exact(image.Pt(1280, 800)),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
	}
	ui.Layout(gtx)
	router.Frame(gtx.Ops)
	gtx.Execute(key.FocusCmd{Tag: &ui.address})
	router.Queue(key.Event{Name: key.NameReturn, State: key.Press})

	gtx.Reset()
	ui.Layout(gtx)
	select {
	case got := <-navigator.navigated:
		if want := "example.com/search"; got != want {
			t.Fatalf("Navigate() URL = %q, want %q", got, want)
		}
	case <-time.After(time.Second):
		t.Fatal("Enter did not start navigation")
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

func TestTextInputReceivesFocusFromPointerPress(t *testing.T) {
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

	ui.layoutDocument(gtx, page)
	router.Frame(gtx.Ops)
	router.Queue(pointer.Event{
		Buttons:  pointer.ButtonPrimary,
		Kind:     pointer.Press,
		Source:   pointer.Mouse,
		Position: f32.Pt(40, 40),
	})
	gtx.Reset()
	ui.layoutDocument(gtx, page)

	editor := ui.inputEditors[inputNode.ID]
	if editor == nil {
		t.Fatal("input editor was not created")
	}
	if !gtx.Focused(editor) {
		t.Fatal("pointer press did not focus the input editor")
	}
}

func TestTextInputWritesKeyboardEditsToDOM(t *testing.T) {
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

	ui.layoutDocument(gtx, page)
	router.Frame(gtx.Ops)
	editor := ui.inputEditors[inputNode.ID]
	gtx.Execute(key.FocusCmd{Tag: editor})
	router.Queue(key.EditEvent{Range: key.Range{Start: 0, End: 0}, Text: "hello"})
	gtx.Reset()
	ui.layoutDocument(gtx, page)

	if got, ok := inputNode.Attribute("value"); !ok || got != "hello" {
		t.Fatalf("DOM input value = (%q, %v), want (hello, true)", got, ok)
	}
	if got, want := editor.Text(), "hello"; got != want {
		t.Fatalf("editor text = %q, want %q", got, want)
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
