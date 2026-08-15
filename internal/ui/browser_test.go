package ui

import (
	"context"
	"image"
	"image/color"
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

	"github.com/Grove-Computing/Growse/internal/browser"
	"github.com/Grove-Computing/Growse/internal/css"
	"github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/events"
	"github.com/Grove-Computing/Growse/internal/forms"
	layoutengine "github.com/Grove-Computing/Growse/internal/layout"
	paintmodel "github.com/Grove-Computing/Growse/internal/paint"
	"github.com/Grove-Computing/Growse/internal/style"
)

func TestCommandClipTranslatesDocumentCoordinatesToCommandCoordinates(t *testing.T) {
	gtx := layout.Context{Metric: unit.Metric{PxPerDp: 2, PxPerSp: 2}}
	got := commandClip(gtx, &layoutengine.Rect{X: 20, Y: 30, Width: 50, Height: 40}, 10, 15)
	if got.Min != image.Pt(20, 30) || got.Max != image.Pt(120, 110) {
		t.Fatalf("clip = %v, want [(20,30)-(120,110)]", got)
	}
}

func TestRasterLinearGradientInterpolatesAllColorStops(t *testing.T) {
	gradient := style.BackgroundImage{
		Kind: style.BackgroundImageLinearGradient, GradientAngle: 90,
		GradientStops: []style.GradientStop{
			{Color: 0xff0000ff, Position: 0},
			{Color: 0x00ff00ff, Position: .5},
			{Color: 0x0000ffff, Position: 1},
		},
	}
	image := rasterLinearGradient(3, 1, gradient)
	if left, middle, right := image.NRGBAAt(0, 0), image.NRGBAAt(1, 0), image.NRGBAAt(2, 0); left.R != 255 || middle.G != 255 || right.B != 255 {
		t.Fatalf("gradient pixels = %v, %v, %v", left, middle, right)
	}
}

func TestRasterRadialGradientUsesCenterAndStops(t *testing.T) {
	gradient := style.BackgroundImage{
		Kind:           style.BackgroundImageRadialGradient,
		GradientCenter: style.BackgroundPosition{X: style.LengthPercentage{Percentage: 50}, Y: style.LengthPercentage{Percentage: 50}},
		GradientStops:  []style.GradientStop{{Color: 0xff0000ff}, {Color: 0x0000ffff, Position: 1}},
	}
	image := rasterRadialGradient(5, 5, gradient)
	if center, edge := image.NRGBAAt(2, 2), image.NRGBAAt(0, 0); center.R <= center.B || edge.B <= edge.R {
		t.Fatalf("radial colors = center %#v edge %#v", center, edge)
	}
}

func TestRasterBackgroundImageRepeatsAndSizesImage(t *testing.T) {
	source := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	source.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
	source.SetNRGBA(1, 0, color.NRGBA{B: 255, A: 255})
	repeated := rasterBackgroundImage(4, 1, source, paintmodel.DrawBox{
		Repeat: style.BackgroundRepeat{X: true},
	}, 1)
	if got := []color.NRGBA{repeated.NRGBAAt(0, 0), repeated.NRGBAAt(1, 0), repeated.NRGBAAt(2, 0), repeated.NRGBAAt(3, 0)}; got[0].R != 255 || got[1].B != 255 || got[2].R != 255 || got[3].B != 255 {
		t.Fatalf("repeated pixels = %v", got)
	}

	green := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	green.SetNRGBA(0, 0, color.NRGBA{G: 255, A: 255})
	sized := rasterBackgroundImage(4, 1, green, paintmodel.DrawBox{
		Size: style.BackgroundSize{Kind: style.BackgroundSizeExplicit, Width: style.SizeValue{Kind: style.SizeLength, Value: style.LengthPercentage{Percentage: 50}}},
	}, 1)
	if sized.NRGBAAt(0, 0).G != 255 || sized.NRGBAAt(1, 0).G != 255 || sized.NRGBAAt(2, 0).A != 0 {
		t.Fatalf("sized pixels = %v, %v, %v", sized.NRGBAAt(0, 0), sized.NRGBAAt(1, 0), sized.NRGBAAt(2, 0))
	}
}

