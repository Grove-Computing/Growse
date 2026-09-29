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
	"github.com/Grove-Computing/Growse/internal/omnibox"
	"github.com/Grove-Computing/Growse/internal/searchdata"
)

type homeTabState struct {
	visible         bool
	editor          *widget.Editor
	search          widget.Clickable
	focusPending    bool
	errorMessage    string
	pipeline        *omnibox.Pipeline
	local           *searchdata.LocalPipeline
	generation      uint64
	localGeneration uint64
	localApplied    uint64
	input           string
	providerKeyword string
	candidates      []omnibox.Candidate
	selected        int
	rows            [omnibox.MaxVisibleCandidates]widget.Clickable
	list            widget.List
}

func newHomeTabState(visible bool) *homeTabState {
	editor := new(widget.Editor)
	editor.SingleLine = true
	editor.Submit = true
	state := &homeTabState{visible: visible, editor: editor, focusPending: visible, selected: -1}
	state.list.Axis = layout.Vertical
	return state
}

func (ui *BrowserUI) ensureHomePipelines(state *homeTabState) {
	if state.pipeline == nil {
		state.pipeline = omnibox.NewPipeline(ui.invalidate)
	}
	if state.local == nil {
		state.local = searchdata.NewLocalPipeline(ui.searchData, ui.invalidate)
	}
}

func (state *homeTabState) closeSuggestions() {
	if state.pipeline != nil {
		state.pipeline.Close()
		state.pipeline = nil
	}
	if state.local != nil {
		state.local.Close()
		state.local = nil
	}
	state.candidates = nil
	state.selected = -1
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
	ui.handleHomeSuggestionKeys(gtx, state)
	for {
		event, ok := state.editor.Update(gtx)
		if !ok {
			break
		}
		switch event.(type) {
		case widget.ChangeEvent:
			ui.refreshHomeSuggestions(state)
		case widget.SubmitEvent:
			ui.submitHomeInput(state, omniboxCurrentTab)
		}
	}
	for state.search.Clicked(gtx) {
		ui.submitHomeInput(state, omniboxCurrentTab)
	}
	for index := range state.candidates {
		if state.rows[index].Clicked(gtx) {
			ui.submitHomeCandidate(state, state.candidates[index], omniboxCurrentTab)
			return
		}
		if state.rows[index].Hovered() {
			state.selected = index
		}
	}
}

func (ui *BrowserUI) handleHomeSuggestionKeys(gtx layout.Context, state *homeTabState) {
	for _, modifiers := range []key.Modifiers{key.ModShift, key.ModAlt} {
		for {
			event, ok := gtx.Event(
				key.Filter{Focus: state.editor, Name: key.NameReturn, Required: modifiers},
				key.Filter{Focus: state.editor, Name: key.NameEnter, Required: modifiers},
			)
			if !ok {
				break
			}
			if pressed, ok := event.(key.Event); ok && pressed.State == key.Press {
				disposition := omniboxNewForegroundTab
				if modifiers == key.ModAlt {
					disposition = omniboxNewBackgroundTab
				}
				ui.submitHomeInput(state, disposition)
			}
		}
	}
	for _, name := range []key.Name{key.NameDownArrow, key.NameUpArrow, key.NamePageDown, key.NamePageUp, key.NameHome, key.NameEnd, key.NameEscape, key.NameTab} {
		for {
			event, ok := gtx.Event(key.Filter{Focus: state.editor, Name: name, Optional: key.ModShift})
			if !ok {
				break
			}
			pressed, ok := event.(key.Event)
			if !ok || pressed.State != key.Press {
				continue
			}
			if name == key.NameEscape {
				if state.pipeline != nil {
					state.pipeline.Cancel()
				}
				if state.local != nil {
					state.local.Cancel()
				}
				state.candidates = nil
				state.selected = -1
				continue
			}
			if name == key.NameTab {
				gtx.Execute(key.FocusCmd{Tag: &state.search})
				continue
			}
			if len(state.candidates) == 0 {
				ui.refreshHomeSuggestions(state)
			}
			if len(state.candidates) == 0 {
				continue
			}
			index := state.selected
			switch name {
			case key.NameDownArrow:
				index = (index + 1) % len(state.candidates)
			case key.NameUpArrow:
				if index <= 0 {
					index = len(state.candidates) - 1
				} else {
					index--
				}
			case key.NamePageDown:
				index = min(max(index, 0)+5, len(state.candidates)-1)
			case key.NamePageUp:
				index = max(index-5, 0)
			case key.NameHome:
				index = 0
			case key.NameEnd:
				index = len(state.candidates) - 1
			}
			state.selected = index
			state.list.ScrollTo(index)
		}
	}
}

