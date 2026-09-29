package ui

import (
	"image"
	"image/color"
	"strings"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/Grove-Computing/Growse/internal/homeconfig"
)

type homeShortcutRow struct {
	edit, remove, up, down widget.Clickable
}

type homeSettingsPanel struct {
	open             bool
	editing          int
	title, rawURL    widget.Editor
	add, save, close widget.Clickable
	rows             [homeconfig.MaxShortcuts]homeShortcutRow
	backgrounds      [5]widget.Clickable
	list             widget.List
	focusPending     bool
	errorMessage     string
}

func newHomeSettingsPanel() homeSettingsPanel {
	panel := homeSettingsPanel{editing: -1}
	panel.title.SingleLine = true
	panel.title.MaxLen = homeconfig.MaxTitleScalars
	panel.rawURL.SingleLine = true
	panel.rawURL.MaxLen = homeconfig.MaxURLBytes
	return panel
}

func (ui *BrowserUI) handleHomeSettingsActions(gtx layout.Context) {
	panel := &ui.homePanel
	if !panel.open {
		return
	}
	ui.handleHomeSettingsTabTraversal(gtx)
	for panel.close.Clicked(gtx) {
		panel.open = false
		panel.errorMessage = ""
	}
	for panel.add.Clicked(gtx) {
		panel.editing = -1
		panel.title.SetText("")
		panel.rawURL.SetText("")
		panel.errorMessage = ""
	}
	for panel.save.Clicked(gtx) {
		next := ui.homeSettings.Clone()
		var err error
		if panel.editing >= 0 {
			panel.editing, err = next.Edit(panel.editing, panel.title.Text(), panel.rawURL.Text())
		} else {
			panel.editing, err = next.Add(panel.title.Text(), panel.rawURL.Text())
		}
		if err != nil {
			panel.errorMessage = err.Error()
			continue
		}
		if !ui.applyHomeSettings(next) {
			panel.errorMessage = "ホーム設定を保存できません"
			continue
		}
		panel.errorMessage = ""
	}
	for index := range ui.homeSettings.Shortcuts {
		row := &panel.rows[index]
		for row.edit.Clicked(gtx) {
			panel.editing = index
			panel.title.SetText(ui.homeSettings.Shortcuts[index].Title)
			panel.rawURL.SetText(ui.homeSettings.Shortcuts[index].URL)
			panel.errorMessage = ""
		}
		for row.remove.Clicked(gtx) {
			next := ui.homeSettings.Clone()
			if err := next.Delete(index); err != nil {
				panel.errorMessage = err.Error()
			} else {
				if !ui.applyHomeSettings(next) {
					panel.errorMessage = "ホーム設定を保存できません"
					continue
				}
				if panel.editing == index {
					panel.editing = -1
					panel.title.SetText("")
					panel.rawURL.SetText("")
				}
			}
		}
		for row.up.Clicked(gtx) {
			if index > 0 {
				next := ui.homeSettings.Clone()
				if err := next.Move(index, index-1); err != nil {
					panel.errorMessage = err.Error()
				} else {
					if !ui.applyHomeSettings(next) {
						panel.errorMessage = "ホーム設定を保存できません"
						continue
					}
					panel.editing = -1
				}
			}
		}
		for row.down.Clicked(gtx) {
			if index+1 < len(ui.homeSettings.Shortcuts) {
				next := ui.homeSettings.Clone()
				if err := next.Move(index, index+1); err != nil {
					panel.errorMessage = err.Error()
				} else {
					if !ui.applyHomeSettings(next) {
						panel.errorMessage = "ホーム設定を保存できません"
						continue
					}
					panel.editing = -1
				}
			}
		}
	}
	for index, preset := range homeconfig.BackgroundPresets() {
		for panel.backgrounds[index].Clicked(gtx) {
			next := ui.homeSettings.Clone()
			next.Background = preset.ID
			if !ui.applyHomeSettings(next) {
				panel.errorMessage = "背景設定を保存できません"
			} else {
				panel.errorMessage = ""
			}
		}
	}
}

func (ui *BrowserUI) handleHomeSettingsTabTraversal(gtx layout.Context) {
	panel := &ui.homePanel
	tags := make([]event.Tag, 0, 10+len(ui.homeSettings.Shortcuts)*4)
	for index := range homeconfig.BackgroundPresets() {
		tags = append(tags, &panel.backgrounds[index])
	}
	tags = append(tags, &panel.title, &panel.rawURL, &panel.save, &panel.add, &panel.close)
	for index := range ui.homeSettings.Shortcuts {
		row := &panel.rows[index]
		tags = append(tags, &row.edit, &row.up, &row.down, &row.remove)
	}
	_ = moveFocusOnTab(gtx, tags)
}

