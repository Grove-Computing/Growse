package ui

import (
	"crypto/sha256"
	"fmt"
	"gioui.org/layout"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/Grove-Computing/Growse/internal/browser"
	"github.com/Grove-Computing/Growse/internal/searchprovider"
	"image/color"
)

// SearchProviders returns a value snapshot of browser-owned configuration.
func (ui *BrowserUI) SearchProviders() searchprovider.Settings { return ui.providers.Clone() }
func (ui *BrowserUI) SetSearchProviders(s searchprovider.Settings) error {
	if s.Validate() != nil {
		return searchprovider.ErrInvalid
	}
	ui.closeSuggestionPopup()
	ui.providers = s.Clone()
	ui.SetSuggestionProvider(ui.providerTransport.Suggestions(s.Default()), s.RemoteSuggestions)
	return nil
}
func (ui *BrowserUI) providerSearchURL(input string) (string, error) {
	p, q, _ := ui.providers.Resolve(input)
	return p.SearchURL(q)
}

// providerPanel is internal chrome; web content cannot access these editors.
type providerPanel struct {
	open                                  bool
	remote                                widget.Clickable
	toggle, close, save, add              widget.Clickable
	list                                  widget.List
	selected                              string
	id, name, keyword, search, suggestion widget.Editor
	rows                                  map[string]*providerRow
}
type providerImportResult struct {
	provider searchprovider.Provider
	err      error
	owner    browser.TabID
	page     *browser.Page
}

type providerRow struct{ edit, choose, remove, disable widget.Clickable }

