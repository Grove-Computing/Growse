package layout

import (
	"strings"

	"github.com/Grove-Computing/Growse/internal/dom"
	stylemodel "github.com/Grove-Computing/Growse/internal/style"
)

// intrinsicKeywordSize resolves the CSS sizing keywords from the same text and
// replaced-element metrics consumed by flex, grid, positioned, and block layout.
func (e *engine) intrinsicKeywordSize(node *dom.Node, value stylemodel.SizeValue, style blockStyle, available float32, horizontal bool) (float32, bool) {
	if value.Kind != stylemodel.SizeMinContent && value.Kind != stylemodel.SizeMaxContent && value.Kind != stylemodel.SizeFitContent {
		return 0, false
	}
	horizontalExtras := style.padding.Left + style.padding.Right + style.border.Left.Width + style.border.Right.Width
	verticalExtras := style.padding.Top + style.padding.Bottom + style.border.Top.Width + style.border.Bottom.Width
	if !horizontal {
		text := normalizeWhitespace(e.inlineText(node))
		_, height, _ := measureStyledText(text, style)
		if isImageElement(node, e.images) {
			height = e.images[node.ID].IntrinsicHeight
		}
		return max(height, style.lineHeight, float32(1)) + verticalExtras, true
	}

	text := normalizeWhitespace(e.inlineText(node))
	maxContent, _, _ := measureStyledText(text, style)
	minContent := float32(0)
	for _, word := range strings.Fields(text) {
		width, _, _ := measureStyledText(word, style)
		minContent = max(minContent, width)
	}
	if isImageElement(node, e.images) {
		resource := e.images[node.ID]
		minContent, maxContent = resource.IntrinsicWidth, resource.IntrinsicWidth
	}
	minContent, maxContent = max(minContent+horizontalExtras, float32(1)), max(maxContent+horizontalExtras, float32(1))
	switch value.Kind {
	case stylemodel.SizeMinContent:
		return minContent, true
	case stylemodel.SizeMaxContent:
		return maxContent, true
	case stylemodel.SizeFitContent:
		return max(minContent, min(maxContent, max(available, float32(0)))), true
	default:
		return 0, false
	}
}
