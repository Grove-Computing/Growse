// Package ui provides Gio widgets for the Growse browser chrome.
package ui

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"image"
	"image/color"
	imagedraw "image/draw"
	"image/png"
	"log/slog"
	"math"
	"net/url"
	"strings"
	"time"

	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/gesture"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"golang.org/x/exp/shiny/materialdesign/icons"
	xdraw "golang.org/x/image/draw"

	"github.com/Grove-Computing/Growse/internal/browser"
	devtoolsmodel "github.com/Grove-Computing/Growse/internal/devtools"
	"github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/forms"
	layoutengine "github.com/Grove-Computing/Growse/internal/layout"
	"github.com/Grove-Computing/Growse/internal/network"
	paintmodel "github.com/Grove-Computing/Growse/internal/paint"
	stylemodel "github.com/Grove-Computing/Growse/internal/style"
	"github.com/Grove-Computing/Growse/internal/updater"
)

//go:embed assets/gopher-blue.png
var gopherPNG []byte

const (
	defaultURL         = "http://localhost:8080"
	tabRailWidth       = unit.Dp(224)
	toolbarHeight      = unit.Dp(92)
	controlHeight      = unit.Dp(44)
	addressBarHeight   = unit.Dp(48)
	gopherButtonWidth  = unit.Dp(72)
	gopherButtonHeight = unit.Dp(52)
	devToolsHeight     = unit.Dp(280)
)

// BrowserUI owns the widgets displayed around the page viewport.
type BrowserUI struct {
	theme            *material.Theme
	navigator        Navigator
	invalidate       func()
	results          chan navigationResult
	navigations      map[browser.TabID]tabNavigation
	nextNavigationID uint64
	loading          bool
	tabs             TabController
	displayedTabID   browser.TabID
	tabRenderStates  map[browser.TabID]tabRenderState

	backButton        widget.Clickable
	forwardButton     widget.Clickable
	reloadButton      widget.Clickable
	goButton          widget.Clickable
	updateButton      widget.Clickable
	devToolsButton    widget.Clickable
	devToolsClose     widget.Clickable
	devToolsClear     widget.Clickable
	devToolsConsole   widget.Clickable
	devToolsInspector widget.Clickable
	devToolsNetwork   widget.Clickable
	devToolsFilter    widget.Clickable
	newTabButton      widget.Clickable
	tabRowButtons     map[browser.TabID]*widget.Clickable
	tabCloseButtons   map[browser.TabID]*widget.Clickable
	tabShortcutDown   map[key.Name]bool
	devToolsStates    map[browser.TabID]devToolsTabState
	inspectorButtons  map[browser.TabID]map[dom.NodeID]*widget.Clickable
	devToolsList      widget.List
	inspectorList     widget.List
	networkList       widget.List
	pageList          widget.List
	tabList           widget.List
	viewportClick     gesture.Click
	address           widget.Editor
	gopher            paint.ImageOp
	gopherCursor      paint.ImageOp
	gopherCursorReady bool
	pointerTag        pointerTag
	pointer           pointerState
	backIcon          *widget.Icon
	forwardIcon       *widget.Icon
	reloadIcon        *widget.Icon
	pageTitle         string
	status            string
	pageStatus        string
	statusHasError    bool
	inputEditors      map[dom.NodeID]*widget.Editor
	inputFocused      map[dom.NodeID]bool
	inputCommitted    map[dom.NodeID]string
	selectButtons     map[dom.NodeID]*widget.Clickable
	checkableButtons  map[dom.NodeID]*widget.Clickable
	formButtons       map[dom.NodeID]*widget.Clickable
	layoutBuild       func(*dom.Document, stylemodel.Map, float32, float32, float32, float32) *layoutengine.Tree
	layoutCache       documentLayoutCache
	scrollRevision    uint64
	updater           ApplicationUpdater
	updateResults     chan applicationUpdateResult
	updateContext     context.Context
	cancelUpdate      context.CancelFunc
	updateRelease     updater.Release
	updateAvailable   bool
	updating          bool
	onUpdateApplied   func()
}

type documentLayoutCache struct {
	page                  *browser.Page
	revision              uint64
	viewportWidth         float32
	viewportHeight        float32
	listFirst, listOffset int
	tree                  *layoutengine.Tree
}

type browserChromeGeometry struct {
	tabRail  image.Rectangle
	toolbar  image.Rectangle
	viewport image.Rectangle
	devTools image.Rectangle
}

type devToolsPanel string

const (
	devToolsPanelConsole   devToolsPanel = "Console"
	devToolsPanelInspector devToolsPanel = "Inspector"
	devToolsPanelNetwork   devToolsPanel = "Network"
)

type devToolsTabState struct {
	Open   bool
	Panel  devToolsPanel
	Filter devtoolsmodel.ConsoleLevel
	NodeID dom.NodeID
}

type tabRenderState struct {
	layoutCache      documentLayoutCache
	scrollRevision   uint64
	pagePosition     layout.Position
	inputEditors     map[dom.NodeID]*widget.Editor
	inputFocused     map[dom.NodeID]bool
	inputCommitted   map[dom.NodeID]string
	selectButtons    map[dom.NodeID]*widget.Clickable
	checkableButtons map[dom.NodeID]*widget.Clickable
	formButtons      map[dom.NodeID]*widget.Clickable
}

// Navigator is the browser capability used by the UI.
type Navigator interface {
	Navigate(ctx context.Context, rawURL string) (*browser.Page, error)
	Back(ctx context.Context) (*browser.Page, error)
	Forward(ctx context.Context) (*browser.Page, error)
	Reload(ctx context.Context) (*browser.Page, error)
	ReloadIgnoringCache(ctx context.Context) (*browser.Page, error)
	CanBack() bool
	CanForward() bool
	Page() *browser.Page
	DispatchClick(nodeID dom.NodeID, x, y float32) bool
	SetInputValue(nodeID dom.NodeID, value string) bool
	SetSelectValue(nodeID dom.NodeID, value string) bool
	ActivateCheckable(nodeID dom.NodeID) bool
	CommitInputValue(nodeID dom.NodeID, value string) bool
	SubmitForm(nodeID dom.NodeID) bool
	UpdateHover(nodeID dom.NodeID, x, y float32) bool
	ClearHover() bool
	UpdateFocus(nodeID dom.NodeID) bool
	MoveFormFocus(reverse bool) bool
	UpdateViewport(width, height float32) bool
}

// TabSource provides immutable browser-session state to the vertical tab rail.
type TabSource interface {
	Tabs() []browser.TabSnapshot
}

// TabController performs operations requested by the browser chrome.
type TabController interface {
	TabSource
	ActiveTab() (browser.TabSnapshot, bool)
	NewTab(initialURL *url.URL) (browser.TabSnapshot, error)
	SelectTab(id browser.TabID) (browser.TabSnapshot, error)
	SelectNext() (browser.TabSnapshot, error)
	SelectPrevious() (browser.TabSnapshot, error)
	CloseTab(id browser.TabID) (browser.TabCloseResult, error)
}

type activeBrowserSource interface {
	ActiveBrowserTarget() (browser.TabID, *browser.Browser, bool)
}

type pageInspector interface {
	InspectPage(func(*browser.Page) bool) bool
}

type tabNavigationStateSink interface {
	BeginTabNavigation(id browser.TabID) (browser.TabSnapshot, error)
	FinishTabNavigation(id browser.TabID, failed bool) (browser.TabSnapshot, error)
}

type animationFrameNavigator interface {
	RunAnimationFrame(time.Time) bool
	HasAnimationFrameCallbacks() bool
}

type historyScrollNavigator interface {
	UpdateHistoryScroll(first, offset int)
}

type navigationResult struct {
	id    uint64
	tabID browser.TabID
	page  *browser.Page
	err   error
}

// ApplicationUpdater provides the release operations used by browser chrome.
type ApplicationUpdater interface {
	Check(context.Context) (updater.Release, bool, error)
	Apply(context.Context, updater.Release) error
}

type applicationUpdateResult struct {
	release   updater.Release
	available bool
	applying  bool
	err       error
}

type tabNavigation struct {
	id     uint64
	cancel context.CancelFunc
}

// NewBrowserUI creates a browser toolbar and an empty viewport.
func NewBrowserUI(navigator Navigator, invalidate func()) *BrowserUI {
	return NewBrowserUIWithTabs(navigator, nil, invalidate)
}

// NewBrowserUIWithTabs creates browser chrome backed by a tab source.
func NewBrowserUIWithTabs(navigator Navigator, tabs TabController, invalidate func()) *BrowserUI {
	return NewBrowserUIWithTabsAndUpdater(navigator, tabs, invalidate, nil, nil)
}

// NewBrowserUIWithTabsAndUpdater creates browser chrome with automatic release updates.
func NewBrowserUIWithTabsAndUpdater(navigator Navigator, tabs TabController, invalidate func(), applicationUpdater ApplicationUpdater, onUpdateApplied func()) *BrowserUI {
	gopherImage, err := png.Decode(bytes.NewReader(gopherPNG))
	if err != nil {
		panic("decode embedded Go Gopher image: " + err.Error())
	}

	cursorImage, cursorErr := loadGopherCursor()
	updateContext, cancelUpdate := context.WithCancel(context.Background())
	ui := &BrowserUI{
		theme:            material.NewTheme(),
		navigator:        navigator,
		tabs:             tabs,
		invalidate:       invalidate,
		results:          make(chan navigationResult, browser.DefaultSessionPolicy().MaxTabs),
		navigations:      make(map[browser.TabID]tabNavigation),
		tabRenderStates:  make(map[browser.TabID]tabRenderState),
		gopher:           paint.NewImageOp(gopherImage),
		backIcon:         mustIcon(widget.NewIcon(icons.NavigationArrowBack)),
		forwardIcon:      mustIcon(widget.NewIcon(icons.NavigationArrowForward)),
		reloadIcon:       mustIcon(widget.NewIcon(icons.NavigationRefresh)),
		pageTitle:        "新しい Web を Go で開く",
		status:           "URLを入力して Gopher ボタンを押してください",
		pageStatus:       "URLを入力して Gopher ボタンを押してください",
		inputEditors:     make(map[dom.NodeID]*widget.Editor),
		inputFocused:     make(map[dom.NodeID]bool),
		inputCommitted:   make(map[dom.NodeID]string),
		selectButtons:    make(map[dom.NodeID]*widget.Clickable),
		checkableButtons: make(map[dom.NodeID]*widget.Clickable),
		formButtons:      make(map[dom.NodeID]*widget.Clickable),
		tabRowButtons:    make(map[browser.TabID]*widget.Clickable),
		tabCloseButtons:  make(map[browser.TabID]*widget.Clickable),
		tabShortcutDown:  make(map[key.Name]bool),
		devToolsStates:   make(map[browser.TabID]devToolsTabState),
		inspectorButtons: make(map[browser.TabID]map[dom.NodeID]*widget.Clickable),
		layoutBuild:      layoutengine.BuildWithScroll,
		updater:          applicationUpdater,
		updateResults:    make(chan applicationUpdateResult, 2),
		updateContext:    updateContext,
		cancelUpdate:     cancelUpdate,
		onUpdateApplied:  onUpdateApplied,
	}
	if cursorErr != nil {
		slog.Error("Gopherカーソルを初期化できませんでした", "component", "ui", "error", cursorErr)
	} else {
		ui.gopherCursor = paint.NewImageOp(cursorImage)
		ui.gopherCursorReady = true
	}
	if ui.invalidate == nil {
		ui.invalidate = func() {}
	}
	ui.address.SingleLine = true
	ui.address.Submit = true
	ui.address.SetText(defaultURL)
	ui.pageList.Axis = layout.Vertical
	ui.tabList.Axis = layout.Vertical
	ui.devToolsList.Axis = layout.Vertical
	ui.inspectorList.Axis = layout.Vertical
	ui.networkList.Axis = layout.Vertical
	ui.startUpdateCheck()
	return ui
}

// Layout draws the vertical tab rail, browser toolbar, and page viewport.
func (ui *BrowserUI) Layout(gtx layout.Context) layout.Dimensions {
	ui.handlePointerEvents(gtx)
	ui.handleKeyboardShortcuts(gtx)
	ui.handleActions(gtx)
	ui.syncActiveTabChrome()

	panelHeight := 0
	if ui.devToolsState().Open {
		panelHeight = gtx.Dp(devToolsHeight)
	}
	geometry := calculateBrowserChromeGeometryWithDevTools(gtx.Constraints.Max, gtx.Dp(tabRailWidth), gtx.Dp(toolbarHeight), panelHeight)
	layoutRegion(gtx, geometry.viewport, ui.layoutViewport)
	layoutRegion(gtx, geometry.devTools, ui.layoutDevTools)
	layoutRegion(gtx, geometry.toolbar, ui.layoutToolbar)
	layoutRegion(gtx, geometry.tabRail, ui.layoutTabRail)
	ui.layoutGopherCursor(gtx)
	ui.registerPointerTracker(gtx)
	return layout.Dimensions{Size: gtx.Constraints.Max}
}

