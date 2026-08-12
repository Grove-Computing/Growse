// Package paint converts layout output into renderer-independent commands.
package paint

import "github.com/saku0512/growse/internal/layout"

// DisplayList is an ordered collection of page painting commands.
type DisplayList struct {
	Width    float32
	Height   float32
	Commands []Command
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

	FontSize float32
	Bold     bool
	Color    uint32
}

func (DrawText) paintCommand() {}

// Build creates a display list from a layout tree.
func Build(tree *layout.Tree) *DisplayList {
	if tree == nil {
		return &DisplayList{}
	}

	list := &DisplayList{Width: tree.Width, Height: tree.Height}
	list.Commands = make([]Command, 0, len(tree.Boxes))
	previousBottom := float32(0)
	for _, box := range tree.Boxes {
		top := box.Y - previousBottom
		if top < 0 {
			top = 0
		}
		list.Commands = append(list.Commands, DrawText{
			Text:     box.Text,
			X:        box.X,
			Y:        box.Y,
			Top:      top,
			Width:    box.Width,
			Height:   box.Height,
			FontSize: box.FontSize,
			Bold:     box.Bold,
			Color:    box.Color,
		})
		previousBottom = box.Y + box.Height
	}
	return list
}