func TestPixelBorderRadiiPreservesEllipticalCorners(t *testing.T) {
	gtx := layout.Context{Metric: unit.Metric{PxPerDp: 2, PxPerSp: 2}}
	got := pixelBorderRadii(gtx, layoutengine.BorderRadii{
		TopLeft: layoutengine.CornerRadius{X: 8, Y: 4}, TopRight: layoutengine.CornerRadius{X: 6, Y: 6},
	})
	if got.TopLeft != (layoutengine.CornerRadius{X: 16, Y: 8}) || got.TopRight != (layoutengine.CornerRadius{X: 12, Y: 12}) || got.BottomRight != (layoutengine.CornerRadius{}) {
		t.Fatalf("rounded clip = %#v", got)
	}
}

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

func (navigator *stubNavigator) ReloadIgnoringCache(context.Context) (*browser.Page, error) {
	return navigator.page, navigator.err
}

func (navigator *stubNavigator) CanBack() bool    { return true }
func (navigator *stubNavigator) CanForward() bool { return true }
func (navigator *stubNavigator) DispatchClick(nodeID dom.NodeID, x, y float32) bool {
	if navigator.page == nil || navigator.page.Document == nil || navigator.page.Events == nil {
		return false
	}
	node, ok := navigator.page.Document.NodeByID(nodeID)
	if !ok || forms.Disabled(node) {
		return false
	}
	handled := navigator.page.Events.Dispatch(events.Event{Type: events.Click, Target: nodeID, X: x, Y: y})
	if forms.IsSubmitButton(node) {
		for current := node.Parent; current != nil; current = current.Parent {
			if current.Type == dom.NodeElement && current.TagName == "form" {
				return navigator.page.Events.Dispatch(events.Event{Type: events.Submit, Target: current.ID}) || handled
			}
		}
	}
	return handled
}
func (navigator *stubNavigator) SetInputValue(nodeID dom.NodeID, value string) bool {
	if navigator.page == nil || navigator.page.Document == nil {
		return false
	}
	node, ok := navigator.page.Document.NodeByID(nodeID)
	if !ok || forms.Disabled(node) || forms.ReadOnly(node) {
		return false
	}
	changed := forms.SetCurrentValue(node, value)
	if changed {
		navigator.recomputeHoverStyles()
	}
	return changed
}
func (navigator *stubNavigator) SetSelectValue(nodeID dom.NodeID, value string) bool {
	if navigator.page == nil || navigator.page.Document == nil {
		return false
	}
	changed := forms.SetSelectedValue(navigator.page.Document, nodeID, value)
	if changed {
		navigator.recomputeHoverStyles()
	}
	return changed
}
func (navigator *stubNavigator) ActivateCheckable(nodeID dom.NodeID) bool {
	if navigator.page == nil || navigator.page.Document == nil {
		return false
	}
	_, changed := forms.ActivateCheckable(navigator.page.Document, nodeID)
	if changed {
		navigator.recomputeHoverStyles()
	}
	return changed
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
func (navigator *stubNavigator) UpdateFocus(nodeID dom.NodeID) bool {
	if navigator.page == nil || navigator.page.Document == nil {
		return false
	}
	if nodeID != 0 {
		node, ok := navigator.page.Document.NodeByID(nodeID)
		if !ok || node.Type != dom.NodeElement || !navigator.page.Document.IsConnected(node) {
			nodeID = 0
		}
	}
	if navigator.page.FocusTarget == nodeID {
		return false
	}
	navigator.page.FocusTarget = nodeID
	navigator.recomputeHoverStyles()
	return true
}
func (navigator *stubNavigator) MoveFormFocus(reverse bool) bool {
	if navigator.page == nil || navigator.page.Document == nil {
		return false
	}
	target := forms.NextFocusable(navigator.page.Document, navigator.page.FocusTarget, reverse)
	return navigator.UpdateFocus(target)
}
func (navigator *stubNavigator) UpdateViewport(width, height float32) bool {
	if navigator.page == nil || navigator.page.Document == nil || width <= 0 || height <= 0 {
		return false
	}
	if navigator.page.ViewportWidth == width && navigator.page.ViewportHeight == height {
		return false
	}
	navigator.page.ViewportWidth, navigator.page.ViewportHeight = width, height
	navigator.recomputeHoverStyles()
	return true
}
func (navigator *stubNavigator) recomputeHoverStyles() {
	hovered := make(map[dom.NodeID]bool, len(navigator.page.HoverPath))
	for _, nodeID := range navigator.page.HoverPath {
		hovered[nodeID] = true
	}
	navigator.page.ComputedStyles = style.ComputeWithEnvironment(
		navigator.page.Document, navigator.page.Stylesheet,
		style.InteractionState{Hovered: hovered, Focused: navigator.page.FocusTarget},
		style.Environment{
			ViewportWidth: navigator.page.ViewportWidth, ViewportHeight: navigator.page.ViewportHeight, RootFontSize: 16,
			ResolutionDPI: 96, ColorScheme: "light", Hover: true, Pointer: "fine",
		},
	)
	navigator.page.StyleRevision++
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

func TestVerticalTabRailUsesLeftSideFixedWidth(t *testing.T) {
	ui := NewBrowserUI(nil, nil)
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Constraints: layout.Exact(image.Pt(1280, 800)),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
	}

	dims := ui.layoutTabRail(gtx)
	if got, want := dims.Size, image.Pt(224, 800); got != want {
		t.Fatalf("vertical tab rail size = %v, want %v", got, want)
	}
}

func TestBrowserChromeHasNoHorizontalTabStrip(t *testing.T) {
	geometry := calculateBrowserChromeGeometry(image.Pt(1280, 800), 224, 92)

	if got, want := geometry.tabRail, image.Rect(0, 0, 224, 800); got != want {
		t.Fatalf("tab rail = %v, want %v", got, want)
	}
	if got, want := geometry.toolbar, image.Rect(224, 0, 1280, 92); got != want {
		t.Fatalf("toolbar = %v, want %v", got, want)
	}
	if got, want := geometry.viewport, image.Rect(224, 92, 1280, 800); got != want {
		t.Fatalf("viewport = %v, want %v", got, want)
	}
	if geometry.toolbar.Min.Y != 0 {
		t.Fatalf("toolbar must touch the window top; an unexpected horizontal tab strip may have been inserted: %v", geometry.toolbar)
	}
}

func TestTabRailCoordinatesAreExcludedFromPageHitTesting(t *testing.T) {
	gtx := layout.Context{
		Constraints: layout.Exact(image.Pt(576, 708)),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
	}
	tests := []struct {
		name     string
		position f32.Point
		want     image.Point
		inside   bool
	}{
		{name: "tab rail", position: f32.Pt(100, 200), inside: false},
		{name: "toolbar", position: f32.Pt(244, 40), inside: false},
		{name: "page viewport", position: f32.Pt(244, 112), want: image.Pt(20, 20), inside: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, inside := viewportPointerPosition(gtx, test.position, true)
			if got != test.want || inside != test.inside {
				t.Fatalf("viewport pointer = (%v, %v), want (%v, %v)", got, inside, test.want, test.inside)
			}
		})
	}
}

