package layout

import "github.com/saku0512/growse/internal/dom"

// HitTest returns the deepest visible DOM node at document coordinates.
func HitTest(tree *Tree, x, y float32) (dom.NodeID, bool) {
	if tree == nil {
		return 0, false
	}
	for boxIndex := len(tree.Boxes) - 1; boxIndex >= 0; boxIndex-- {
		box := tree.Boxes[boxIndex]
		if box.Clip != nil && !containsPoint(box.Clip.X, box.Clip.Y, box.Clip.Width, box.Clip.Height, x, y) {
			continue
		}
		if !containsPoint(box.X, box.Y, box.Width, box.Height, x, y) {
			continue
		}

		runX := box.X
		for _, run := range box.Runs {
			if containsPoint(runX, box.Y, run.Width, box.Height, x, y) {
				return run.NodeID, true
			}
			runX += run.Width
		}
		return box.NodeID, true
	}
	for decorationIndex := len(tree.Decorations) - 1; decorationIndex >= 0; decorationIndex-- {
		decoration := tree.Decorations[decorationIndex]
		if decoration.Clip != nil && !containsPoint(decoration.Clip.X, decoration.Clip.Y, decoration.Clip.Width, decoration.Clip.Height, x, y) {
			continue
		}
		if containsPoint(decoration.X, decoration.Y, decoration.Width, decoration.Height, x, y) {
			return decoration.NodeID, true
		}
	}
	return 0, false
}

func containsPoint(left, top, width, height, x, y float32) bool {
	return width > 0 && height > 0 && x >= left && x < left+width && y >= top && y < top+height
}
