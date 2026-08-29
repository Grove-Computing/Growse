package paint

import (
	"testing"

	"github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/layout"
	"github.com/Grove-Computing/Growse/internal/style"
)

func TestApplyAnimatedStylesChangesPaintWithoutLayoutGeometry(t *testing.T) {
	nodeID := dom.NodeID(9)
	tree := &layout.Tree{
		Width: 320, Height: 240,
		Decorations: []layout.Decoration{{
			Order: 1, NodeID: nodeID, Rect: layout.Rect{X: 10, Y: 20, Width: 100, Height: 40},
			Background: 0xff0000ff, Opacity: 1, Transform: style.IdentityMatrix(),
		}},
		Boxes: []layout.Box{{
			Order: 2, NodeID: nodeID, Text: "animated", X: 10, Y: 20, Width: 100, Height: 40,
			Color: 0xff0000ff, Opacity: 1, Transform: style.IdentityMatrix(),
		}},
	}
	list := Build(tree)
	originalBox := list.Commands[0].(DrawBox)
	originalText := list.Commands[1].(DrawText)
	animated := style.Map{nodeID: {
		Color: 0x0000ffff, BackgroundColor: 0x00ff00ff, Opacity: 0.4,
		Transform:       []style.TransformFunction{{Kind: style.TransformTranslate, X: style.LengthPercentage{Pixels: 30}}},
		TransformOrigin: style.BackgroundPosition{},
	}}

	ApplyAnimatedStyles(list, animated)
	box := list.Commands[0].(DrawBox)
	text := list.Commands[1].(DrawText)
	if box.Color != 0x00ff00ff || text.Color != 0x0000ffff || box.Opacity != 0.4 || text.Opacity != 0.4 {
		t.Fatalf("animated paint values = box:%#v text:%#v", box, text)
	}
	if box.Transform.E != 30 || text.Transform.E != 30 {
		t.Fatalf("animated transforms = box:%#v text:%#v", box.Transform, text.Transform)
	}
	if box.X != originalBox.X || box.Y != originalBox.Y || box.Width != originalBox.Width || box.Height != originalBox.Height ||
		text.X != originalText.X || text.Y != originalText.Y || text.Width != originalText.Width || text.Height != originalText.Height {
		t.Fatal("paint-only animation changed layout geometry")
	}
	if tree.Decorations[0].Background != 0xff0000ff || tree.Boxes[0].Color != 0xff0000ff {
		t.Fatal("paint-only animation mutated the cached layout tree")
	}
}

func TestApplyAnimatedLayoutReusesStaticDisplayListStorage(t *testing.T) {
	nodeID := dom.NodeID(9)
	tree := &layout.Tree{
		Width: 320, Height: 240, Parents: map[dom.NodeID]dom.NodeID{nodeID: 0},
		Decorations: []layout.Decoration{{Order: 1, NodeID: nodeID, Rect: layout.Rect{X: 10, Y: 20, Width: 100, Height: 40}, Opacity: 1, Transform: style.IdentityMatrix()}},
		Boxes:       []layout.Box{{Order: 2, NodeID: nodeID, Text: "animated", X: 10, Y: 20, Width: 100, Height: 40, Opacity: 1, Transform: style.IdentityMatrix()}},
	}
	list := Build(tree)
	commands := &list.Commands[0]
	animatedTree := layout.Clone(tree)
	animated := style.Map{nodeID: {Opacity: 0.4, Transform: []style.TransformFunction{{Kind: style.TransformTranslate, X: style.LengthPercentage{Pixels: 30}}}}}
	layout.ApplyAnimatedStyles(animatedTree, animated)
	ApplyAnimatedLayout(list, animatedTree)

	if commands != &list.Commands[0] {
		t.Fatal("animation frame replaced static display-list storage")
	}
	box := list.Commands[0].(DrawBox)
	text := list.Commands[1].(DrawText)
	if box.Opacity != 0.4 || text.Opacity != 0.4 || box.Transform.E != 30 || text.Transform.E != 30 {
		t.Fatalf("composited display list = box:%#v text:%#v", box, text)
	}
}
