package paint

import (
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/css"
	"github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/layout"
	"github.com/Grove-Computing/Growse/internal/style"
)

func TestBuildPreservesFlexOverflowGeometry(t *testing.T) {
	document := dom.NewDocument()
	container := document.CreateElement("div", map[string]string{"class": "container"})
	item := document.CreateElement("div", map[string]string{"class": "item"})
	if err := document.AppendChild(document.Root, container); err != nil {
		t.Fatal(err)
	}
	if err := document.AppendChild(container, item); err != nil {
		t.Fatal(err)
	}
	stylesheet, err := css.Parse(strings.NewReader(`
.container { display:flex; width:100px; height:30px; overflow:hidden; }
.item { flex:0 0 200px; height:30px; background-color:#ddd; }
`))
	if err != nil {
		t.Fatal(err)
	}
	tree := layout.Build(document, style.Compute(document, stylesheet), 160)
	list := Build(tree)
	var command DrawBox
	found := false
	for _, candidate := range list.Commands {
		box, ok := candidate.(DrawBox)
		if ok && box.NodeID == item.ID {
			command, found = box, true
			break
		}
	}
	if !found || command.Width != 200 || command.Clip == nil || command.Clip.Width != 100 {
		t.Fatalf("flex paint command = %#v", command)
	}
	if list.ScrollWidth != tree.ScrollWidth || list.ScrollHeight != tree.ScrollHeight {
		t.Fatalf("scroll geometry = list (%v,%v), tree (%v,%v)", list.ScrollWidth, list.ScrollHeight, tree.ScrollWidth, tree.ScrollHeight)
	}
	if _, ok := layout.HitTest(tree, command.X+150, command.Y+1); ok {
		t.Fatal("painted clip and hit testing geometry diverged")
	}
}

func TestBuildPreservesPaintOrder(t *testing.T) {
	tree := &layout.Tree{Width: 400, Height: 100, Boxes: []layout.Box{
		{Text: "first", Y: 10, Runs: []layout.TextRun{{Text: "first", Color: 0x123456ff}}},
		{Text: "second", Y: 40},
	}}

	list := Build(tree)
	if got, want := len(list.Commands), 2; got != want {
		t.Fatalf("command count = %d, want %d", got, want)
	}
	first, ok := list.Commands[0].(DrawText)
	if !ok || first.Text != "first" {
		t.Fatalf("first command = %#v, want DrawText first", list.Commands[0])
	}
	if len(first.Runs) != 1 || first.Runs[0].Color != 0x123456ff {
		t.Fatalf("first runs = %#v, want preserved inline style", first.Runs)
	}
}

func TestBuildPreservesLinearGradientStops(t *testing.T) {
	tree := &layout.Tree{Decorations: []layout.Decoration{{
		Image: style.BackgroundImage{Kind: style.BackgroundImageLinearGradient, GradientAngle: 90, GradientStops: []style.GradientStop{{Color: 0xff0000ff}, {Color: 0x0000ffff, Position: 1}}},
	}}}
	list := Build(tree)
	background, ok := list.Commands[0].(DrawBox)
	if !ok || background.Image.Kind != style.BackgroundImageLinearGradient || len(background.Image.GradientStops) != 2 {
		t.Fatalf("background command = %#v", list.Commands)
	}
	tree.Decorations[0].Image.GradientStops[0].Color = 0
	if background.Image.GradientStops[0].Color != 0xff0000ff {
		t.Fatal("display list shares mutable gradient stops with layout tree")
	}
}

func TestBuildPreservesIndependentBackgroundLayers(t *testing.T) {
	tree := &layout.Tree{Decorations: []layout.Decoration{{
		Layers: []style.BackgroundLayer{
			{Image: style.BackgroundImage{Kind: style.BackgroundImageLinearGradient, GradientStops: []style.GradientStop{{Color: 0xff0000ff}, {Color: 0x0000ffff}}}, Repeat: style.BackgroundRepeat{}},
			{Image: style.BackgroundImage{Kind: style.BackgroundImageRadialGradient, GradientStops: []style.GradientStop{{Color: 0xffffffff}, {Color: 0x000000ff}}}, Repeat: style.BackgroundRepeat{X: true}},
		},
	}}}
	command := Build(tree).Commands[0].(DrawBox)
	if len(command.Layers) != 2 || command.Layers[1].Image.Kind != style.BackgroundImageRadialGradient || !command.Layers[1].Repeat.X {
		t.Fatalf("display-list layers = %#v", command.Layers)
	}
	tree.Decorations[0].Layers[1].Image.GradientStops[0].Color = 0
	if command.Layers[1].Image.GradientStops[0].Color != 0xffffffff {
		t.Fatal("display list shares mutable background layers")
	}
}