func calculateBrowserChromeGeometry(size image.Point, railWidth, toolbarHeight int) browserChromeGeometry {
	return calculateBrowserChromeGeometryWithDevTools(size, railWidth, toolbarHeight, 0)
}

func calculateBrowserChromeGeometryWithDevTools(size image.Point, railWidth, toolbarHeight, panelHeight int) browserChromeGeometry {
	railWidth = min(max(railWidth, 0), max(size.X, 0))
	contentHeight := max(size.Y, 0)
	toolbarHeight = min(max(toolbarHeight, 0), contentHeight)
	panelHeight = min(max(panelHeight, 0), max(contentHeight-toolbarHeight, 0))
	contentLeft := railWidth
	contentRight := max(size.X, contentLeft)
	panelTop := contentHeight - panelHeight
	return browserChromeGeometry{
		tabRail:  image.Rect(0, 0, railWidth, contentHeight),
		toolbar:  image.Rect(contentLeft, 0, contentRight, toolbarHeight),
		viewport: image.Rect(contentLeft, toolbarHeight, contentRight, panelTop),
		devTools: image.Rect(contentLeft, panelTop, contentRight, contentHeight),
	}
}

func layoutRegion(gtx layout.Context, region image.Rectangle, widget layout.Widget) layout.Dimensions {
	if region.Empty() {
		return layout.Dimensions{Size: region.Size()}
	}
	defer op.Offset(region.Min).Push(gtx.Ops).Pop()
	gtx.Constraints = layout.Exact(region.Size())
	return widget(gtx)
}

func (ui *BrowserUI) layoutTabRail(gtx layout.Context) layout.Dimensions {
	width := gtx.Dp(tabRailWidth)
	if width > gtx.Constraints.Max.X {
		width = gtx.Constraints.Max.X
	}
	gtx.Constraints.Min.X = width
	gtx.Constraints.Max.X = width

	defer clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()
	paint.Fill(gtx.Ops, color.NRGBA{R: 31, G: 41, B: 55, A: 255})
	paint.FillShape(gtx.Ops,
		color.NRGBA{R: 55, G: 65, B: 81, A: 255},
		clip.Rect{Min: image.Pt(width-1, 0), Max: image.Pt(width, gtx.Constraints.Max.Y)}.Op(),
	)

	return layout.Inset{Top: unit.Dp(18), Right: unit.Dp(10), Bottom: unit.Dp(14), Left: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		tabs := ui.tabSnapshots()
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					label := material.H6(ui.theme, "Growse")
					label.Color = color.NRGBA{R: 248, G: 250, B: 252, A: 255}
					return label.Layout(gtx)
				})
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(18)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return ui.newTabButton.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X = gtx.Constraints.Max.X
					return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						label := material.Body1(ui.theme, "＋  新しいタブ")
						label.Color = color.NRGBA{R: 203, G: 213, B: 225, A: 255}
						return label.Layout(gtx)
					})
				})
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return ui.layoutTabList(gtx, tabs) }),
		)
	})
}

func (ui *BrowserUI) layoutTabList(gtx layout.Context, tabs []browser.TabSnapshot) layout.Dimensions {
	defer clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()
	return material.List(ui.theme, &ui.tabList).Layout(gtx, len(tabs), func(gtx layout.Context, index int) layout.Dimensions {
		return ui.layoutTabRow(gtx, tabs[index])
	})
}

func (ui *BrowserUI) tabSnapshots() []browser.TabSnapshot {
	if ui.tabs != nil {
		return ui.tabs.Tabs()
	}
	return []browser.TabSnapshot{{
		Active: true, Title: ui.pageTitle, Loading: ui.loading, Error: ui.statusHasError,
	}}
}

func (ui *BrowserUI) layoutTabRow(gtx layout.Context, tab browser.TabSnapshot) layout.Dimensions {
	height := gtx.Dp(unit.Dp(64))
	if height > gtx.Constraints.Max.Y {
		height = gtx.Constraints.Max.Y
	}
	gtx.Constraints.Min.X = gtx.Constraints.Max.X
	gtx.Constraints.Min.Y = height
	gtx.Constraints.Max.Y = height
	background := color.NRGBA{R: 31, G: 41, B: 55, A: 255}
	if tab.Active {
		background = color.NRGBA{R: 51, G: 65, B: 85, A: 255}
	}
	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, background, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, gtx.Dp(unit.Dp(9))).Op(gtx.Ops))
			if tab.Active {
				paint.FillShape(gtx.Ops, color.NRGBA{R: 56, G: 189, B: 248, A: 255}, clip.Rect{Max: image.Pt(gtx.Dp(unit.Dp(3)), gtx.Constraints.Min.Y)}.Op())
			}
			return layout.Dimensions{Size: gtx.Constraints.Min}
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return ui.tabRowButton(tab.ID).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min = gtx.Constraints.Max
						return layout.Inset{Top: unit.Dp(9), Bottom: unit.Dp(7), Left: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									label := material.Body1(ui.theme, tabDisplayTitle(tab))
									label.Color = color.NRGBA{R: 248, G: 250, B: 252, A: 255}
									label.MaxLines = 1
									return label.Layout(gtx)
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									label := material.Caption(ui.theme, tabStateLabel(tab))
									label.Color = tabStateColor(tab)
									label.MaxLines = 1
									return label.Layout(gtx)
								}),
							)
						})
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					width := gtx.Dp(unit.Dp(36))
					gtx.Constraints = layout.Exact(image.Pt(width, height))
					return ui.tabCloseButton(tab.ID).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							label := material.Body1(ui.theme, "×")
							label.Color = color.NRGBA{R: 203, G: 213, B: 225, A: 255}
							return label.Layout(gtx)
						})
					})
				}),
			)
		}),
	)
}

func (ui *BrowserUI) tabRowButton(id browser.TabID) *widget.Clickable {
	button := ui.tabRowButtons[id]
	if button == nil {
		button = new(widget.Clickable)
		ui.tabRowButtons[id] = button
	}
	return button
}

func (ui *BrowserUI) tabCloseButton(id browser.TabID) *widget.Clickable {
	button := ui.tabCloseButtons[id]
	if button == nil {
		button = new(widget.Clickable)
		ui.tabCloseButtons[id] = button
	}
	return button
}

func tabDisplayTitle(tab browser.TabSnapshot) string {
	if title := strings.TrimSpace(tab.Title); title != "" {
		return title
	}
	if parsed, err := url.Parse(tab.URL); err == nil && parsed.Hostname() != "" {
		return parsed.Hostname()
	}
	return "新しいタブ"
}

func tabStateLabel(tab browser.TabSnapshot) string {
	states := make([]string, 0, 3)
	if tab.Active {
		states = append(states, "選択中")
	}
	if tab.Loading {
		states = append(states, "読込中")
	}
	if tab.Error {
		states = append(states, "エラー")
	}
	if tab.PendingUpdate {
		states = append(states, "更新あり")
	}
	if len(states) == 0 {
		return "待機中"
	}
	return strings.Join(states, " · ")
}

func tabStateColor(tab browser.TabSnapshot) color.NRGBA {
	switch {
	case tab.Error:
		return color.NRGBA{R: 253, G: 164, B: 175, A: 255}
	case tab.Loading:
		return color.NRGBA{R: 125, G: 211, B: 252, A: 255}
	case tab.PendingUpdate:
		return color.NRGBA{R: 253, G: 224, B: 71, A: 255}
	default:
		return color.NRGBA{R: 148, G: 163, B: 184, A: 255}
	}
}

func (ui *BrowserUI) handleKeyboardShortcuts(gtx layout.Context) {
	ui.handleTabKeyboardShortcuts(gtx)
	for {
		event, ok := gtx.Event(key.Filter{Name: key.NameF12})
		if !ok {
			break
		}
		keyEvent, ok := event.(key.Event)
		if ok && keyEvent.State == key.Press {
			state := ui.devToolsState()
			state.Open = !state.Open
			ui.setDevToolsState(state)
		}
	}
	for {
		event, ok := gtx.Event(key.Filter{Name: "R", Required: key.ModShortcut, Optional: key.ModShift})
		if !ok {
			return
		}
		keyEvent, ok := event.(key.Event)
		tabID, navigator := ui.activeNavigationTarget()
		if !ok || keyEvent.State != key.Press || navigator == nil || navigator.Page() == nil {
			continue
		}
		if keyEvent.Modifiers.Contain(key.ModShift) {
			ui.startPageLoad(tabID, navigator, "キャッシュを無視して再読み込み中", navigator.ReloadIgnoringCache)
			continue
		}
		ui.startPageLoad(tabID, navigator, "ページを再読み込み中", navigator.Reload)
	}
}

func (ui *BrowserUI) handleTabKeyboardShortcuts(gtx layout.Context) {
	if ui.tabs == nil {
		return
	}
	for _, name := range []key.Name{"T", "W"} {
		for {
			event, ok := gtx.Event(key.Filter{Name: name, Required: key.ModShortcut})
			if !ok {
				break
			}
			keyEvent, ok := event.(key.Event)
			if !ok {
				continue
			}
			if keyEvent.State == key.Release {
				delete(ui.tabShortcutDown, name)
				continue
			}
			if keyEvent.State != key.Press || ui.tabShortcutDown[name] {
				continue
			}
			ui.tabShortcutDown[name] = true
			switch name {
			case "T":
				ui.createTab(gtx)
			case "W":
				if active, ok := ui.tabs.ActiveTab(); ok {
					ui.closeTab(active.ID)
				}
			}
		}
	}
	for {
		event, ok := gtx.Event(key.Filter{Name: key.NameTab, Required: key.ModShortcut, Optional: key.ModShift})
		if !ok {
			break
		}
		keyEvent, ok := event.(key.Event)
		if !ok || keyEvent.State != key.Press {
			continue
		}
		var err error
		if keyEvent.Modifiers.Contain(key.ModShift) {
			_, err = ui.tabs.SelectPrevious()
		} else {
			_, err = ui.tabs.SelectNext()
		}
		if err != nil {
			ui.reportTabOperationError("Tabを切り替えられません", err)
		}
	}
}

func (ui *BrowserUI) handleActions(gtx layout.Context) {
	ui.consumeNavigationResult()
	ui.consumeUpdateResults()
	ui.handleTabActions(gtx)

	for {
		event, ok := ui.address.Update(gtx)
		if !ok {
			break
		}
		if submitted, ok := event.(widget.SubmitEvent); ok {
			ui.startNavigation(submitted.Text)
		}
	}
	for ui.goButton.Clicked(gtx) {
		ui.startNavigation(ui.address.Text())
	}
	for ui.backButton.Clicked(gtx) {
		if tabID, navigator := ui.activeNavigationTarget(); navigator != nil && navigator.CanBack() {
			ui.startPageLoad(tabID, navigator, "前のページを読み込み中", navigator.Back)
		}
	}
	for ui.forwardButton.Clicked(gtx) {
		if tabID, navigator := ui.activeNavigationTarget(); navigator != nil && navigator.CanForward() {
			ui.startPageLoad(tabID, navigator, "次のページを読み込み中", navigator.Forward)
		}
	}
	for ui.reloadButton.Clicked(gtx) {
		if tabID, navigator := ui.activeNavigationTarget(); navigator != nil && navigator.Page() != nil {
			ui.startPageLoad(tabID, navigator, "ページを再読み込み中", navigator.Reload)
		}
	}
	for ui.updateButton.Clicked(gtx) {
		ui.startUpdate()
	}
	for ui.devToolsButton.Clicked(gtx) {
		state := ui.devToolsState()
		state.Open = !state.Open
		ui.setDevToolsState(state)
	}
	for ui.devToolsClose.Clicked(gtx) {
		state := ui.devToolsState()
		state.Open = false
		ui.setDevToolsState(state)
	}
	for ui.devToolsConsole.Clicked(gtx) {
		state := ui.devToolsState()
		state.Panel = devToolsPanelConsole
		ui.setDevToolsState(state)
	}
	for ui.devToolsInspector.Clicked(gtx) {
		state := ui.devToolsState()
		state.Panel = devToolsPanelInspector
		ui.setDevToolsState(state)
	}
	for ui.devToolsNetwork.Clicked(gtx) {
		state := ui.devToolsState()
		state.Panel = devToolsPanelNetwork
		ui.setDevToolsState(state)
	}
	for ui.devToolsFilter.Clicked(gtx) {
		state := ui.devToolsState()
		state.Filter = nextConsoleFilter(state.Filter)
		ui.setDevToolsState(state)
	}
	for ui.devToolsClear.Clicked(gtx) {
		if navigator := ui.activeNavigator(); navigator != nil {
			if page := navigator.Page(); page != nil && page.DevTools != nil {
				if ui.devToolsState().Panel == devToolsPanelNetwork {
					page.DevTools.ClearNetwork()
				} else {
					page.DevTools.ClearConsole()
				}
			}
		}
	}
	activeID, _ := ui.activeNavigationTarget()
	for nodeID, button := range ui.inspectorButtons[activeID] {
		for button.Clicked(gtx) {
			state := ui.devToolsState()
			state.NodeID = nodeID
			ui.setDevToolsState(state)
		}
	}
}

