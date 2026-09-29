package ui

import (
	"fmt"
	"image"
	"image/color"
	"strings"

	"gioui.org/io/key"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/Grove-Computing/Growse/internal/browser"
	"github.com/Grove-Computing/Growse/internal/omnibox"
)

type searchPanelState struct {
	open            bool
	editor          *widget.Editor
	scope           omnibox.Scope
	query           string
	localGeneration uint64
	localApplied    uint64
	candidates      []omnibox.Candidate
	selected        int
	list            widget.List
	allButton       widget.Clickable
	tabsButton      widget.Clickable
	historyButton   widget.Clickable
	bookmarksButton widget.Clickable
	closeButton     widget.Clickable
	rows            [omnibox.MaxMergedCandidates]widget.Clickable
}

func newSearchPanelState() searchPanelState {
	editor := new(widget.Editor)
	editor.SingleLine = true
	editor.Submit = true
	state := searchPanelState{editor: editor, selected: -1}
	state.list.Axis = layout.Vertical
	return state
}

func (ui *BrowserUI) handleSearchPanelKeyboardShortcuts(gtx layout.Context) {
	for {
		event, ok := gtx.Event(key.Filter{Name: "A", Required: key.ModShortcut | key.ModShift})
		if !ok {
			break
		}
		keyEvent, ok := event.(key.Event)
		if !ok || keyEvent.State != key.Press {
			continue
		}
		ui.openSearchPanel(gtx)
	}
	panel := &ui.searchPanel
	if !panel.open {
		return
	}
	for {
		event, ok := gtx.Event(key.Filter{Focus: panel.editor, Name: key.NameEscape})
		if !ok {
			break
		}
		keyEvent, ok := event.(key.Event)
		if ok && keyEvent.State == key.Press {
			ui.closeSearchPanel(gtx)
		}
	}
}

func (ui *BrowserUI) openSearchPanel(gtx layout.Context) {
	panel := &ui.searchPanel
	panel.open = true
	panel.editor.SetCaret(0, panel.editor.Len())
	ui.providerPanel.open = false
	ui.closeSuggestionPopup()
	ui.refreshSearchPanel()
	gtx.Execute(key.FocusCmd{Tag: panel.editor})
}

func (ui *BrowserUI) closeSearchPanel(gtx layout.Context) {
	panel := &ui.searchPanel
	panel.open = false
	panel.selected = -1
	panel.candidates = nil
	if ui.searchPanelSuggestions != nil {
		ui.searchPanelSuggestions.Cancel()
	}
	gtx.Execute(key.FocusCmd{Tag: ui.address})
}

func (ui *BrowserUI) refreshSearchPanel() {
	panel := &ui.searchPanel
	panel.query = panel.editor.Text()
	panel.localApplied = 0
	panel.selected = -1
	panel.list.Position = layout.Position{}
	if ui.searchPanelSuggestions != nil {
		panel.localGeneration = ui.searchPanelSuggestions.Update(panel.query)
	} else {
		panel.localGeneration = 0
	}
	ui.updateSearchPanelResults(omnibox.Snapshot{})
}

func (ui *BrowserUI) syncSearchPanel() {
	panel := &ui.searchPanel
	if !panel.open || ui.searchPanelSuggestions == nil || panel.localGeneration == 0 || panel.localApplied == panel.localGeneration {
		return
	}
	generation, snapshot := ui.searchPanelSuggestions.Results()
	if generation != panel.localGeneration {
		return
	}
	panel.localApplied = generation
	ui.updateSearchPanelResults(snapshot)
}

func (ui *BrowserUI) updateSearchPanelResults(snapshot omnibox.Snapshot) {
	for _, tab := range ui.tabSnapshots() {
		snapshot.Tabs = append(snapshot.Tabs, omnibox.Candidate{Primary: tabDisplayTitle(tab), URL: tab.URL, TabID: uint64(tab.ID)})
	}
	ui.searchPanel.candidates = rankSearchPanel(ui.searchPanel.query, ui.searchPanel.scope, snapshot)
}

func rankSearchPanel(query string, scope omnibox.Scope, snapshot omnibox.Snapshot) []omnibox.Candidate {
	input := strings.TrimSpace(query)
	if scope != "" {
		input = "@" + string(scope)
		if query = strings.TrimSpace(query); query != "" {
			input += " " + query
		}
	}
	ranked := omnibox.Rank(input, snapshot, nil)
	results := ranked[:0]
	for _, candidate := range ranked {
		if candidate.Source == omnibox.TabSource || candidate.Source == omnibox.HistorySource || candidate.Source == omnibox.BookmarkSource {
			results = append(results, candidate)
		}
	}
	return results
}

func (ui *BrowserUI) setSearchPanelScope(scope omnibox.Scope) {
	if ui.searchPanel.scope == scope {
		return
	}
	ui.searchPanel.scope = scope
	ui.refreshSearchPanel()
}

