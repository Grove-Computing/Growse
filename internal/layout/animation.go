package layout

import (
	"github.com/Grove-Computing/Growse/internal/dom"
	stylemodel "github.com/Grove-Computing/Growse/internal/style"
)

// Clone returns a frame-local copy of a layout tree. Geometry can be cached
// while ApplyAnimatedStyles mutates paint and hit-test state on the copy.
func Clone(tree *Tree) *Tree {
	if tree == nil {
		return nil
	}
	clone := *tree
	clone.Decorations = append([]Decoration(nil), tree.Decorations...)
	clone.Boxes = append([]Box(nil), tree.Boxes...)
	clone.StackingContexts = append([]StackingContext(nil), tree.StackingContexts...)
	clone.Parents = make(map[dom.NodeID]dom.NodeID, len(tree.Parents))
	for nodeID, parentID := range tree.Parents {
		clone.Parents[nodeID] = parentID
	}
	clone.Bounds = make(map[dom.NodeID]Rect, len(tree.Bounds))
	for nodeID, bounds := range tree.Bounds {
		clone.Bounds[nodeID] = bounds
	}
	for index := range clone.Boxes {
		clone.Boxes[index].Runs = append([]TextRun(nil), tree.Boxes[index].Runs...)
	}
	return &clone
}

// ApplyAnimatedStyles applies paint and hit-test state to an existing layout
// tree without changing its geometry. Paint.Build and HitTest consequently
// observe the exact same transform matrix.
func ApplyAnimatedStyles(tree *Tree, styles stylemodel.Map) {
	if tree == nil {
		return
	}
	for index := range tree.Decorations {
		decoration := &tree.Decorations[index]
		computed, ok := styles[decoration.NodeID]
		if !ok {
			continue
		}
		decoration.Background = computed.BackgroundColor
		decoration.Border.Top.Color = computed.Border.Top.Color
		decoration.Border.Right.Color = computed.Border.Right.Color
		decoration.Border.Bottom.Color = computed.Border.Bottom.Color
		decoration.Border.Left.Color = computed.Border.Left.Color
		decoration.Outline.Color = computed.Outline.Color
		decoration.Opacity = cumulativeOpacity(tree, styles, decoration.NodeID)
		decoration.Transform = cumulativeTransform(tree, styles, decoration.NodeID, decoration.Rect)
		decoration.Hidden = computed.Visibility == stylemodel.VisibilityHidden
		if _, invertible := decoration.Transform.Inverse(); !invertible {
			decoration.Hidden = true
		}
	}
	for index := range tree.Boxes {
		box := &tree.Boxes[index]
		computed, ok := styles[box.NodeID]
		if ok {
			box.Color = computed.Color
			box.Background = computed.BackgroundColor
			box.DecorationColor = computed.DecorationColor
			box.Opacity = cumulativeOpacity(tree, styles, box.NodeID)
			box.Transform = cumulativeTransform(tree, styles, box.NodeID, box.Rect())
			box.Hidden = computed.Visibility == stylemodel.VisibilityHidden
			if _, invertible := box.Transform.Inverse(); !invertible {
				box.Hidden = true
			}
		}
		for runIndex := range box.Runs {
			if runStyle, exists := styles[box.Runs[runIndex].NodeID]; exists {
				box.Runs[runIndex].Color = runStyle.Color
				box.Runs[runIndex].Background = runStyle.BackgroundColor
				box.Runs[runIndex].DecorationColor = runStyle.DecorationColor
				box.Runs[runIndex].Opacity = cumulativeOpacity(tree, styles, box.Runs[runIndex].NodeID)
			}
		}
	}
	for index := range tree.StackingContexts {
		context := &tree.StackingContexts[index]
		if computed, ok := styles[context.NodeID]; ok {
			context.Opacity = computed.Opacity
			context.Offscreen = computed.Opacity < 1
		}
	}
}

func (box Box) Rect() Rect {
	return Rect{X: box.X, Y: box.Y, Width: box.Width, Height: box.Height}
}

func animatedTransform(computed stylemodel.ComputedStyle, rectangle Rect) stylemodel.Matrix {
	originX := rectangle.X + computed.TransformOrigin.X.Resolve(rectangle.Width)
	originY := rectangle.Y + computed.TransformOrigin.Y.Resolve(rectangle.Height)
	result := stylemodel.Matrix{A: 1, D: 1, E: originX, F: originY}.Multiply(stylemodel.ResolveTransform(computed.Transform, rectangle.Width, rectangle.Height))
	return result.Multiply(stylemodel.Matrix{A: 1, D: 1, E: -originX, F: -originY})
}

func cumulativeOpacity(tree *Tree, styles stylemodel.Map, nodeID dom.NodeID) float32 {
	opacity := float32(1)
	for _, ancestor := range nodePath(tree, nodeID) {
		if computed, ok := styles[ancestor]; ok {
			opacity *= computed.Opacity
		}
	}
	return opacity
}

func cumulativeTransform(tree *Tree, styles stylemodel.Map, nodeID dom.NodeID, fallback Rect) stylemodel.Matrix {
	result := stylemodel.IdentityMatrix()
	for _, ancestor := range nodePath(tree, nodeID) {
		computed, ok := styles[ancestor]
		bounds, bounded := tree.Bounds[ancestor]
		if !bounded && ancestor == nodeID {
			bounds, bounded = fallback, true
		}
		if !ok || !bounded || len(computed.Transform) == 0 {
			continue
		}
		result = result.Multiply(animatedTransform(computed, bounds))
	}
	return result
}

func nodePath(tree *Tree, nodeID dom.NodeID) []dom.NodeID {
	if tree == nil || nodeID == 0 {
		return nil
	}
	reversed := make([]dom.NodeID, 0, 8)
	seen := make(map[dom.NodeID]bool)
	for current := nodeID; current != 0 && !seen[current]; current = tree.Parents[current] {
		seen[current] = true
		reversed = append(reversed, current)
	}
	path := make([]dom.NodeID, len(reversed))
	for index := range reversed {
		path[len(reversed)-1-index] = reversed[index]
	}
	return path
}
