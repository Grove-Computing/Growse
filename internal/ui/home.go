package ui

import (
	"image"
	"image/color"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/Grove-Computing/Growse/internal/browser"
)

// homeVisible reports whether the active tab is displaying browser-owned
// content. Empty tabs become home tabs without initiating a navigation.
func (ui *BrowserUI) homeVisible() bool {
	tabID, navigator := ui.activeNavigationTarget()
	if visible, ok := ui.homeTabs[tabID]; ok {
		return visible
	}
	if ui.tabs != nil {
		active, ok := ui.tabs.ActiveTab()
		if !ok {
			return false
		}
		visible := active.URL == "" && (navigator == nil || navigator.Page() == nil)
		ui.homeTabs[active.ID] = visible
		return visible
	}
	return navigator == nil || navigator.Page() == nil
}

func (ui *BrowserUI) showHome() {
	tabID, navigator := ui.activeNavigationTarget()
	ui.cancelTabNavigation(tabID)
	if navigator != nil {
		navigator.ClearHover()
	}
	ui.closeSuggestionPopup()
	ui.homeTabs[tabID] = true
	ui.loading = false
	ui.statusHasError = false
	ui.pageTitle = "新しいタブ"
	ui.status = "Growse ホーム"
	ui.pageStatus = ui.status
	ui.setCommittedOmniboxURL(tabID, "", true)
	ui.invalidate()
}

func (ui *BrowserUI) layoutHome(gtx layout.Context) layout.Dimensions {
	ui.imagePaintCache.prepare(nil)
	paint.Fill(gtx.Ops, color.NRGBA{R: 238, G: 243, B: 248, A: 255})
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		maxWidth := gtx.Dp(unit.Dp(560))
		if gtx.Constraints.Max.X > maxWidth {
			gtx.Constraints.Max.X = maxWidth
		}
		return widget.Border{
			Color: color.NRGBA{R: 211, G: 218, B: 228, A: 255}, CornerRadius: unit.Dp(20), Width: unit.Dp(1),
		}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Stack{Alignment: layout.Center}.Layout(gtx,
				layout.Expanded(func(gtx layout.Context) layout.Dimensions {
					paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255},
						clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, gtx.Dp(unit.Dp(20))).Op(gtx.Ops))
					return layout.Dimensions{Size: gtx.Constraints.Min}
				}),
				layout.Stacked(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(44), Right: unit.Dp(56), Bottom: unit.Dp(44), Left: unit.Dp(56)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(88), gtx.Dp(72)))
								return widget.Image{Src: ui.gopher, Fit: widget.Contain, Position: layout.Center}.Layout(gtx)
							}),
							layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
							layout.Rigid(material.H3(ui.theme, "Growse").Layout),
							layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
							layout.Rigid(material.Body1(ui.theme, "新しいWebを検索またはURLで開く").Layout),
						)
					})
				}),
			)
		})
	})
}

func (ui *BrowserUI) setHomeVisible(tabID browser.TabID, visible bool) {
	ui.homeTabs[tabID] = visible
}
