package layout

import (
	"testing"

	"github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/style"
)

func TestCompositingLayersPromoteEffectsAndProduceLocalDamage(t *testing.T) {
	tree := &Tree{
		Width: 800, Height: 600,
		Bounds:  map[dom.NodeID]Rect{1: {X: 10, Y: 20, Width: 100, Height: 50}, 2: {X: 20, Y: 100, Width: 80, Height: 30}},
		Parents: map[dom.NodeID]dom.NodeID{2: 1},
	}
	styles := style.Map{
		1: {
			Opacity: 0.5, Position: style.PositionFixed, OverflowX: style.OverflowScroll,
			Transform: []style.TransformFunction{{Kind: style.TransformTranslate, X: style.LengthPercentage{Pixels: 10}}},
			Filters:   []style.Filter{{Kind: style.FilterBlur, Radius: 2}},
		},
		2: {Opacity: 1, Position: style.PositionSticky},
	}
	buildCompositingLayers(tree, styles)
	if len(tree.CompositingLayers) != 2 {
		t.Fatalf("layers = %#v", tree.CompositingLayers)
	}
	want := LayerTransform | LayerOpacity | LayerClip | LayerScroll | LayerFixed | LayerFilter
	if tree.CompositingLayers[0].Reasons&want != want || tree.CompositingLayers[0].Clip == nil || tree.CompositingLayers[1].Reasons != LayerSticky || tree.CompositingLayers[1].Parent != 0 {
		t.Fatalf("promotion metadata = %#v", tree.CompositingLayers)
	}
	updated := styles[1]
	updated.Transform = []style.TransformFunction{{Kind: style.TransformTranslate, X: style.LengthPercentage{Pixels: 30}}}
	styles[1] = updated
	damage := UpdateCompositingLayers(tree, styles)
	if len(damage) != 1 || damage[0].X != 20 || damage[0].Y != 20 || damage[0].Width != 120 || damage[0].Height != 50 {
		t.Fatalf("damage = %#v", damage)
	}
}

func TestCompositingLayerCountIsBounded(t *testing.T) {
	tree := &Tree{Width: 800, Height: 600, Bounds: make(map[dom.NodeID]Rect), Parents: make(map[dom.NodeID]dom.NodeID)}
	styles := make(style.Map)
	for index := 1; index <= maxCompositingLayers+10; index++ {
		nodeID := dom.NodeID(index)
		tree.Bounds[nodeID] = Rect{X: float32(index), Width: 10, Height: 10}
		styles[nodeID] = style.ComputedStyle{Opacity: 0.5}
	}
	buildCompositingLayers(tree, styles)
	if len(tree.CompositingLayers) != maxCompositingLayers || len(tree.Fallbacks) != 1 || tree.Fallbacks[0].Reason != "compositing layer limit exceeded" {
		t.Fatalf("bounded layers=%d fallbacks=%#v", len(tree.CompositingLayers), tree.Fallbacks)
	}
}
