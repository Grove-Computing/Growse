package layout

import (
	"github.com/Grove-Computing/Growse/internal/dom"
	stylemodel "github.com/Grove-Computing/Growse/internal/style"
)

type floatRegion struct {
	Rect
	side stylemodel.Float
}

func (e *engine) addFloat(node *dom.Node, style blockStyle, containingX, containingWidth, containingHeight float32, heightDefinite bool) {
	outerWidth, outerHeight, _ := e.flexIntrinsicSizes(node, style, flexAxis{horizontal: true}, containingWidth, containingWidth, containingHeight, heightDefinite)
	if isImageElement(node, e.images) {
		ratio := style.aspectRatio
		if resource := e.images[node.ID]; ratio <= 0 && resource.IntrinsicWidth > 0 && resource.IntrinsicHeight > 0 {
			ratio = resource.IntrinsicWidth / resource.IntrinsicHeight
		}
		horizontalExtras := style.padding.Left + style.padding.Right + style.border.Left.Width + style.border.Right.Width
		verticalExtras := style.padding.Top + style.padding.Bottom + style.border.Top.Width + style.border.Bottom.Width
		if ratio > 0 && style.width.Kind == stylemodel.SizeLength && style.height.Kind == stylemodel.SizeAuto {
			outerHeight = max(outerWidth-horizontalExtras, float32(0))/ratio + verticalExtras
		} else if ratio > 0 && style.height.Kind == stylemodel.SizeLength && style.width.Kind == stylemodel.SizeAuto {
			outerWidth = max(outerHeight-verticalExtras, float32(0))*ratio + horizontalExtras
		}
	}
	outerWidth += style.margin.Left + style.margin.Right
	outerHeight += style.margin.Top + style.margin.Bottom
	outerWidth = min(max(outerWidth, float32(1)), containingWidth)
	outerHeight = max(outerHeight, float32(1))

	y := e.y
	var x float32
	for {
		left, right := e.floatEdges(containingX, containingWidth, y, outerHeight)
		if right-left >= outerWidth {
			if style.float == stylemodel.FloatRight {
				x = right - outerWidth
			} else {
				x = left
			}
			break
		}
		nextY := e.nextFloatBottom(y)
		if nextY <= y {
			x = containingX
			break
		}
		y = nextY
	}

	contentX, contentY := x+style.margin.Left, y+style.margin.Top
	contentWidth := max(outerWidth-style.margin.Left-style.margin.Right, float32(1))
	contentHeight := max(outerHeight-style.margin.Top-style.margin.Bottom, float32(1))
	renderStyle := style
	renderStyle.float, renderStyle.clear = stylemodel.FloatNone, stylemodel.ClearNone
	renderStyle.margin = stylemodel.Edges{}
	if renderStyle.display == stylemodel.DisplayInline || renderStyle.display == stylemodel.DisplayInlineBlock {
		renderStyle.display = stylemodel.DisplayBlock
	}
	e.renderGridItem(node, renderStyle, contentX, contentY, contentWidth, contentHeight)
	e.tree.Bounds[node.ID] = Rect{X: contentX, Y: contentY, Width: contentWidth, Height: contentHeight}
	e.floats = append(e.floats, floatRegion{Rect: Rect{X: x, Y: y, Width: outerWidth, Height: outerHeight}, side: style.float})
}

func (e *engine) floatLineArea(x, width, y, lineHeight float32) (float32, float32) {
	if lineHeight <= 0 {
		lineHeight = 1
	}
	left, right := e.floatEdges(x, width, y, lineHeight)
	if right-left < 1 {
		nextY := e.nextFloatBottom(y)
		if nextY > y {
			e.y = nextY
			left, right = e.floatEdges(x, width, nextY, lineHeight)
		}
	}
	return left, max(right-left, float32(1))
}

func (e *engine) floatEdges(x, width, y, height float32) (float32, float32) {
	left, right := x, x+width
	for _, region := range e.floats {
		if y+height <= region.Y || y >= region.Y+region.Height {
			continue
		}
		if region.side == stylemodel.FloatLeft {
			left = max(left, region.X+region.Width)
		} else if region.side == stylemodel.FloatRight {
			right = min(right, region.X)
		}
	}
	return left, right
}

func (e *engine) nextFloatBottom(y float32) float32 {
	next := float32(0)
	for _, region := range e.floats {
		bottom := region.Y + region.Height
		if bottom > y && (next == 0 || bottom < next) {
			next = bottom
		}
	}
	return next
}

func (e *engine) clearFloats(clear stylemodel.Clear) {
	for _, region := range e.floats {
		matches := clear == stylemodel.ClearBoth || clear == stylemodel.ClearLeft && region.side == stylemodel.FloatLeft || clear == stylemodel.ClearRight && region.side == stylemodel.FloatRight
		if matches {
			e.y = max(e.y, region.Y+region.Height)
		}
	}
}
