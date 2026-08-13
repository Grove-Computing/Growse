// Package paint converts layout output into renderer-independent commands.
package paint

import (
	"github.com/saku0512/growse/internal/dom"
	"github.com/saku0512/growse/internal/layout"
)

// DisplayList is an ordered collection of page painting commands.
type DisplayList struct {
	Width      float32
	Height     float32
	Background uint32
	Commands   []Command
}

// Command is implemented by every display-list operation.
type Command interface {
	paintCommand()
}

// DrawText paints one pre-laid-out line of text.
type DrawText struct {
	Text string

	X      float32
	Y      float32
	Top    float32
	Width  float32
	Height float32

	FontSize   float32
	Bold       bool
	Color      uint32
	Background uint32
	Runs       []TextRun
}

func (DrawText) paintCommand() {}

// DrawInput は編集可能な1行テキスト入力を描画する。
type DrawInput struct {
	NodeID dom.NodeID
	Value  string
	X      float32
	Y      float32
	Top    float32
	Width  float32
	Height float32
	Color  uint32
}

func (DrawInput) paintCommand() {}

// TextRun is one styled fragment within a DrawText line.
type TextRun struct {
	NodeID dom.NodeID
	Tag    string
	Text   string
	Width  float32

	FontSize   float32
	Bold       bool
	Color      uint32
	Background uint32
}

// Build creates a display list from a layout tree.
func Build(tree *layout.Tree) *DisplayList {
	if tree == nil {
		return &DisplayList{}
	}

	list := &DisplayList{Width: tree.Width, Height: tree.Height, Background: tree.Background}
	list.Commands = make([]Command, 0, len(tree.Boxes))
	previousBottom := float32(0)
	for _, box := range tree.Boxes {
		top := box.Y - previousBottom
		if top < 0 {
			top = 0
		}
		if box.Input {
			list.Commands = append(list.Commands, DrawInput{
				NodeID: box.NodeID,
				Value:  box.Text,
				X:      box.X,
				Y:      box.Y,
				Top:    top,
				Width:  box.Width,
				Height: box.Height,
				Color:  box.Color,
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
		}
		command.Runs = make([]TextRun, 0, len(box.Runs))
		for _, run := range box.Runs {
			command.Runs = append(command.Runs, TextRun{
				NodeID: run.NodeID, Tag: run.Tag, Text: run.Text, Width: run.Width,
				FontSize: run.FontSize, Bold: run.Bold, Color: run.Color, Background: run.Background,
			})
		}
		list.Commands = append(list.Commands, command)
		previousBottom = box.Y + box.Height
	}
	return list
}
