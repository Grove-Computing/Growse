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

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"golang.org/x/exp/shiny/materialdesign/icons"

	"github.com/saku0512/growse/internal/browser"
)

//go:embed assets/gopher-blue.png
var gopherPNG []byte

const (
	defaultURL         = "http://localhost:8080"
	toolbarHeight      = unit.Dp(72)
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

	backButton    widget.Clickable
	forwardButton widget.Clickable
	reloadButton  widget.Clickable
	goButton      widget.Clickable
	address       widget.Editor
	gopher        paint.ImageOp
	backIcon      *widget.Icon
	forwardIcon   *widget.Icon
	reloadIcon    *widget.Icon
	status        string
}

// Navigator is the browser capability used by the UI.
type Navigator interface {
	Navigate(ctx context.Context, rawURL string) (*browser.Page, error)
	Page() *browser.Page
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

	ui := &BrowserUI{
		theme:       material.NewTheme(),
		navigator:   navigator,
		invalidate:  invalidate,
		results:     make(chan navigationResult, 1),
		gopher:      paint.NewImageOp(gopherImage),
		backIcon:    mustIcon(widget.NewIcon(icons.NavigationArrowBack)),
		forwardIcon: mustIcon(widget.NewIcon(icons.NavigationArrowForward)),
		reloadIcon:  mustIcon(widget.NewIcon(icons.NavigationRefresh)),
		status:      "URLを入力して Gopher ボタンを押してください",
	}
	if ui.invalidate == nil {
		ui.invalidate = func() {}
	}
	ui.address.SingleLine = true
	ui.address.SetText(defaultURL)
	return ui
}

// Layout draws the browser toolbar and page viewport.
func (ui *BrowserUI) Layout(gtx layout.Context) layout.Dimensions {
	ui.handleActions(gtx)

	viewport := layout.Inset{Top: toolbarHeight}.Layout(gtx, ui.layoutViewport)
	ui.layoutToolbar(gtx)
	return viewport
}

func (ui *BrowserUI) handleActions(gtx layout.Context) {
	ui.consumeNavigationResult()

	for ui.goButton.Clicked(gtx) {
		ui.startNavigation(ui.address.Text())
	}
	for ui.backButton.Clicked(gtx) {
		ui.status = "Back はまだ未実装です"
	}
	for ui.forwardButton.Clicked(gtx) {
		ui.status = "Forward はまだ未実装です"
	}
	for ui.reloadButton.Clicked(gtx) {
		rawURL := ui.address.Text()
		if ui.navigator != nil && ui.navigator.Page() != nil && ui.navigator.Page().URL != nil {
			rawURL = ui.navigator.Page().URL.String()
		}
		ui.startNavigation(rawURL)
	}
}

func (ui *BrowserUI) startNavigation(rawURL string) {
	if ui.navigator == nil {
		ui.status = "Navigationを利用できません"
		return
	}
	if ui.cancelNavigation != nil {
		ui.cancelNavigation()
	}

	ctx, cancel := context.WithCancel(context.Background())
	ui.cancelNavigation = cancel
	ui.navigationID++
	navigationID := ui.navigationID
	ui.loading = true
	ui.status = "読み込み中: " + rawURL

	go func() {
		page, err := ui.navigator.Navigate(ctx, rawURL)
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
			if result.err != nil {
				ui.status = "読み込みエラー: " + result.err.Error()
				return
			}
			if result.page == nil || result.page.URL == nil {
				ui.status = "読み込みエラー: ページ情報がありません"
				return
			}

			ui.address.SetText(result.page.URL.String())
			ui.status = fmt.Sprintf("取得完了 · HTTP %d · %d bytes · %s", result.page.StatusCode, len(result.page.Source), result.page.ContentType)
		default:
			return
		}
	}
}

// Close cancels an in-flight navigation when the window closes.
func (ui *BrowserUI) Close() {
	ui.navigationID++
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

	return layout.Inset{Top: unit.Dp(10), Right: unit.Dp(14), Bottom: unit.Dp(10), Left: unit.Dp(14)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return ui.layoutToolbarButton(gtx, &ui.backButton, ui.backIcon, "戻る")
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return ui.layoutToolbarButton(gtx, &ui.forwardButton, ui.forwardIcon, "次へ")
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return ui.layoutToolbarButton(gtx, &ui.reloadButton, ui.reloadIcon, "再読込")
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(14)}.Layout),
			layout.Flexed(1, ui.layoutAddressBar),
			layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
			layout.Rigid(ui.layoutGopherButton),
		)
	})
}

func (ui *BrowserUI) layoutToolbarButton(gtx layout.Context, button *widget.Clickable, icon *widget.Icon, description string) layout.Dimensions {
	size := gtx.Dp(controlHeight)
	gtx.Constraints = layout.Exact(image.Pt(size, size))

	style := material.IconButton(ui.theme, button, icon, description)
	style.Background = color.NRGBA{R: 228, G: 234, B: 242, A: 255}
	style.Color = color.NRGBA{R: 52, G: 64, B: 84, A: 255}
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
							layout.Rigid(material.H5(ui.theme, "新しい Web を Go で開く").Layout),
							layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
							layout.Rigid(material.Body1(ui.theme, ui.status).Layout),
						)
					})
				}),
			)
		})
	})
}
