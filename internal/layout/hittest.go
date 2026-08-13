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
		if containsRoundedRect(decoration.Rect, decoration.Radius, x, y) {
			return decoration.NodeID, true
		}
	}
	return 0, false
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