func (ui *BrowserUI) layoutProviderButton(gtx layout.Context) layout.Dimensions {
	if ui.providerPanel.toggle.Clicked(gtx) {
		ui.closeSuggestionPopup()
		ui.providerPanel.open = !ui.providerPanel.open
	}
	return material.Button(ui.theme, &ui.providerPanel.toggle, "検索設定").Layout(gtx)
}
func (ui *BrowserUI) selectProvider(p searchprovider.Provider) {
	panel := &ui.providerPanel
	panel.selected = p.ID
	panel.id.SetText(p.ID)
	panel.name.SetText(p.Name)
	panel.keyword.SetText(p.Keyword)
	panel.search.SetText(p.SearchTemplate)
	panel.suggestion.SetText(p.SuggestionTemplate)
}
func (ui *BrowserUI) changeProviderSettings(s searchprovider.Settings) {
	if err := ui.SetSearchProviders(s); err != nil {
		ui.status = err.Error()
		ui.statusHasError = true
		return
	}
	ui.status = "検索provider設定を変更しました"
	ui.statusHasError = false
}
func (ui *BrowserUI) layoutProviderSettings(gtx layout.Context) layout.Dimensions {
	panel := &ui.providerPanel
	select {
	case result := <-ui.providerImports:
		ui.providerImportPending = false
		owner, _ := ui.activeNavigationTarget()
		nav := ui.activeNavigator()
		if owner == result.owner && nav != nil && nav.Page() == result.page {
			if result.err != nil {
				ui.status = result.err.Error()
				ui.statusHasError = true
			} else {
				s := ui.SearchProviders()
				if err := s.Put(result.provider); err != nil {
					ui.status = err.Error()
					ui.statusHasError = true
				} else {
					ui.changeProviderSettings(s)
				}
			}
		}
	default:
	}
	paint.Fill(gtx.Ops, color.NRGBA{R: 244, G: 247, B: 251, A: 255})
	if panel.rows == nil {
		panel.rows = map[string]*providerRow{}
		panel.list.Axis = layout.Vertical
		for _, e := range []*widget.Editor{&panel.id, &panel.name, &panel.keyword, &panel.search, &panel.suggestion} {
			e.SingleLine = true
		}
	}
	if panel.close.Clicked(gtx) {
		panel.open = false
	}
	if panel.add.Clicked(gtx) {
		ui.selectProvider(searchprovider.Provider{})
	}
	if panel.save.Clicked(gtx) {
		s := ui.SearchProviders()
		p := searchprovider.Provider{ID: panel.id.Text(), Name: panel.name.Text(), Keyword: panel.keyword.Text(), SearchTemplate: panel.search.Text(), SuggestionTemplate: panel.suggestion.Text()}
		if err := s.Put(p); err != nil {
			ui.status = err.Error()
			ui.statusHasError = true
		} else {
			ui.changeProviderSettings(s)
		}
	}
	if panel.remote.Clicked(gtx) {
		s := ui.SearchProviders()
		s.RemoteSuggestions = !s.RemoteSuggestions
		ui.changeProviderSettings(s)
	}
	remoteLabel := "外部候補を有効にする (opt-in)"
	if ui.providers.RemoteSuggestions {
		remoteLabel = "外部候補を今すぐ無効にする"
	}
	destination := ui.providers.Default().SuggestionTemplate
	if destination == "" {
		destination = "候補endpointなし"
	}
	children := []layout.Widget{
		material.Body1(ui.theme, "外部候補の送信先: "+destination).Layout,
		material.Body1(ui.theme, "有効にすると入力した検索語を選択providerへ送信します。URL・command・credential・localhostは送信しません。keyword切替時はそのproviderへ送信します。ここでいつでも無効にできます。").Layout,
		func(gtx layout.Context) layout.Dimensions {
			return material.Button(ui.theme, &panel.remote, remoteLabel).Layout(gtx)
		},
		material.H6(ui.theme, "検索provider設定").Layout,
		func(gtx layout.Context) layout.Dimensions {
			return material.Button(ui.theme, &panel.close, "閉じる").Layout(gtx)
		},
	}
	for _, p := range ui.providers.Providers {
		row := panel.rows[p.ID]
		if row == nil {
			row = &providerRow{}
			panel.rows[p.ID] = row
		}
		if row.edit.Clicked(gtx) {
			ui.selectProvider(p)
		}
		if row.choose.Clicked(gtx) {
			s := ui.SearchProviders()
			s.DefaultID = p.ID
			ui.changeProviderSettings(s)
		}
		if row.remove.Clicked(gtx) {
			s := ui.SearchProviders()
			if err := s.Delete(p.ID); err != nil {
				ui.status = err.Error()
				ui.statusHasError = true
			} else {
				ui.changeProviderSettings(s)
			}
		}
		if row.disable.Clicked(gtx) {
			s := ui.SearchProviders()
			p.Disabled = !p.Disabled
			if err := s.Put(p); err != nil {
				ui.status = err.Error()
				ui.statusHasError = true
			} else {
				ui.changeProviderSettings(s)
			}
		}
		label := p.Name + " (" + p.Keyword + ")"
		if p.ID == ui.providers.DefaultID {
			label += " — 既定"
		}
		if p.Disabled {
			label += " — 無効"
		}
		children = append(children, material.Body1(ui.theme, label).Layout,
			func(gtx layout.Context) layout.Dimensions {
				return material.Button(ui.theme, &row.choose, "既定にする").Layout(gtx)
			},
			func(gtx layout.Context) layout.Dimensions {
				return material.Button(ui.theme, &row.edit, "編集").Layout(gtx)
			},
			func(gtx layout.Context) layout.Dimensions {
				return material.Button(ui.theme, &row.disable, "有効 / 無効").Layout(gtx)
			})
		if p.ID != "duckduckgo" {
			children = append(children, func(gtx layout.Context) layout.Dimensions {
				return material.Button(ui.theme, &row.remove, "削除").Layout(gtx)
			})
		}
	}
	if nav := ui.activeNavigator(); nav != nil && nav.Page() != nil {
		page := nav.Page()
		base := page.BaseURL
		if base == nil {
			base = page.URL
		}
		for _, d := range searchprovider.Discover(page.Document, base) {
			button := ui.providerDiscoveryButtons[d.URL]
			if button == nil {
				button = &widget.Clickable{}
				ui.providerDiscoveryButtons[d.URL] = button
			}
			if button.Clicked(gtx) {
				owner, _ := ui.activeNavigationTarget()
				hash := fmt.Sprintf("%x", sha256.Sum256([]byte(d.URL)))[:8]
				if !ui.providerImportPending {
					ui.providerImportPending = true
					go func() {
						p, err := ui.providerTransport.ImportDescription(ui.updateContext, d, "site-"+hash, "site"+hash, true)
						select {
						case ui.providerImports <- providerImportResult{p, err, owner, page}:
							ui.invalidate()
						case <-ui.updateContext.Done():
						}
					}()
				}
			}
			children = append(children, material.Body1(ui.theme, "検出したOpenSearch: "+d.Title+" — "+d.URL).Layout,
				func(gtx layout.Context) layout.Dimensions {
					return material.Button(ui.theme, button, "この送信先を確認して取得・登録").Layout(gtx)
				})
		}
	}
	children = append(children, func(gtx layout.Context) layout.Dimensions {
		return material.Button(ui.theme, &panel.add, "providerを追加").Layout(gtx)
	})
	for _, field := range []struct {
		label  string
		editor *widget.Editor
	}{{"ID", &panel.id}, {"表示名", &panel.name}, {"keyword", &panel.keyword}, {"HTTPS検索URL ({searchTerms})", &panel.search}, {"HTTPS候補URL (任意)", &panel.suggestion}} {
		children = append(children, material.Body1(ui.theme, field.label).Layout, func(gtx layout.Context) layout.Dimensions {
			return material.Editor(ui.theme, field.editor, field.label).Layout(gtx)
		})
	}
	children = append(children, func(gtx layout.Context) layout.Dimensions {
		return material.Button(ui.theme, &panel.save, "保存").Layout(gtx)
	})
	return layout.UniformInset(unit.Dp(12)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return material.List(ui.theme, &panel.list).Layout(gtx, len(children), func(gtx layout.Context, i int) layout.Dimensions {
			return layout.Inset{Bottom: unit.Dp(8)}.Layout(gtx, children[i])
		})
	})
}