func (ui *BrowserUI) refreshHomeSuggestions(state *homeTabState) {
	ui.ensureHomePipelines(state)
	state.input = state.editor.Text()
	query := state.input
	if classification := omnibox.Classify(query); classification.Kind == omnibox.Command {
		query = classification.Query
	}
	state.localGeneration = state.local.Update(query)
	state.localApplied = 0
	request := ui.buildSuggestionRequest(state.input, ui.suggestionSnapshot)
	state.providerKeyword = request.providerKeyword
	state.generation = state.pipeline.Update(request.input, request.snapshot, request.fetch, request.remoteEnabled)
	state.selected = -1
	ui.syncHomeSuggestions(state)
}

func (ui *BrowserUI) syncHomeSuggestions(state *homeTabState) {
	ui.ensureHomePipelines(state)
	if state.localGeneration != 0 && state.localApplied != state.localGeneration {
		if generation, local := state.local.Results(); generation == state.localGeneration {
			local.Now = ui.suggestionSnapshot.Now
			local.History = append(append([]omnibox.Candidate(nil), ui.suggestionSnapshot.History...), local.History...)
			local.Bookmarks = append(append([]omnibox.Candidate(nil), ui.suggestionSnapshot.Bookmarks...), local.Bookmarks...)
			state.localApplied = generation
			request := ui.buildSuggestionRequest(state.input, local)
			state.providerKeyword = request.providerKeyword
			state.generation = state.pipeline.Update(request.input, request.snapshot, request.fetch, request.remoteEnabled)
		}
	}
	generation, candidates := state.pipeline.Results()
	if generation != state.generation {
		return
	}
	if len(candidates) > omnibox.MaxVisibleCandidates {
		candidates = candidates[:omnibox.MaxVisibleCandidates]
	}
	state.candidates = candidates
	if state.selected >= len(candidates) {
		state.selected = -1
	}
}

func (ui *BrowserUI) submitHomeSearch(state *homeTabState) {
	ui.submitHomeInput(state, omniboxCurrentTab)
}

func (ui *BrowserUI) submitHomeInput(state *homeTabState, disposition omniboxDisposition) {
	input := strings.TrimSpace(state.editor.Text())
	if input == "" {
		state.errorMessage = "検索語またはURLを入力してください"
		ui.status = state.errorMessage
		ui.statusHasError = true
		return
	}
	state.errorMessage = ""
	if state.selected >= 0 && state.selected < len(state.candidates) {
		ui.submitHomeCandidate(state, state.candidates[state.selected], disposition)
		return
	}
	ui.startNavigationWithDisposition(input, disposition)
}

func (ui *BrowserUI) submitHomeCandidate(state *homeTabState, candidate omnibox.Candidate, disposition omniboxDisposition) {
	query := state.editor.Text()
	ui.executeSuggestion(candidate, state.providerKeyword, disposition)
	state.editor.SetText(query)
}

func (ui *BrowserUI) layoutHome(gtx layout.Context) layout.Dimensions {
	ui.imagePaintCache.prepare(nil)
	paint.Fill(gtx.Ops, color.NRGBA{R: 238, G: 243, B: 248, A: 255})
	tabID, _ := ui.activeNavigationTarget()
	state := ui.homeState(tabID)
	ui.syncHomeSuggestions(state)
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
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return ui.layoutHomeCandidates(gtx, state)
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

func (ui *BrowserUI) layoutHomeCandidates(gtx layout.Context, state *homeTabState) layout.Dimensions {
	if len(state.candidates) == 0 || state.editor.Text() == "" {
		return layout.Dimensions{}
	}
	height := min(gtx.Dp(unit.Dp(48))*len(state.candidates), gtx.Dp(unit.Dp(192)))
	gtx.Constraints.Min.Y = height
	gtx.Constraints.Max.Y = height
	return material.List(ui.theme, &state.list).Layout(gtx, len(state.candidates), func(gtx layout.Context, index int) layout.Dimensions {
		candidate := state.candidates[index]
		return state.rows[index].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			background := color.NRGBA{R: 248, G: 250, B: 252, A: 255}
			if index == state.selected {
				background = color.NRGBA{R: 219, G: 234, B: 254, A: 255}
			}
			paint.FillShape(gtx.Ops, background, clip.Rect{Max: gtx.Constraints.Min}.Op())
			return layout.Inset{Top: unit.Dp(6), Right: unit.Dp(10), Bottom: unit.Dp(6), Left: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(material.Body2(ui.theme, candidate.Primary).Layout),
					layout.Rigid(material.Caption(ui.theme, candidate.Source.Label()+" · "+candidate.Secondary).Layout),
				)
			})
		})
	})
}
