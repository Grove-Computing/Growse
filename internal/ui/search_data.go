package ui

import (
	"image"
	"image/color"

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
	icon := ui.bookmarkBorderIcon
	description := "Bookmarkへ追加"
	if ui.searchData != nil {
		if navigator := ui.activeNavigator(); navigator != nil && navigator.Page() != nil && navigator.Page().URL != nil {
			if _, exists := ui.searchData.Bookmark(navigator.Page().URL.String()); exists {
				icon = ui.bookmarkIcon
				description = "Bookmarkから削除"
			}
		}
	}
	size := gtx.Dp(unit.Dp(40))
	gtx.Constraints = layout.Exact(image.Pt(size, size))
	button := material.IconButton(ui.theme, &ui.bookmarkButton, icon, description)
	button.Background = color.NRGBA{}
	button.Color = color.NRGBA{R: 52, G: 64, B: 84, A: 255}
	button.Size = unit.Dp(22)
	button.Inset = layout.UniformInset(unit.Dp(9))
	return button.Layout(gtx)
}
