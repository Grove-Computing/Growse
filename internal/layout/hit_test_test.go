package layout

import (
	"testing"

	"github.com/saku0512/growse/internal/dom"
	stylemodel "github.com/saku0512/growse/internal/style"
)

func TestHitTestPrefersInlineRun(t *testing.T) {
	tree := &Tree{Boxes: []Box{{
		NodeID: 2, X: 10, Y: 20, Width: 200, Height: 24,
		Runs: []TextRun{
			{NodeID: 3, Text: "Hello ", Width: 60},
			{NodeID: 4, Tag: "a", Text: "Growse", Width: 70},
		},
	}}}

	tests := []struct {
		name string
		x    float32
		y    float32
		want dom.NodeID
		ok   bool
	}{
		{name: "first run", x: 20, y: 30, want: 3, ok: true},
		{name: "link run", x: 80, y: 30, want: 4, ok: true},
		{name: "line remainder", x: 180, y: 30, want: 2, ok: true},
		{name: "right edge is outside", x: 210, y: 30, ok: false},
		{name: "below line", x: 20, y: 44, ok: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := HitTest(tree, test.x, test.y)
			if got != test.want || ok != test.ok {
				t.Fatalf("HitTest(%v, %v) = (%v, %v), want (%v, %v)", test.x, test.y, got, ok, test.want, test.ok)
			}
		})
	}
}

func TestHitTestUsesLastPaintedOverlappingBox(t *testing.T) {
	tree := &Tree{Boxes: []Box{
		{NodeID: 1, X: 0, Y: 0, Width: 100, Height: 100},
		{NodeID: 2, X: 20, Y: 20, Width: 40, Height: 40},
	}}
	got, ok := HitTest(tree, 30, 30)
	if !ok || got != 2 {
		t.Fatalf("HitTest() = (%v, %v), want deepest painted node 2", got, ok)
	}
}

func TestHitTestHandlesNilTree(t *testing.T) {
	if got, ok := HitTest(nil, 0, 0); ok || got != 0 {
		t.Fatalf("HitTest(nil) = (%v, %v), want miss", got, ok)
	}
}

func TestHitTestUsesInverseTransformAndRoundedNestedClip(t *testing.T) {
	tree := &Tree{Decorations: []Decoration{{
		NodeID: 7, Rect: Rect{X: 10, Y: 10, Width: 40, Height: 40}, Transform: stylemodel.Matrix{A: 1, D: 1, E: 100},
		Clips: []ClipRegion{{Rect: Rect{X: 10, Y: 10, Width: 40, Height: 40}, Radius: BorderRadii{TopLeft: CornerRadius{X: 20, Y: 20}}}},
	}}}
	if hit, ok := HitTest(tree, 130, 30); !ok || hit != 7 {
		t.Fatalf("transformed hit = (%d, %v)", hit, ok)
	}
	if hit, ok := HitTest(tree, 111, 11); ok || hit != 0 {
		t.Fatalf("rounded clipped corner hit = (%d, %v), want miss", hit, ok)
	}
	if hit, ok := HitTest(tree, 30, 30); ok || hit != 0 {
		t.Fatalf("untransformed location hit = (%d, %v), want miss", hit, ok)
	}
}

func TestHitTestSkipsVisibilityHiddenButKeepsOpacityZero(t *testing.T) {
	tree := &Tree{Decorations: []Decoration{
		{Order: 1, NodeID: 1, Rect: Rect{Width: 50, Height: 50}, Opacity: 0},
		{Order: 2, NodeID: 2, Rect: Rect{Width: 50, Height: 50}, Hidden: true},
	}}
	if hit, ok := HitTest(tree, 10, 10); !ok || hit != 1 {
		t.Fatalf("visibility/opacity hit = (%d, %v), want opacity-zero node 1", hit, ok)
	}
}