func (ui *BrowserUI) handleSearchPanelActions(gtx layout.Context) {
	panel := &ui.searchPanel
	if !panel.open {
		return
	}
	ui.syncSearchPanel()
	ui.handleSearchPanelResultKeys(gtx)
	for {
		event, ok := panel.editor.Update(gtx)
		if !ok {
			break
		}
		if _, changed := event.(widget.ChangeEvent); changed {
			ui.refreshSearchPanel()
		}
	}
	for panel.allButton.Clicked(gtx) {
		ui.setSearchPanelScope("")
	}
	for panel.tabsButton.Clicked(gtx) {
		ui.setSearchPanelScope(omnibox.Tabs)
	}
	for panel.historyButton.Clicked(gtx) {
		ui.setSearchPanelScope(omnibox.History)
	}
	for panel.bookmarksButton.Clicked(gtx) {
		ui.setSearchPanelScope(omnibox.Bookmarks)
	}
	for panel.closeButton.Clicked(gtx) {
		ui.closeSearchPanel(gtx)
	}
	for index := range panel.candidates {
		for panel.rows[index].Clicked(gtx) {
			ui.executeSearchPanelCandidate(gtx, panel.candidates[index], omniboxCurrentTab)
			return
		}
	}
}

func (ui *BrowserUI) handleSearchPanelResultKeys(gtx layout.Context) {
	panel := &ui.searchPanel
	for _, name := range []key.Name{key.NameDownArrow, key.NameUpArrow, key.NamePageDown, key.NamePageUp, key.NameHome, key.NameEnd} {
		for {
			event, ok := gtx.Event(key.Filter{Focus: panel.editor, Name: name})
			if !ok {
				break
			}
			keyEvent, ok := event.(key.Event)
			if !ok || keyEvent.State != key.Press || len(panel.candidates) == 0 {
				continue
			}
			index := panel.selected
			switch name {
			case key.NameDownArrow:
				index = (index + 1) % len(panel.candidates)
			case key.NameUpArrow:
				if index <= 0 {
					index = len(panel.candidates) - 1
				} else {
					index--
				}
			case key.NamePageDown:
				index = min(max(index, 0)+5, len(panel.candidates)-1)
			case key.NamePageUp:
				index = max(index-5, 0)
			case key.NameHome:
				index = 0
			case key.NameEnd:
				index = len(panel.candidates) - 1
			}
			panel.selected = index
			panel.list.ScrollTo(index)
		}
	}
	for {
		event, ok := gtx.Event(
			key.Filter{Focus: panel.editor, Name: key.NameReturn, Optional: key.ModShift | key.ModAlt},
			key.Filter{Focus: panel.editor, Name: key.NameEnter, Optional: key.ModShift | key.ModAlt},
		)
		if !ok {
			break
		}
		keyEvent, ok := event.(key.Event)
		if !ok || keyEvent.State != key.Press || panel.selected < 0 || panel.selected >= len(panel.candidates) {
			continue
		}
		disposition := omniboxCurrentTab
		if keyEvent.Modifiers.Contain(key.ModAlt) {
			disposition = omniboxNewBackgroundTab
		} else if keyEvent.Modifiers.Contain(key.ModShift) {
			disposition = omniboxNewForegroundTab
		}
		ui.executeSearchPanelCandidate(gtx, panel.candidates[panel.selected], disposition)
		return
	}
}

func (ui *BrowserUI) executeSearchPanelCandidate(gtx layout.Context, candidate omnibox.Candidate, disposition omniboxDisposition) {
	ui.closeSearchPanel(gtx)
	if candidate.Source == omnibox.TabSource {
		if ui.tabs == nil {
			ui.status = "Tabを選択できません"
			ui.statusHasError = true
			return
		}
		if _, err := ui.tabs.SelectTab(browser.TabID(candidate.TabID)); err != nil {
			ui.reportTabOperationError("検索結果のTabを選択できません", err)
			return
		}
		ui.syncActiveTabChrome()
		ui.status = "既存のTabへ切り替えました"
		ui.statusHasError = false
		return
	}
	if candidate.Source != omnibox.HistorySource && candidate.Source != omnibox.BookmarkSource {
		return
	}
	ui.startNavigationWithDisposition(candidate.URL, disposition)
}

func (ui *BrowserUI) layoutSearchPanel(gtx layout.Context) layout.Dimensions {
	ui.syncSearchPanel()
	paint.Fill(gtx.Ops, color.NRGBA{R: 248, G: 250, B: 252, A: 255})
	return layout.Inset{Top: unit.Dp(20), Right: unit.Dp(24), Bottom: unit.Dp(20), Left: unit.Dp(24)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						label := material.H5(ui.theme, "Tabs, History, and Bookmarks")
						label.Color = color.NRGBA{R: 30, G: 41, B: 59, A: 255}
						return label.Layout(gtx)
					}),
					layout.Flexed(1, layout.Spacer{}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						button := material.Button(ui.theme, &ui.searchPanel.closeButton, "Close")
						button.Background = color.NRGBA{R: 71, G: 85, B: 105, A: 255}
						return button.Layout(gtx)
					}),
				)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(14)}.Layout),
			layout.Rigid(ui.layoutSearchPanelEditor),
			layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
			layout.Rigid(ui.layoutSearchPanelScopes),
			layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				text := searchPanelCountLabel(ui.searchPanel.scope, len(ui.searchPanel.candidates))
				label := material.Caption(ui.theme, text)
				label.Color = color.NRGBA{R: 71, G: 85, B: 105, A: 255}
				semantic.DescriptionOp(text).Add(gtx.Ops)
				return label.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
			layout.Flexed(1, ui.layoutSearchPanelResults),
		)
	})
}

