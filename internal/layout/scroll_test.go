package layout

import (
	"testing"

	"github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/style"
)

func TestApplyScrollOffsetMovesOnlyFixedAndStickySubtrees(t *testing.T) {
	tree := &Tree{
		Width: 400, Height: 600, ScrollY: 0,
		Bounds:  map[dom.NodeID]Rect{1: {Y: 10, Width: 100, Height: 20}, 2: {Y: 100, Width: 100, Height: 20}, 3: {Y: 200, Width: 100, Height: 20}},
		Parents: map[dom.NodeID]dom.NodeID{},
		Boxes:   []Box{{NodeID: 1, Y: 10, Width: 100, Height: 20}, {NodeID: 2, Y: 100, Width: 100, Height: 20}, {NodeID: 3, Y: 200, Width: 100, Height: 20}},
	}
	styles := style.Map{
		1: {Position: style.PositionFixed},
		2: {Position: style.PositionSticky, Inset: style.Insets{Top: style.SizeValue{Kind: style.SizeLength, Value: style.LengthPercentage{Pixels: 5}}}},
		3: {Position: style.PositionStatic},
	}
	dirty := ApplyScrollOffset(tree, styles, 0, 150)
	if len(dirty) != 2 || tree.Bounds[1].Y != 160 || tree.Bounds[2].Y != 155 || tree.Bounds[3].Y != 200 {
		t.Fatalf("scroll dirty=%v bounds=%#v", dirty, tree.Bounds)
	}
}