func (ui *BrowserUI) startUpdateCheck() {
	if ui.updater == nil {
		return
	}
	go func() {
		release, available, err := ui.updater.Check(ui.updateContext)
		ui.updateResults <- applicationUpdateResult{release: release, available: available, err: err}
		ui.invalidate()
	}()
}

func (ui *BrowserUI) startUpdate() {
	if ui.updater == nil || !ui.updateAvailable || ui.updating {
		return
	}
	ui.updating = true
	ui.statusHasError = false
	ui.status = "Growse " + ui.updateRelease.Version + " をダウンロードして検証中"
	release := ui.updateRelease
	go func() {
		err := ui.updater.Apply(ui.updateContext, release)
		ui.updateResults <- applicationUpdateResult{release: release, applying: true, err: err}
		ui.invalidate()
	}()
}

func (ui *BrowserUI) consumeUpdateResults() {
	for {
		select {
		case result := <-ui.updateResults:
			if !result.applying {
				if result.err != nil {
					slog.Warn("Growseの最新版を確認できませんでした", "component", "updater", "error", result.err)
					continue
				}
				ui.updateRelease = result.release
				ui.updateAvailable = result.available
				continue
			}
			ui.updating = false
			if result.err != nil {
				ui.statusHasError = true
				ui.status = "Growseを更新できませんでした: " + result.err.Error()
				continue
			}
			ui.updateAvailable = false
			ui.statusHasError = false
			ui.status = "Growse " + result.release.Version + " へ更新しました。再起動します"
			if ui.onUpdateApplied != nil {
				ui.onUpdateApplied()
			}
		default:
			return
		}
	}
}

func (ui *BrowserUI) handleTabActions(gtx layout.Context) {
	if ui.tabs == nil {
		return
	}
	for ui.newTabButton.Clicked(gtx) {
		ui.createTab(gtx)
	}
	closed := make(map[browser.TabID]bool)
	for id, button := range ui.tabCloseButtons {
		for button.Clicked(gtx) {
			if !ui.closeTab(id) {
				continue
			}
			closed[id] = true
		}
	}
	for id, button := range ui.tabRowButtons {
		for button.Clicked(gtx) {
			if closed[id] {
				continue
			}
			if _, err := ui.tabs.SelectTab(id); err != nil {
				ui.reportTabOperationError("Tabを選択できません", err)
			}
		}
	}
}

func (ui *BrowserUI) createTab(gtx layout.Context) {
	tab, err := ui.tabs.NewTab(nil)
	if err != nil {
		ui.reportTabOperationError("新しいTabを作成できません", err)
		return
	}
	if _, err := ui.tabs.SelectTab(tab.ID); err != nil {
		ui.reportTabOperationError("新しいTabを選択できません", err)
		return
	}
	gtx.Execute(key.FocusCmd{Tag: &ui.address})
}

func (ui *BrowserUI) closeTab(id browser.TabID) bool {
	ui.cancelTabNavigation(id)
	if _, err := ui.tabs.CloseTab(id); err != nil {
		ui.reportTabOperationError("Tabを終了できません", err)
		return false
	}
	delete(ui.tabRenderStates, id)
	delete(ui.devToolsStates, id)
	delete(ui.inspectorButtons, id)
	delete(ui.tabRowButtons, id)
	delete(ui.tabCloseButtons, id)
	if ui.displayedTabID == id {
		ui.displayedTabID = 0
	}
	return true
}

func (ui *BrowserUI) reportTabOperationError(message string, err error) {
	ui.status = message + ": " + err.Error()
	ui.statusHasError = true
}

func (ui *BrowserUI) startNavigation(rawURL string) {
	tabID, navigator := ui.activeNavigationTarget()
	if navigator == nil {
		ui.status = "Navigationを利用できません"
		ui.statusHasError = true
		return
	}
	ui.startPageLoad(tabID, navigator, navigationLoadingStatus(rawURL), func(ctx context.Context) (*browser.Page, error) {
		return navigator.Navigate(ctx, rawURL)
	})
}

func (ui *BrowserUI) startPageLoad(tabID browser.TabID, navigator Navigator, status string, load func(context.Context) (*browser.Page, error)) {
	ui.persistHistoryScroll()
	if navigator != nil {
		navigator.ClearHover()
	}
	ui.cancelTabNavigation(tabID)

	ctx, cancel := context.WithCancel(context.Background())
	ui.nextNavigationID++
	navigationID := ui.nextNavigationID
	ui.navigations[tabID] = tabNavigation{id: navigationID, cancel: cancel}
	if sink, ok := ui.tabs.(tabNavigationStateSink); ok && tabID != 0 {
		if _, err := sink.BeginTabNavigation(tabID); err != nil {
			cancel()
			delete(ui.navigations, tabID)
			ui.reportTabOperationError("TabのNavigation状態を更新できません", err)
			return
		}
	}
	ui.loading = true
	ui.statusHasError = false
	ui.status = status

	go func() {
		page, err := load(ctx)
		ui.results <- navigationResult{id: navigationID, tabID: tabID, page: page, err: err}
		ui.invalidate()
	}()
}

func (ui *BrowserUI) cancelTabNavigation(tabID browser.TabID) bool {
	navigation, ok := ui.navigations[tabID]
	if !ok {
		return false
	}
	navigation.cancel()
	delete(ui.navigations, tabID)
	return true
}

func (ui *BrowserUI) activeNavigator() Navigator {
	_, navigator := ui.activeNavigationTarget()
	return navigator
}

func (ui *BrowserUI) activeNavigationTarget() (browser.TabID, Navigator) {
	if tabs, ok := ui.tabs.(activeBrowserSource); ok {
		if tabID, navigator, ok := tabs.ActiveBrowserTarget(); ok {
			return tabID, navigator
		}
	}
	return 0, ui.navigator
}

func (ui *BrowserUI) syncActiveTabChrome() {
	if ui.tabs == nil {
		return
	}
	active, ok := ui.tabs.ActiveTab()
	if !ok || active.ID == ui.displayedTabID {
		return
	}
	tabID, navigator := ui.activeNavigationTarget()
	if navigator == nil || tabID != active.ID {
		return
	}
	if ui.displayedTabID != 0 {
		ui.tabRenderStates[ui.displayedTabID] = tabRenderState{
			layoutCache: ui.layoutCache, scrollRevision: ui.scrollRevision, pagePosition: ui.pageList.Position,
			inputEditors: ui.inputEditors, inputFocused: ui.inputFocused, inputCommitted: ui.inputCommitted,
			selectButtons: ui.selectButtons, checkableButtons: ui.checkableButtons, formButtons: ui.formButtons,
		}
	}
	ui.displayedTabID = active.ID
	ui.navigator = navigator
	ui.loading = active.Loading
	ui.statusHasError = active.Error
	if state, ok := ui.tabRenderStates[active.ID]; ok {
		ui.layoutCache = state.layoutCache
		ui.scrollRevision = state.scrollRevision
		ui.pageList.Position = state.pagePosition
		ui.inputEditors = state.inputEditors
		ui.inputFocused = state.inputFocused
		ui.inputCommitted = state.inputCommitted
		ui.selectButtons = state.selectButtons
		ui.checkableButtons = state.checkableButtons
		ui.formButtons = state.formButtons
	} else {
		ui.layoutCache = documentLayoutCache{}
		ui.scrollRevision = 0
		ui.inputEditors = make(map[dom.NodeID]*widget.Editor)
		ui.inputFocused = make(map[dom.NodeID]bool)
		ui.inputCommitted = make(map[dom.NodeID]string)
		ui.selectButtons = make(map[dom.NodeID]*widget.Clickable)
		ui.checkableButtons = make(map[dom.NodeID]*widget.Clickable)
		ui.formButtons = make(map[dom.NodeID]*widget.Clickable)
	}
	if page := navigator.Page(); page != nil {
		if page.URL != nil {
			ui.address.SetText(page.URL.String())
		}
		if _, restored := ui.tabRenderStates[active.ID]; !restored {
			ui.pageList.Position = layout.Position{First: page.ScrollFirst, Offset: page.ScrollOffset}
		}
		if _, restored := ui.tabRenderStates[active.ID]; !restored {
			ui.scrollRevision = page.ScrollRevision
		}
		ui.pageTitle = active.Title
		if ui.pageTitle == "" && page.Document != nil {
			ui.pageTitle = page.Document.Title()
		}
		if ui.pageTitle == "" && page.URL != nil {
			ui.pageTitle = page.URL.Hostname()
		}
	} else {
		ui.address.SetText(active.URL)
		ui.pageList.Position = layout.Position{}
		ui.scrollRevision = 0
		ui.pageTitle = tabDisplayTitle(active)
	}
	if active.Status != "" {
		ui.status = active.Status
		ui.pageStatus = active.Status
	} else if navigator.Page() == nil {
		ui.status = "URLを入力して Gopher ボタンを押してください"
		ui.pageStatus = ui.status
	}
}

func (ui *BrowserUI) consumeNavigationResult() {
	for {
		select {
		case result := <-ui.results:
			navigation, ok := ui.navigations[result.tabID]
			if !ok || result.id != navigation.id {
				continue
			}
			delete(ui.navigations, result.tabID)
			if sink, ok := ui.tabs.(tabNavigationStateSink); ok && result.tabID != 0 {
				if _, err := sink.FinishTabNavigation(result.tabID, result.err != nil); err != nil {
					ui.reportTabOperationError("TabのNavigation結果を更新できません", err)
				}
			}
			if result.tabID != 0 && !ui.tabIsActive(result.tabID) {
				ui.loading = false
				continue
			}
			ui.loading = false
			ui.inputEditors = make(map[dom.NodeID]*widget.Editor)
			ui.inputFocused = make(map[dom.NodeID]bool)
			ui.inputCommitted = make(map[dom.NodeID]string)
			ui.selectButtons = make(map[dom.NodeID]*widget.Clickable)
			ui.checkableButtons = make(map[dom.NodeID]*widget.Clickable)
			ui.formButtons = make(map[dom.NodeID]*widget.Clickable)
			if result.err != nil {
				ui.status = "読み込みエラー: " + result.err.Error()
				ui.pageStatus = ui.status
				ui.statusHasError = true
				return
			}
			if result.page == nil || result.page.URL == nil {
				ui.status = "読み込みエラー: ページ情報がありません"
				ui.pageStatus = ui.status
				ui.statusHasError = true
				return
			}

			ui.address.SetText(result.page.URL.String())
			ui.pageList.Position = layout.Position{First: result.page.ScrollFirst, Offset: result.page.ScrollOffset}
			ui.scrollRevision = result.page.ScrollRevision
			ui.pageTitle = result.page.URL.Host
			domSummary := "DOM未生成"
			if result.page.Document != nil {
				domSummary = fmt.Sprintf("DOM %dノード / %d要素", result.page.Document.NodeCount(), result.page.Document.ElementCount())
				if title := result.page.Document.Title(); title != "" {
					ui.pageTitle = title
				}
			}
			ui.pageStatus = fmt.Sprintf("取得完了 · %s · HTTP %d · %d bytes", domSummary, result.page.StatusCode, len(result.page.Source))
			if len(result.page.ScriptErrors) > 0 {
				ui.pageStatus += fmt.Sprintf(" · Go script error %d件", len(result.page.ScriptErrors))
				ui.statusHasError = true
			}
			if result.page.RuntimeStarted {
				ui.pageStatus += " · Go Runtime起動済み"
			} else if result.page.RuntimeError != "" {
				ui.pageStatus += " · Go Runtimeエラー: " + result.page.RuntimeError
				ui.statusHasError = true
			}
			ui.status = ui.pageStatus
		default:
			return
		}
	}
}

func (ui *BrowserUI) tabIsActive(id browser.TabID) bool {
	if ui.tabs == nil {
		return id == 0
	}
	active, ok := ui.tabs.ActiveTab()
	return ok && active.ID == id
}

// Close cancels an in-flight navigation when the window closes.
func (ui *BrowserUI) Close() {
	ui.cancelUpdate()
	if ui.navigator != nil {
		ui.navigator.ClearHover()
	}
	for tabID := range ui.navigations {
		ui.cancelTabNavigation(tabID)
	}
}

