package ui

import (
	"image"
	"image/color"
	"strings"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/Grove-Computing/Growse/internal/browser"
)

type homeTabState struct {
	visible      bool
	editor       *widget.Editor
	search       widget.Clickable
	focusPending bool
	errorMessage string
}

func newHomeTabState(visible bool) *homeTabState {
	editor := new(widget.Editor)
	editor.SingleLine = true
	editor.Submit = true
	return &homeTabState{visible: visible, editor: editor, focusPending: visible}
}

func (ui *BrowserUI) homeState(tabID browser.TabID) *homeTabState {
	if state := ui.homeTabs[tabID]; state != nil {
		return state
	}
	visible := false
	if ui.tabs != nil {
		if active, ok := ui.tabs.ActiveTab(); ok && active.ID == tabID {
			_, navigator := ui.activeNavigationTarget()
			visible = active.URL == "" && (navigator == nil || navigator.Page() == nil)
		}
	} else {
		_, navigator := ui.activeNavigationTarget()
		visible = navigator == nil || navigator.Page() == nil
	}
	state := newHomeTabState(visible)
	ui.homeTabs[tabID] = state
	return state
}

func (ui *BrowserUI) homeVisible() bool {
	tabID, _ := ui.activeNavigationTarget()
	return ui.homeState(tabID).visible
}

func (ui *BrowserUI) showHome() {
	tabID, navigator := ui.activeNavigationTarget()
	ui.cancelTabNavigation(tabID)
	if navigator != nil {
		navigator.ClearHover()
	}
	ui.closeSuggestionPopup()
	state := ui.homeState(tabID)
	state.visible = true
	state.focusPending = true
	state.errorMessage = ""
	ui.loading = false
	ui.statusHasError = false
	ui.pageTitle = "新しいタブ"
	ui.status = "Growse ホーム"
	ui.pageStatus = ui.status
	ui.setCommittedOmniboxURL(tabID, "", true)
	ui.invalidate()
}

func (ui *BrowserUI) handleHomeActions(gtx layout.Context) {
	if !ui.homeVisible() {
		return
	}
	tabID, _ := ui.activeNavigationTarget()
	state := ui.homeState(tabID)
	for {
		event, ok := state.editor.Update(gtx)
		if !ok {
			break
		}
		if _, ok := event.(widget.SubmitEvent); ok {
			ui.submitHomeSearch(state)
		}
	}
	for state.search.Clicked(gtx) {
		ui.submitHomeSearch(state)
	}
}

func (ui *BrowserUI) submitHomeSearch(state *homeTabState) {
	input := strings.TrimSpace(state.editor.Text())
	if input == "" {
		state.errorMessage = "検索語またはURLを入力してください"
		ui.status = state.errorMessage
		ui.statusHasError = true
		return
	}
	state.errorMessage = ""
	ui.startNavigationWithDisposition(input, omniboxCurrentTab)
}

func (ui *BrowserUI) layoutHome(gtx layout.Context) layout.Dimensions {
	ui.imagePaintCache.prepare(nil)
	paint.Fill(gtx.Ops, color.NRGBA{R: 238, G: 243, B: 248, A: 255})
	tabID, _ := ui.activeNavigationTarget()
	state := ui.homeState(tabID)
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		maxWidth := gtx.Dp(unit.Dp(620))
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
					return layout.Inset{Top: unit.Dp(40), Right: unit.Dp(48), Bottom: unit.Dp(40), Left: unit.Dp(48)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						dims := layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(88), gtx.Dp(72)))
								return widget.Image{Src: ui.gopher, Fit: widget.Contain, Position: layout.Center}.Layout(gtx)
							}),
							layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
							layout.Rigid(material.H3(ui.theme, "Growse").Layout),
							layout.Rigid(layout.Spacer{Height: unit.Dp(22)}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return widget.Border{Color: color.NRGBA{R: 148, G: 163, B: 184, A: 255}, CornerRadius: unit.Dp(14), Width: unit.Dp(1)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return layout.Inset{Top: unit.Dp(10), Right: unit.Dp(14), Bottom: unit.Dp(10), Left: unit.Dp(14)}.Layout(gtx,
										material.Editor(ui.theme, state.editor, "検索語またはURLを入力").Layout)
								})
							}),
							layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								button := material.Button(ui.theme, &state.search, ui.homeSearchLabel())
								button.CornerRadius = unit.Dp(12)
								return button.Layout(gtx)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								if state.errorMessage == "" {
									return layout.Dimensions{}
								}
								return layout.Inset{Top: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									label := material.Body2(ui.theme, state.errorMessage)
									label.Color = color.NRGBA{R: 185, G: 28, B: 28, A: 255}
									return label.Layout(gtx)
								})
							}),
						)
						if state.focusPending {
							state.focusPending = false
							gtx.Execute(key.FocusCmd{Tag: state.editor})
						}
						return dims
					})
				}),
			)
		})
	})
}

func (ui *BrowserUI) setHomeVisible(tabID browser.TabID, visible bool) {
	state := ui.homeState(tabID)
	state.visible = visible
	if visible {
		state.focusPending = true
	}
}

func (ui *BrowserUI) homeSearchLabel() string {
	provider := ui.providers.Default().Name
	if provider == "" {
		provider = "検索"
	}
	return provider + " で検索"
}
