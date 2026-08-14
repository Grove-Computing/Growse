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

	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/gesture"
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
	"github.com/Grove-Computing/Growse/internal/dom"
	layoutengine "github.com/Grove-Computing/Growse/internal/layout"
	paintmodel "github.com/Grove-Computing/Growse/internal/paint"
	stylemodel "github.com/Grove-Computing/Growse/internal/style"
)

//go:embed assets/gopher-blue.png
var gopherPNG []byte

const (
	defaultURL         = "http://localhost:8080"
	toolbarHeight      = unit.Dp(92)
	controlHeight      = unit.Dp(44)
	addressBarHeight   = unit.Dp(48)
	gopherButtonWidth  = unit.Dp(92)
	gopherButtonHeight = unit.Dp(52)
)

// BrowserUI owns the widgets displayed around the page viewport.
type BrowserUI struct {
	theme            *material.Theme
	navigator        Navigator
	invalidate       func()
	results          chan navigationResult
	cancelNavigation context.CancelFunc
	navigationID     uint64
	loading          bool

	backButton        widget.Clickable
	forwardButton     widget.Clickable
	reloadButton      widget.Clickable
	goButton          widget.Clickable
	pageList          widget.List
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
}

// Navigator is the browser capability used by the UI.
type Navigator interface {
	Navigate(ctx context.Context, rawURL string) (*browser.Page, error)
	Back(ctx context.Context) (*browser.Page, error)
	Forward(ctx context.Context) (*browser.Page, error)
	Reload(ctx context.Context) (*browser.Page, error)
	CanBack() bool
	CanForward() bool
	Page() *browser.Page
	DispatchClick(nodeID dom.NodeID, x, y float32) bool
	SetInputValue(nodeID dom.NodeID, value string) bool
	CommitInputValue(nodeID dom.NodeID, value string) bool
	SubmitForm(nodeID dom.NodeID) bool
	UpdateHover(nodeID dom.NodeID, x, y float32) bool
	ClearHover() bool
	UpdateFocus(nodeID dom.NodeID) bool
	UpdateViewport(width, height float32) bool
}

type navigationResult struct {
	id   uint64
	page *browser.Page
	err  error
}

// NewBrowserUI creates a browser toolbar and an empty viewport.
func NewBrowserUI(navigator Navigator, invalidate func()) *BrowserUI {
	gopherImage, err := png.Decode(bytes.NewReader(gopherPNG))
	if err != nil {
		panic("decode embedded Go Gopher image: " + err.Error())
	}

	cursorImage, cursorErr := loadGopherCursor()
	ui := &BrowserUI{
		theme:          material.NewTheme(),
		navigator:      navigator,
		invalidate:     invalidate,
		results:        make(chan navigationResult, 1),
		gopher:         paint.NewImageOp(gopherImage),
		backIcon:       mustIcon(widget.NewIcon(icons.NavigationArrowBack)),
		forwardIcon:    mustIcon(widget.NewIcon(icons.NavigationArrowForward)),
		reloadIcon:     mustIcon(widget.NewIcon(icons.NavigationRefresh)),
		pageTitle:      "新しい Web を Go で開く",
		status:         "URLを入力して Gopher ボタンを押してください",
		pageStatus:     "URLを入力して Gopher ボタンを押してください",
		inputEditors:   make(map[dom.NodeID]*widget.Editor),
		inputFocused:   make(map[dom.NodeID]bool),
		inputCommitted: make(map[dom.NodeID]string),
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
	return ui
}

// Layout draws the browser toolbar and page viewport.
func (ui *BrowserUI) Layout(gtx layout.Context) layout.Dimensions {
	ui.handlePointerEvents(gtx)
	ui.handleActions(gtx)

	viewport := layout.Inset{Top: toolbarHeight}.Layout(gtx, ui.layoutViewport)
	ui.layoutToolbar(gtx)
	ui.layoutGopherCursor(gtx)
	ui.registerPointerTracker(gtx)
	return viewport
}

func (ui *BrowserUI) handleActions(gtx layout.Context) {
	ui.consumeNavigationResult()

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
		if ui.navigator != nil && ui.navigator.CanBack() {
			ui.startPageLoad("前のページを読み込み中", ui.navigator.Back)
		}
	}
	for ui.forwardButton.Clicked(gtx) {
		if ui.navigator != nil && ui.navigator.CanForward() {
			ui.startPageLoad("次のページを読み込み中", ui.navigator.Forward)
		}
	}
	for ui.reloadButton.Clicked(gtx) {
		if ui.navigator != nil && ui.navigator.Page() != nil {
			ui.startPageLoad("ページを再読み込み中", ui.navigator.Reload)
		}
	}
}

func (ui *BrowserUI) startNavigation(rawURL string) {
	if ui.navigator == nil {
		ui.status = "Navigationを利用できません"
		ui.statusHasError = true
		return
	}
	ui.startPageLoad("読み込み中: "+rawURL, func(ctx context.Context) (*browser.Page, error) {
		return ui.navigator.Navigate(ctx, rawURL)
	})
}