func TestNarrowWindowKeepsChromeRegionsSafeAndDisjoint(t *testing.T) {
	sizes := []image.Point{
		{},
		image.Pt(1, 1),
		image.Pt(160, 80),
		image.Pt(223, 91),
		image.Pt(224, 92),
		image.Pt(300, 180),
	}
	for _, size := range sizes {
		t.Run(size.String(), func(t *testing.T) {
			geometry := calculateBrowserChromeGeometry(size, 224, 92)
			window := image.Rectangle{Max: size}
			for name, region := range map[string]image.Rectangle{
				"tab rail": geometry.tabRail,
				"toolbar":  geometry.toolbar,
				"viewport": geometry.viewport,
			} {
				if region.Dx() < 0 || region.Dy() < 0 || !region.In(window) {
					t.Fatalf("%s region %v is invalid for window %v", name, region, window)
				}
			}
			if geometry.tabRail.Overlaps(geometry.toolbar) || geometry.tabRail.Overlaps(geometry.viewport) || geometry.toolbar.Overlaps(geometry.viewport) {
				t.Fatalf("chrome regions overlap for window %v: %+v", size, geometry)
			}

			ui := NewBrowserUI(nil, nil)
			gtx := layout.Context{Ops: new(op.Ops), Constraints: layout.Exact(size), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}
			if got := ui.Layout(gtx).Size; got != size {
				t.Fatalf("narrow browser UI size = %v, want %v", got, size)
			}
		})
	}
}

