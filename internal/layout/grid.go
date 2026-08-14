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
		node                               *dom.Node
		style                              blockStyle
		rowStart, rowEnd, colStart, colEnd int
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
	for _, area := range containerStyle.gridTemplateAreas {
		columnCount = max(columnCount, area.ColumnEnd)
	}
	if columnCount == 0 {
		columnCount = 1
	}
	rowCount := len(containerStyle.gridTemplateRows)
	occupied := make(map[[2]int]bool)
	for index := range items {
		item := &items[index]
		if area, ok := containerStyle.gridTemplateAreas[item.style.gridAreaName]; ok {
			item.rowStart, item.rowEnd, item.colStart, item.colEnd = area.RowStart, area.RowEnd, area.ColumnStart, area.ColumnEnd
		} else {
			item.colStart, item.colEnd = resolveGridAxis(item.style.gridColumn, containerStyle.gridColumnLines, columnCount)
			item.rowStart, item.rowEnd = resolveGridAxis(item.style.gridRow, containerStyle.gridRowLines, max(rowCount, 1))
		}
		columnCount = max(columnCount, item.colEnd)
		rowCount = max(rowCount, item.rowEnd)
		if item.colStart >= 0 && item.rowStart >= 0 {
			occupyGridCells(occupied, item.rowStart, item.rowEnd, item.colStart, item.colEnd)
		}
	}
	cursor := 0
	for index := range items {
		item := &items[index]
		if item.colStart >= 0 && item.rowStart >= 0 {
			continue
		}
		colSpan, rowSpan := max(item.colEnd-item.colStart, 1), max(item.rowEnd-item.rowStart, 1)
		for {
			row, column := cursor/columnCount, cursor%columnCount
			cursor++
			if column+colSpan <= columnCount && gridCellsFree(occupied, row, row+rowSpan, column, column+colSpan) {
				item.rowStart, item.rowEnd, item.colStart, item.colEnd = row, row+rowSpan, column, column+colSpan
				occupyGridCells(occupied, item.rowStart, item.rowEnd, item.colStart, item.colEnd)
				rowCount = max(rowCount, item.rowEnd)
				break
			}
		}
	}
	columnMaxContent, columnMinContent := make([]float32, columnCount), make([]float32, columnCount)
	for _, item := range items {
		maxContent, _, minContent := e.flexIntrinsicSizes(item.node, item.style, flexAxis{horizontal: true}, width, width, containingHeight, heightDefinite)
		horizontalMargin := item.style.margin.Left + item.style.margin.Right
		span := max(item.colEnd-item.colStart, 1)
		for column := item.colStart; column < item.colEnd; column++ {
			columnMaxContent[column] = max(columnMaxContent[column], (maxContent+horizontalMargin)/float32(span))
			columnMinContent[column] = max(columnMinContent[column], (minContent+horizontalMargin)/float32(span))
		}
	}
	columnGap := containerStyle.columnGap.Resolve(width)
	rowGap := containerStyle.rowGap.Resolve(containingHeight)
	columns := resolveGridTracks(containerStyle.gridTemplateColumns, containerStyle.gridAutoColumns, columnCount, width, true, columnGap, columnMinContent, columnMaxContent)
	rowMaxContent := make([]float32, rowCount)
	for _, item := range items {
		itemWidth := trackSpanSize(columns, item.colStart, item.colEnd, columnGap)
		_, intrinsicHeight, _ := e.flexIntrinsicSizes(item.node, item.style, flexAxis{horizontal: true}, itemWidth, itemWidth, containingHeight, heightDefinite)
		rowSpan := max(item.rowEnd-item.rowStart, 1)
		for row := item.rowStart; row < item.rowEnd; row++ {
			rowMaxContent[row] = max(rowMaxContent[row], (intrinsicHeight+item.style.margin.Top+item.style.margin.Bottom)/float32(rowSpan))
		}
	}
	rows := resolveGridTracks(containerStyle.gridTemplateRows, containerStyle.gridAutoRows, rowCount, containingHeight, heightDefinite, rowGap, rowMaxContent, rowMaxContent)
	startY := e.y
	for _, item := range items {
		itemX, itemY := x+trackOffset(columns, item.colStart, columnGap), startY+trackOffset(rows, item.rowStart, rowGap)
		itemWidth, itemHeight := trackSpanSize(columns, item.colStart, item.colEnd, columnGap), trackSpanSize(rows, item.rowStart, item.rowEnd, rowGap)
		e.renderGridItem(item.node, item.style, itemX, itemY, itemWidth, itemHeight)
	}
	e.y = startY + trackOffset(rows, len(rows), rowGap)
}

func resolveGridAxis(placement stylemodel.GridPlacement, named map[string][]int, explicitTracks int) (int, int) {
	start, end := resolveGridLine(placement.Start, named, explicitTracks), resolveGridLine(placement.End, named, explicitTracks)
	if start >= 0 && placement.End.Span > 0 {
		end = start + placement.End.Span
	}
	if end >= 0 && placement.Start.Span > 0 {
		start = max(end-placement.Start.Span, 0)
	}
	if start >= 0 && end < 0 {
		end = start + 1
	}
	return start, end
}

func resolveGridLine(line stylemodel.GridLine, named map[string][]int, explicitTracks int) int {
	if line.Name != "" {
		if matches := named[line.Name]; len(matches) != 0 {
			return matches[0]
		}
	}
	if line.Index > 0 {
		return line.Index - 1
	}
	if line.Index < 0 {
		return max(explicitTracks+line.Index+1, 0)
	}
	return -1
}

func occupyGridCells(occupied map[[2]int]bool, rowStart, rowEnd, colStart, colEnd int) {
	for row := rowStart; row < rowEnd; row++ {
		for column := colStart; column < colEnd; column++ {
			occupied[[2]int{row, column}] = true
		}
	}
}

func gridCellsFree(occupied map[[2]int]bool, rowStart, rowEnd, colStart, colEnd int) bool {
	for row := rowStart; row < rowEnd; row++ {
		for column := colStart; column < colEnd; column++ {
			if occupied[[2]int{row, column}] {
				return false
			}
		}
	}
	return true
}

func trackSpanSize(tracks []float32, start, end int, gap float32) float32 {
	return trackOffset(tracks, end, gap) - trackOffset(tracks, start, gap) - gap
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
			if track.FitLimit != nil {
				result[index] = min(result[index], max(track.FitLimit.Resolve(basis), contentContribution(minContent, index)))
			}
		case stylemodel.GridTrackFraction:
			result[index] = contentContribution(minContent, index)
			flexTotal += track.Flex
		case stylemodel.GridTrackAuto:
			result[index] = contentContribution(maxContent, index)
			autoCount++
		}
		if track.MinSet {
			minimum := gridTrackMinimum(track, index, basis, basisDefinite, minContent, maxContent)
			result[index] = max(result[index], minimum)
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

func gridTrackMinimum(track stylemodel.GridTrackSize, index int, basis float32, basisDefinite bool, minContent, maxContent []float32) float32 {
	switch track.MinKind {
	case stylemodel.GridTrackLength:
		if track.MinValue.Percentage == 0 || basisDefinite {
			return max(track.MinValue.Resolve(basis), float32(0))
		}
		return contentContribution(maxContent, index)
	case stylemodel.GridTrackMinContent:
		return contentContribution(minContent, index)
	case stylemodel.GridTrackMaxContent, stylemodel.GridTrackAuto:
		return contentContribution(maxContent, index)
	default:
		return 0
	}
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
