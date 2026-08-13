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
	"github.com/saku0512/growse/internal/css"
	"github.com/saku0512/growse/internal/dom"
	"github.com/saku0512/growse/internal/events"
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
func (navigator *stubNavigator) CommitInputValue(nodeID dom.NodeID, value string) bool {
	if navigator.page == nil || navigator.page.Events == nil {
		return false
	}
	return navigator.page.Events.Dispatch(events.Event{Type: events.Change, Target: nodeID, Value: value})
}
func (navigator *stubNavigator) SubmitForm(nodeID dom.NodeID) bool {
	if navigator.page == nil || navigator.page.Document == nil || navigator.page.Events == nil {
		return false
	}
	node, ok := navigator.page.Document.NodeByID(nodeID)
	if !ok {
		return false
	}
	for current := node; current != nil; current = current.Parent {
		if current.Type == dom.NodeElement && current.TagName == "form" {
			return navigator.page.Events.Dispatch(events.Event{Type: events.Submit, Target: current.ID})
		}
	}
	return false
}
func (navigator *stubNavigator) UpdateHover(nodeID dom.NodeID, _, _ float32) bool {
	if navigator.page == nil || navigator.page.Document == nil {
		return false
	}
	node, ok := navigator.page.Document.NodeByID(nodeID)
	if !ok || !navigator.page.Document.IsConnected(node) {
		return navigator.ClearHover()
	}
	var reversed []dom.NodeID
	for current := node; current != nil; current = current.Parent {
		if current.Type == dom.NodeElement {
			reversed = append(reversed, current.ID)
		}
	}
	path := make([]dom.NodeID, len(reversed))
	for index := range reversed {
		path[len(reversed)-1-index] = reversed[index]
	}
	if equalNodeIDPath(navigator.page.HoverPath, path) {
		return false
	}
	navigator.page.HoverTarget = nodeID
	navigator.page.HoverPath = path
	navigator.recomputeHoverStyles()
	return true
}
func (navigator *stubNavigator) ClearHover() bool {
	if navigator.page == nil || len(navigator.page.HoverPath) == 0 {
		return false
	}
	navigator.page.HoverTarget = 0
	navigator.page.HoverPath = nil
	navigator.recomputeHoverStyles()
	return true
}
func (navigator *stubNavigator) recomputeHoverStyles() {
	hovered := make(map[dom.NodeID]bool, len(navigator.page.HoverPath))
	for _, nodeID := range navigator.page.HoverPath {
		hovered[nodeID] = true
	}
	navigator.page.ComputedStyles = style.ComputeWithState(
		navigator.page.Document, navigator.page.Stylesheet, style.InteractionState{Hovered: hovered},
	)
}
func equalNodeIDPath(left, right []dom.NodeID) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func TestToolbarHasFixedHeight(t *testing.T) {
	ui := NewBrowserUI(nil, nil)
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Constraints: layout.Exact(image.Pt(1280, 800)),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
	}

	dims := ui.layoutToolbar(gtx)
	if got, want := dims.Size.Y, 92; got != want {
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

func TestPointerMoveAppliesAndClearsHoverStyle(t *testing.T) {
	document := dom.NewDocument()
	button := document.CreateElement("button", map[string]string{"id": "save"})
	if err := document.AppendChild(document.Root, button); err != nil {
		t.Fatal(err)
	}
	if err := document.AppendChild(button, document.CreateText("Save")); err != nil {
		t.Fatal(err)
	}
	stylesheet, err := css.Parse(strings.NewReader(`button:hover { color: red; font-size: 24px }`))
	if err != nil {
		t.Fatal(err)
	}
	page := &browser.Page{Document: document, Stylesheet: stylesheet, ComputedStyles: style.Compute(document, stylesheet)}
	navigator := &stubNavigator{page: page}
	ui := NewBrowserUI(navigator, nil)
	router := new(input.Router)
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Source:      router.Source(),
		Constraints: layout.Exact(image.Pt(800, 600)),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
	}

	ui.Layout(gtx)
	router.Frame(gtx.Ops)
	router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: f32.Pt(40, float32(toolbarHeight)+40)})
	gtx.Reset()
	ui.Layout(gtx)

	if page.HoverTarget != button.ID {
		t.Fatalf("hover target = %d, want %d", page.HoverTarget, button.ID)
	}
	hovered, _ := page.ComputedStyles.For(button)
	if hovered.Color != 0xff0000ff || hovered.FontSize != 24 {
		t.Fatalf("hovered style = %#v", hovered)
	}

	router.Frame(gtx.Ops)
	router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: f32.Pt(700, float32(toolbarHeight)+500)})
	gtx.Reset()
	ui.Layout(gtx)
	if page.HoverTarget != 0 || len(page.HoverPath) != 0 {
		t.Fatalf("hover state remains target:%d path:%v", page.HoverTarget, page.HoverPath)
	}
	normal, _ := page.ComputedStyles.For(button)
	if normal.Color == 0xff0000ff || normal.FontSize == 24 {
		t.Fatalf("hover style remains after pointer leaves element: %#v", normal)
	}
}

