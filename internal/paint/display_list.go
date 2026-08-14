// Package paint converts layout output into renderer-independent commands.
package paint

import (
	"github.com/saku0512/growse/internal/dom"
	"github.com/saku0512/growse/internal/layout"
	stylemodel "github.com/saku0512/growse/internal/style"
)

// DisplayList is an ordered collection of page painting commands.
type DisplayList struct {
	Width        float32
	Height       float32
	ScrollWidth  float32
	ScrollHeight float32
	Background   uint32
	Commands     []Command
}

// Command is implemented by every display-list operation.
type Command interface {
	paintCommand()
}

// DrawText paints one pre-laid-out line of text.
type DrawText struct {
	Text string

	X        float32
	Y        float32
	Top      float32
	Width    float32
	Height   float32
	Baseline float32
	Clip     *layout.Rect

	FontSize        float32
	Bold            bool
	Color           uint32
	Background      uint32
	Decoration      stylemodel.TextDecorationLine
	DecorationColor uint32
	Opacity         float32
	Runs            []TextRun
}

func (DrawText) paintCommand() {}

// DrawInput は編集可能な1行テキスト入力を描画する。
type DrawInput struct {
	NodeID  dom.NodeID
	Value   string
	X       float32
	Y       float32
	Top     float32
	Width   float32
	Height  float32
	Color   uint32
	Opacity float32
	Clip    *layout.Rect
}

func (DrawInput) paintCommand() {}

// DrawBox paints an element background without advancing by its painted height.
// Its Top value only moves the list cursor to the element's document position.
type DrawBox struct {
	NodeID   dom.NodeID
	X        float32
	Y        float32
	Top      float32
	Width    float32
	Height   float32
	Color    uint32
	Image    stylemodel.BackgroundImage
	Repeat   stylemodel.BackgroundRepeat
	Position stylemodel.BackgroundPosition
	Size     stylemodel.BackgroundSize
	Border   stylemodel.Borders
	Radius   layout.BorderRadii
	Opacity  float32
	Clip     *layout.Rect
}

func (DrawBox) paintCommand() {}

// TextRun is one styled fragment within a DrawText line.
type TextRun struct {
	NodeID dom.NodeID
	Tag    string
	Text   string
	Width  float32

	FontSize        float32
	Bold            bool
	Color           uint32
	Background      uint32
	Baseline        float32
	Decoration      stylemodel.TextDecorationLine
	DecorationColor uint32
	Opacity         float32
}

// Build creates a display list from a layout tree.
func Build(tree *layout.Tree) *DisplayList {
	if tree == nil {
		return &DisplayList{}
	}

	list := &DisplayList{
		Width: tree.Width, Height: tree.Height, ScrollWidth: tree.ScrollWidth,
		ScrollHeight: tree.ScrollHeight, Background: tree.Background,
	}
	type orderedItem struct {
		order      int
		decoration *layout.Decoration
		box        *layout.Box
	}
	entries := tree.OrderedPaintEntries()
	items := make([]orderedItem, 0, len(entries))
	for _, entry := range entries {
		if entry.DecorationIndex >= 0 {
			items = append(items, orderedItem{order: entry.Order, decoration: &tree.Decorations[entry.DecorationIndex]})
		} else {
			items = append(items, orderedItem{order: entry.Order, box: &tree.Boxes[entry.BoxIndex]})
		}
	}

	list.Commands = make([]Command, 0, len(items))
	previousBottom := float32(0)
	for _, item := range items {
		if item.decoration != nil {
			decoration := item.decoration
			top := max(decoration.Y-previousBottom, float32(0))
			list.Commands = append(list.Commands, DrawBox{
				NodeID: decoration.NodeID, X: decoration.X, Y: decoration.Y, Top: top,
				Width: decoration.Width, Height: decoration.Height, Color: decoration.Background,
				Image: cloneBackgroundImage(decoration.Image), Repeat: decoration.Repeat,
				Position: decoration.Position, Size: decoration.Size, Clip: cloneLayoutRect(decoration.Clip),
				Border: decoration.Border, Radius: decoration.Radius, Opacity: decoration.Opacity,
			})
			previousBottom += top
			continue
		}
		box := *item.box
		top := box.Y - previousBottom
		if top < 0 {
			top = 0
		}
		if box.Input {
			list.Commands = append(list.Commands, DrawInput{
				NodeID:  box.NodeID,
				Value:   box.Text,
				X:       box.X,
				Y:       box.Y,
				Top:     top,
				Width:   box.Width,
				Height:  box.Height,
				Color:   box.Color,
				Opacity: box.Opacity,
				Clip:    cloneLayoutRect(box.Clip),
			})
			previousBottom = box.Y + box.Height
			continue
		}
		command := DrawText{
			Text:       box.Text,
			X:          box.X,
			Y:          box.Y,
			Top:        top,
			Width:      box.Width,
			Height:     box.Height,
			FontSize:   box.FontSize,
			Bold:       box.Bold,
			Color:      box.Color,
			Background: box.Background,
			Decoration: box.Decoration, DecorationColor: box.DecorationColor, Opacity: box.Opacity,
			Baseline: box.Baseline - box.Y,
			Clip:     cloneLayoutRect(box.Clip),
		}
		command.Runs = make([]TextRun, 0, len(box.Runs))
		for _, run := range box.Runs {
			command.Runs = append(command.Runs, TextRun{
				NodeID: run.NodeID, Tag: run.Tag, Text: run.Text, Width: run.Width,
				FontSize: run.FontSize, Bold: run.Bold, Color: run.Color, Background: run.Background,
				Baseline: run.Baseline - box.Y, Decoration: run.Decoration,
				DecorationColor: run.DecorationColor, Opacity: run.Opacity,
			})
		}
		list.Commands = append(list.Commands, command)
		previousBottom = box.Y + box.Height
	}
	return list
}

func cloneLayoutRect(source *layout.Rect) *layout.Rect {
	if source == nil {
		return nil
	}
	copy := *source
	return &copy
}

func cloneBackgroundImage(source stylemodel.BackgroundImage) stylemodel.BackgroundImage {
	result := source
	result.GradientStops = append([]stylemodel.GradientStop(nil), source.GradientStops...)
	return result
}
