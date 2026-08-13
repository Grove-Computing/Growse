package paint

import (
	"testing"

	"github.com/saku0512/growse/internal/layout"
	"github.com/saku0512/growse/internal/style"
)

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