func TestLinkHoverShowsResolvedURLAndRestoresPageStatus(t *testing.T) {
	document := dom.NewDocument()
	anchor := document.CreateElement("a", map[string]string{"href": "../next?q=1"})
	label := document.CreateElement("span", nil)
	outside := document.CreateElement("p", nil)
	for _, edge := range [][2]*dom.Node{
		{document.Root, anchor},
		{anchor, label},
		{document.Root, outside},
	} {
		if err := document.AppendChild(edge[0], edge[1]); err != nil {
			t.Fatal(err)
		}
	}
	pageURL, err := url.Parse("https://example.com/docs/current")
	if err != nil {
		t.Fatal(err)
	}
	page := &browser.Page{URL: pageURL, Document: document}
	ui := NewBrowserUI(&stubNavigator{page: page}, nil)
	ui.pageStatus = "取得完了 · HTTP 200"
	ui.status = ui.pageStatus

	ui.updateLinkPreview(page, label.ID)
	if got, want := ui.status, "https://example.com/next?q=1"; got != want {
		t.Fatalf("link status = %q, want %q", got, want)
	}
	ui.updateLinkPreview(page, outside.ID)
	if got, want := ui.status, ui.pageStatus; got != want {
		t.Fatalf("restored status = %q, want %q", got, want)
	}
}

func TestLinkPreviewRedactsCredentialsAndIgnoresInvalidURL(t *testing.T) {
	document := dom.NewDocument()
	secret := document.CreateElement("a", map[string]string{"href": "https://alice:secret@example.com/private"})
	invalid := document.CreateElement("a", map[string]string{"href": "http://[::1"})
	for _, node := range []*dom.Node{secret, invalid} {
		if err := document.AppendChild(document.Root, node); err != nil {
			t.Fatal(err)
		}
	}
	pageURL, err := url.Parse("https://example.com")
	if err != nil {
		t.Fatal(err)
	}
	page := &browser.Page{URL: pageURL, Document: document}
	ui := NewBrowserUI(&stubNavigator{page: page}, nil)
	ui.pageStatus = "通常状態"
	ui.status = ui.pageStatus

	ui.updateLinkPreview(page, secret.ID)
	if strings.Contains(ui.status, "secret") || ui.status != "https://alice:xxxxx@example.com/private" {
		t.Fatalf("credential link status = %q, want redacted URL", ui.status)
	}
	ui.updateLinkPreview(page, invalid.ID)
	if got, want := ui.status, ui.pageStatus; got != want {
		t.Fatalf("invalid link status = %q, want %q", got, want)
	}
}