func (ui *BrowserUI) layoutToolbar(gtx layout.Context) layout.Dimensions {
	height := gtx.Dp(toolbarHeight)
	if height > gtx.Constraints.Max.Y {
		height = gtx.Constraints.Max.Y
	}
	gtx.Constraints.Min.Y = height
	gtx.Constraints.Max.Y = height

	defer clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()
	paint.Fill(gtx.Ops, color.NRGBA{R: 244, G: 247, B: 251, A: 255})
	paint.FillShape(gtx.Ops,
		color.NRGBA{R: 211, G: 218, B: 228, A: 255},
		clip.Rect{Min: image.Pt(0, height-1), Max: image.Pt(gtx.Constraints.Max.X, height)}.Op(),
	)

	return layout.Inset{Top: unit.Dp(8), Right: unit.Dp(8), Bottom: unit.Dp(6), Left: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		navigator := ui.activeNavigator()
		canBack := navigator != nil && navigator.CanBack()
		canForward := navigator != nil && navigator.CanForward()
		canReload := navigator != nil && navigator.Page() != nil
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return ui.layoutToolbarButton(gtx, &ui.backButton, ui.backIcon, "戻る", canBack)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return ui.layoutToolbarButton(gtx, &ui.forwardButton, ui.forwardIcon, "次へ", canForward)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return ui.layoutToolbarButton(gtx, &ui.reloadButton, ui.reloadIcon, "再読込", canReload)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
					layout.Rigid(ui.layoutUpdateButton),
					layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
					layout.Rigid(ui.layoutDevToolsButton),
					layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
					layout.Flexed(1, ui.layoutAddressBar),
					layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
					layout.Rigid(ui.layoutGopherButton),
				)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(2)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				label := material.Caption(ui.theme, ui.status)
				label.Color = color.NRGBA{R: 72, G: 84, B: 102, A: 255}
				label.MaxLines = 1
				return label.Layout(gtx)
			}),
		)
	})
}

func (ui *BrowserUI) layoutDevToolsButton(gtx layout.Context) layout.Dimensions {
	gtx.Constraints.Min.Y = gtx.Dp(controlHeight)
	gtx.Constraints.Max.Y = gtx.Dp(controlHeight)
	button := material.Button(ui.theme, &ui.devToolsButton, "DevTools")
	button.Background = color.NRGBA{R: 51, G: 65, B: 85, A: 255}
	button.Color = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	button.CornerRadius = unit.Dp(10)
	button.Inset = layout.Inset{Top: unit.Dp(8), Right: unit.Dp(12), Bottom: unit.Dp(8), Left: unit.Dp(12)}
	return button.Layout(gtx)
}

func (ui *BrowserUI) devToolsState() devToolsTabState {
	id, _ := ui.activeNavigationTarget()
	state := ui.devToolsStates[id]
	if state.Panel == "" {
		state.Panel = devToolsPanelConsole
	}
	return state
}

func (ui *BrowserUI) setDevToolsState(state devToolsTabState) {
	id, _ := ui.activeNavigationTarget()
	ui.devToolsStates[id] = state
}

func nextConsoleFilter(current devtoolsmodel.ConsoleLevel) devtoolsmodel.ConsoleLevel {
	switch current {
	case "":
		return devtoolsmodel.ConsoleLog
	case devtoolsmodel.ConsoleLog:
		return devtoolsmodel.ConsoleInfo
	case devtoolsmodel.ConsoleInfo:
		return devtoolsmodel.ConsoleWarn
	case devtoolsmodel.ConsoleWarn:
		return devtoolsmodel.ConsoleError
	default:
		return ""
	}
}

func (ui *BrowserUI) layoutDevTools(gtx layout.Context) layout.Dimensions {
	paint.Fill(gtx.Ops, color.NRGBA{R: 15, G: 23, B: 42, A: 255})
	state := ui.devToolsState()
	return layout.Inset{Top: unit.Dp(8), Right: unit.Dp(10), Bottom: unit.Dp(8), Left: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(ui.devToolsTab(&ui.devToolsConsole, devToolsPanelConsole, state.Panel == devToolsPanelConsole)),
					layout.Rigid(ui.devToolsTab(&ui.devToolsInspector, devToolsPanelInspector, state.Panel == devToolsPanelInspector)),
					layout.Rigid(ui.devToolsTab(&ui.devToolsNetwork, devToolsPanelNetwork, state.Panel == devToolsPanelNetwork)),
					layout.Flexed(1, layout.Spacer{}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if state.Panel != devToolsPanelConsole {
							return layout.Dimensions{}
						}
						return ui.devToolsAction(&ui.devToolsFilter, consoleFilterLabel(state.Filter))(gtx)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
					layout.Rigid(ui.devToolsAction(&ui.devToolsClear, "Clear")),
					layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
					layout.Rigid(ui.devToolsAction(&ui.devToolsClose, "Close")),
				)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				switch state.Panel {
				case devToolsPanelInspector:
					return ui.layoutDevToolsInspector(gtx, state)
				case devToolsPanelNetwork:
					return ui.layoutDevToolsNetwork(gtx)
				default:
					return ui.layoutDevToolsConsole(gtx, state.Filter)
				}
			}),
		)
	})
}

func (ui *BrowserUI) layoutDevToolsNetwork(gtx layout.Context) layout.Dimensions {
	var records []devtoolsmodel.NetworkRecord
	if navigator := ui.activeNavigator(); navigator != nil {
		if page := navigator.Page(); page != nil && page.DevTools != nil {
			records = page.DevTools.Network()
		}
	}
	if len(records) == 0 {
		return ui.layoutDevToolsPlaceholder(gtx, "Network has no requests for this page")
	}
	return material.List(ui.theme, &ui.networkList).Layout(gtx, len(records), func(gtx layout.Context, index int) layout.Dimensions {
		record := records[index]
		status := fmt.Sprint(record.StatusCode)
		if record.ErrorCategory != "" {
			status = "error:" + record.ErrorCategory
		}
		flags := record.CacheStatus
		if record.Redirected {
			if flags != "" {
				flags += ","
			}
			flags += "redirect"
		}
		if flags == "" {
			flags = "-"
		}
		label := material.Body2(ui.theme, networkRecordLabel(record, status, flags))
		label.Color = color.NRGBA{R: 203, G: 213, B: 225, A: 255}
		if record.ErrorCategory != "" || record.StatusCode >= 400 {
			label.Color = color.NRGBA{R: 253, G: 164, B: 175, A: 255}
		}
		return layout.Inset{Top: unit.Dp(3), Bottom: unit.Dp(3), Left: unit.Dp(6)}.Layout(gtx, label.Layout)
	})
}

func networkRecordLabel(record devtoolsmodel.NetworkRecord, status, flags string) string {
	return fmt.Sprintf("%04d  %-10s %-5s %-15s %8s %7d B  %-18s  %s",
		record.Sequence, record.Kind, record.Method, status, record.Duration.Round(time.Microsecond), record.ResponseBytes, flags, record.URL)
}

func (ui *BrowserUI) layoutDevToolsInspector(gtx layout.Context, state devToolsTabState) layout.Dimensions {
	navigator := ui.activeNavigator()
	if navigator == nil || navigator.Page() == nil || navigator.Page().Document == nil {
		return ui.layoutDevToolsPlaceholder(gtx, "Inspector has no active document")
	}
	page := navigator.Page()
	var tree *layoutengine.Tree
	if ui.layoutCache.page == page {
		tree = ui.layoutCache.tree
	}
	var snapshot devtoolsmodel.InspectorSnapshot
	inspect := func(current *browser.Page) bool {
		if current != page {
			return false
		}
		snapshot = devtoolsmodel.SnapshotInspector(current.Document, current.ComputedStyles, tree, state.NodeID)
		return true
	}
	if inspector, ok := navigator.(pageInspector); ok {
		if !inspector.InspectPage(inspect) {
			return ui.layoutDevToolsPlaceholder(gtx, "Inspector snapshot is temporarily unavailable")
		}
	} else {
		inspect(page)
	}
	if state.NodeID != 0 && snapshot.Selected == 0 {
		state.NodeID = 0
		ui.setDevToolsState(state)
	}
	tabID, _ := ui.activeNavigationTarget()
	buttons := ui.inspectorButtons[tabID]
	if buttons == nil {
		buttons = make(map[dom.NodeID]*widget.Clickable)
		ui.inspectorButtons[tabID] = buttons
	}
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		layout.Flexed(0.58, func(gtx layout.Context) layout.Dimensions {
			return material.List(ui.theme, &ui.inspectorList).Layout(gtx, len(snapshot.Nodes), func(gtx layout.Context, index int) layout.Dimensions {
				node := snapshot.Nodes[index]
				button := buttons[node.ID]
				if button == nil {
					button = &widget.Clickable{}
					buttons[node.ID] = button
				}
				return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					label := material.Body2(ui.theme, inspectorNodeLabel(node))
					label.Color = color.NRGBA{R: 203, G: 213, B: 225, A: 255}
					if snapshot.Selected == node.ID {
						paint.Fill(gtx.Ops, color.NRGBA{R: 12, G: 74, B: 110, A: 255})
						label.Color = color.NRGBA{R: 240, G: 249, B: 255, A: 255}
					}
					return layout.Inset{Top: unit.Dp(3), Right: unit.Dp(4), Bottom: unit.Dp(3), Left: unit.Dp(6)}.Layout(gtx, label.Layout)
				})
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			width := gtx.Dp(unit.Dp(1))
			gtx.Constraints = layout.Exact(image.Pt(width, gtx.Constraints.Max.Y))
			paint.Fill(gtx.Ops, color.NRGBA{R: 51, G: 65, B: 85, A: 255})
			return layout.Dimensions{Size: gtx.Constraints.Max}
		}),
		layout.Flexed(0.42, func(gtx layout.Context) layout.Dimensions {
			return ui.layoutInspectorDetails(gtx, snapshot)
		}),
	)
}

func inspectorNodeLabel(node devtoolsmodel.DOMNode) string {
	indent := strings.Repeat("  ", node.Depth)
	if node.Kind == "text" {
		text := strings.Join(strings.Fields(node.Text), " ")
		runes := []rune(text)
		if len(runes) > 80 {
			text = string(runes[:80]) + "…"
		}
		return fmt.Sprintf("%s#text %q", indent, text)
	}
	label := indent + node.Name
	for _, attribute := range node.Attributes {
		if attribute.Name == "id" {
			label += "#" + attribute.Value
		}
		if attribute.Name == "class" {
			label += "." + strings.ReplaceAll(attribute.Value, " ", ".")
		}
	}
	return label
}

func (ui *BrowserUI) layoutInspectorDetails(gtx layout.Context, snapshot devtoolsmodel.InspectorSnapshot) layout.Dimensions {
	if snapshot.SelectedNode == nil {
		return ui.layoutDevToolsPlaceholder(gtx, "Select a DOM node to inspect attributes, styles, and layout")
	}
	lines := []string{fmt.Sprintf("%s  node=%d", snapshot.SelectedNode.Name, snapshot.SelectedNode.ID), "", "Attributes"}
	if len(snapshot.SelectedNode.Attributes) == 0 {
		lines = append(lines, "  (none)")
	}
	for _, attribute := range snapshot.SelectedNode.Attributes {
		lines = append(lines, fmt.Sprintf("  %s=%q", attribute.Name, attribute.Value))
	}
	lines = append(lines, "", "Computed Style")
	if len(snapshot.Styles) == 0 {
		lines = append(lines, "  (not available)")
	}
	for _, property := range snapshot.Styles {
		lines = append(lines, fmt.Sprintf("  %-18s %s", property.Name, property.Value))
	}
	lines = append(lines, "", "Layout Box")
	if snapshot.Layout == nil {
		lines = append(lines, "  (not rendered)")
	} else {
		lines = append(lines,
			fmt.Sprintf("  x %.2f  y %.2f", snapshot.Layout.X, snapshot.Layout.Y),
			fmt.Sprintf("  width %.2f  height %.2f", snapshot.Layout.Width, snapshot.Layout.Height),
		)
	}
	if snapshot.Truncated {
		lines = append(lines, "", "Snapshot truncated at safety limit")
	}
	label := material.Body2(ui.theme, strings.Join(lines, "\n"))
	label.Color = color.NRGBA{R: 203, G: 213, B: 225, A: 255}
	return layout.Inset{Top: unit.Dp(6), Right: unit.Dp(6), Left: unit.Dp(10)}.Layout(gtx, label.Layout)
}

func (ui *BrowserUI) devToolsTab(button *widget.Clickable, panel devToolsPanel, active bool) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		style := material.Button(ui.theme, button, string(panel))
		style.Background = color.NRGBA{R: 30, G: 41, B: 59, A: 255}
		if active {
			style.Background = color.NRGBA{R: 3, G: 105, B: 161, A: 255}
		}
		style.Inset = layout.Inset{Top: unit.Dp(6), Right: unit.Dp(12), Bottom: unit.Dp(6), Left: unit.Dp(12)}
		return style.Layout(gtx)
	}
}

func (ui *BrowserUI) devToolsAction(button *widget.Clickable, label string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		style := material.Button(ui.theme, button, label)
		style.Background = color.NRGBA{R: 51, G: 65, B: 85, A: 255}
		style.Inset = layout.Inset{Top: unit.Dp(5), Right: unit.Dp(9), Bottom: unit.Dp(5), Left: unit.Dp(9)}
		return style.Layout(gtx)
	}
}

func consoleFilterLabel(filter devtoolsmodel.ConsoleLevel) string {
	if filter == "" {
		return "Level: all"
	}
	return "Level: " + string(filter)
}