func TestBuildPreservesBackgroundImagePlacement(t *testing.T) {
	tree := &layout.Tree{Decorations: []layout.Decoration{{
		Image:  style.BackgroundImage{Kind: style.BackgroundImageURL, URL: "https://example.com/card.png"},
		Repeat: style.BackgroundRepeat{X: true}, Position: style.BackgroundPosition{X: style.LengthPercentage{Percentage: 50}},
		Size: style.BackgroundSize{Kind: style.BackgroundSizeContain},
	}}}
	background := Build(tree).Commands[0].(DrawBox)
	if background.Image.URL != "https://example.com/card.png" || !background.Repeat.X || background.Repeat.Y || background.Position.X.Percentage != 50 || background.Size.Kind != style.BackgroundSizeContain {
		t.Fatalf("background placement = %#v", background)
	}
}

func TestBuildPreservesBorderRadiusDecorationAndOpacity(t *testing.T) {
	tree := &layout.Tree{
		Decorations: []layout.Decoration{{
			Border: style.Borders{Top: style.BorderSide{Width: 2, Style: style.BorderDashed, Color: 0xff0000ff}},
			Radius: layout.BorderRadii{TopLeft: layout.CornerRadius{X: 8, Y: 4}}, Opacity: .5,
		}},
		Boxes: []layout.Box{{Order: 1, Y: 10, Runs: []layout.TextRun{{
			Text: "line", Decoration: style.TextDecorationLineThrough, DecorationColor: 0x0000ffff, Opacity: .25,
		}}}},
	}
	list := Build(tree)
	background := list.Commands[0].(DrawBox)
	text := list.Commands[1].(DrawText)
	if background.Border.Top.Style != style.BorderDashed || background.Radius.TopLeft.X != 8 || background.Opacity != .5 {
		t.Fatalf("background effect = %#v", background)
	}
	if text.Runs[0].Decoration != style.TextDecorationLineThrough || text.Runs[0].DecorationColor != 0x0000ffff || text.Runs[0].Opacity != .25 {
		t.Fatalf("text effect = %#v", text.Runs[0])
	}
}

func TestBuildPreservesShadowsAndOutline(t *testing.T) {
	shadow := style.Shadow{OffsetX: 2, OffsetY: 3, Blur: 4, Spread: 1, Color: 0x123456ff}
	tree := &layout.Tree{
		Decorations: []layout.Decoration{{BoxShadows: []style.Shadow{shadow}, Outline: style.BorderSide{Width: 2, Style: style.BorderDotted, Color: 0xff0000ff}, OutlineOffset: 3}},
		Boxes:       []layout.Box{{Order: 1, TextShadows: []style.Shadow{shadow}}},
	}
	list := Build(tree)
	box := list.Commands[0].(DrawBox)
	text := list.Commands[1].(DrawText)
	if len(box.BoxShadows) != 1 || box.BoxShadows[0] != shadow || box.Outline.Style != style.BorderDotted || box.OutlineOffset != 3 {
		t.Fatalf("box effects = %#v", box)
	}
	if len(text.TextShadows) != 1 || text.TextShadows[0] != shadow {
		t.Fatalf("text shadows = %#v", text.TextShadows)
	}
}

func TestBuildPreservesTransformMatrix(t *testing.T) {
	matrix := style.Matrix{A: 2, B: 1, C: .5, D: 3, E: 10, F: 20}
	tree := &layout.Tree{Decorations: []layout.Decoration{{Transform: matrix}}, Boxes: []layout.Box{{Order: 1, Transform: matrix}}}
	list := Build(tree)
	if list.Commands[0].(DrawBox).Transform != matrix || list.Commands[1].(DrawText).Transform != matrix {
		t.Fatalf("display-list transforms = %#v", list.Commands)
	}
}