func TestLinkPreviewDoesNotOverrideLoadingOrErrorStatus(t *testing.T) {
	document := dom.NewDocument()
	anchor := document.CreateElement("a", map[string]string{"href": "/next"})
	if err := document.AppendChild(document.Root, anchor); err != nil {
		t.Fatal(err)
	}
	pageURL, err := url.Parse("https://example.com/current")
	if err != nil {
		t.Fatal(err)
	}
	page := &browser.Page{URL: pageURL, Document: document}
	ui := NewBrowserUI(&stubNavigator{page: page}, nil)

	ui.loading = true
	ui.status = "読み込み中"
	ui.updateLinkPreview(page, anchor.ID)
	if ui.status != "読み込み中" {
		t.Fatalf("loading status was overwritten: %q", ui.status)
	}
	ui.loading = false
	ui.statusHasError = true
	ui.status = "Runtimeエラー"
	ui.updateLinkPreview(page, anchor.ID)
	if ui.status != "Runtimeエラー" {
		t.Fatalf("error status was overwritten: %q", ui.status)
	}
}

func TestPointerHoveringLinkDoesNotStartNavigation(t *testing.T) {
	document := dom.NewDocument()
	anchor := document.CreateElement("a", map[string]string{"href": "/next"})
	if err := document.AppendChild(document.Root, anchor); err != nil {
		t.Fatal(err)
	}
	if err := document.AppendChild(anchor, document.CreateText("Next")); err != nil {
		t.Fatal(err)
	}
	pageURL, err := url.Parse("https://example.com/current")
	if err != nil {
		t.Fatal(err)
	}
	page := &browser.Page{URL: pageURL, Document: document, ComputedStyles: style.Compute(document, nil)}
	navigator := &recordingNavigator{
		stubNavigator: stubNavigator{page: page},
		navigated:     make(chan string, 1),
	}
	ui := NewBrowserUI(navigator, nil)
	ui.pageStatus = "通常状態"
	ui.status = ui.pageStatus
	router := new(input.Router)
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Source:      router.Source(),
		Constraints: layout.Exact(image.Pt(800, 600)),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
	}

	ui.Layout(gtx)
	router.Frame(gtx.Ops)
	router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: f32.Pt(40, float32(toolbarHeight)+40)})
	gtx.Reset()
	ui.Layout(gtx)

	if got, want := ui.status, "https://example.com/next"; got != want {
		t.Fatalf("hover status = %q, want %q", got, want)
	}
	select {
	case requested := <-navigator.navigated:
		t.Fatalf("hover unexpectedly navigated to %q", requested)
	default:
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

func TestTextInputEnterDispatchesChangeAfterEdit(t *testing.T) {
	document := dom.NewDocument()
	form := document.CreateElement("form", map[string]string{"id": "search-form"})
	inputNode := document.CreateElement("input", map[string]string{"id": "query"})
	if err := document.AppendChild(document.Root, form); err != nil {
		t.Fatal(err)
	}
	if err := document.AppendChild(form, inputNode); err != nil {
		t.Fatal(err)
	}
	page := &browser.Page{
		Document: document, ComputedStyles: style.Compute(document, nil), Events: events.NewDispatcher(),
	}
	var changes []events.Event
	var submissions []events.Event
	page.Events.AddEventListener(inputNode.ID, events.Change, func(event events.Event) {
		changes = append(changes, event)
	})
	page.Events.AddEventListener(form.ID, events.Submit, func(event events.Event) {
		submissions = append(submissions, event)
	})
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
	gtx.Reset()
	ui.layoutDocument(gtx, page)
	router.Frame(gtx.Ops)
	router.Queue(key.EditEvent{Range: key.Range{Start: 0, End: 0}, Text: "hello"})
	gtx.Reset()
	ui.layoutDocument(gtx, page)
	router.Frame(gtx.Ops)
	router.Queue(key.Event{Name: key.NameReturn, State: key.Press})
	gtx.Reset()
	ui.layoutDocument(gtx, page)

	if got, want := len(changes), 1; got != want {
		t.Fatalf("change event count = %d, want %d", got, want)
	}
	if changes[0].Value != "hello" {
		t.Fatalf("change value = %q, want hello", changes[0].Value)
	}
	if got, want := len(submissions), 1; got != want {
		t.Fatalf("submit event count = %d, want %d", got, want)
	}
	if submissions[0].Target != form.ID {
		t.Fatalf("submit target = %d, want %d", submissions[0].Target, form.ID)
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