func (ui *BrowserUI) layoutDevToolsConsole(gtx layout.Context, filter devtoolsmodel.ConsoleLevel) layout.Dimensions {
	var records []devtoolsmodel.ConsoleRecord
	if navigator := ui.activeNavigator(); navigator != nil {
		if page := navigator.Page(); page != nil && page.DevTools != nil {
			for _, record := range page.DevTools.Console() {
				if filter == "" || record.Level == filter {
					records = append(records, record)
				}
			}
		}
	}
	if len(records) == 0 {
		return ui.layoutDevToolsPlaceholder(gtx, "Console has no matching messages")
	}
	return material.List(ui.theme, &ui.devToolsList).Layout(gtx, len(records), func(gtx layout.Context, index int) layout.Dimensions {
		record := records[index]
		label := material.Body2(ui.theme, fmt.Sprintf("%04d  %-5s  %-8s  %s", record.Sequence, record.Level, record.Source, record.Message))
		label.Color = devToolsLevelColor(record.Level)
		return layout.Inset{Top: unit.Dp(3), Bottom: unit.Dp(3), Left: unit.Dp(6)}.Layout(gtx, label.Layout)
	})
}

func (ui *BrowserUI) layoutDevToolsPlaceholder(gtx layout.Context, message string) layout.Dimensions {
	label := material.Body2(ui.theme, message)
	label.Color = color.NRGBA{R: 148, G: 163, B: 184, A: 255}
	return layout.Inset{Top: unit.Dp(12), Left: unit.Dp(8)}.Layout(gtx, label.Layout)
}

func devToolsLevelColor(level devtoolsmodel.ConsoleLevel) color.NRGBA {
	switch level {
	case devtoolsmodel.ConsoleWarn:
		return color.NRGBA{R: 253, G: 224, B: 71, A: 255}
	case devtoolsmodel.ConsoleError:
		return color.NRGBA{R: 253, G: 164, B: 175, A: 255}
	case devtoolsmodel.ConsoleInfo:
		return color.NRGBA{R: 125, G: 211, B: 252, A: 255}
	default:
		return color.NRGBA{R: 226, G: 232, B: 240, A: 255}
	}
}

func (ui *BrowserUI) layoutUpdateButton(gtx layout.Context) layout.Dimensions {
	if !ui.updateAvailable && !ui.updating {
		return layout.Dimensions{}
	}
	label := "更新 " + ui.updateRelease.Version
	if ui.updating {
		label = "更新中…"
		gtx = gtx.Disabled()
	}
	gtx.Constraints.Min.Y = gtx.Dp(controlHeight)
	gtx.Constraints.Max.Y = gtx.Dp(controlHeight)
	button := material.Button(ui.theme, &ui.updateButton, label)
	button.Background = color.NRGBA{R: 37, G: 99, B: 235, A: 255}
	button.Color = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	button.CornerRadius = unit.Dp(10)
	button.Inset = layout.Inset{Top: unit.Dp(8), Right: unit.Dp(12), Bottom: unit.Dp(8), Left: unit.Dp(12)}
	return button.Layout(gtx)
}

func (ui *BrowserUI) layoutToolbarButton(gtx layout.Context, button *widget.Clickable, icon *widget.Icon, description string, enabled bool) layout.Dimensions {
	size := gtx.Dp(controlHeight)
	gtx.Constraints = layout.Exact(image.Pt(size, size))

	style := material.IconButton(ui.theme, button, icon, description)
	style.Background = color.NRGBA{R: 228, G: 234, B: 242, A: 255}
	style.Color = color.NRGBA{R: 52, G: 64, B: 84, A: 255}
	if !enabled {
		gtx = gtx.Disabled()
		style.Background = color.NRGBA{R: 238, G: 242, B: 247, A: 255}
		style.Color = color.NRGBA{R: 160, G: 169, B: 182, A: 255}
	}
	style.Size = unit.Dp(22)
	style.Inset = layout.UniformInset(unit.Dp(11))
	return style.Layout(gtx)
}

func (ui *BrowserUI) layoutAddressBar(gtx layout.Context) layout.Dimensions {
	// Match the Gopher button's row height so the URL field and icon share one
	// visual center line even when the icon grows.
	height := gtx.Dp(addressBarHeight)
	if height > gtx.Constraints.Max.Y {
		height = gtx.Constraints.Max.Y
	}
	gtx.Constraints.Min.Y = height
	gtx.Constraints.Max.Y = height

	return widget.Border{
		Color:        color.NRGBA{R: 183, G: 193, B: 207, A: 255},
		CornerRadius: unit.Dp(12),
		Width:        unit.Dp(1),
	}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Stack{Alignment: layout.Center}.Layout(gtx,
			layout.Expanded(func(gtx layout.Context) layout.Dimensions {
				paint.FillShape(gtx.Ops,
					color.NRGBA{R: 255, G: 255, B: 255, A: 255},
					clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, gtx.Dp(unit.Dp(12))).Op(gtx.Ops),
				)
				return layout.Dimensions{Size: gtx.Constraints.Min}
			}),
			layout.Stacked(func(gtx layout.Context) layout.Dimensions {
				return layout.UniformInset(unit.Dp(10)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					// Stack children receive a zero minimum width by default. Keep the
					// editor's hit area as wide as the visible address bar instead of
					// letting it shrink to its placeholder text.
					gtx.Constraints.Min.X = gtx.Constraints.Max.X
					return material.Editor(ui.theme, &ui.address, "URLを入力").Layout(gtx)
				})
			}),
		)
	})
}

func (ui *BrowserUI) layoutGopherButton(gtx layout.Context) layout.Dimensions {
	width := gtx.Dp(gopherButtonWidth)
	height := gtx.Dp(gopherButtonHeight)
	gtx.Constraints = layout.Exact(image.Pt(width, height))

	return ui.goButton.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return widget.Image{
			Src:      ui.gopher,
			Fit:      widget.Contain,
			Position: layout.Center,
		}.Layout(gtx)
	})
}

func mustIcon(icon *widget.Icon, err error) *widget.Icon {
	if err != nil {
		panic("decode material icon: " + err.Error())
	}
	return icon
}

func (ui *BrowserUI) layoutViewport(gtx layout.Context) layout.Dimensions {
	if ui.navigator != nil {
		if page := ui.navigator.Page(); page != nil && page.Document != nil {
			return ui.layoutDocument(gtx, page)
		}
	}

	paint.Fill(gtx.Ops, color.NRGBA{R: 238, G: 243, B: 248, A: 255})

	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		maxWidth := gtx.Dp(unit.Dp(520))
		if gtx.Constraints.Max.X > maxWidth {
			gtx.Constraints.Max.X = maxWidth
		}

		return widget.Border{
			Color:        color.NRGBA{R: 211, G: 218, B: 228, A: 255},
			CornerRadius: unit.Dp(18),
			Width:        unit.Dp(1),
		}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Stack{Alignment: layout.Center}.Layout(gtx,
				layout.Expanded(func(gtx layout.Context) layout.Dimensions {
					paint.FillShape(gtx.Ops,
						color.NRGBA{R: 255, G: 255, B: 255, A: 255},
						clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, gtx.Dp(unit.Dp(18))).Op(gtx.Ops),
					)
					return layout.Dimensions{Size: gtx.Constraints.Min}
				}),
				layout.Stacked(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(36), Right: unit.Dp(48), Bottom: unit.Dp(36), Left: unit.Dp(48)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								state := "GROWSE · READY"
								if ui.loading {
									state = "GROWSE · LOADING"
								}
								label := material.Caption(ui.theme, state)
								label.Color = color.NRGBA{R: 0, G: 137, B: 173, A: 255}
								return label.Layout(gtx)
							}),
							layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
							layout.Rigid(material.H5(ui.theme, ui.pageTitle).Layout),
							layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
							layout.Rigid(material.Body1(ui.theme, ui.status).Layout),
						)
					})
				}),
			)
		})
	})
}

func (ui *BrowserUI) layoutDocument(gtx layout.Context, page *browser.Page) layout.Dimensions {
	paint.Fill(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
	if ui.scrollRevision != page.ScrollRevision {
		ui.pageList.Position = layout.Position{First: page.ScrollFirst, Offset: page.ScrollOffset}
		ui.scrollRevision = page.ScrollRevision
		ui.layoutCache = documentLayoutCache{}
	}
	if navigator, ok := ui.navigator.(animationFrameNavigator); ok {
		navigator.RunAnimationFrame(gtx.Now)
	}
	ui.handleFormTraversal(gtx, page)
	ui.syncFormFocus(gtx, page.FocusTarget)

	viewportWidth := float32(gtx.Constraints.Max.X) / gtx.Metric.PxPerDp
	viewportHeight := float32(gtx.Constraints.Max.Y) / gtx.Metric.PxPerDp
	if ui.navigator != nil {
		ui.navigator.UpdateViewport(viewportWidth, viewportHeight)
	}
	frameStyles := page.AnimatedStyles(gtx.Now)
	tree := ui.cachedDocumentTree(page, viewportWidth, viewportHeight, gtx.Metric.PxPerDp)
	layoutengine.ApplyAnimatedStyles(tree, frameStyles)
	displayList := paintmodel.Build(tree)
	paint.Fill(gtx.Ops, rgba(displayList.Background))
	ui.updateViewportHover(gtx, page, tree, displayList)
	ui.handleViewportClicks(gtx, page, tree, displayList)

	area := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
	dimensions := material.List(ui.theme, &ui.pageList).Layout(gtx, len(displayList.Commands), func(gtx layout.Context, index int) layout.Dimensions {
		switch command := displayList.Commands[index].(type) {
		case paintmodel.DrawText:
			return ui.layoutDrawText(gtx, command)
		case paintmodel.DrawInput:
			return ui.layoutDrawInput(gtx, command)
		case paintmodel.DrawSelect:
			return ui.layoutDrawSelect(gtx, command)
		case paintmodel.DrawCheckable:
			return ui.layoutDrawCheckable(gtx, command)
		case paintmodel.DrawButton:
			return ui.layoutDrawButton(gtx, command)
		case paintmodel.DrawBox:
			return layoutDrawBox(gtx, command, page.BackgroundImages)
		default:
			return layout.Dimensions{}
		}
	})
	pass := pointer.PassOp{}.Push(gtx.Ops)
	ui.viewportClick.Add(gtx.Ops)
	pass.Pop()
	area.Pop()
	if page.ActiveAnimations(gtx.Now) && ui.invalidate != nil {
		ui.invalidate()
	}
	if navigator, ok := ui.navigator.(animationFrameNavigator); ok && navigator.HasAnimationFrameCallbacks() && ui.invalidate != nil {
		ui.invalidate()
	}
	ui.persistHistoryScroll()
	return dimensions
}

func (ui *BrowserUI) persistHistoryScroll() {
	if navigator, ok := ui.navigator.(historyScrollNavigator); ok {
		navigator.UpdateHistoryScroll(ui.pageList.Position.First, ui.pageList.Position.Offset)
	}
}

func (ui *BrowserUI) cachedDocumentTree(page *browser.Page, viewportWidth, viewportHeight, pxPerDp float32) *layoutengine.Tree {
	position := ui.pageList.Position
	cache := &ui.layoutCache
	if cache.tree != nil && cache.page == page && cache.revision == page.StyleRevision &&
		cache.viewportWidth == viewportWidth && cache.viewportHeight == viewportHeight &&
		cache.listFirst == position.First && cache.listOffset == position.Offset {
		return layoutengine.Clone(cache.tree)
	}

	build := ui.layoutBuild
	if build == nil {
		build = layoutengine.BuildWithScroll
	}
	tree := build(page.Document, page.ComputedStyles, viewportWidth, viewportHeight, 0, 0)
	displayList := paintmodel.Build(tree)
	if position.First >= 0 && position.First < len(displayList.Commands) {
		if firstY, ok := commandDocumentY(displayList.Commands[position.First]); ok {
			scrollY := max(firstY+float32(position.Offset)/pxPerDp, float32(0))
			if scrollY > 0 {
				tree = build(page.Document, page.ComputedStyles, viewportWidth, viewportHeight, 0, scrollY)
			}
		}
	}
	*cache = documentLayoutCache{
		page: page, revision: page.StyleRevision, viewportWidth: viewportWidth, viewportHeight: viewportHeight,
		listFirst: position.First, listOffset: position.Offset, tree: tree,
	}
	return layoutengine.Clone(tree)
}

func (ui *BrowserUI) updateViewportHover(gtx layout.Context, page *browser.Page, tree *layoutengine.Tree, displayList *paintmodel.DisplayList) {
	if ui.navigator == nil {
		return
	}
	position, inside := viewportPointerPosition(gtx, ui.pointer.position, ui.pointer.inside)
	if !inside {
		ui.navigator.ClearHover()
		ui.updateLinkPreview(page, 0)
		return
	}
	x, y, ok := ui.documentPoint(position, displayList, gtx.Metric.PxPerDp)
	if !ok {
		ui.navigator.ClearHover()
		ui.updateLinkPreview(page, 0)
		return
	}
	nodeID, ok := layoutengine.HitTest(tree, x, y)
	if !ok {
		ui.navigator.ClearHover()
		ui.updateLinkPreview(page, 0)
		return
	}
	ui.navigator.UpdateHover(nodeID, x, y)
	ui.updateLinkPreview(page, nodeID)
}

func viewportPointerPosition(gtx layout.Context, position f32.Point, insideWindow bool) (image.Point, bool) {
	viewportX := position.X - float32(gtx.Dp(tabRailWidth))
	viewportY := position.Y - float32(gtx.Dp(toolbarHeight))
	if !insideWindow || viewportX < 0 || viewportX >= float32(gtx.Constraints.Max.X) || viewportY < 0 || viewportY >= float32(gtx.Constraints.Max.Y) {
		return image.Point{}, false
	}
	return image.Pt(int(math.Round(float64(viewportX))), int(math.Round(float64(viewportY)))), true
}

func (ui *BrowserUI) updateLinkPreview(page *browser.Page, nodeID dom.NodeID) {
	if ui.loading || ui.statusHasError {
		return
	}
	if linkURL, ok := page.LinkURL(nodeID); ok {
		ui.status = network.RedactedURL(linkURL)
		return
	}
	ui.status = ui.pageStatus
}

func (ui *BrowserUI) handleViewportClicks(gtx layout.Context, page *browser.Page, tree *layoutengine.Tree, displayList *paintmodel.DisplayList) {
	for {
		click, ok := ui.viewportClick.Update(gtx.Source)
		if !ok {
			return
		}
		if click.Kind != gesture.KindClick || ui.pageList.List.Dragging() {
			continue
		}
		x, y, ok := ui.documentPoint(click.Position, displayList, gtx.Metric.PxPerDp)
		if !ok {
			continue
		}
		nodeID, ok := layoutengine.HitTest(tree, x, y)
		if !ok {
			continue
		}
		if _, handledByButton := ui.formButtons[nodeID]; handledByButton {
			continue
		}
		ui.navigator.UpdateFocus(focusableNodeID(page.Document, nodeID))
		if ui.navigator.DispatchClick(nodeID, x, y) {
			continue
		}
		linkURL, target, ok := page.LinkDestination(nodeID)
		if !ok {
			continue
		}
		if target == "_blank" {
			ui.openURLInNewTab(linkURL)
			continue
		}
		ui.startNavigation(linkURL.String())
	}
}

func (ui *BrowserUI) openURLInNewTab(target *url.URL) {
	if ui.tabs == nil || target == nil {
		ui.status = "新しいTabでNavigationを開始できません"
		ui.statusHasError = true
		return
	}
	tab, err := ui.tabs.NewTab(target)
	if err != nil {
		ui.reportTabOperationError("新しいTabを作成できません", err)
		return
	}
	if _, err := ui.tabs.SelectTab(tab.ID); err != nil {
		ui.reportTabOperationError("新しいTabを選択できません", err)
		return
	}
	tabID, navigator := ui.activeNavigationTarget()
	if tabID != tab.ID || navigator == nil {
		ui.status = "新しいTabのNavigationを開始できません"
		ui.statusHasError = true
		return
	}
	ui.startPageLoad(tabID, navigator, navigationLoadingStatus(target.String()), func(ctx context.Context) (*browser.Page, error) {
		return navigator.Navigate(ctx, target.String())
	})
}

func navigationLoadingStatus(rawURL string) string {
	target, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return "読み込み中"
	}
	return "読み込み中: " + network.RedactedURL(target)
}