func TestTabRowDisplaysTitleFallbackAndLifecycleState(t *testing.T) {
	tests := []struct {
		name      string
		tab       browser.TabSnapshot
		wantTitle string
		wantState string
	}{
		{name: "title", tab: browser.TabSnapshot{Title: "Dashboard", URL: "https://example.com/", Active: true}, wantTitle: "Dashboard", wantState: "選択中"},
		{name: "host fallback", tab: browser.TabSnapshot{URL: "https://docs.example.com/path", Loading: true}, wantTitle: "docs.example.com", wantState: "読込中"},
		{name: "blank fallback", tab: browser.TabSnapshot{Error: true}, wantTitle: "新しいタブ", wantState: "エラー"},
		{name: "pending update", tab: browser.TabSnapshot{PendingUpdate: true}, wantTitle: "新しいタブ", wantState: "更新あり"},
		{name: "combined", tab: browser.TabSnapshot{Active: true, Loading: true, Error: true, PendingUpdate: true}, wantTitle: "新しいタブ", wantState: "選択中 · 読込中 · エラー · 更新あり"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := tabDisplayTitle(test.tab); got != test.wantTitle {
				t.Fatalf("tab title = %q, want %q", got, test.wantTitle)
			}
			if got := tabStateLabel(test.tab); got != test.wantState {
				t.Fatalf("tab state = %q, want %q", got, test.wantState)
			}
		})
	}
}

func TestActiveTabRowHasVisibleFixedHeight(t *testing.T) {
	ui := NewBrowserUI(nil, nil)
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Constraints: layout.Constraints{Max: image.Pt(204, 400)},
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
	}

	dims := ui.layoutTabRow(gtx, browser.TabSnapshot{Active: true, Title: "Dashboard"})
	if got, want := dims.Size, image.Pt(204, 64); got != want {
		t.Fatalf("active tab row size = %v, want %v", got, want)
	}
}

func TestVerticalTabPointerControlsCreateSelectAndCloseTabs(t *testing.T) {
	session := browser.NewSession()
	first, err := session.NewTab(nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := session.NewTab(nil)
	if err != nil {
		t.Fatal(err)
	}
	ui := NewBrowserUIWithTabs(nil, session, nil)
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Constraints: layout.Exact(image.Pt(224, 400)),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
	}
	ui.layoutTabRail(gtx)

	ui.tabRowButtons[second.ID].Click()
	ui.handleTabActions(gtx)
	if active, ok := session.ActiveTab(); !ok || active.ID != second.ID {
		t.Fatalf("active tab after row click = (%+v, %v), want %d", active, ok, second.ID)
	}

	ui.tabCloseButtons[first.ID].Click()
	ui.handleTabActions(gtx)
	if tabs := session.Tabs(); len(tabs) != 1 || tabs[0].ID != second.ID {
		t.Fatalf("tabs after close click = %+v, want only tab %d", tabs, second.ID)
	}

	ui.newTabButton.Click()
	ui.handleTabActions(gtx)
	if got := len(session.Tabs()); got != 2 {
		t.Fatalf("tab count after new tab click = %d, want 2", got)
	}
}

