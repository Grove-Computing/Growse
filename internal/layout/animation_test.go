package layout_test

import (
	"testing"

	"github.com/Grove-Computing/Growse/internal/dom"
	layoutmodel "github.com/Grove-Computing/Growse/internal/layout"
	paintmodel "github.com/Grove-Computing/Growse/internal/paint"
	stylemodel "github.com/Grove-Computing/Growse/internal/style"
)

func TestAnimatedTransformPaintAndHitTestingUseSameMatrix(t *testing.T) {
	nodeID := dom.NodeID(7)
	tree := &layoutmodel.Tree{Decorations: []layoutmodel.Decoration{{
		NodeID: nodeID, Rect: layoutmodel.Rect{X: 10, Y: 20, Width: 40, Height: 30},
		Background: 0xff0000ff, Transform: stylemodel.IdentityMatrix(),
	}}}
	animated := stylemodel.Map{nodeID: {
		BackgroundColor: 0xff0000ff, Opacity: 1,
		Transform:       []stylemodel.TransformFunction{{Kind: stylemodel.TransformTranslate, X: stylemodel.LengthPercentage{Pixels: 100}}},
		TransformOrigin: stylemodel.BackgroundPosition{},
	}}
	layoutmodel.ApplyAnimatedStyles(tree, animated)

	command := paintmodel.Build(tree).Commands[0].(paintmodel.DrawBox)
	if command.Transform != tree.Decorations[0].Transform || command.Transform.E != 100 {
		t.Fatalf("paint/hit transform = command:%#v tree:%#v", command.Transform, tree.Decorations[0].Transform)
	}
	if hit, ok := layoutmodel.HitTest(tree, 120, 30); !ok || hit != nodeID {
		t.Fatalf("transformed hit = (%d, %v), want node 7", hit, ok)
	}
	if hit, ok := layoutmodel.HitTest(tree, 20, 30); ok || hit != 0 {
		t.Fatalf("old-position hit = (%d, %v), want miss", hit, ok)
	}
}

func TestNonInvertibleAnimatedTransformIsNotHitTested(t *testing.T) {
	nodeID := dom.NodeID(8)
	tree := &layoutmodel.Tree{Decorations: []layoutmodel.Decoration{{
		NodeID: nodeID, Rect: layoutmodel.Rect{Width: 40, Height: 30}, Transform: stylemodel.IdentityMatrix(),
	}}}
	layoutmodel.ApplyAnimatedStyles(tree, stylemodel.Map{nodeID: {
		Opacity: 1, Transform: []stylemodel.TransformFunction{{Kind: stylemodel.TransformScale, A: 0, D: 0}},
	}})
	if hit, ok := layoutmodel.HitTest(tree, 0, 0); ok || hit != 0 {
		t.Fatalf("non-invertible transform hit = (%d, %v), want miss", hit, ok)
	}
}

func TestCloneKeepsCachedLayoutPaintStateImmutable(t *testing.T) {
	nodeID := dom.NodeID(9)
	original := &layoutmodel.Tree{
		Decorations: []layoutmodel.Decoration{{NodeID: nodeID, Background: 0xff0000ff, Opacity: 1}},
		Boxes:       []layoutmodel.Box{{NodeID: nodeID, Color: 0xff0000ff, Runs: []layoutmodel.TextRun{{NodeID: nodeID, Color: 0xff0000ff}}}},
	}
	frame := layoutmodel.Clone(original)
	layoutmodel.ApplyAnimatedStyles(frame, stylemodel.Map{nodeID: {
		Color: 0x0000ffff, BackgroundColor: 0x0000ffff, Opacity: 0.5,
	}})
	if original.Decorations[0].Background != 0xff0000ff || original.Decorations[0].Opacity != 1 ||
		original.Boxes[0].Color != 0xff0000ff || original.Boxes[0].Runs[0].Color != 0xff0000ff {
		t.Fatalf("cached layout was mutated: %#v", original)
	}
}
