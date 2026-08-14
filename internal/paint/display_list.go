// Package paint converts layout output into renderer-independent commands.
package paint

import (
	"github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/forms"
	"github.com/Grove-Computing/Growse/internal/layout"
	stylemodel "github.com/Grove-Computing/Growse/internal/style"
)

// DisplayList is an ordered collection of page painting commands.
type DisplayList struct {
	Width             float32
	Height            float32
	ScrollWidth       float32
	ScrollHeight      float32
	Background        uint32
	Commands          []Command
	CompositingLayers []layout.StackingContext
}

// Command is implemented by every display-list operation.
type Command interface {
	paintCommand()
}

// DrawText paints one pre-laid-out line of text.
type DrawText struct {
	NodeID dom.NodeID
	Text   string

	X        float32
	Y        float32
	Top      float32
	Width    float32
	Height   float32
	Baseline float32
	Clip     *layout.Rect
	Clips    []layout.ClipRegion

	FontSize        float32
	Bold            bool
	Color           uint32
	Background      uint32
	Decoration      stylemodel.TextDecorationLine
	DecorationColor uint32
	Opacity         float32
	Runs            []TextRun
	TextShadows     []stylemodel.Shadow
	Transform       stylemodel.Matrix
}

func (DrawText) paintCommand() {}

// DrawInput は編集可能な単一行または複数行のテキスト入力を描画する。
type DrawInput struct {
	NodeID    dom.NodeID
	Value     string
	InputType string
	Multiline bool
	X         float32
	Y         float32
	Top       float32
	Width     float32
	Height    float32
	Color     uint32
	Opacity   float32
	Clip      *layout.Rect
}

func (DrawInput) paintCommand() {}

// DrawSelect paints one single-selection control.
type DrawSelect struct {
	NodeID   dom.NodeID
	Options  []forms.Option
	Selected int
	Label    string
	X        float32
	Y        float32
	Top      float32
	Width    float32
	Height   float32
	Color    uint32
	Opacity  float32
	Clip     *layout.Rect
}

func (DrawSelect) paintCommand() {}

// DrawCheckable paints a checkbox or radio control.
type DrawCheckable struct {
	NodeID    dom.NodeID
	InputType string
	Checked   bool
	X         float32
	Y         float32
	Top       float32
	Width     float32
	Height    float32
	Color     uint32
	Opacity   float32
	Clip      *layout.Rect
}

func (DrawCheckable) paintCommand() {}

// DrawBox paints an element background without advancing by its painted height.
// Its Top value only moves the list cursor to the element's document position.
type DrawBox struct {
	NodeID        dom.NodeID
	X             float32
	Y             float32
	Top           float32
	Width         float32
	Height        float32
	Color         uint32
	Image         stylemodel.BackgroundImage
	Layers        []stylemodel.BackgroundLayer
	Repeat        stylemodel.BackgroundRepeat
	Position      stylemodel.BackgroundPosition
	Size          stylemodel.BackgroundSize
	Border        stylemodel.Borders
	Radius        layout.BorderRadii
	Opacity       float32
	Clip          *layout.Rect
	Clips         []layout.ClipRegion
	BoxShadows    []stylemodel.Shadow
	Outline       stylemodel.BorderSide
	OutlineOffset float32
	Transform     stylemodel.Matrix
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
	TextShadows     []stylemodel.Shadow
}

// Build creates a display list from a layout tree.
func Build(tree *layout.Tree) *DisplayList {
	if tree == nil {
		return &DisplayList{}
	}

	list := &DisplayList{
		Width: tree.Width, Height: tree.Height, ScrollWidth: tree.ScrollWidth,
		ScrollHeight: tree.ScrollHeight, Background: tree.Background,
		CompositingLayers: append([]layout.StackingContext(nil), tree.StackingContexts...),
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
			if tree.Decorations[entry.DecorationIndex].Hidden {
				continue
			}
			items = append(items, orderedItem{order: entry.Order, decoration: &tree.Decorations[entry.DecorationIndex]})
		} else {
			if tree.Boxes[entry.BoxIndex].Hidden {
				continue
			}
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
				Image: cloneBackgroundImage(decoration.Image), Layers: cloneBackgroundLayers(decoration.Layers), Repeat: decoration.Repeat,
				Position: decoration.Position, Size: decoration.Size, Clip: cloneLayoutRect(decoration.Clip),
				Border: decoration.Border, Radius: decoration.Radius, Opacity: decoration.Opacity,
				BoxShadows: append([]stylemodel.Shadow(nil), decoration.BoxShadows...), Outline: decoration.Outline, OutlineOffset: decoration.OutlineOffset,
				Transform: decoration.Transform,
				Clips:     cloneClipRegions(decoration.Clips),
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
				NodeID:    box.NodeID,
				Value:     box.Text,
				InputType: box.InputType,
				Multiline: box.Multiline,
				X:         box.X,
				Y:         box.Y,
				Top:       top,
				Width:     box.Width,
				Height:    box.Height,
				Color:     box.Color,
				Opacity:   box.Opacity,
				Clip:      cloneLayoutRect(box.Clip),
			})
			previousBottom = box.Y + box.Height
			continue
		}
		if box.Select {
			list.Commands = append(list.Commands, DrawSelect{
				NodeID: box.NodeID, Options: append([]forms.Option(nil), box.Options...), Selected: box.Selected, Label: box.Text,
				X: box.X, Y: box.Y, Top: top, Width: box.Width, Height: box.Height,
				Color: box.Color, Opacity: box.Opacity, Clip: cloneLayoutRect(box.Clip),
			})
			previousBottom = box.Y + box.Height
			continue
		}
		if box.Checkable {
			list.Commands = append(list.Commands, DrawCheckable{
				NodeID: box.NodeID, InputType: box.InputType, Checked: box.Checked,
				X: box.X, Y: box.Y, Top: top, Width: box.Width, Height: box.Height,
				Color: box.Color, Opacity: box.Opacity, Clip: cloneLayoutRect(box.Clip),
			})
			previousBottom = box.Y + box.Height
			continue
		}
		command := DrawText{
			NodeID:     box.NodeID,
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
			Baseline:    box.Baseline - box.Y,
			Clip:        cloneLayoutRect(box.Clip),
			TextShadows: append([]stylemodel.Shadow(nil), box.TextShadows...),
			Transform:   box.Transform,
			Clips:       cloneClipRegions(box.Clips),
		}
		command.Runs = make([]TextRun, 0, len(box.Runs))
		for _, run := range box.Runs {
			command.Runs = append(command.Runs, TextRun{
				NodeID: run.NodeID, Tag: run.Tag, Text: run.Text, Width: run.Width,
				FontSize: run.FontSize, Bold: run.Bold, Color: run.Color, Background: run.Background,
				Baseline: run.Baseline - box.Y, Decoration: run.Decoration,
				DecorationColor: run.DecorationColor, Opacity: run.Opacity,
				TextShadows: append([]stylemodel.Shadow(nil), run.TextShadows...),
			})
		}
		list.Commands = append(list.Commands, command)
		previousBottom = box.Y + box.Height
	}
	return list
}