func (ui *BrowserUI) applyHomeSettings(settings homeconfig.Settings) bool {
	if settings.Validate() != nil {
		return false
	}
	if ui.homeStore != nil {
		if err := ui.homeStore.Save(settings); err != nil {
			return false
		}
	}
	ui.homeSettings = settings.Clone()
	return true
}

func shortcutLetter(title string) string {
	for _, value := range strings.TrimSpace(title) {
		return strings.ToUpper(string(value))
	}
	return "?"
}

func (ui *BrowserUI) layoutHomeShortcuts(gtx layout.Context) layout.Dimensions {
	columns := 5
	if gtx.Constraints.Max.X < gtx.Dp(unit.Dp(520)) {
		columns = 2
	}
	return ui.layoutHomeShortcutsWithColumns(gtx, columns)
}

func (ui *BrowserUI) layoutHomeShortcutsWithColumns(gtx layout.Context, columns int) layout.Dimensions {
	count := len(ui.homeSettings.Shortcuts)
	if count == 0 {
		label := material.Body2(ui.theme, "ショートカットはまだありません")
		label.Color = color.NRGBA{R: 100, G: 116, B: 139, A: 255}
		return label.Layout(gtx)
	}
	rows := (count + columns - 1) / columns
	children := make([]layout.FlexChild, 0, rows*2)
	for row := 0; row < rows; row++ {
		start := row * columns
		end := min(start+columns, count)
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			items := make([]layout.FlexChild, 0, columns*2)
			for index := start; index < end; index++ {
				index := index
				items = append(items,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return ui.homeShortcuts[index].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							semantic.ClassOp(semantic.Button).Add(gtx.Ops)
							semantic.DescriptionOp("ショートカット: " + ui.homeSettings.Shortcuts[index].Title).Add(gtx.Ops)
							gtx.Constraints.Min.X = gtx.Constraints.Max.X
							return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									size := gtx.Dp(unit.Dp(36))
									gtx.Constraints = layout.Exact(image.Pt(size, size))
									paint.FillShape(gtx.Ops, color.NRGBA{R: 37, G: 99, B: 235, A: 255}, clipCircle(gtx, size))
									label := material.Body1(ui.theme, shortcutLetter(ui.homeSettings.Shortcuts[index].Title))
									label.Color = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
									return layout.Center.Layout(gtx, label.Layout)
								}),
								layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									label := material.Caption(ui.theme, ui.homeSettings.Shortcuts[index].Title)
									label.MaxLines = 1
									return label.Layout(gtx)
								}),
							)
						})
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
				)
			}
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx, items...)
		}))
		if row+1 < rows {
			children = append(children, layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout))
		}
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func clipCircle(gtx layout.Context, size int) clip.Op {
	return clip.Ellipse(image.Rect(0, 0, size, size)).Op(gtx.Ops)
}

func (ui *BrowserUI) layoutHomeSettingsEditor(gtx layout.Context, editor *widget.Editor, hint, description string) layout.Dimensions {
	height := gtx.Dp(unit.Dp(44))
	gtx.Constraints.Min.X = gtx.Constraints.Max.X
	gtx.Constraints.Min.Y = height
	gtx.Constraints.Max.Y = height
	defer clip.Rect{Max: gtx.Constraints.Min}.Push(gtx.Ops).Pop()
	semantic.ClassOp(semantic.Editor).Add(gtx.Ops)
	semantic.DescriptionOp(description).Add(gtx.Ops)
	borderColor, borderWidth := homeSearchBorder(gtx.Focused(editor))
	return widget.Border{Color: borderColor, CornerRadius: unit.Dp(9), Width: borderWidth}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: unit.Dp(8), Right: unit.Dp(10), Bottom: unit.Dp(8), Left: unit.Dp(10)}.Layout(gtx,
			material.Editor(ui.theme, editor, hint).Layout)
	})
}

