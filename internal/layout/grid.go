package layout

import (
	"strings"

	"github.com/saku0512/growse/internal/dom"
	stylemodel "github.com/saku0512/growse/internal/style"
)

// addGridChildren establishes the initial one-column grid formatting context.
// Track construction and placement extend this entry point without falling back
// to block/inline formatting for direct grid items.
func (e *engine) addGridChildren(container *dom.Node, x, width, containingHeight float32, heightDefinite bool) {
	for _, child := range container.Children {
		if child.Type == dom.NodeText {
			if text := normalizeWhitespace(child.Text); text != "" {
				e.addText(child.ID, "text", text, e.styleFor(container), x, width)
			}
			continue
		}
		if child.Type != dom.NodeElement {
			continue
		}
		childStyle := e.styleFor(child)
		if childStyle.display == stylemodel.DisplayNone {
			continue
		}
		if isTextInput(child) {
			e.addInput(child, childStyle, x, width, containingHeight, heightDefinite)
			continue
		}
		if childStyle.display == stylemodel.DisplayInlineGrid {
			childStyle.display = stylemodel.DisplayGrid
		} else if childStyle.display == stylemodel.DisplayInlineFlex {
			childStyle.display = stylemodel.DisplayFlex
		} else if childStyle.display == stylemodel.DisplayInline || childStyle.display == stylemodel.DisplayInlineBlock {
			childStyle.display = stylemodel.DisplayBlock
		}
		e.addBlock(child, childStyle, x, width, containingHeight, heightDefinite, nil)
	}
}

func (e *engine) resolveInlineGridSize(node *dom.Node, containerStyle blockStyle, containingWidth float32) (float32, float32, float32) {
	contentWidth, contentHeight, baseline := float32(0), float32(0), float32(0)
	for _, child := range node.Children {
		if child.Type == dom.NodeText && strings.TrimSpace(child.Text) == "" {
			continue
		}
		childStyle := e.styleFor(child)
		if childStyle.display == stylemodel.DisplayNone {
			continue
		}
		width, height, _ := e.flexIntrinsicSizes(child, childStyle, flexAxis{horizontal: true}, containingWidth, containingWidth, 0, false)
		contentWidth = max(contentWidth, width+childStyle.margin.Left+childStyle.margin.Right)
		contentHeight += height + childStyle.margin.Top + childStyle.margin.Bottom
		if baseline == 0 {
			_, _, ascent := measureText("Mg", childStyle.fontSize, childStyle.bold)
			baseline = ascent + childStyle.margin.Top
		}
	}
	horizontalExtras := containerStyle.padding.Left + containerStyle.padding.Right + containerStyle.border.Left.Width + containerStyle.border.Right.Width
	verticalExtras := containerStyle.padding.Top + containerStyle.padding.Bottom + containerStyle.border.Top.Width + containerStyle.border.Bottom.Width
	width, height := contentWidth+horizontalExtras, contentHeight+verticalExtras
	if resolved, ok := resolveSize(containerStyle.width, containingWidth, true); ok {
		width = resolved
		if containerStyle.boxSizing == stylemodel.BoxSizingContentBox {
			width += horizontalExtras
		}
	}
	if resolved, ok := resolveSize(containerStyle.height, 0, false); ok {
		height = resolved
		if containerStyle.boxSizing == stylemodel.BoxSizingContentBox {
			height += verticalExtras
		}
	}
	baseline += containerStyle.border.Top.Width + containerStyle.padding.Top
	if baseline <= 0 || baseline > height {
		baseline = height
	}
	return max(width, float32(1)), max(height, float32(1)), baseline
}

func (e *engine) renderInlineGrid(run inlineRun, x, y float32) {
	style := run.style
	style.display = stylemodel.DisplayGrid
	style.margin = stylemodel.Edges{}
	style.marginAuto = stylemodel.AutoEdges{}
	style.boxSizing = stylemodel.BoxSizingBorderBox
	style.width, style.height = pixelSize(run.width), pixelSize(run.height)
	startBoxes, startDecorations := len(e.tree.Boxes), len(e.tree.Decorations)
	savedY, savedClip := e.y, e.clip
	e.y, e.clip = 0, nil
	e.addBlock(run.node, style, 0, run.width, run.height, true, nil)
	e.y, e.clip = savedY, savedClip
	translateFlexGeometry(e.tree, startBoxes, startDecorations, x, y, savedClip)
}