// ApplyAnimatedStyles updates paint-only properties on an existing display
// list. Geometry and text metrics are intentionally left untouched, so a
// frame containing only v0.7 animatable properties does not require layout.
func ApplyAnimatedStyles(list *DisplayList, styles stylemodel.Map) {
	if list == nil {
		return
	}
	for index, raw := range list.Commands {
		switch command := raw.(type) {
		case DrawBox:
			computed, ok := styles[command.NodeID]
			if !ok {
				continue
			}
			command.Color = computed.BackgroundColor
			command.Border.Top.Color = computed.Border.Top.Color
			command.Border.Right.Color = computed.Border.Right.Color
			command.Border.Bottom.Color = computed.Border.Bottom.Color
			command.Border.Left.Color = computed.Border.Left.Color
			command.Outline.Color = computed.Outline.Color
			command.Opacity = computed.Opacity
			command.Transform = resolvedPaintTransform(computed, command.X, command.Y, command.Width, command.Height)
			list.Commands[index] = command
		case DrawText:
			computed, ok := styles[command.NodeID]
			if ok {
				command.Color = computed.Color
				command.Background = computed.BackgroundColor
				command.DecorationColor = computed.DecorationColor
				command.Opacity = computed.Opacity
				command.Transform = resolvedPaintTransform(computed, command.X, command.Y, command.Width, command.Height)
			}
			for runIndex := range command.Runs {
				if runStyle, exists := styles[command.Runs[runIndex].NodeID]; exists {
					command.Runs[runIndex].Color = runStyle.Color
					command.Runs[runIndex].Background = runStyle.BackgroundColor
					command.Runs[runIndex].DecorationColor = runStyle.DecorationColor
					command.Runs[runIndex].Opacity = runStyle.Opacity
				}
			}
			list.Commands[index] = command
		case DrawInput:
			computed, ok := styles[command.NodeID]
			if !ok {
				continue
			}
			command.Color = computed.Color
			command.Opacity = computed.Opacity
			list.Commands[index] = command
		case DrawSelect:
			computed, ok := styles[command.NodeID]
			if !ok {
				continue
			}
			command.Color = computed.Color
			command.Opacity = computed.Opacity
			list.Commands[index] = command
		case DrawCheckable:
			computed, ok := styles[command.NodeID]
			if !ok {
				continue
			}
			command.Color = computed.Color
			command.Opacity = computed.Opacity
			list.Commands[index] = command
		}
	}
}

func resolvedPaintTransform(computed stylemodel.ComputedStyle, x, y, width, height float32) stylemodel.Matrix {
	originX := x + computed.TransformOrigin.X.Resolve(width)
	originY := y + computed.TransformOrigin.Y.Resolve(height)
	result := stylemodel.Matrix{A: 1, D: 1, E: originX, F: originY}.Multiply(stylemodel.ResolveTransform(computed.Transform, width, height))
	return result.Multiply(stylemodel.Matrix{A: 1, D: 1, E: -originX, F: -originY})
}

func cloneLayoutRect(source *layout.Rect) *layout.Rect {
	if source == nil {
		return nil
	}
	copy := *source
	return &copy
}

func cloneClipRegions(source []layout.ClipRegion) []layout.ClipRegion {
	return append([]layout.ClipRegion(nil), source...)
}

func cloneBackgroundImage(source stylemodel.BackgroundImage) stylemodel.BackgroundImage {
	result := source
	result.GradientStops = append([]stylemodel.GradientStop(nil), source.GradientStops...)
	return result
}

func cloneBackgroundLayers(source []stylemodel.BackgroundLayer) []stylemodel.BackgroundLayer {
	result := append([]stylemodel.BackgroundLayer(nil), source...)
	for index := range result {
		result[index].Image = cloneBackgroundImage(result[index].Image)
	}
	return result
}