func (ui *BrowserUI) layoutSearchPanelEditor(gtx layout.Context) layout.Dimensions {
	height := gtx.Dp(unit.Dp(48))
	gtx.Constraints.Min.Y = height
	gtx.Constraints.Max.Y = height
	return widget.Border{Color: color.NRGBA{R: 148, G: 163, B: 184, A: 255}, CornerRadius: unit.Dp(10), Width: unit.Dp(1)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, gtx.Dp(unit.Dp(10))).Op(gtx.Ops))
		editor := material.Editor(ui.theme, ui.searchPanel.editor, "Search open tabs, history, and bookmarks")
		return layout.Inset{Top: unit.Dp(10), Right: unit.Dp(14), Bottom: unit.Dp(8), Left: unit.Dp(14)}.Layout(gtx, editor.Layout)
	})
}

func (ui *BrowserUI) layoutSearchPanelScopes(gtx layout.Context) layout.Dimensions {
	panel := &ui.searchPanel
	return layout.Flex{Spacing: layout.SpaceStart}.Layout(gtx,
		layout.Rigid(ui.layoutSearchPanelScopeButton(&panel.allButton, "All", panel.scope == "")),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(ui.layoutSearchPanelScopeButton(&panel.tabsButton, "Tabs", panel.scope == omnibox.Tabs)),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(ui.layoutSearchPanelScopeButton(&panel.historyButton, "History", panel.scope == omnibox.History)),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(ui.layoutSearchPanelScopeButton(&panel.bookmarksButton, "Bookmarks", panel.scope == omnibox.Bookmarks)),
	)
}

func (ui *BrowserUI) layoutSearchPanelScopeButton(clickable *widget.Clickable, label string, active bool) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		button := material.Button(ui.theme, clickable, label)
		button.CornerRadius = unit.Dp(18)
		button.Background = color.NRGBA{R: 226, G: 232, B: 240, A: 255}
		button.Color = color.NRGBA{R: 51, G: 65, B: 85, A: 255}
		if active {
			button.Background = color.NRGBA{R: 37, G: 99, B: 235, A: 255}
			button.Color = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
		}
		return button.Layout(gtx)
	}
}

func (ui *BrowserUI) layoutSearchPanelResults(gtx layout.Context) layout.Dimensions {
	panel := &ui.searchPanel
	if len(panel.candidates) == 0 {
		label := material.Body1(ui.theme, "No matching tabs, history, or bookmarks")
		label.Color = color.NRGBA{R: 100, G: 116, B: 139, A: 255}
		return layout.Center.Layout(gtx, label.Layout)
	}
	return material.List(ui.theme, &panel.list).Layout(gtx, len(panel.candidates), func(gtx layout.Context, index int) layout.Dimensions {
		candidate := panel.candidates[index]
		return panel.rows[index].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			height := gtx.Dp(unit.Dp(64))
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			gtx.Constraints.Min.Y = height
			gtx.Constraints.Max.Y = height
			background := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
			if index == panel.selected {
				background = color.NRGBA{R: 219, G: 234, B: 254, A: 255}
			}
			paint.FillShape(gtx.Ops, background, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, gtx.Dp(unit.Dp(8))).Op(gtx.Ops))
			semantic.DescriptionOp(candidate.Source.Label() + ": " + candidate.Primary).Add(gtx.Ops)
			semantic.SelectedOp(index == panel.selected).Add(gtx.Ops)
			return layout.Inset{Top: unit.Dp(8), Right: unit.Dp(12), Bottom: unit.Dp(8), Left: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						label := material.Body1(ui.theme, candidate.Primary)
						label.MaxLines = 1
						return label.Layout(gtx)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						label := material.Caption(ui.theme, candidate.Source.Label()+" · "+candidate.Secondary)
						label.Color = color.NRGBA{R: 71, G: 85, B: 105, A: 255}
						label.MaxLines = 1
						return label.Layout(gtx)
					}),
				)
			})
		})
	})
}

func searchPanelCountLabel(scope omnibox.Scope, count int) string {
	label := "All"
	switch scope {
	case omnibox.Tabs:
		label = "Tabs"
	case omnibox.History:
		label = "History"
	case omnibox.Bookmarks:
		label = "Bookmarks"
	}
	return fmt.Sprintf("%s · %d results", label, count)
}