func (ui *BrowserUI) layoutHomePresetButton(gtx layout.Context, button *widget.Clickable, preset homeconfig.BackgroundPreset, selected bool) layout.Dimensions {
	return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		semantic.ClassOp(semantic.RadioButton).Add(gtx.Ops)
		semantic.DescriptionOp("ホーム背景: " + preset.Name).Add(gtx.Ops)
		semantic.SelectedOp(selected).Add(gtx.Ops)
		background := color.NRGBA{R: 226, G: 232, B: 240, A: 255}
		foreground := color.NRGBA{R: 30, G: 41, B: 59, A: 255}
		if selected {
			background = color.NRGBA{R: 37, G: 99, B: 235, A: 255}
			foreground = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
		}
		minWidth := gtx.Dp(unit.Dp(80))
		if minWidth > gtx.Constraints.Max.X {
			minWidth = gtx.Constraints.Max.X
		}
		gtx.Constraints.Min.X = max(gtx.Constraints.Min.X, minWidth)
		gtx.Constraints.Min.Y = gtx.Dp(unit.Dp(40))
		return layout.Inset{Top: unit.Dp(8), Right: unit.Dp(12), Bottom: unit.Dp(8), Left: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, background, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, gtx.Dp(unit.Dp(8))).Op(gtx.Ops))
			label := material.Body2(ui.theme, preset.Name)
			label.Color = foreground
			return label.Layout(gtx)
		})
	})
}

func (ui *BrowserUI) layoutHomeSettings(gtx layout.Context) layout.Dimensions {
	paint.Fill(gtx.Ops, color.NRGBA{R: 238, G: 243, B: 248, A: 255})
	panel := &ui.homePanel
	panel.list.Axis = layout.Vertical
	return material.List(ui.theme, &panel.list).Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		return layout.Inset{Top: unit.Dp(24), Right: unit.Dp(16), Bottom: unit.Dp(24), Left: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.N.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				if gtx.Constraints.Max.X > gtx.Dp(unit.Dp(720)) {
					gtx.Constraints.Max.X = gtx.Dp(unit.Dp(720))
				}
				children := []layout.FlexChild{
					layout.Rigid(material.H5(ui.theme, "ホームのショートカット").Layout),
					layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						presets := homeconfig.BackgroundPresets()
						if gtx.Constraints.Max.X < gtx.Dp(unit.Dp(520)) {
							items := make([]layout.FlexChild, 0, len(presets)*2)
							for index, preset := range presets {
								index, preset := index, preset
								items = append(items,
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										gtx.Constraints.Min.X = gtx.Constraints.Max.X
										return ui.layoutHomePresetButton(gtx, &panel.backgrounds[index], preset, preset.ID == ui.homeSettings.Background)
									}),
									layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
								)
							}
							return layout.Flex{Axis: layout.Vertical}.Layout(gtx, items...)
						}
						items := make([]layout.FlexChild, 0, len(presets)*2)
						for index, preset := range presets {
							index, preset := index, preset
							items = append(items,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return ui.layoutHomePresetButton(gtx, &panel.backgrounds[index], preset, preset.ID == ui.homeSettings.Background)
								}),
								layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
							)
						}
						return layout.Flex{}.Layout(gtx, items...)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return ui.layoutHomeSettingsEditor(gtx, &panel.title, "タイトル", "shortcutのタイトル")
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return ui.layoutHomeSettingsEditor(gtx, &panel.rawURL, "https://example.com/", "shortcutのURL")
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{}.Layout(gtx,
							layout.Rigid(material.Button(ui.theme, &panel.save, "保存").Layout),
							layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
							layout.Rigid(material.Button(ui.theme, &panel.add, "新規入力").Layout),
							layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
							layout.Rigid(material.Button(ui.theme, &panel.close, "完了").Layout),
						)
					}),
				}
				if panel.errorMessage != "" {
					children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						semantic.DescriptionOp("設定エラー: " + panel.errorMessage).Add(gtx.Ops)
						label := material.Body2(ui.theme, panel.errorMessage)
						label.Color = color.NRGBA{R: 185, G: 28, B: 28, A: 255}
						return label.Layout(gtx)
					}))
				}
				for index, shortcut := range ui.homeSettings.Shortcuts {
					index, shortcut := index, shortcut
					children = append(children,
						layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							row := &panel.rows[index]
							return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
								layout.Flexed(1, material.Body1(ui.theme, shortcut.Title+" · "+shortcut.URL).Layout),
								layout.Rigid(material.Button(ui.theme, &row.edit, "編集").Layout),
								layout.Rigid(material.Button(ui.theme, &row.up, "↑").Layout),
								layout.Rigid(material.Button(ui.theme, &row.down, "↓").Layout),
								layout.Rigid(material.Button(ui.theme, &row.remove, "削除").Layout),
							)
						}),
					)
				}
				dimensions := layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
				if panel.focusPending {
					panel.focusPending = false
					gtx.Execute(key.FocusCmd{Tag: &panel.title})
				}
				return dimensions
			})
		})
	})
}