func focusableNodeID(document *dom.Document, nodeID dom.NodeID) dom.NodeID {
	if document == nil {
		return 0
	}
	node, ok := document.NodeByID(nodeID)
	if !ok {
		return 0
	}
	if control := forms.LabeledControl(document, node); control != nil && !forms.Disabled(control) {
		return control.ID
	}
	for current := node; current != nil; current = current.Parent {
		if current.Type != dom.NodeElement {
			continue
		}
		if forms.Disabled(current) {
			continue
		}
		if _, hasTabIndex := current.Attribute("tabindex"); hasTabIndex {
			return current.ID
		}
		switch current.TagName {
		case "input", "button", "select", "textarea":
			return current.ID
		case "a", "area":
			if _, linked := current.Attribute("href"); linked {
				return current.ID
			}
		}
	}
	return 0
}

func (ui *BrowserUI) documentPoint(position image.Point, displayList *paintmodel.DisplayList, pixelsPerDP float32) (float32, float32, bool) {
	if displayList == nil || pixelsPerDP <= 0 || len(displayList.Commands) == 0 {
		return 0, 0, false
	}
	first := ui.pageList.Position.First
	if first < 0 || first >= len(displayList.Commands) {
		return 0, 0, false
	}
	firstDocumentY, ok := commandDocumentY(displayList.Commands[first])
	if !ok {
		return 0, 0, false
	}
	x := float32(position.X) / pixelsPerDP
	y := firstDocumentY + float32(position.Y+ui.pageList.Position.Offset)/pixelsPerDP
	return x, y, true
}

func commandDocumentY(command paintmodel.Command) (float32, bool) {
	switch command := command.(type) {
	case paintmodel.DrawText:
		return command.Y - command.Top, true
	case paintmodel.DrawInput:
		return command.Y - command.Top, true
	case paintmodel.DrawSelect:
		return command.Y - command.Top, true
	case paintmodel.DrawCheckable:
		return command.Y - command.Top, true
	case paintmodel.DrawButton:
		return command.Y - command.Top, true
	case paintmodel.DrawBox:
		return command.Y - command.Top, true
	default:
		return 0, false
	}
}

func layoutDrawBox(gtx layout.Context, command paintmodel.DrawBox, backgroundImages map[string]image.Image) layout.Dimensions {
	return layout.Inset{Top: unit.Dp(command.Top), Left: unit.Dp(command.X)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		if command.Transform != (stylemodel.Matrix{}) && command.Transform != stylemodel.IdentityMatrix() {
			defer pushCSSMatrix(gtx, command.Transform, command.X, command.Y).Pop()
		}
		if command.Clip != nil {
			defer commandClip(gtx, command.Clip, command.X, command.Y).Push(gtx.Ops).Pop()
		}
		for _, region := range command.Clips {
			defer commandRoundedClip(gtx, region, command.X, command.Y).Push(gtx.Ops).Pop()
		}
		width := min(gtx.Dp(unit.Dp(command.Width)), gtx.Constraints.Max.X)
		height := min(gtx.Dp(unit.Dp(command.Height)), gtx.Constraints.Max.Y)
		bounds := clip.Rect{Max: image.Pt(width, height)}
		if command.Opacity < 1 {
			defer paint.PushOpacity(gtx.Ops, max(command.Opacity, 0)).Pop()
		}
		for _, shadow := range command.BoxShadows {
			if shadow.Inset {
				continue
			}
			spread := gtx.Dp(unit.Dp(shadow.Spread + shadow.Blur/2))
			offsetX, offsetY := gtx.Dp(unit.Dp(shadow.OffsetX)), gtx.Dp(unit.Dp(shadow.OffsetY))
			paint.FillShape(gtx.Ops, rgba(shadow.Color), clip.Rect{Min: image.Pt(offsetX-spread, offsetY-spread), Max: image.Pt(width+offsetX+spread, height+offsetY+spread)}.Op())
		}
		rounded := roundedClip(gtx, command.Radius, width, height).Push(gtx.Ops)
		defer rounded.Pop()
		if command.Color != 0 {
			paint.FillShape(gtx.Ops, rgba(command.Color), bounds.Op())
		}
		layers := command.Layers
		if len(layers) == 0 && command.Image.Kind != stylemodel.BackgroundImageNone {
			layers = []stylemodel.BackgroundLayer{{Image: command.Image, Repeat: command.Repeat, Position: command.Position, Size: command.Size}}
		}
		for index := len(layers) - 1; index >= 0; index-- {
			layer := layers[index]
			var raster image.Image
			if layer.Image.Kind == stylemodel.BackgroundImageLinearGradient && width > 0 && height > 0 {
				raster = rasterLinearGradient(width, height, layer.Image)
			} else if layer.Image.Kind == stylemodel.BackgroundImageRadialGradient && width > 0 && height > 0 {
				raster = rasterRadialGradient(width, height, layer.Image)
			} else if layer.Image.Kind == stylemodel.BackgroundImageURL && backgroundImages[layer.Image.URL] != nil && width > 0 && height > 0 {
				layerCommand := command
				layerCommand.Image, layerCommand.Repeat, layerCommand.Position, layerCommand.Size = layer.Image, layer.Repeat, layer.Position, layer.Size
				raster = rasterBackgroundImage(width, height, backgroundImages[layer.Image.URL], layerCommand, gtx.Metric.PxPerDp)
			}
			if raster != nil {
				area := bounds.Push(gtx.Ops)
				widget.Image{Src: paint.NewImageOp(raster), Fit: widget.Unscaled, Scale: 1 / gtx.Metric.PxPerDp}.Layout(gtx)
				area.Pop()
			}
		}
		paintBoxBorder(gtx, command.Border, width, height)
		paintOutline(gtx, command.Outline, command.OutlineOffset, width, height)
		return layout.Dimensions{}
	})
}

func paintOutline(gtx layout.Context, outline stylemodel.BorderSide, offset float32, width, height int) {
	if outline.Style == stylemodel.BorderNone || outline.Width <= 0 {
		return
	}
	thickness, distance := gtx.Dp(unit.Dp(outline.Width)), gtx.Dp(unit.Dp(offset))
	outer := image.Rect(-distance-thickness, -distance-thickness, width+distance+thickness, height+distance+thickness)
	paintBorderStrip(gtx, image.Rect(outer.Min.X, outer.Min.Y, outer.Max.X, outer.Min.Y+thickness), outline, true)
	paintBorderStrip(gtx, image.Rect(outer.Min.X, outer.Max.Y-thickness, outer.Max.X, outer.Max.Y), outline, true)
	paintBorderStrip(gtx, image.Rect(outer.Min.X, outer.Min.Y, outer.Min.X+thickness, outer.Max.Y), outline, false)
	paintBorderStrip(gtx, image.Rect(outer.Max.X-thickness, outer.Min.Y, outer.Max.X, outer.Max.Y), outline, false)
}

func roundedClip(gtx layout.Context, radius layoutengine.BorderRadii, width, height int) clip.Op {
	radius = pixelBorderRadii(gtx, radius)
	if radius == (layoutengine.BorderRadii{}) {
		return clip.Rect{Max: image.Pt(width, height)}.Op()
	}
	const control = float32(.55228475)
	var path clip.Path
	path.Begin(gtx.Ops)
	path.MoveTo(f32.Pt(radius.TopLeft.X, 0))
	path.LineTo(f32.Pt(float32(width)-radius.TopRight.X, 0))
	path.CubeTo(
		f32.Pt(float32(width)-radius.TopRight.X+radius.TopRight.X*control, 0),
		f32.Pt(float32(width), radius.TopRight.Y-radius.TopRight.Y*control),
		f32.Pt(float32(width), radius.TopRight.Y),
	)
	path.LineTo(f32.Pt(float32(width), float32(height)-radius.BottomRight.Y))
	path.CubeTo(
		f32.Pt(float32(width), float32(height)-radius.BottomRight.Y+radius.BottomRight.Y*control),
		f32.Pt(float32(width)-radius.BottomRight.X+radius.BottomRight.X*control, float32(height)),
		f32.Pt(float32(width)-radius.BottomRight.X, float32(height)),
	)
	path.LineTo(f32.Pt(radius.BottomLeft.X, float32(height)))
	path.CubeTo(
		f32.Pt(radius.BottomLeft.X-radius.BottomLeft.X*control, float32(height)),
		f32.Pt(0, float32(height)-radius.BottomLeft.Y+radius.BottomLeft.Y*control),
		f32.Pt(0, float32(height)-radius.BottomLeft.Y),
	)
	path.LineTo(f32.Pt(0, radius.TopLeft.Y))
	path.CubeTo(
		f32.Pt(0, radius.TopLeft.Y-radius.TopLeft.Y*control),
		f32.Pt(radius.TopLeft.X-radius.TopLeft.X*control, 0),
		f32.Pt(radius.TopLeft.X, 0),
	)
	path.Close()
	return clip.Outline{Path: path.End()}.Op()
}

func pixelBorderRadii(gtx layout.Context, radius layoutengine.BorderRadii) layoutengine.BorderRadii {
	convert := func(value layoutengine.CornerRadius) layoutengine.CornerRadius {
		return layoutengine.CornerRadius{
			X: float32(max(gtx.Dp(unit.Dp(value.X)), 0)),
			Y: float32(max(gtx.Dp(unit.Dp(value.Y)), 0)),
		}
	}
	return layoutengine.BorderRadii{
		TopLeft: convert(radius.TopLeft), TopRight: convert(radius.TopRight),
		BottomRight: convert(radius.BottomRight), BottomLeft: convert(radius.BottomLeft),
	}
}