func TestOverflowingTabRailScrollIsIndependentFromPageScroll(t *testing.T) {
	session := browser.NewSession()
	for index := 0; index < 12; index++ {
		if _, err := session.NewTab(nil); err != nil {
			t.Fatal(err)
		}
	}
	ui := NewBrowserUIWithTabs(nil, session, nil)
	ui.tabList.Position = layout.Position{First: 5, Offset: 3}
	ui.pageList.Position = layout.Position{First: 2, Offset: 17}
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Constraints: layout.Exact(image.Pt(204, 128)),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
	}

	dims := ui.layoutTabList(gtx, session.Tabs())
	if got, want := dims.Size, image.Pt(204, 128); got != want {
		t.Fatalf("overflowing tab list size = %v, want %v", got, want)
	}
	if ui.tabList.Position.First == 0 {
		t.Fatalf("tab list scroll position was reset: %+v", ui.tabList.Position)
	}
	if got, want := ui.pageList.Position, (layout.Position{First: 2, Offset: 17}); got != want {
		t.Fatalf("page scroll changed with tab rail: %+v, want %+v", got, want)
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

type reloadRecordingNavigator struct {
	stubNavigator
	reloads chan bool
}

func (navigator *reloadRecordingNavigator) Reload(context.Context) (*browser.Page, error) {
	navigator.reloads <- false
	return navigator.page, navigator.err
}

func (navigator *reloadRecordingNavigator) ReloadIgnoringCache(context.Context) (*browser.Page, error) {
	navigator.reloads <- true
	return navigator.page, navigator.err
}

func TestKeyboardReloadShortcuts(t *testing.T) {
	tests := []struct {
		name        string
		modifiers   key.Modifiers
		ignoreCache bool
		status      string
	}{
		{name: "Ctrl+R", modifiers: key.ModShortcut, status: "ページを再読み込み中"},
		{name: "Ctrl+Shift+R", modifiers: key.ModShortcut | key.ModShift, ignoreCache: true, status: "キャッシュを無視して再読み込み中"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pageURL, err := url.Parse("https://example.com/page")
			if err != nil {
				t.Fatal(err)
			}
			navigator := &reloadRecordingNavigator{
				stubNavigator: stubNavigator{page: &browser.Page{URL: pageURL}},
				reloads:       make(chan bool, 1),
			}
			ui := NewBrowserUI(navigator, nil)
			defer ui.Close()
			router := new(input.Router)
			gtx := layout.Context{
				Ops:         new(op.Ops),
				Source:      router.Source(),
				Constraints: layout.Exact(image.Pt(1280, 800)),
				Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			}
			ui.Layout(gtx)
			router.Frame(gtx.Ops)
			router.Queue(key.Event{Name: "R", Modifiers: test.modifiers, State: key.Press})

			gtx.Reset()
			ui.Layout(gtx)
			select {
			case got := <-navigator.reloads:
				if got != test.ignoreCache {
					t.Fatalf("ignore cache = %t, want %t", got, test.ignoreCache)
				}
			case <-time.After(time.Second):
				t.Fatal("reload shortcut did not start navigation")
			}
			if ui.status != test.status {
				t.Fatalf("status = %q, want %q", ui.status, test.status)
			}
		})
	}
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
	router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: f32.Pt(float32(tabRailWidth)+40, float32(toolbarHeight)+40)})
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
	if strings.Contains(ui.status, "secret") || strings.Contains(ui.status, "alice") || ui.status != "https://example.com/private" {
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
	router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: f32.Pt(float32(tabRailWidth)+40, float32(toolbarHeight)+40)})
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
	if got, want := page.FocusTarget, inputNode.ID; got != want {
		t.Fatalf("page focus target = %d, want %d", got, want)
	}
}

func TestPaintOnlyAnimationFramesReuseLayoutTree(t *testing.T) {
	document := dom.NewDocument()
	target := document.CreateElement("div", nil)
	if err := document.AppendChild(document.Root, target); err != nil {
		t.Fatal(err)
	}
	stylesheet, err := css.Parse(strings.NewReader(`
@keyframes fade { from { opacity: 0; } to { opacity: 1; } }
div { width: 100px; height: 100px; animation: fade 1s linear infinite; }
`))
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, time.August, 14, 12, 0, 0, 0, time.UTC)
	computed := style.Compute(document, stylesheet)
	page := &browser.Page{
		Document: document, Stylesheet: stylesheet, ComputedStyles: computed,
		Animations: style.NewAnimationRegistry(), StyleRevision: 1,
	}
	page.Animations.Reconcile(computed, start)
	ui := NewBrowserUI(&stubNavigator{page: page}, nil)
	builds := 0
	ui.layoutBuild = func(document *dom.Document, styles style.Map, width, height, scrollX, scrollY float32) *layoutengine.Tree {
		builds++
		return layoutengine.BuildWithScroll(document, styles, width, height, scrollX, scrollY)
	}
	gtx := layout.Context{
		Now: start, Ops: new(op.Ops), Constraints: layout.Exact(image.Pt(800, 600)),
		Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
	}

	ui.layoutDocument(gtx, page)
	gtx.Reset()
	gtx.Now = start.Add(500 * time.Millisecond)
	ui.layoutDocument(gtx, page)
	if builds != 1 {
		t.Fatalf("layout builds across animation frames = %d, want 1", builds)
	}
}

