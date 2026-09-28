package ui

import (
	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget/material"
	"github.com/Grove-Computing/Growse/internal/searchdata"
)

// SetSearchDataStore connects profile history and bookmarks to browser chrome.
func (ui *BrowserUI) SetSearchDataStore(store *searchdata.Store) {
	if ui.localSuggestions != nil {
		ui.localSuggestions.Close()
	}
	ui.searchData = store
	ui.localSuggestions = searchdata.NewLocalPipeline(store, ui.invalidate)
	if ui.suggestionPopup.open {
		ui.refreshSuggestions()
	}
}

func (ui *BrowserUI) toggleActiveBookmark() {
	if ui.searchData == nil {
		ui.status = "Bookmark profileを利用できません"
		ui.statusHasError = true
		return
	}
	navigator := ui.activeNavigator()
	if navigator == nil || navigator.Page() == nil || navigator.Page().URL == nil {
		ui.status = "Bookmarkへ追加できるPageがありません"
		ui.statusHasError = true
		return
	}
	page := navigator.Page()
	rawURL := page.URL.String()
	if _, exists := ui.searchData.Bookmark(rawURL); exists {
		if err := ui.searchData.DeleteBookmark(rawURL); err != nil {
			ui.status = "Bookmarkを削除できませんでした"
			ui.statusHasError = true
			return
		}
		ui.status = "Bookmarkを削除しました"
		ui.statusHasError = false
		if ui.suggestionPopup.open {
			ui.refreshSuggestions()
		}
		return
	}
	title := ui.pageTitle
	if page.Document != nil && page.Document.Title() != "" {
		title = page.Document.Title()
	}
	if _, err := ui.searchData.SaveBookmark("", rawURL, title); err != nil {
		ui.status = "Bookmarkへ追加できませんでした"
		ui.statusHasError = true
		return
	}
	ui.status = "Bookmarkへ追加しました"
	ui.statusHasError = false
	if ui.suggestionPopup.open {
		ui.refreshSuggestions()
	}
}

func (ui *BrowserUI) layoutBookmarkButton(gtx layout.Context) layout.Dimensions {
	label := "☆"
	if ui.searchData != nil {
		if navigator := ui.activeNavigator(); navigator != nil && navigator.Page() != nil && navigator.Page().URL != nil {
			if _, exists := ui.searchData.Bookmark(navigator.Page().URL.String()); exists {
				label = "★"
			}
		}
	}
	gtx.Constraints.Min.Y = gtx.Dp(controlHeight)
	gtx.Constraints.Max.Y = gtx.Dp(controlHeight)
	button := material.Button(ui.theme, &ui.bookmarkButton, label)
	button.CornerRadius = unit.Dp(10)
	return button.Layout(gtx)
}