func paintBoxBorder(gtx layout.Context, border stylemodel.Borders, width, height int) {
	paintBorderStrip(gtx, image.Rect(0, 0, width, gtx.Dp(unit.Dp(border.Top.Width))), border.Top, true)
	rightWidth := gtx.Dp(unit.Dp(border.Right.Width))
	paintBorderStrip(gtx, image.Rect(width-rightWidth, 0, width, height), border.Right, false)
	bottomHeight := gtx.Dp(unit.Dp(border.Bottom.Width))
	paintBorderStrip(gtx, image.Rect(0, height-bottomHeight, width, height), border.Bottom, true)
	paintBorderStrip(gtx, image.Rect(0, 0, gtx.Dp(unit.Dp(border.Left.Width)), height), border.Left, false)
}

func paintBorderStrip(gtx layout.Context, rectangle image.Rectangle, side stylemodel.BorderSide, horizontal bool) {
	if side.Width <= 0 || side.Style == stylemodel.BorderNone || rectangle.Empty() {
		return
	}
	fill := func(rectangle image.Rectangle) { paint.FillShape(gtx.Ops, rgba(side.Color), clip.Rect(rectangle).Op()) }
	switch side.Style {
	case stylemodel.BorderDouble:
		thickness := rectangle.Dy()
		if !horizontal {
			thickness = rectangle.Dx()
		}
		stripe := max(thickness/3, 1)
		if horizontal {
			fill(image.Rect(rectangle.Min.X, rectangle.Min.Y, rectangle.Max.X, rectangle.Min.Y+stripe))
			fill(image.Rect(rectangle.Min.X, rectangle.Max.Y-stripe, rectangle.Max.X, rectangle.Max.Y))
		} else {
			fill(image.Rect(rectangle.Min.X, rectangle.Min.Y, rectangle.Min.X+stripe, rectangle.Max.Y))
			fill(image.Rect(rectangle.Max.X-stripe, rectangle.Min.Y, rectangle.Max.X, rectangle.Max.Y))
		}
	case stylemodel.BorderDotted, stylemodel.BorderDashed:
		thickness := max(rectangle.Dy(), 1)
		length := rectangle.Dx()
		if !horizontal {
			thickness, length = max(rectangle.Dx(), 1), rectangle.Dy()
		}
		segment := thickness
		if side.Style == stylemodel.BorderDashed {
			segment *= 3
		}
		for offset := 0; offset < length; offset += segment + thickness {
			end := min(offset+segment, length)
			if horizontal {
				fill(image.Rect(rectangle.Min.X+offset, rectangle.Min.Y, rectangle.Min.X+end, rectangle.Max.Y))
			} else {
				fill(image.Rect(rectangle.Min.X, rectangle.Min.Y+offset, rectangle.Max.X, rectangle.Min.Y+end))
			}
		}
	default:
		fill(rectangle)
	}
}

func rasterBackgroundImage(width, height int, source image.Image, command paintmodel.DrawBox, pixelsPerDP float32) *image.NRGBA {
	result := image.NewNRGBA(image.Rect(0, 0, width, height))
	if source == nil || width <= 0 || height <= 0 || pixelsPerDP <= 0 {
		return result
	}
	sourceWidth, sourceHeight := source.Bounds().Dx(), source.Bounds().Dy()
	if sourceWidth <= 0 || sourceHeight <= 0 {
		return result
	}
	boxWidth, boxHeight := float32(width)/pixelsPerDP, float32(height)/pixelsPerDP
	tileWidth, tileHeight := float32(sourceWidth), float32(sourceHeight)
	switch command.Size.Kind {
	case stylemodel.BackgroundSizeCover, stylemodel.BackgroundSizeContain:
		scaleX, scaleY := boxWidth/tileWidth, boxHeight/tileHeight
		scale := min(scaleX, scaleY)
		if command.Size.Kind == stylemodel.BackgroundSizeCover {
			scale = max(scaleX, scaleY)
		}
		tileWidth, tileHeight = tileWidth*scale, tileHeight*scale
	case stylemodel.BackgroundSizeExplicit:
		widthSpecified := command.Size.Width.Kind == stylemodel.SizeLength
		heightSpecified := command.Size.Height.Kind == stylemodel.SizeLength
		if widthSpecified {
			tileWidth = command.Size.Width.Value.Resolve(boxWidth)
		}
		if heightSpecified {
			tileHeight = command.Size.Height.Value.Resolve(boxHeight)
		}
		if widthSpecified && !heightSpecified {
			tileHeight = tileWidth * float32(sourceHeight) / float32(sourceWidth)
		} else if heightSpecified && !widthSpecified {
			tileWidth = tileHeight * float32(sourceWidth) / float32(sourceHeight)
		}
	}
	tileWidthPixels := max(int(math.Round(float64(tileWidth*pixelsPerDP))), 1)
	tileHeightPixels := max(int(math.Round(float64(tileHeight*pixelsPerDP))), 1)
	if tileWidthPixels > maxBackgroundRasterDimension(width) || tileHeightPixels > maxBackgroundRasterDimension(height) {
		return result
	}
	tile := image.NewNRGBA(image.Rect(0, 0, tileWidthPixels, tileHeightPixels))
	xdraw.CatmullRom.Scale(tile, tile.Bounds(), source, source.Bounds(), imagedraw.Src, nil)
	positionX := command.Position.X.Pixels*pixelsPerDP + command.Position.X.Percentage/100*float32(width-tileWidthPixels)
	positionY := command.Position.Y.Pixels*pixelsPerDP + command.Position.Y.Percentage/100*float32(height-tileHeightPixels)
	startX, startY := int(math.Round(float64(positionX))), int(math.Round(float64(positionY)))
	if command.Repeat.X {
		for startX > 0 {
			startX -= tileWidthPixels
		}
	} else if startX >= width || startX+tileWidthPixels <= 0 {
		return result
	}
	if command.Repeat.Y {
		for startY > 0 {
			startY -= tileHeightPixels
		}
	} else if startY >= height || startY+tileHeightPixels <= 0 {
		return result
	}
	for y := startY; y < height; y += tileHeightPixels {
		for x := startX; x < width; x += tileWidthPixels {
			imagedraw.Draw(result, image.Rect(x, y, x+tileWidthPixels, y+tileHeightPixels), tile, image.Point{}, imagedraw.Over)
			if !command.Repeat.X {
				break
			}
		}
		if !command.Repeat.Y {
			break
		}
	}
	return result
}

func maxBackgroundRasterDimension(container int) int {
	return max(container*4, 4096)
}

func rasterLinearGradient(width, height int, background stylemodel.BackgroundImage) *image.NRGBA {
	result := image.NewNRGBA(image.Rect(0, 0, width, height))
	if len(background.GradientStops) == 0 {
		return result
	}
	radians := float64(background.GradientAngle) * math.Pi / 180
	directionX, directionY := float32(math.Sin(radians)), float32(-math.Cos(radians))
	span := float32(math.Abs(float64(directionX)))*float32(max(width-1, 0)) + float32(math.Abs(float64(directionY)))*float32(max(height-1, 0))
	if span == 0 {
		span = 1
	}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			projection := directionX*(float32(x)-float32(width-1)/2) + directionY*(float32(y)-float32(height-1)/2)
			position := projection/span + .5
			result.SetNRGBA(x, y, gradientColor(background.GradientStops, position))
		}
	}
	return result
}

func rasterRadialGradient(width, height int, background stylemodel.BackgroundImage) *image.NRGBA {
	result := image.NewNRGBA(image.Rect(0, 0, width, height))
	if len(background.GradientStops) == 0 || width <= 0 || height <= 0 {
		return result
	}
	centerX := background.GradientCenter.X.Resolve(float32(width))
	centerY := background.GradientCenter.Y.Resolve(float32(height))
	radiusX, radiusY := max(centerX, float32(width)-centerX), max(centerY, float32(height)-centerY)
	if background.RadialCircle {
		radius := max(radiusX, radiusY)
		radiusX, radiusY = radius, radius
	}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			dx, dy := (float32(x)-centerX)/max(radiusX, 1), (float32(y)-centerY)/max(radiusY, 1)
			result.SetNRGBA(x, y, gradientColor(background.GradientStops, min(float32(math.Sqrt(float64(dx*dx+dy*dy))), 1)))
		}
	}
	return result
}

func gradientColor(stops []stylemodel.GradientStop, position float32) color.NRGBA {
	if position <= stops[0].Position {
		return rgba(stops[0].Color)
	}
	for index := 1; index < len(stops); index++ {
		if position > stops[index].Position {
			continue
		}
		left, right := stops[index-1], stops[index]
		amount := float32(0)
		if right.Position > left.Position {
			amount = (position - left.Position) / (right.Position - left.Position)
		}
		return mixColor(rgba(left.Color), rgba(right.Color), amount)
	}
	return rgba(stops[len(stops)-1].Color)
}

func mixColor(left, right color.NRGBA, amount float32) color.NRGBA {
	mix := func(a, b uint8) uint8 { return uint8(float32(a) + (float32(b)-float32(a))*amount + .5) }
	return color.NRGBA{R: mix(left.R, right.R), G: mix(left.G, right.G), B: mix(left.B, right.B), A: mix(left.A, right.A)}
}

func (ui *BrowserUI) layoutDrawInput(gtx layout.Context, command paintmodel.DrawInput) layout.Dimensions {
	left := unit.Dp(command.X)
	viewportWidth := float32(gtx.Constraints.Max.X) / gtx.Metric.PxPerDp
	rightValue := viewportWidth - command.X - command.Width
	if rightValue < 0 {
		rightValue = 0
	}
	right := unit.Dp(rightValue)
	return layout.Inset{Top: unit.Dp(command.Top), Left: left, Right: right}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		if command.Clip != nil {
			defer commandClip(gtx, command.Clip, command.X, command.Y).Push(gtx.Ops).Pop()
		}
		if command.Opacity < 1 {
			defer paint.PushOpacity(gtx.Ops, max(command.Opacity, 0)).Pop()
		}
		if command.Disabled {
			gtx = gtx.Disabled()
		}
		height := gtx.Dp(unit.Dp(command.Height))
		gtx.Constraints.Min.Y = height
		gtx.Constraints.Max.Y = height
		editor := ui.inputEditors[command.NodeID]
		wasFocused := ui.inputFocused[command.NodeID]
		if editor == nil {
			editor = new(widget.Editor)
			editor.SingleLine = !command.Multiline
			editor.Submit = !command.Multiline
			if command.InputType == "password" {
				editor.Mask = '•'
			}
			editor.SetText(command.Value)
			ui.inputEditors[command.NodeID] = editor
			ui.inputCommitted[command.NodeID] = command.Value
		} else if editor.Text() != command.Value {
			editor.SetText(command.Value)
			ui.inputCommitted[command.NodeID] = command.Value
		}
		if command.InputType == "password" {
			editor.Mask = '•'
		} else {
			editor.Mask = 0
		}
		editor.ReadOnly = command.ReadOnly || command.Disabled
		for {
			event, ok := editor.Update(gtx)
			if !ok {
				break
			}
			if _, changed := event.(widget.ChangeEvent); changed && ui.navigator != nil {
				ui.navigator.SetInputValue(command.NodeID, editor.Text())
			}
			if _, submitted := event.(widget.SubmitEvent); submitted {
				ui.commitInput(command.NodeID, editor.Text())
				if ui.navigator != nil {
					ui.navigator.SubmitForm(command.NodeID)
				}
			}
		}
		focused := gtx.Focused(editor)
		if focused != wasFocused && ui.navigator != nil {
			if focused {
				ui.navigator.UpdateFocus(command.NodeID)
			} else {
				page := ui.navigator.Page()
				if page == nil || page.FocusTarget == command.NodeID {
					ui.navigator.UpdateFocus(0)
				}
			}
		}
		if wasFocused && !focused {
			ui.commitInput(command.NodeID, editor.Text())
		}
		ui.inputFocused[command.NodeID] = focused
		return widget.Border{
			Color:        color.NRGBA{R: 150, G: 160, B: 175, A: 255},
			CornerRadius: unit.Dp(6),
			Width:        unit.Dp(1),
		}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, clip.Rect{Max: gtx.Constraints.Min}.Op())
			style := material.Editor(ui.theme, editor, "")
			style.Color = rgba(command.Color)
			return layout.UniformInset(unit.Dp(8)).Layout(gtx, style.Layout)
		})
	})
}

