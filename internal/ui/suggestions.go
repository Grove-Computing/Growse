package ui

import (
	"image"
	"image/color"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
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

// SetSuggestionSnapshot supplies local source snapshots on the UI thread.
// History and bookmark persistence are owned by their respective data sources.
func (ui *BrowserUI) SetSuggestionSnapshot(snapshot omnibox.Snapshot) {
	ui.suggestionSnapshot = omnibox.Snapshot{
		Now:       snapshot.Now,
		History:   append([]omnibox.Candidate(nil), snapshot.History...),
		Bookmarks: append([]omnibox.Candidate(nil), snapshot.Bookmarks...),
	}
	if ui.suggestionPopup.open {
		ui.refreshSuggestions()
	} else {
		ui.suggestions.Cancel()
	}
}

// SetSuggestionProvider cancels the previous provider before replacing it.
// External suggestions stay disabled unless explicitly enabled by the caller.
func (ui *BrowserUI) SetSuggestionProvider(fetch omnibox.RemoteFetcher, enabled bool) {
	ui.suggestions.Cancel()
	ui.suggestionFetcher, ui.remoteSuggestions = fetch, enabled
	if ui.suggestionPopup.open {
		ui.refreshSuggestions()
	}
}

func (ui *BrowserUI) refreshSuggestions() { ui.refreshSuggestionsFor(ui.address.Text()) }

func (ui *BrowserUI) refreshSuggestionsFor(input string) {
	ui.recordOmniboxText()
	snapshot := ui.suggestionSnapshot
	for _, tab := range ui.tabSnapshots() {
		snapshot.Tabs = append(snapshot.Tabs, omnibox.Candidate{Primary: tabDisplayTitle(tab), URL: tab.URL, TabID: uint64(tab.ID)})
	}
	ui.suggestions.Update(input, snapshot, ui.suggestionFetcher, ui.remoteSuggestions)
	ui.suggestionPopup.owner, _ = ui.activeNavigationTarget()
	ui.suggestionPopup.open = true
	ui.suggestionPopup.selected = -1
	ui.suggestionPopup.list.Position = layout.Position{}
	ui.syncSuggestions()
	tabID, _ := ui.activeNavigationTarget()
	ui.setOmniboxPreview(tabID, "")
}

// suggestionPopup keeps selection/preview separate from the native editor.
type suggestionPopup struct {
	owner          browser.TabID
	open           bool
	generation     uint64
	candidates     []omnibox.Candidate
	selected       int
	rows           [omnibox.MaxVisibleCandidates]widget.Clickable
	hovered        [omnibox.MaxVisibleCandidates]bool
	list           widget.List
	bounds         image.Rectangle
	addressTag     struct{}
	addressPressed bool
}

func (ui *BrowserUI) syncSuggestions() {
	generation, candidates := ui.suggestions.Results()
	var previous omnibox.Candidate
	selected := ui.suggestionPopup.selected
	if selected >= 0 && selected < len(ui.suggestionPopup.candidates) {
		previous = ui.suggestionPopup.candidates[selected]
	}
	if len(candidates) > omnibox.MaxVisibleCandidates {
		candidates = candidates[:omnibox.MaxVisibleCandidates]
	}
	ui.suggestionPopup.candidates = candidates
	ui.suggestionPopup.selected = -1
	if generation == ui.suggestionPopup.generation && selected >= 0 {
		for i, c := range candidates {
			if sameSuggestion(c, previous) {
				ui.suggestionPopup.selected = i
				break
			}
		}
	}
	if ui.suggestionPopup.open && ui.suggestionPopup.selected < 0 {
		ui.setOmniboxPreview(ui.suggestionPopup.owner, "")
	}
	ui.suggestionPopup.generation = generation
}

func sameSuggestion(a, b omnibox.Candidate) bool {
	return a.Source == b.Source && a.TabID == b.TabID && a.URL == b.URL && a.Query == b.Query
}

func (ui *BrowserUI) closeSuggestionPopup() {
	if ui.suggestionPopup.open {
		if state, ok := ui.omniboxStates[ui.suggestionPopup.owner]; ok {
			state.preview = ""
			ui.omniboxStates[ui.suggestionPopup.owner] = state
		}
	}
	ui.suggestionPopup.open = false
	ui.suggestionPopup.selected = -1
	ui.suggestionPopup.candidates = nil
	ui.suggestions.Cancel()
}

func (ui *BrowserUI) selectSuggestion(index int) {
	popup := &ui.suggestionPopup
	if index < 0 || index >= len(popup.candidates) {
		return
	}
	popup.selected = index
	candidate := popup.candidates[index]
	preview := candidate.URL
	if preview == "" {
		preview = candidate.Query
	}
	if preview == "" {
		preview = candidate.Primary
	}
	tabID, _ := ui.activeNavigationTarget()
	ui.setOmniboxPreview(tabID, preview)
}

func (ui *BrowserUI) handleSuggestionKeys(gtx layout.Context) {
	ui.syncSuggestions()
	names := []key.Name{key.NameDownArrow, key.NameUpArrow}
	if ui.suggestionPopup.open {
		names = append(names, key.NamePageUp, key.NamePageDown, key.NameHome, key.NameEnd, key.NameEscape, key.NameTab)
	}
	for _, name := range names {
		for {
			event, ok := gtx.Event(key.Filter{Focus: ui.address, Name: name, Optional: key.ModShift})
			if !ok {
				break
			}
			e, ok := event.(key.Event)
			if !ok || e.State != key.Press {
				continue
			}
			if name == key.NameEscape || name == key.NameTab {
				ui.closeSuggestionPopup()
				if name == key.NameTab {
					if e.Modifiers.Contain(key.ModShift) {
						gtx.Execute(key.FocusCmd{Tag: &ui.devToolsButton})
					} else {
						gtx.Execute(key.FocusCmd{Tag: &ui.goButton})
					}
				}
				continue
			}
			if !ui.suggestionPopup.open {
				ui.refreshSuggestions()
				ui.syncSuggestions()
			}
			popup := &ui.suggestionPopup
			if len(popup.candidates) == 0 {
				continue
			}
			index := popup.selected
			switch name {
			case key.NameDownArrow:
				index = (index + 1) % len(popup.candidates)
			case key.NameUpArrow:
				if index <= 0 {
					index = len(popup.candidates) - 1
				} else {
					index--
				}
			case key.NamePageDown:
				index = min(max(index, 0)+5, len(popup.candidates)-1)
			case key.NamePageUp:
				index = max(index-5, 0)
			case key.NameHome:
				index = 0
			case key.NameEnd:
				index = len(popup.candidates) - 1
			}
			ui.selectSuggestion(index)
			popup.list.ScrollTo(index)
		}
	}
}

func (ui *BrowserUI) submitSuggestion(candidate omnibox.Candidate, disposition omniboxDisposition) {
	ui.closeSuggestionPopup()
	if candidate.Source == omnibox.TabSource && ui.tabs != nil {
		if _, err := ui.tabs.SelectTab(browser.TabID(candidate.TabID)); err != nil {
			ui.reportTabOperationError("候補Tabを選択できません", err)
		}
		ui.syncActiveTabChrome()
		return
	}
	target := candidate.URL
	if target == "" {
		target = candidate.Query
	}
	ui.startNavigationWithDisposition(target, disposition)
}

func (ui *BrowserUI) handleSuggestionMouseAndFocus(gtx layout.Context) {
	popup := &ui.suggestionPopup
	if !popup.open {
		return
	}
	for i := range popup.candidates {
		if popup.rows[i].Clicked(gtx) {
			ui.submitSuggestion(popup.candidates[i], omniboxCurrentTab)
			return
		}
		hovered := popup.rows[i].Hovered()
		if hovered && !popup.hovered[i] {
			ui.selectSuggestion(i)
		}
		popup.hovered[i] = hovered
	}
	if !gtx.Focused(ui.address) {
		ui.closeSuggestionPopup()
	}
}

func (ui *BrowserUI) registerSuggestionAddress(gtx layout.Context) {
	defer clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()
	defer pointer.PassOp{}.Push(gtx.Ops).Pop()
	event.Op(gtx.Ops, &ui.suggestionPopup.addressTag)
}

func (ui *BrowserUI) readSuggestionAddressPress(gtx layout.Context) {
	ui.suggestionPopup.addressPressed = false
	for {
		_, ok := gtx.Event(pointer.Filter{Target: &ui.suggestionPopup.addressTag, Kinds: pointer.Press})
		if !ok {
			break
		}
		ui.suggestionPopup.addressPressed = true
	}
}

func (ui *BrowserUI) layoutSuggestions(gtx layout.Context, viewport image.Rectangle) {
	popup := &ui.suggestionPopup
	if !popup.open || len(popup.candidates) == 0 {
		return
	}
	height := min(gtx.Dp(unit.Dp(56))*len(popup.candidates), viewport.Dy())
	popup.bounds = image.Rect(viewport.Min.X, viewport.Min.Y, viewport.Max.X, viewport.Min.Y+height)
	popup.list.Axis = layout.Vertical
	layoutRegion(gtx, popup.bounds, func(gtx layout.Context) layout.Dimensions {
		paint.Fill(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
		return material.List(ui.theme, &popup.list).Layout(gtx, len(popup.candidates), func(gtx layout.Context, index int) layout.Dimensions {
			candidate := popup.candidates[index]
			return popup.rows[index].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = gtx.Constraints.Max.X
				gtx.Constraints.Min.Y = gtx.Dp(unit.Dp(56))
				background := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
				if index == popup.selected {
					background = color.NRGBA{R: 219, G: 234, B: 254, A: 255}
				}
				paint.FillShape(gtx.Ops, background, clip.Rect{Max: gtx.Constraints.Min}.Op())
				semantic.DescriptionOp(candidate.Source.Label() + ": " + candidate.Primary).Add(gtx.Ops)
				semantic.SelectedOp(index == popup.selected).Add(gtx.Ops)
				return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							label := material.Body1(ui.theme, candidate.Primary)
							label.MaxLines = 1
							return label.Layout(gtx)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							label := material.Caption(ui.theme, candidate.Source.Label()+" · "+candidate.Secondary)
							label.MaxLines = 1
							return label.Layout(gtx)
						}),
					)
				})
			})
		})
	})
}

