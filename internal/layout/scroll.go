package layout

import (
	"math"

	"github.com/Grove-Computing/Growse/internal/dom"
	stylemodel "github.com/Grove-Computing/Growse/internal/style"
)

// ApplyScrollOffset repositions only viewport-attached and sticky subtrees.
// Normal-flow fragments, glyph runs, and image geometry remain untouched.
func ApplyScrollOffset(tree *Tree, styles stylemodel.Map, scrollX, scrollY float32) []dom.NodeID {
	if tree == nil || tree.ScrollX == scrollX && tree.ScrollY == scrollY {
		return nil
	}
	var dirty []dom.NodeID
	for nodeID, computed := range styles {
		bounds, exists := tree.Bounds[nodeID]
		if !exists {
			continue
		}
		dx, dy := float32(0), float32(0)
		switch computed.Position {
		case stylemodel.PositionFixed:
			dx, dy = scrollX-tree.ScrollX, scrollY-tree.ScrollY
		case stylemodel.PositionSticky:
			top, hasTop := fixedLength(computed.Inset.Top)
			if !hasTop {
				continue
			}
			oldAnchor := tree.ScrollY + top
			if float32(math.Abs(float64(bounds.Y-oldAnchor))) < 0.01 {
				dy = scrollY - tree.ScrollY
			} else {
				dy = max(scrollY+top-bounds.Y, float32(0))
			}
		default:
			continue
		}
		if dx == 0 && dy == 0 {
			continue
		}
		translateNodeSubtree(tree, nodeID, dx, dy)
		dirty = append(dirty, nodeID)
	}
	tree.ScrollX, tree.ScrollY = scrollX, scrollY
	UpdateCompositingLayers(tree, styles)
	return dirty
}

func fixedLength(value stylemodel.SizeValue) (float32, bool) {
	return value.Value.Pixels, value.Kind == stylemodel.SizeLength && value.Value.Percentage == 0
}

func translateNodeSubtree(tree *Tree, root dom.NodeID, dx, dy float32) {
	belongs := func(nodeID dom.NodeID) bool {
		for current := nodeID; current != 0; current = tree.Parents[current] {
			if current == root {
				return true
			}
		}
		return false
	}
	for nodeID, bounds := range tree.Bounds {
		if belongs(nodeID) {
			bounds.X, bounds.Y = bounds.X+dx, bounds.Y+dy
			tree.Bounds[nodeID] = bounds
		}
	}
	for index := range tree.Boxes {
		if belongs(tree.Boxes[index].NodeID) {
			translateBox(&tree.Boxes[index], dx, dy)
		}
	}
	for index := range tree.Decorations {
		if belongs(tree.Decorations[index].NodeID) {
			translateDecoration(&tree.Decorations[index], dx, dy)
		}
	}
}

func translateBox(box *Box, dx, dy float32) {
	box.X, box.Y, box.Baseline = box.X+dx, box.Y+dy, box.Baseline+dy
	box.ImageRect.X, box.ImageRect.Y = box.ImageRect.X+dx, box.ImageRect.Y+dy
	box.ImageClip.X, box.ImageClip.Y = box.ImageClip.X+dx, box.ImageClip.Y+dy
	if box.Clip != nil {
		box.Clip.X, box.Clip.Y = box.Clip.X+dx, box.Clip.Y+dy
	}
	for index := range box.Clips {
		box.Clips[index].X, box.Clips[index].Y = box.Clips[index].X+dx, box.Clips[index].Y+dy
	}
	for index := range box.Runs {
		box.Runs[index].Baseline += dy
	}
}

func translateDecoration(decoration *Decoration, dx, dy float32) {
	decoration.X, decoration.Y = decoration.X+dx, decoration.Y+dy
	if decoration.Clip != nil {
		decoration.Clip.X, decoration.Clip.Y = decoration.Clip.X+dx, decoration.Clip.Y+dy
	}
	for index := range decoration.Clips {
		decoration.Clips[index].X, decoration.Clips[index].Y = decoration.Clips[index].X+dx, decoration.Clips[index].Y+dy
	}
}