func TestBuildPreservesNestedClipsAndCompositingLayers(t *testing.T) {
	clips := []layout.ClipRegion{{Rect: layout.Rect{Width: 100, Height: 100}, Radius: layout.BorderRadii{TopLeft: layout.CornerRadius{X: 20, Y: 20}}}, {Rect: layout.Rect{Width: 80, Height: 80}, Radius: layout.BorderRadii{TopLeft: layout.CornerRadius{X: 10, Y: 10}}}}
	tree := &layout.Tree{
		StackingContexts: []layout.StackingContext{{Parent: -1}, {Parent: 0, NodeID: 7, Opacity: .5, Offscreen: true}},
		Decorations:      []layout.Decoration{{Clips: clips}},
	}
	list := Build(tree)
	command := list.Commands[0].(DrawBox)
	if len(command.Clips) != 2 || len(list.CompositingLayers) != 2 || !list.CompositingLayers[1].Offscreen {
		t.Fatalf("clip/layer display data = %#v / %#v", command.Clips, list.CompositingLayers)
	}
	tree.Decorations[0].Clips[0].Width = 1
	if command.Clips[0].Width != 100 {
		t.Fatal("display list shares clip regions")
	}
}

func TestBuildUsesExactLayoutDecorationGeometry(t *testing.T) {
	tree := &layout.Tree{Decorations: []layout.Decoration{{
		NodeID: 12, Rect: layout.Rect{X: 14, Y: 25, Width: 120, Height: 60},
		Radius: layout.BorderRadii{TopLeft: layout.CornerRadius{X: 18, Y: 9}},
		Clip:   &layout.Rect{X: 10, Y: 20, Width: 100, Height: 50},
	}}}
	command := Build(tree).Commands[0].(DrawBox)
	if command.NodeID != 12 || command.X != 14 || command.Y != 25 || command.Width != 120 || command.Height != 60 ||
		command.Radius != tree.Decorations[0].Radius || command.Clip == tree.Decorations[0].Clip || *command.Clip != *tree.Decorations[0].Clip {
		t.Fatalf("paint geometry = %#v, layout = %#v", command, tree.Decorations[0])
	}
}

func TestBuildCreatesInputCommand(t *testing.T) {
	tree := &layout.Tree{Width: 400, Height: 100, Boxes: []layout.Box{{
		NodeID: 7, Tag: "input", Text: "hello", Input: true,
		X: 20, Y: 30, Width: 280, Height: 40,
	}}}

	list := Build(tree)
	if got, want := len(list.Commands), 1; got != want {
		t.Fatalf("command count = %d, want %d", got, want)
	}
	input, ok := list.Commands[0].(DrawInput)
	if !ok || input.NodeID != 7 || input.Value != "hello" || input.Width != 280 || input.Height != 40 {
		t.Fatalf("input command = %#v, want node 7 with value and geometry", list.Commands[0])
	}
}

func TestBuildPreservesMultilineTextareaCommand(t *testing.T) {
	tree := &layout.Tree{Width: 400, Height: 120, Boxes: []layout.Box{{
		NodeID: 8, Tag: "textarea", Text: "first\nsecond", Input: true, Multiline: true,
		X: 20, Y: 30, Width: 280, Height: 96,
	}}}

	list := Build(tree)
	input, ok := list.Commands[0].(DrawInput)
	if !ok || !input.Multiline || input.Value != "first\nsecond" {
		t.Fatalf("textarea command = %#v", list.Commands[0])
	}
}

func TestBuildPaintsBoxBackgroundBeforeContent(t *testing.T) {
	tree := &layout.Tree{
		Decorations: []layout.Decoration{{Order: 1, NodeID: 7, Rect: layout.Rect{X: 10, Y: 20, Width: 100, Height: 40}, Background: 0x123456ff}},
		Boxes:       []layout.Box{{Order: 2, NodeID: 7, Text: "card", X: 18, Y: 28, Width: 84, Height: 20}},
	}
	list := Build(tree)
	if len(list.Commands) != 2 {
		t.Fatalf("commands = %#v", list.Commands)
	}
	background, backgroundOK := list.Commands[0].(DrawBox)
	content, contentOK := list.Commands[1].(DrawText)
	if !backgroundOK || !contentOK || background.Color != 0x123456ff || background.Width != 100 || content.Text != "card" {
		t.Fatalf("commands = %#v", list.Commands)
	}
}