func (ui *BrowserUI) layoutDrawSelect(gtx layout.Context, command paintmodel.DrawSelect) layout.Dimensions {
	left := unit.Dp(command.X)
	viewportWidth := float32(gtx.Constraints.Max.X) / gtx.Metric.PxPerDp
	right := unit.Dp(max(viewportWidth-command.X-command.Width, float32(0)))
	return layout.Inset{Top: unit.Dp(command.Top), Left: left, Right: right}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		if command.Clip != nil {
			defer commandClip(gtx, command.Clip, command.X, command.Y).Push(gtx.Ops).Pop()
		}
		if command.Opacity < 1 {
			defer paint.PushOpacity(gtx.Ops, max(command.Opacity, 0)).Pop()
		}
		if command.Disabled {
			gtx = gtx.Disabled()
		}
		button := ui.selectButtons[command.NodeID]
		if button == nil {
			button = new(widget.Clickable)
			ui.selectButtons[command.NodeID] = button
		}
		for button.Clicked(gtx) {
			if next, ok := forms.NextEnabledValue(command.Options, command.Selected); ok && ui.navigator != nil {
				ui.navigator.UpdateFocus(command.NodeID)
				ui.navigator.SetSelectValue(command.NodeID, next)
			}
		}
		gtx.Constraints.Min.Y = gtx.Dp(unit.Dp(command.Height))
		gtx.Constraints.Max.Y = gtx.Constraints.Min.Y
		label := command.Label
		if label == "" {
			label = "選択してください"
		}
		style := material.Button(ui.theme, button, label+" ▾")
		style.Color = rgba(command.Color)
		return style.Layout(gtx)
	})
}

func (ui *BrowserUI) layoutDrawCheckable(gtx layout.Context, command paintmodel.DrawCheckable) layout.Dimensions {
	left := unit.Dp(command.X)
	viewportWidth := float32(gtx.Constraints.Max.X) / gtx.Metric.PxPerDp
	right := unit.Dp(max(viewportWidth-command.X-command.Width, float32(0)))
	return layout.Inset{Top: unit.Dp(command.Top), Left: left, Right: right}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		if command.Clip != nil {
			defer commandClip(gtx, command.Clip, command.X, command.Y).Push(gtx.Ops).Pop()
		}
		if command.Opacity < 1 {
			defer paint.PushOpacity(gtx.Ops, max(command.Opacity, 0)).Pop()
		}
		if command.Disabled {
			gtx = gtx.Disabled()
		}
		button := ui.checkableButtons[command.NodeID]
		if button == nil {
			button = new(widget.Clickable)
			ui.checkableButtons[command.NodeID] = button
		}
		for button.Clicked(gtx) {
			if ui.navigator != nil {
				ui.navigator.UpdateFocus(command.NodeID)
				ui.navigator.ActivateCheckable(command.NodeID)
			}
		}
		gtx.Constraints.Min.Y = gtx.Dp(unit.Dp(command.Height))
		gtx.Constraints.Max.Y = gtx.Constraints.Min.Y
		label := "☐"
		if command.InputType == "radio" {
			label = "○"
		}
		if command.Checked {
			label = "☑"
			if command.InputType == "radio" {
				label = "●"
			}
		}
		style := material.Button(ui.theme, button, label)
		style.Color = rgba(command.Color)
		return style.Layout(gtx)
	})
}

func (ui *BrowserUI) layoutDrawButton(gtx layout.Context, command paintmodel.DrawButton) layout.Dimensions {
	left := unit.Dp(command.X)
	viewportWidth := float32(gtx.Constraints.Max.X) / gtx.Metric.PxPerDp
	right := unit.Dp(max(viewportWidth-command.X-command.Width, float32(0)))
	return layout.Inset{Top: unit.Dp(command.Top), Left: left, Right: right}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		if command.Clip != nil {
			defer commandClip(gtx, command.Clip, command.X, command.Y).Push(gtx.Ops).Pop()
		}
		if command.Opacity < 1 {
			defer paint.PushOpacity(gtx.Ops, max(command.Opacity, 0)).Pop()
		}
		if command.Disabled {
			gtx = gtx.Disabled()
		}
		button := ui.formButtons[command.NodeID]
		if button == nil {
			button = new(widget.Clickable)
			ui.formButtons[command.NodeID] = button
		}
		for button.Clicked(gtx) {
			if ui.navigator != nil {
				ui.navigator.UpdateFocus(command.NodeID)
				ui.navigator.DispatchClick(command.NodeID, command.X, command.Y)
			}
		}
		gtx.Constraints.Min.Y = gtx.Dp(unit.Dp(command.Height))
		gtx.Constraints.Max.Y = gtx.Constraints.Min.Y
		style := material.Button(ui.theme, button, command.Label)
		style.Color = rgba(command.Color)
		return style.Layout(gtx)
	})
}

func (ui *BrowserUI) handleFormTraversal(gtx layout.Context, page *browser.Page) {
	for {
		event, ok := gtx.Event(key.Filter{Name: key.NameTab, Optional: key.ModShift})
		if !ok {
			return
		}
		keyEvent, ok := event.(key.Event)
		if !ok || keyEvent.State != key.Press || ui.navigator == nil {
			continue
		}
		ui.navigator.MoveFormFocus(keyEvent.Modifiers.Contain(key.ModShift))
	}
}

func (ui *BrowserUI) syncFormFocus(gtx layout.Context, nodeID dom.NodeID) {
	var tag any
	if editor := ui.inputEditors[nodeID]; editor != nil {
		tag = editor
	} else if button := ui.selectButtons[nodeID]; button != nil {
		tag = button
	} else if button := ui.checkableButtons[nodeID]; button != nil {
		tag = button
	} else if button := ui.formButtons[nodeID]; button != nil {
		tag = button
	}
	if tag != nil && !gtx.Focused(tag) {
		gtx.Execute(key.FocusCmd{Tag: tag})
	}
}

func (ui *BrowserUI) commitInput(nodeID dom.NodeID, value string) {
	if ui.inputCommitted[nodeID] == value {
		return
	}
	ui.inputCommitted[nodeID] = value
	if ui.navigator != nil {
		ui.navigator.CommitInputValue(nodeID, value)
	}
}

func (ui *BrowserUI) layoutDrawText(gtx layout.Context, command paintmodel.DrawText) layout.Dimensions {
	left := unit.Dp(command.X)
	viewportWidth := float32(gtx.Constraints.Max.X) / gtx.Metric.PxPerDp
	rightValue := viewportWidth - command.X - command.Width
	if rightValue < 0 {
		rightValue = 0
	}
	right := unit.Dp(rightValue)
	return layout.Inset{Top: unit.Dp(command.Top), Left: left, Right: right}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		if command.Transform != (stylemodel.Matrix{}) && command.Transform != stylemodel.IdentityMatrix() {
			defer pushCSSMatrix(gtx, command.Transform, command.X, command.Y).Pop()
		}
		if command.Clip != nil {
			defer commandClip(gtx, command.Clip, command.X, command.Y).Push(gtx.Ops).Pop()
		}
		for _, region := range command.Clips {
			defer commandRoundedClip(gtx, region, command.X, command.Y).Push(gtx.Ops).Pop()
		}
		height := gtx.Dp(unit.Dp(command.Height))
		if height < 1 {
			height = 1
		}
		gtx.Constraints.Min.Y = height
		gtx.Constraints.Max.Y = height
		if command.Background != 0 {
			paint.FillShape(gtx.Ops, rgba(command.Background), clip.Rect{Max: gtx.Constraints.Min}.Op())
		}
		if len(command.Runs) > 0 {
			children := make([]layout.FlexChild, 0, len(command.Runs))
			for _, run := range command.Runs {
				run := run
				children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return ui.layoutTextRun(gtx, run, height)
				}))
			}
			return layout.Flex{Alignment: layout.Baseline}.Layout(gtx, children...)
		}
		if command.Opacity < 1 {
			defer paint.PushOpacity(gtx.Ops, max(command.Opacity, 0)).Pop()
		}

		return ui.layoutShadowedText(gtx, command.Text, command.FontSize, command.Bold, command.Color, command.Decoration, command.DecorationColor, command.Baseline, command.TextShadows)
	})
}

func pushCSSMatrix(gtx layout.Context, matrix stylemodel.Matrix, originX, originY float32) op.TransformStack {
	translationX := matrix.A*originX + matrix.C*originY + matrix.E - originX
	translationY := matrix.B*originX + matrix.D*originY + matrix.F - originY
	affine := f32.NewAffine2D(matrix.A, matrix.C, float32(gtx.Dp(unit.Dp(translationX))), matrix.B, matrix.D, float32(gtx.Dp(unit.Dp(translationY))))
	return op.Affine(affine).Push(gtx.Ops)
}

func commandClip(gtx layout.Context, rectangle *layoutengine.Rect, originX, originY float32) clip.Rect {
	return clip.Rect{
		Min: image.Pt(
			gtx.Dp(unit.Dp(rectangle.X-originX)),
			gtx.Dp(unit.Dp(rectangle.Y-originY)),
		),
		Max: image.Pt(
			gtx.Dp(unit.Dp(rectangle.X+rectangle.Width-originX)),
			gtx.Dp(unit.Dp(rectangle.Y+rectangle.Height-originY)),
		),
	}
}

func commandRoundedClip(gtx layout.Context, region layoutengine.ClipRegion, originX, originY float32) clip.Op {
	left := gtx.Dp(unit.Dp(region.X - originX))
	top := gtx.Dp(unit.Dp(region.Y - originY))
	right := gtx.Dp(unit.Dp(region.X + region.Width - originX))
	bottom := gtx.Dp(unit.Dp(region.Y + region.Height - originY))
	radius := func(corner layoutengine.CornerRadius) int { return gtx.Dp(unit.Dp(max(corner.X, corner.Y))) }
	return clip.RRect{Rect: image.Rect(left, top, right, bottom), NW: radius(region.Radius.TopLeft), NE: radius(region.Radius.TopRight), SE: radius(region.Radius.BottomRight), SW: radius(region.Radius.BottomLeft)}.Op(gtx.Ops)
}

func (ui *BrowserUI) layoutTextRun(gtx layout.Context, run paintmodel.TextRun, height int) layout.Dimensions {
	gtx.Constraints.Min.X = 0
	gtx.Constraints.Min.Y = height
	gtx.Constraints.Max.Y = height
	if run.Opacity < 1 {
		defer paint.PushOpacity(gtx.Ops, max(run.Opacity, 0)).Pop()
	}
	text := func(gtx layout.Context) layout.Dimensions {
		return ui.layoutShadowedText(gtx, run.Text, run.FontSize, run.Bold, run.Color, run.Decoration, run.DecorationColor, run.Baseline, run.TextShadows)
	}
	if run.Background == 0 {
		return text(gtx)
	}
	return layout.Stack{Alignment: layout.W}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, rgba(run.Background), clip.Rect{Max: gtx.Constraints.Min}.Op())
			return layout.Dimensions{Size: gtx.Constraints.Min}
		}),
		layout.Stacked(text),
	)
}

func (ui *BrowserUI) layoutShadowedText(gtx layout.Context, text string, size float32, bold bool, color uint32, decoration stylemodel.TextDecorationLine, decorationColor uint32, baseline float32, shadows []stylemodel.Shadow) layout.Dimensions {
	labelLayout := func(color uint32) layout.Widget {
		return func(gtx layout.Context) layout.Dimensions {
			label := material.Label(ui.theme, unit.Sp(size), text)
			label.Color, label.MaxLines = rgba(color), 1
			if bold {
				label.Font.Weight = font.Bold
			}
			return layoutDecoratedLabel(gtx, label.Layout, decoration, decorationColor, baseline, size)
		}
	}
	if len(shadows) == 0 {
		return labelLayout(color)(gtx)
	}
	children := make([]layout.StackChild, 0, len(shadows)+1)
	for _, shadow := range shadows {
		shadow := shadow
		children = append(children, layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(shadow.OffsetY), Left: unit.Dp(shadow.OffsetX)}.Layout(gtx, labelLayout(shadow.Color))
		}))
	}
	children = append(children, layout.Stacked(labelLayout(color)))
	return layout.Stack{Alignment: layout.NW}.Layout(gtx, children...)
}

func layoutDecoratedLabel(gtx layout.Context, label layout.Widget, decoration stylemodel.TextDecorationLine, decorationColor uint32, baseline, fontSize float32) layout.Dimensions {
	macro := op.Record(gtx.Ops)
	dimensions := label(gtx)
	call := macro.Stop()
	call.Add(gtx.Ops)
	if decoration == stylemodel.TextDecorationNone || dimensions.Size.X <= 0 {
		return dimensions
	}
	thickness := max(gtx.Dp(unit.Dp(fontSize/16)), 1)
	baselinePixels := gtx.Dp(unit.Dp(baseline))
	drawLine := func(y int) {
		y = min(max(y, 0), max(dimensions.Size.Y-thickness, 0))
		paint.FillShape(gtx.Ops, rgba(decorationColor), clip.Rect{Min: image.Pt(0, y), Max: image.Pt(dimensions.Size.X, y+thickness)}.Op())
	}
	if decoration&stylemodel.TextDecorationOverline != 0 {
		drawLine(gtx.Dp(unit.Dp(fontSize * .1)))
	}
	if decoration&stylemodel.TextDecorationLineThrough != 0 {
		drawLine(baselinePixels - gtx.Dp(unit.Dp(fontSize*.3)))
	}
	if decoration&stylemodel.TextDecorationUnderline != 0 {
		drawLine(baselinePixels + thickness)
	}
	return dimensions
}

func rgba(value uint32) color.NRGBA {
	return color.NRGBA{
		R: uint8(value >> 24),
		G: uint8(value >> 16),
		B: uint8(value >> 8),
		A: uint8(value),
	}
}
