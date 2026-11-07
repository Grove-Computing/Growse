package ui

import (
	"image/color"
	"sort"

	"gioui.org/io/event"
	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/Grove-Computing/Growse/internal/omnibox"
)

func focusBorder(focused bool) (color.NRGBA, unit.Dp) {
	if focused {
		return color.NRGBA{R: 37, G: 99, B: 235, A: 255}, unit.Dp(2)
	}
	return color.NRGBA{R: 183, G: 193, B: 207, A: 255}, unit.Dp(1)
}

func focusSurface(focused bool, normal color.NRGBA) color.NRGBA {
	if focused {
		return color.NRGBA{R: 191, G: 219, B: 254, A: 255}
	}
	return normal
}

func (ui *BrowserUI) layoutTextButton(gtx layout.Context, button *widget.Clickable, label string) layout.Dimensions {
	style := material.Button(ui.theme, button, label)
	if gtx.Focused(button) {
		style.Background = color.NRGBA{R: 30, G: 64, B: 175, A: 255}
		style.Color = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	}
	return style.Layout(gtx)
}

func (ui *BrowserUI) handleFocusTraversal(gtx layout.Context) {
	if panel := &ui.searchPanel; panel.open {
		tags := []event.Tag{panel.editor, &panel.allButton, &panel.tabsButton, &panel.historyButton, &panel.bookmarksButton}
		if panel.confirmClearHistory {
			tags = append(tags, &panel.cancelClearHistoryButton, &panel.confirmClearHistoryButton, &panel.closeButton)
		} else {
			for index, candidate := range panel.candidates {
				tags = append(tags, &panel.rows[index])
				if candidate.Source == omnibox.HistorySource || candidate.Source == omnibox.BookmarkSource {
					tags = append(tags, &panel.deleteButtons[index])
				}
			}
			if ui.searchPanelHistoryCount() > 0 {
				tags = append(tags, &panel.clearHistoryButton)
			}
			tags = append(tags, &panel.closeButton)
		}
		moveFocusOnTab(gtx, tags)
		return
	}
	if ui.providerPanel.open {
		ui.handleProviderFocusTraversal(gtx)
		return
	}
	if state := ui.activeFindState(); state != nil && state.open {
		moveFocusOnTab(gtx, []event.Tag{state.editor, &state.previous, &state.next, &state.caseToggle, &state.wordToggle, &state.close})
		return
	}
	if ui.homePanel.open {
		return
	}

	tags := make([]event.Tag, 0, 16+len(ui.bookmarkBarButtons))
	navigator := ui.activeNavigator()
	tabID, _ := ui.activeNavigationTarget()
	if ui.canTraverseHomeHistory(tabID, -1) || navigator != nil && navigator.CanBack() {
		tags = append(tags, &ui.backButton)
	}
	if ui.canTraverseHomeHistory(tabID, 1) || navigator != nil && navigator.CanForward() {
		tags = append(tags, &ui.forwardButton)
	}
	if navigator != nil && navigator.Page() != nil {
		tags = append(tags, &ui.reloadButton)
	}
	tags = append(tags, &ui.homeButton)
	if ui.updateAvailable || ui.updating {
		tags = append(tags, &ui.updateButton)
	}
	tags = append(tags, &ui.engineButton, &ui.devToolsButton, &ui.providerPanel.toggle, ui.address, &ui.bookmarkButton, &ui.goButton)
	if ui.searchData != nil {
		for _, bookmark := range ui.searchData.Bookmarks() {
			if button := ui.bookmarkBarButtons[bookmark.URL]; button != nil {
				tags = append(tags, button)
			}
		}
	}
	beforeAddress := gtx.Focused(ui.address)
	if moveFocusOnTab(gtx, tags) && beforeAddress {
		ui.closeSuggestionPopup()
	}
}

func (ui *BrowserUI) handleProviderFocusTraversal(gtx layout.Context) {
	panel := &ui.providerPanel
	tags := []event.Tag{&panel.remote, &panel.close}
	for _, provider := range ui.providers.Providers {
		if row := panel.rows[provider.ID]; row != nil {
			tags = append(tags, &row.choose, &row.edit, &row.disable)
			if provider.ID != "duckduckgo" {
				tags = append(tags, &row.remove)
			}
		}
	}
	urls := make([]string, 0, len(ui.providerDiscoveryButtons))
	for rawURL := range ui.providerDiscoveryButtons {
		urls = append(urls, rawURL)
	}
	sort.Strings(urls)
	for _, rawURL := range urls {
		tags = append(tags, ui.providerDiscoveryButtons[rawURL])
	}
	tags = append(tags, &panel.add, &panel.id, &panel.name, &panel.keyword, &panel.search, &panel.suggestion, &panel.save)
	moveFocusOnTab(gtx, tags)
}
