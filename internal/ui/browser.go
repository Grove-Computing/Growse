// Package ui provides Gio widgets for the Growse browser chrome.
package ui

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log/slog"
	"math"

	"gioui.org/font"
	"gioui.org/gesture"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"golang.org/x/exp/shiny/materialdesign/icons"

	"github.com/saku0512/growse/internal/browser"
	"github.com/saku0512/growse/internal/dom"
	layoutengine "github.com/saku0512/growse/internal/layout"
	paintmodel "github.com/saku0512/growse/internal/paint"
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
	tree := layoutengine.Build(page.Document, page.ComputedStyles, viewportWidth)
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
	default:
		return 0, false
	}
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

		label := material.Label(ui.theme, unit.Sp(command.FontSize), command.Text)
		label.Color = rgba(command.Color)
		label.MaxLines = 1
		if command.Bold {
			label.Font.Weight = font.Bold
		}
		return layout.W.Layout(gtx, label.Layout)
	})
}

func (ui *BrowserUI) layoutTextRun(gtx layout.Context, run paintmodel.TextRun, height int) layout.Dimensions {
	gtx.Constraints.Min.X = 0
	gtx.Constraints.Min.Y = height
	gtx.Constraints.Max.Y = height
	text := func(gtx layout.Context) layout.Dimensions {
		label := material.Label(ui.theme, unit.Sp(run.FontSize), run.Text)
		label.Color = rgba(run.Color)
		label.MaxLines = 1
		if run.Bold {
			label.Font.Weight = font.Bold
		}
		return layout.W.Layout(gtx, label.Layout)
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

func rgba(value uint32) color.NRGBA {
	return color.NRGBA{
		R: uint8(value >> 24),
		G: uint8(value >> 16),
		B: uint8(value >> 8),
		A: uint8(value),
	}
}