func (ui *BrowserUI) layoutSuggestionPreview(gtx layout.Context) layout.Dimensions {
	tabID, _ := ui.activeNavigationTarget()
	preview := ui.omniboxStates[tabID].preview
	if preview == "" || !ui.suggestionPopup.open {
		return layout.Dimensions{}
	}
	return layout.UniformInset(unit.Dp(10)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, clip.Rect{Max: gtx.Constraints.Max}.Op())
		label := material.Body1(ui.theme, preview)
		label.MaxLines = 1
		return label.Layout(gtx)
	})
}

// Gio emits ChangeEvent for SetText as well as native input. Record programmatic
// updates so they cannot reopen suggestions after navigation or popup dismissal.
func (ui *BrowserUI) recordOmniboxText() {
	tabID, _ := ui.activeNavigationTarget()
	state, ok := ui.omniboxStates[tabID]
	if !ok || state.editor != ui.address {
		return
	}
	state.observedText = ui.address.Text()
	ui.omniboxStates[tabID] = state
}

func (ui *BrowserUI) handleOmniboxChange(gtx layout.Context) {
	tabID, _ := ui.activeNavigationTarget()
	state, ok := ui.omniboxStates[tabID]
	if !ok || state.editor != ui.address || state.observedText == ui.address.Text() {
		return
	}
	ui.recordOmniboxText()
	if gtx.Focused(ui.address) {
		ui.refreshSuggestions()
	}
}
