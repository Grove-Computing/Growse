package ui

import (
	"context"
	"image"
	"image/color"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/Grove-Computing/Growse/internal/browser"
	"github.com/Grove-Computing/Growse/internal/searchdata"
)

// SetSearchDataStore connects profile history and bookmarks to browser chrome.
func (ui *BrowserUI) SetSearchDataStore(store *searchdata.Store) {
	if ui.localSuggestions != nil {
		ui.localSuggestions.Close()
	}
	ui.searchData = store
	ui.localSuggestions = searchdata.NewLocalPipeline(store, ui.invalidate)
	if ui.searchPanelSuggestions != nil {
		ui.searchPanelSuggestions.Close()
	}
	ui.searchPanelSuggestions = searchdata.NewLocalPipeline(store, ui.invalidate)
	if ui.suggestionPopup.open {
		ui.refreshSuggestions()
	}
	if ui.searchPanel.open {
		ui.refreshSearchPanel()
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

func (ui *BrowserUI) handleBookmarkBarActions(gtx layout.Context) {
	if ui.searchData == nil {
		return
	}
	for _, entry := range ui.searchData.Bookmarks() {
		button := ui.bookmarkBarButtons[entry.URL]
		if button == nil {
			continue
		}
		for button.Clicked(gtx) {
			ui.openBookmark(entry.URL)
			return
		}
	}
}

func (ui *BrowserUI) openBookmark(rawURL string) {
	ui.recordOmniboxText()
	ui.closeSuggestionPopup()
	tabID, navigator := ui.activeNavigationTarget()
	if navigator == nil {
		ui.status = "Bookmarkを開けません"
		ui.statusHasError = true
		return
	}
	ui.startPageLoad(tabID, navigator, navigationLoadingStatus(rawURL), func(ctx context.Context) (*browser.Page, error) {
		return navigator.Navigate(ctx, rawURL)
	})
}

func (ui *BrowserUI) layoutBookmarkBar(gtx layout.Context) layout.Dimensions {
	height := gtx.Dp(bookmarkBarHeight)
	gtx.Constraints.Min.Y = height
	gtx.Constraints.Max.Y = height

	var bookmarks []searchdata.Bookmark
	if ui.searchData != nil {
		bookmarks = ui.searchData.Bookmarks()
	}
	if len(bookmarks) == 0 {
		label := material.Caption(ui.theme, "Bookmarkはまだありません")
		label.Color = color.NRGBA{R: 112, G: 124, B: 143, A: 255}
		return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(8)}.Layout(gtx, label.Layout)
	}
	return material.List(ui.theme, &ui.bookmarkBarList).Layout(gtx, len(bookmarks), func(gtx layout.Context, index int) layout.Dimensions {
		entry := bookmarks[index]
		button := ui.bookmarkBarButtons[entry.URL]
		if button == nil {
			button = &widget.Clickable{}
			ui.bookmarkBarButtons[entry.URL] = button
		}
		return ui.layoutBookmarkBarItem(gtx, button, entry)
	})
}

func (ui *BrowserUI) layoutBookmarkBarItem(gtx layout.Context, button *widget.Clickable, entry searchdata.Bookmark) layout.Dimensions {
	height := gtx.Dp(bookmarkBarHeight)
	gtx.Constraints.Min.X = 0
	gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(unit.Dp(200)))
	gtx.Constraints.Min.Y = height
	gtx.Constraints.Max.Y = height
	title := entry.Title
	if title == "" {
		title = entry.URL
	}
	return layout.Inset{Right: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return material.Clickable(gtx, button, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(4), Right: unit.Dp(8), Bottom: unit.Dp(4), Left: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						size := gtx.Dp(unit.Dp(14))
						gtx.Constraints = layout.Exact(image.Pt(size, size))
						return ui.bookmarkIcon.Layout(gtx, color.NRGBA{R: 52, G: 64, B: 84, A: 255})
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(5)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						label := material.Body2(ui.theme, title)
						label.Color = color.NRGBA{R: 52, G: 64, B: 84, A: 255}
						label.MaxLines = 1
						return label.Layout(gtx)
					}),
				)
			})
		})
	})
}
