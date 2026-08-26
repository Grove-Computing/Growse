package layout

import (
	"github.com/Grove-Computing/Growse/internal/dom"
	stylemodel "github.com/Grove-Computing/Growse/internal/style"
)

// Hit is a hit-test result tied to the layout revision that produced it.
type Hit struct {
	NodeID   dom.NodeID
	Revision uint64
}

// HitTestWithRevision returns a result that callers can compare with paint and inspector snapshots.
func HitTestWithRevision(tree *Tree, x, y float32) (Hit, bool) {
	nodeID, ok := HitTest(tree, x, y)
	if !ok {
		return Hit{}, false
	}
	return Hit{NodeID: nodeID, Revision: tree.Revision}, true
}

// HitTest returns the deepest visible DOM node at document coordinates.
func HitTest(tree *Tree, x, y float32) (dom.NodeID, bool) {
	if tree == nil {
		return 0, false
	}
	entries := tree.OrderedPaintEntries()
	for entryIndex := len(entries) - 1; entryIndex >= 0; entryIndex-- {
		entry := entries[entryIndex]
		if entry.BoxIndex < 0 {
			decoration := tree.Decorations[entry.DecorationIndex]
			if decoration.Hidden {
				continue
			}
			localX, localY, valid := inversePoint(decoration.Transform, x, y)
			if !valid || !insideVisualClips(decoration.Clip, decoration.Clips, localX, localY) {
				continue
			}
			if containsRoundedRect(decoration.Rect, decoration.Radius, localX, localY) {
				return decoration.NodeID, true
			}
			continue
		}
		box := tree.Boxes[entry.BoxIndex]
		if box.Hidden {
			continue
		}
		localX, localY, valid := inversePoint(box.Transform, x, y)
		if !valid || !insideVisualClips(box.Clip, box.Clips, localX, localY) || !containsPoint(box.X, box.Y, box.Width, box.Height, localX, localY) {
			continue
		}

		runX := box.X
		for _, run := range box.Runs {
			if containsPoint(runX, box.Y, run.Width, box.Height, localX, localY) {
				return run.NodeID, true
			}
			runX += run.Width
		}
		return box.NodeID, true
	}
	return 0, false
}

func inversePoint(matrix stylemodel.Matrix, x, y float32) (float32, float32, bool) {
	if matrix == (stylemodel.Matrix{}) {
		matrix = stylemodel.IdentityMatrix()
	}
	inverse, valid := matrix.Inverse()
	if !valid {
		return 0, 0, false
	}
	localX, localY := inverse.TransformPoint(x, y)
	return localX, localY, true
}

func insideVisualClips(clip *Rect, clips []ClipRegion, x, y float32) bool {
	if clip != nil && !containsPoint(clip.X, clip.Y, clip.Width, clip.Height, x, y) {
		return false
	}
	for _, region := range clips {
		if !containsRoundedRect(region.Rect, region.Radius, x, y) {
			return false
		}
	}
	return true
}

func containsRoundedRect(rectangle Rect, radius BorderRadii, x, y float32) bool {
	if !containsPoint(rectangle.X, rectangle.Y, rectangle.Width, rectangle.Height, x, y) {
		return false
	}
	type corner struct {
		radius           CornerRadius
		centerX, centerY float32
		inside           func(float32, float32) bool
	}
	corners := []corner{
		{radius.TopLeft, rectangle.X + radius.TopLeft.X, rectangle.Y + radius.TopLeft.Y, func(x, y float32) bool { return x < rectangle.X+radius.TopLeft.X && y < rectangle.Y+radius.TopLeft.Y }},
		{radius.TopRight, rectangle.X + rectangle.Width - radius.TopRight.X, rectangle.Y + radius.TopRight.Y, func(x, y float32) bool {
			return x > rectangle.X+rectangle.Width-radius.TopRight.X && y < rectangle.Y+radius.TopRight.Y
		}},
		{radius.BottomRight, rectangle.X + rectangle.Width - radius.BottomRight.X, rectangle.Y + rectangle.Height - radius.BottomRight.Y, func(x, y float32) bool {
			return x > rectangle.X+rectangle.Width-radius.BottomRight.X && y > rectangle.Y+rectangle.Height-radius.BottomRight.Y
		}},
		{radius.BottomLeft, rectangle.X + radius.BottomLeft.X, rectangle.Y + rectangle.Height - radius.BottomLeft.Y, func(x, y float32) bool {
			return x < rectangle.X+radius.BottomLeft.X && y > rectangle.Y+rectangle.Height-radius.BottomLeft.Y
		}},
	}
	for _, corner := range corners {
		if corner.radius.X <= 0 || corner.radius.Y <= 0 || !corner.inside(x, y) {
			continue
		}
		dx, dy := (x-corner.centerX)/corner.radius.X, (y-corner.centerY)/corner.radius.Y
		return dx*dx+dy*dy <= 1
	}
	return true
}

func containsPoint(left, top, width, height, x, y float32) bool {
	return width > 0 && height > 0 && x >= left && x < left+width && y >= top && y < top+height
}
