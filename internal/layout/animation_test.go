package layout_test

import (
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/css"
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

func TestAnimatedParentTransformAndOpacityPropagateToDescendants(t *testing.T) {
	document := dom.NewDocument()
	parent := document.CreateElement("div", map[string]string{"class": "parent"})
	child := document.CreateElement("div", map[string]string{"class": "child"})
	if err := document.AppendChild(document.Root, parent); err != nil {
		t.Fatal(err)
	}
	if err := document.AppendChild(parent, child); err != nil {
		t.Fatal(err)
	}
	stylesheet, err := css.Parse(strings.NewReader(`
.parent { width: 100px; height: 100px; }
.child { width: 20px; height: 20px; background-color: red; }
`))
	if err != nil {
		t.Fatal(err)
	}
	computed := stylemodel.Compute(document, stylesheet)
	tree := layoutmodel.Build(document, computed, 320)
	animated := make(stylemodel.Map, len(computed))
	for nodeID, value := range computed {
		animated[nodeID] = value
	}
	parentStyle := animated[parent.ID]
	parentStyle.Opacity = 0.5
	parentStyle.Transform = []stylemodel.TransformFunction{{Kind: stylemodel.TransformTranslate, X: stylemodel.LengthPercentage{Pixels: 100}}}
	animated[parent.ID] = parentStyle
	layoutmodel.ApplyAnimatedStyles(tree, animated)

	var childDecoration *layoutmodel.Decoration
	for index := range tree.Decorations {
		if tree.Decorations[index].NodeID == child.ID {
			childDecoration = &tree.Decorations[index]
			break
		}
	}
	if childDecoration == nil {
		t.Fatal("child decoration is missing")
	}
	if childDecoration.Opacity != 0.5 || childDecoration.Transform.E != 100 {
		t.Fatalf("descendant animation state = opacity:%v transform:%#v", childDecoration.Opacity, childDecoration.Transform)
	}
	if hit, ok := layoutmodel.HitTest(tree, childDecoration.X+101, childDecoration.Y+1); !ok || hit != child.ID {
		t.Fatalf("transformed descendant hit = (%d, %v), want child %d", hit, ok, child.ID)
	}
}
