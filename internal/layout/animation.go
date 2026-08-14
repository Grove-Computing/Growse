package layout

import stylemodel "github.com/Grove-Computing/Growse/internal/style"

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
		decoration.Opacity = computed.Opacity
		decoration.Transform = animatedTransform(computed, decoration.Rect)
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
			box.Opacity = computed.Opacity
			box.Transform = animatedTransform(computed, box.Rect())
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
				box.Runs[runIndex].Opacity = runStyle.Opacity
			}
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