func TestFocusableNodeIDFindsInteractiveAncestor(t *testing.T) {
	document := dom.NewDocument()
	link := document.CreateElement("a", map[string]string{"href": "/next"})
	label := document.CreateText("next")
	plain := document.CreateElement("div", nil)
	if err := document.AppendChild(document.Root, link); err != nil {
		t.Fatal(err)
	}
	if err := document.AppendChild(link, label); err != nil {
		t.Fatal(err)
	}
	if err := document.AppendChild(document.Root, plain); err != nil {
		t.Fatal(err)
	}
	if got, want := focusableNodeID(document, label.ID), link.ID; got != want {
		t.Fatalf("link focus target = %d, want %d", got, want)
	}
	if got := focusableNodeID(document, plain.ID); got != 0 {
		t.Fatalf("plain focus target = %d, want 0", got)
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

	if got := forms.CurrentValue(inputNode); got != "hello" {
		t.Fatalf("DOM input value = %q, want hello", got)
	}
	if got, want := editor.Text(), "hello"; got != want {
		t.Fatalf("editor text = %q, want %q", got, want)
	}
}

func TestTextareaEditorAcceptsNewlineAndWritesDOMValue(t *testing.T) {
	document := dom.NewDocument()
	textarea := document.CreateElement("textarea", nil)
	if err := document.AppendChild(document.Root, textarea); err != nil {
		t.Fatal(err)
	}
	if err := document.AppendChild(textarea, document.CreateText("first")); err != nil {
		t.Fatal(err)
	}
	page := &browser.Page{Document: document, ComputedStyles: style.Compute(document, nil)}
	ui := NewBrowserUI(&stubNavigator{page: page}, nil)
	router := new(input.Router)
	gtx := layout.Context{
		Ops: new(op.Ops), Source: router.Source(), Constraints: layout.Exact(image.Pt(800, 600)),
		Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
	}

	ui.layoutDocument(gtx, page)
	router.Frame(gtx.Ops)
	editor := ui.inputEditors[textarea.ID]
	if editor == nil {
		t.Fatal("textarea editor was not created")
	}
	if editor.SingleLine || editor.Text() != "first" {
		t.Fatalf("textarea editor singleLine=%v text=%q", editor.SingleLine, editor.Text())
	}
	gtx.Execute(key.FocusCmd{Tag: editor})
	router.Queue(key.EditEvent{Range: key.Range{Start: 5, End: 5}, Text: "\nsecond"})
	gtx.Reset()
	ui.layoutDocument(gtx, page)

	if got := forms.CurrentValue(textarea); got != "first\nsecond" {
		t.Fatalf("textarea DOM value = %q", got)
	}
	if editor.Text() != "first\nsecond" {
		t.Fatalf("textarea editor text = %q", editor.Text())
	}
}

func TestPasswordEditorMasksDisplayWithoutChangingValue(t *testing.T) {
	document := dom.NewDocument()
	password := document.CreateElement("input", map[string]string{"type": "password", "value": "secret"})
	if err := document.AppendChild(document.Root, password); err != nil {
		t.Fatal(err)
	}
	page := &browser.Page{Document: document, ComputedStyles: style.Compute(document, nil)}
	ui := NewBrowserUI(&stubNavigator{page: page}, nil)
	gtx := layout.Context{Ops: new(op.Ops), Constraints: layout.Exact(image.Pt(800, 600)), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}

	ui.layoutDocument(gtx, page)
	editor := ui.inputEditors[password.ID]
	if editor == nil {
		t.Fatal("password editor was not created")
	}
	if editor.Mask != '•' || editor.Text() != "secret" {
		t.Fatalf("password mask=%q text=%q", editor.Mask, editor.Text())
	}
}

func TestReadonlyAndDisabledEditorsRejectEditing(t *testing.T) {
	document := dom.NewDocument()
	readonly := document.CreateElement("input", map[string]string{"value": "fixed", "readonly": ""})
	disabled := document.CreateElement("input", map[string]string{"value": "disabled", "disabled": ""})
	if err := document.AppendChild(document.Root, readonly); err != nil {
		t.Fatal(err)
	}
	if err := document.AppendChild(document.Root, disabled); err != nil {
		t.Fatal(err)
	}
	page := &browser.Page{Document: document, ComputedStyles: style.Compute(document, nil)}
	ui := NewBrowserUI(&stubNavigator{page: page}, nil)
	gtx := layout.Context{Ops: new(op.Ops), Constraints: layout.Exact(image.Pt(800, 600)), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}

	ui.layoutDocument(gtx, page)
	if !ui.inputEditors[readonly.ID].ReadOnly || !ui.inputEditors[disabled.ID].ReadOnly {
		t.Fatal("readonly or disabled editor remained editable")
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

func TestSelectButtonDisplaysAndChangesSelectedOption(t *testing.T) {
	document := dom.NewDocument()
	selectNode := document.CreateElement("select", nil)
	first := document.CreateElement("option", map[string]string{"value": "one", "selected": ""})
	second := document.CreateElement("option", map[string]string{"value": "two"})
	for _, edge := range [][2]*dom.Node{{document.Root, selectNode}, {selectNode, first}, {first, document.CreateText("One")}, {selectNode, second}, {second, document.CreateText("Two")}} {
		if err := document.AppendChild(edge[0], edge[1]); err != nil {
			t.Fatal(err)
		}
	}
	page := &browser.Page{Document: document, ComputedStyles: style.Compute(document, nil), Events: events.NewDispatcher()}
	ui := NewBrowserUI(&stubNavigator{page: page}, nil)
	gtx := layout.Context{Ops: new(op.Ops), Constraints: layout.Exact(image.Pt(800, 600)), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}

	ui.layoutDocument(gtx, page)
	button := ui.selectButtons[selectNode.ID]
	if button == nil {
		t.Fatal("select button was not created")
	}
	button.Click()
	gtx.Reset()
	ui.layoutDocument(gtx, page)
	if got := forms.CurrentValue(selectNode); got != "two" {
		t.Fatalf("selected value = %q, want two", got)
	}
}

func TestCheckableButtonUsesSharedActivationPath(t *testing.T) {
	document := dom.NewDocument()
	checkbox := document.CreateElement("input", map[string]string{"type": "checkbox"})
	if err := document.AppendChild(document.Root, checkbox); err != nil {
		t.Fatal(err)
	}
	page := &browser.Page{Document: document, ComputedStyles: style.Compute(document, nil), Events: events.NewDispatcher()}
	ui := NewBrowserUI(&stubNavigator{page: page}, nil)
	gtx := layout.Context{Ops: new(op.Ops), Constraints: layout.Exact(image.Pt(800, 600)), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}

	ui.layoutDocument(gtx, page)
	button := ui.checkableButtons[checkbox.ID]
	if button == nil {
		t.Fatal("checkable button was not created")
	}
	button.Click()
	gtx.Reset()
	ui.layoutDocument(gtx, page)
	if !forms.CurrentChecked(checkbox) {
		t.Fatal("checkbox was not activated")
	}
}

func TestCheckableButtonActivatesWithSpaceKey(t *testing.T) {
	document := dom.NewDocument()
	checkbox := document.CreateElement("input", map[string]string{"type": "checkbox"})
	if err := document.AppendChild(document.Root, checkbox); err != nil {
		t.Fatal(err)
	}
	page := &browser.Page{Document: document, ComputedStyles: style.Compute(document, nil), Events: events.NewDispatcher()}
	ui := NewBrowserUI(&stubNavigator{page: page}, nil)
	router := new(input.Router)
	gtx := layout.Context{Ops: new(op.Ops), Source: router.Source(), Constraints: layout.Exact(image.Pt(800, 600)), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}

	ui.layoutDocument(gtx, page)
	router.Frame(gtx.Ops)
	button := ui.checkableButtons[checkbox.ID]
	gtx.Execute(key.FocusCmd{Tag: button})
	gtx.Reset()
	ui.layoutDocument(gtx, page)
	router.Frame(gtx.Ops)
	router.Queue(
		key.Event{Name: key.NameSpace, State: key.Press},
		key.Event{Name: key.NameSpace, State: key.Release},
	)
	gtx.Reset()
	ui.layoutDocument(gtx, page)

	if !forms.CurrentChecked(checkbox) {
		t.Fatal("Space did not activate checkbox")
	}
}

func TestTabAndShiftTabMoveFormFocusInDOMOrder(t *testing.T) {
	document := dom.NewDocument()
	first := document.CreateElement("input", nil)
	last := document.CreateElement("input", map[string]string{"type": "checkbox"})
	if err := document.AppendChild(document.Root, first); err != nil {
		t.Fatal(err)
	}
	if err := document.AppendChild(document.Root, last); err != nil {
		t.Fatal(err)
	}
	page := &browser.Page{Document: document, ComputedStyles: style.Compute(document, nil), Events: events.NewDispatcher()}
	ui := NewBrowserUI(&stubNavigator{page: page}, nil)
	router := new(input.Router)
	gtx := layout.Context{Ops: new(op.Ops), Source: router.Source(), Constraints: layout.Exact(image.Pt(800, 600)), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}

	ui.layoutDocument(gtx, page)
	router.Frame(gtx.Ops)
	router.Queue(key.Event{Name: key.NameTab, State: key.Press})
	gtx.Reset()
	ui.layoutDocument(gtx, page)
	if page.FocusTarget != first.ID {
		t.Fatalf("Tab focus = %d, want %d", page.FocusTarget, first.ID)
	}
	router.Frame(gtx.Ops)
	router.Queue(key.Event{Name: key.NameTab, Modifiers: key.ModShift, State: key.Press})
	gtx.Reset()
	ui.layoutDocument(gtx, page)
	if page.FocusTarget != last.ID {
		t.Fatalf("Shift+Tab focus = %d, want %d", page.FocusTarget, last.ID)
	}
}

func TestSubmitButtonActivatesWithSpaceKey(t *testing.T) {
	document := dom.NewDocument()
	form := document.CreateElement("form", nil)
	buttonNode := document.CreateElement("button", nil)
	if err := document.AppendChild(document.Root, form); err != nil {
		t.Fatal(err)
	}
	if err := document.AppendChild(form, buttonNode); err != nil {
		t.Fatal(err)
	}
	if err := document.AppendChild(buttonNode, document.CreateText("Send")); err != nil {
		t.Fatal(err)
	}
	page := &browser.Page{Document: document, ComputedStyles: style.Compute(document, nil), Events: events.NewDispatcher()}
	submissions := 0
	page.Events.AddEventListener(form.ID, events.Submit, func(events.Event) { submissions++ })
	ui := NewBrowserUI(&stubNavigator{page: page}, nil)
	router := new(input.Router)
	gtx := layout.Context{Ops: new(op.Ops), Source: router.Source(), Constraints: layout.Exact(image.Pt(800, 600)), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}

	ui.layoutDocument(gtx, page)
	router.Frame(gtx.Ops)
	button := ui.formButtons[buttonNode.ID]
	gtx.Execute(key.FocusCmd{Tag: button})
	gtx.Reset()
	ui.layoutDocument(gtx, page)
	router.Frame(gtx.Ops)
	router.Queue(key.Event{Name: key.NameSpace, State: key.Press}, key.Event{Name: key.NameSpace, State: key.Release})
	gtx.Reset()
	ui.layoutDocument(gtx, page)
	if submissions != 1 {
		t.Fatalf("submissions = %d, want 1", submissions)
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