func (ui *BrowserUI) startPageLoad(status string, load func(context.Context) (*browser.Page, error)) {
	if ui.navigator != nil {
		ui.navigator.ClearHover()
	}
	if ui.cancelNavigation != nil {
		ui.cancelNavigation()
	}

	ctx, cancel := context.WithCancel(context.Background())
	ui.cancelNavigation = cancel
	ui.navigationID++
	navigationID := ui.navigationID
	ui.loading = true
	ui.statusHasError = false
	ui.status = status

	go func() {
		page, err := load(ctx)
		ui.results <- navigationResult{id: navigationID, page: page, err: err}
		ui.invalidate()
	}()
}

func (ui *BrowserUI) consumeNavigationResult() {
	for {
		select {
		case result := <-ui.results:
			if result.id != ui.navigationID {
				continue
			}
			ui.loading = false
			ui.cancelNavigation = nil
			ui.inputEditors = make(map[dom.NodeID]*widget.Editor)
			ui.inputFocused = make(map[dom.NodeID]bool)
			ui.inputCommitted = make(map[dom.NodeID]string)
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
			ui.pageList.Position = layout.Position{}
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

// Close cancels an in-flight navigation when the window closes.
func (ui *BrowserUI) Close() {
	ui.navigationID++
	if ui.navigator != nil {
		ui.navigator.ClearHover()
	}
	if ui.cancelNavigation != nil {
		ui.cancelNavigation()
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

	return layout.Inset{Top: unit.Dp(8), Right: unit.Dp(14), Bottom: unit.Dp(6), Left: unit.Dp(14)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		canBack := ui.navigator != nil && ui.navigator.CanBack()
		canForward := ui.navigator != nil && ui.navigator.CanForward()
		canReload := ui.navigator != nil && ui.navigator.Page() != nil
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return ui.layoutToolbarButton(gtx, &ui.backButton, ui.backIcon, "戻る", canBack)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return ui.layoutToolbarButton(gtx, &ui.forwardButton, ui.forwardIcon, "次へ", canForward)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return ui.layoutToolbarButton(gtx, &ui.reloadButton, ui.reloadIcon, "再読込", canReload)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(14)}.Layout),
					layout.Flexed(1, ui.layoutAddressBar),
					layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
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

	viewportWidth := float32(gtx.Constraints.Max.X) / gtx.Metric.PxPerDp
	viewportHeight := float32(gtx.Constraints.Max.Y) / gtx.Metric.PxPerDp
	if ui.navigator != nil {
		ui.navigator.UpdateViewport(viewportWidth, viewportHeight)
	}
	tree := layoutengine.BuildWithViewport(page.Document, page.ComputedStyles, viewportWidth, viewportHeight)
	displayList := paintmodel.Build(tree)
	if first := ui.pageList.Position.First; first >= 0 && first < len(displayList.Commands) {
		if firstY, ok := commandDocumentY(displayList.Commands[first]); ok {
			scrollY := max(firstY+float32(ui.pageList.Position.Offset)/gtx.Metric.PxPerDp, float32(0))
			if scrollY > 0 {
				tree = layoutengine.BuildWithScroll(page.Document, page.ComputedStyles, viewportWidth, viewportHeight, 0, scrollY)
				displayList = paintmodel.Build(tree)
			}
		}
	}
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
	return dimensions
}

func (ui *BrowserUI) updateViewportHover(gtx layout.Context, page *browser.Page, tree *layoutengine.Tree, displayList *paintmodel.DisplayList) {
	if ui.navigator == nil {
		return
	}
	viewportY := ui.pointer.position.Y - float32(gtx.Dp(toolbarHeight))
	if !ui.pointer.inside || viewportY < 0 || viewportY >= float32(gtx.Constraints.Max.Y) {
		ui.navigator.ClearHover()
		ui.updateLinkPreview(page, 0)
		return
	}
	position := image.Pt(int(math.Round(float64(ui.pointer.position.X))), int(math.Round(float64(viewportY))))
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

func (ui *BrowserUI) updateLinkPreview(page *browser.Page, nodeID dom.NodeID) {
	if ui.loading || ui.statusHasError {
		return
	}
	if linkURL, ok := page.LinkURL(nodeID); ok {
		ui.status = linkURL.Redacted()
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
		ui.navigator.UpdateFocus(focusableNodeID(page.Document, nodeID))
		if ui.navigator.DispatchClick(nodeID, x, y) {
			continue
		}
		linkURL, ok := page.LinkURL(nodeID)
		if !ok {
			continue
		}
		ui.startNavigation(linkURL.String())
	}
}

func focusableNodeID(document *dom.Document, nodeID dom.NodeID) dom.NodeID {
	if document == nil {
		return 0
	}
	node, ok := document.NodeByID(nodeID)
	if !ok {
		return 0
	}
	for current := node; current != nil; current = current.Parent {
		if current.Type != dom.NodeElement {
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
		height := gtx.Dp(unit.Dp(command.Height))
		gtx.Constraints.Min.Y = height
		gtx.Constraints.Max.Y = height
		editor := ui.inputEditors[command.NodeID]
		wasFocused := ui.inputFocused[command.NodeID]
		if editor == nil {
			editor = new(widget.Editor)
			editor.SingleLine = true
			editor.Submit = true
			editor.SetText(command.Value)
			ui.inputEditors[command.NodeID] = editor
			ui.inputCommitted[command.NodeID] = command.Value
		} else if editor.Text() != command.Value {
			editor.SetText(command.Value)
			ui.inputCommitted[command.NodeID] = command.Value
		}
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
				ui.navigator.UpdateFocus(0)
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
