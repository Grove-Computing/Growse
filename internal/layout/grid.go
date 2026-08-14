package layout

import (
	"strings"

	"github.com/saku0512/growse/internal/dom"
	stylemodel "github.com/saku0512/growse/internal/style"
)

// addGridChildren establishes the initial one-column grid formatting context.
// Track construction and placement extend this entry point without falling back
// to block/inline formatting for direct grid items.
func (e *engine) addGridChildren(container *dom.Node, containerStyle blockStyle, x, width, containingHeight float32, heightDefinite bool) {
	type gridItem struct {
		node  *dom.Node
		style blockStyle
	}
	items := make([]gridItem, 0, len(container.Children))
	for _, child := range container.Children {
		if child.Type == dom.NodeText {
			if text := normalizeWhitespace(child.Text); text != "" {
				anonymous := &dom.Node{ID: child.ID, Type: dom.NodeText, Text: text}
				items = append(items, gridItem{node: anonymous, style: e.styleFor(container)})
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
		if childStyle.display == stylemodel.DisplayInlineGrid {
			childStyle.display = stylemodel.DisplayGrid
		} else if childStyle.display == stylemodel.DisplayInlineFlex {
			childStyle.display = stylemodel.DisplayFlex
		} else if childStyle.display == stylemodel.DisplayInline || childStyle.display == stylemodel.DisplayInlineBlock {
			childStyle.display = stylemodel.DisplayBlock
		}
		items = append(items, gridItem{node: child, style: childStyle})
	}
	if len(items) == 0 {
		return
	}
	columnCount := len(containerStyle.gridTemplateColumns)
	if columnCount == 0 {
		columnCount = 1
	}
	rowCount := max(len(containerStyle.gridTemplateRows), (len(items)+columnCount-1)/columnCount)
	columnMaxContent, columnMinContent := make([]float32, columnCount), make([]float32, columnCount)
	for index, item := range items {
		column := index % columnCount
		maxContent, _, minContent := e.flexIntrinsicSizes(item.node, item.style, flexAxis{horizontal: true}, width, width, containingHeight, heightDefinite)
		horizontalMargin := item.style.margin.Left + item.style.margin.Right
		columnMaxContent[column] = max(columnMaxContent[column], maxContent+horizontalMargin)
		columnMinContent[column] = max(columnMinContent[column], minContent+horizontalMargin)
	}
	columnGap := containerStyle.columnGap.Resolve(width)
	rowGap := containerStyle.rowGap.Resolve(containingHeight)
	columns := resolveGridTracks(containerStyle.gridTemplateColumns, containerStyle.gridAutoColumns, columnCount, width, true, columnGap, columnMinContent, columnMaxContent)
	rowMaxContent := make([]float32, rowCount)
	for index, item := range items {
		row, column := index/columnCount, index%columnCount
		_, intrinsicHeight, _ := e.flexIntrinsicSizes(item.node, item.style, flexAxis{horizontal: true}, columns[column], columns[column], containingHeight, heightDefinite)
		rowMaxContent[row] = max(rowMaxContent[row], intrinsicHeight+item.style.margin.Top+item.style.margin.Bottom)
	}
	rows := resolveGridTracks(containerStyle.gridTemplateRows, containerStyle.gridAutoRows, rowCount, containingHeight, heightDefinite, rowGap, rowMaxContent, rowMaxContent)
	startY := e.y
	for index, item := range items {
		row, column := index/columnCount, index%columnCount
		itemX, itemY := x+trackOffset(columns, column, columnGap), startY+trackOffset(rows, row, rowGap)
		e.renderGridItem(item.node, item.style, itemX, itemY, columns[column], rows[row])
	}
	e.y = startY + trackOffset(rows, len(rows), rowGap)
}

func resolveGridTracks(explicit, implicit []stylemodel.GridTrackSize, count int, basis float32, basisDefinite bool, gap float32, minContent, maxContent []float32) []float32 {
	result := make([]float32, count)
	tracks := make([]stylemodel.GridTrackSize, count)
	autoCount, flexTotal, used := 0, float32(0), gap*float32(max(count-1, 0))
	for index := range result {
		track := stylemodel.GridTrackSize{Kind: stylemodel.GridTrackAuto}
		if index < len(explicit) {
			track = explicit[index]
		} else if len(implicit) != 0 {
			track = implicit[(index-len(explicit))%len(implicit)]
		}
		tracks[index] = track
		switch track.Kind {
		case stylemodel.GridTrackLength:
			if track.Value.Percentage == 0 || basisDefinite {
				result[index] = max(track.Value.Resolve(basis), float32(0))
			} else {
				result[index] = contentContribution(maxContent, index)
			}
		case stylemodel.GridTrackMinContent:
			result[index] = contentContribution(minContent, index)
		case stylemodel.GridTrackMaxContent:
			result[index] = contentContribution(maxContent, index)
		case stylemodel.GridTrackFraction:
			result[index] = contentContribution(minContent, index)
			flexTotal += track.Flex
		case stylemodel.GridTrackAuto:
			result[index] = contentContribution(maxContent, index)
			autoCount++
		}
		used += result[index]
	}
	if flexTotal > 0 && basisDefinite {
		free := max(basis-used, float32(0))
		for index := range result {
			if tracks[index].Kind == stylemodel.GridTrackFraction {
				addition := free * tracks[index].Flex / flexTotal
				result[index] += addition
			}
		}
		used += free
	}
	if autoCount > 0 && basisDefinite && basis > used {
		share := (basis - used) / float32(autoCount)
		for index := range result {
			if tracks[index].Kind == stylemodel.GridTrackAuto {
				result[index] += share
			}
		}
	}
	return result
}

func contentContribution(values []float32, index int) float32 {
	if index >= 0 && index < len(values) {
		return values[index]
	}
	return 0
}

func trackOffset(tracks []float32, end int, gap float32) float32 {
	offset := gap * float32(end)
	for index := 0; index < end && index < len(tracks); index++ {
		offset += tracks[index]
	}
	return offset
}

func (e *engine) renderGridItem(node *dom.Node, style blockStyle, x, y, width, height float32) {
	style.margin = stylemodel.Edges{}
	style.marginAuto = stylemodel.AutoEdges{}
	style.boxSizing = stylemodel.BoxSizingBorderBox
	style.width, style.height = pixelSize(width), pixelSize(height)
	startBoxes, startDecorations := len(e.tree.Boxes), len(e.tree.Decorations)
	savedY, savedClip := e.y, e.clip
	e.y, e.clip = 0, nil
	if node.Type == dom.NodeText {
		e.addText(node.ID, "text", normalizeWhitespace(node.Text), style, 0, width)
	} else if isTextInput(node) {
		e.addInput(node, style, 0, width, height, true)
	} else {
		e.addBlock(node, style, 0, width, height, true, nil)
	}
	e.y, e.clip = savedY, savedClip
	translateFlexGeometry(e.tree, startBoxes, startDecorations, x, y, savedClip)
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
