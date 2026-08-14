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
	columnGap := containerStyle.columnGap.Resolve(width)
	rowGap := containerStyle.rowGap.Resolve(containingHeight)
	columnTemplate := expandAutoRepeatTracks(containerStyle.gridTemplateColumns, width, columnGap, len(items))
	rowTemplate := expandAutoRepeatTracks(containerStyle.gridTemplateRows, containingHeight, rowGap, len(items))
	columnCount := len(columnTemplate)
	for _, area := range containerStyle.gridTemplateAreas {
		columnCount = max(columnCount, area.ColumnEnd)
	}
	if columnCount == 0 {
		columnCount = 1
	}
	rowCount := len(rowTemplate)
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
		if containerStyle.gridAutoFlow.Dense {
			cursor = 0
		}
		colSpan := placementSpan(item.style.gridColumn, item.colStart, item.colEnd)
		rowSpan := placementSpan(item.style.gridRow, item.rowStart, item.rowEnd)
		for {
			row, column := autoPlacementCell(cursor, containerStyle.gridAutoFlow.Column, columnCount, max(rowCount, 1))
			cursor++
			if item.colStart >= 0 {
				column = item.colStart
			}
			if item.rowStart >= 0 {
				row = item.rowStart
			}
			if containerStyle.gridAutoFlow.Column && column+colSpan > columnCount {
				columnCount = column + colSpan
			}
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
	columns := resolveGridTracks(columnTemplate, containerStyle.gridAutoColumns, columnCount, width, true, columnGap, columnMinContent, columnMaxContent)
	rowMaxContent := make([]float32, rowCount)
	for _, item := range items {
		itemWidth := trackSpanSize(columns, item.colStart, item.colEnd, columnGap)
		_, intrinsicHeight, _ := e.flexIntrinsicSizes(item.node, item.style, flexAxis{horizontal: true}, itemWidth, itemWidth, containingHeight, heightDefinite)
		rowSpan := max(item.rowEnd-item.rowStart, 1)
		for row := item.rowStart; row < item.rowEnd; row++ {
			rowMaxContent[row] = max(rowMaxContent[row], (intrinsicHeight+item.style.margin.Top+item.style.margin.Bottom)/float32(rowSpan))
		}
	}
	rows := resolveGridTracks(rowTemplate, containerStyle.gridAutoRows, rowCount, containingHeight, heightDefinite, rowGap, rowMaxContent, rowMaxContent)
	startY := e.y
	columnOffset, distributedColumnGap := justifySpacing(containerStyle.justifyContent, max(width-trackSpanSize(columns, 0, len(columns), columnGap), float32(0)), len(columns), false)
	rowOffset, distributedRowGap := float32(0), float32(0)
	if heightDefinite {
		rowOffset, distributedRowGap = alignContentSpacing(containerStyle.alignContent, max(containingHeight-trackSpanSize(rows, 0, len(rows), rowGap), float32(0)), len(rows))
	}
	columnGap += distributedColumnGap
	rowGap += distributedRowGap
	for _, item := range items {
		itemX, itemY := x+columnOffset+trackOffset(columns, item.colStart, columnGap), startY+rowOffset+trackOffset(rows, item.rowStart, rowGap)
		itemWidth, itemHeight := trackSpanSize(columns, item.colStart, item.colEnd, columnGap), trackSpanSize(rows, item.rowStart, item.rowEnd, rowGap)
		itemX, itemY, itemWidth, itemHeight = e.alignGridItem(item.node, item.style, containerStyle, itemX, itemY, itemWidth, itemHeight)
		e.renderGridItem(item.node, item.style, itemX, itemY, itemWidth, itemHeight)
	}
	e.y = startY + rowOffset + trackSpanSize(rows, 0, len(rows), rowGap)
}

func expandAutoRepeatTracks(tracks []stylemodel.GridTrackSize, basis, gap float32, itemCount int) []stylemodel.GridTrackSize {
	var result []stylemodel.GridTrackSize
	for _, track := range tracks {
		if track.Kind != stylemodel.GridTrackAutoRepeat || len(track.RepeatPattern) == 0 {
			result = append(result, track)
			continue
		}
		patternSize := gap * float32(max(len(track.RepeatPattern)-1, 0))
		for _, patternTrack := range track.RepeatPattern {
			patternSize += autoRepeatMinimum(patternTrack, basis)
		}
		count := 1
		if patternSize > 0 {
			count = max(int((basis+gap)/(patternSize+gap)), 1)
		}
		if track.AutoRepeat == stylemodel.GridAutoRepeatFit {
			count = min(count, max((itemCount+len(track.RepeatPattern)-1)/len(track.RepeatPattern), 1))
		}
		count = min(count, 1000/max(len(track.RepeatPattern), 1))
		for index := 0; index < count; index++ {
			result = append(result, track.RepeatPattern...)
		}
	}
	return result
}

func autoRepeatMinimum(track stylemodel.GridTrackSize, basis float32) float32 {
	if track.MinSet && track.MinKind == stylemodel.GridTrackLength {
		return max(track.MinValue.Resolve(basis), float32(1))
	}
	if track.Kind == stylemodel.GridTrackLength {
		return max(track.Value.Resolve(basis), float32(1))
	}
	if track.FitLimit != nil {
		return max(track.FitLimit.Resolve(basis), float32(1))
	}
	return 1
}

func (e *engine) alignGridItem(node *dom.Node, item, container blockStyle, x, y, cellWidth, cellHeight float32) (float32, float32, float32, float32) {
	intrinsicWidth, intrinsicHeight, _ := e.flexIntrinsicSizes(node, item, flexAxis{horizontal: true}, cellWidth, cellWidth, cellHeight, true)
	justify := item.justifySelf
	if justify == stylemodel.AlignAuto {
		justify = container.justifyItems
	}
	align := item.alignSelf
	if align == stylemodel.AlignAuto {
		align = container.alignItems
	}
	width, widthDefinite := gridItemOuterSize(item.width, item, cellWidth, true)
	height, heightDefinite := gridItemOuterSize(item.height, item, cellHeight, false)
	availableWidth := max(cellWidth-item.margin.Left-item.margin.Right, float32(0))
	availableHeight := max(cellHeight-item.margin.Top-item.margin.Bottom, float32(0))
	if !widthDefinite {
		if justify == stylemodel.AlignStretch && !item.marginAuto.Left && !item.marginAuto.Right {
			width = availableWidth
		} else {
			width = min(intrinsicWidth, availableWidth)
		}
	}
	if !heightDefinite {
		if align == stylemodel.AlignStretch && !item.marginAuto.Top && !item.marginAuto.Bottom {
			height = availableHeight
		} else {
			height = min(intrinsicHeight, availableHeight)
		}
	}
	x += item.margin.Left + gridAlignmentOffset(max(availableWidth-width, float32(0)), justify, item.marginAuto.Left, item.marginAuto.Right)
	y += item.margin.Top + gridAlignmentOffset(max(availableHeight-height, float32(0)), align, item.marginAuto.Top, item.marginAuto.Bottom)
	return x, y, max(width, float32(0)), max(height, float32(0))
}

func gridItemOuterSize(value stylemodel.SizeValue, item blockStyle, basis float32, horizontal bool) (float32, bool) {
	size, definite := resolveSize(value, basis, true)
	if !definite || item.boxSizing == stylemodel.BoxSizingBorderBox {
		return size, definite
	}
	if horizontal {
		size += item.padding.Left + item.padding.Right + item.border.Left.Width + item.border.Right.Width
	} else {
		size += item.padding.Top + item.padding.Bottom + item.border.Top.Width + item.border.Bottom.Width
	}
	return size, true
}

func gridAlignmentOffset(free float32, alignment stylemodel.Align, autoStart, autoEnd bool) float32 {
	if autoStart && autoEnd {
		return free / 2
	}
	if autoStart {
		return free
	}
	if autoEnd {
		return 0
	}
	switch alignment {
	case stylemodel.AlignFlexEnd:
		return free
	case stylemodel.AlignCenter:
		return free / 2
	default:
		return 0
	}
}

func placementSpan(placement stylemodel.GridPlacement, start, end int) int {
	if end > start && start >= 0 {
		return end - start
	}
	if placement.Start.Span > 0 {
		return placement.Start.Span
	}
	if placement.End.Span > 0 {
		return placement.End.Span
	}
	return 1
}

func autoPlacementCell(cursor int, columnFlow bool, columnCount, rowCount int) (int, int) {
	if columnFlow {
		return cursor % rowCount, cursor / rowCount
	}
	return cursor / columnCount, cursor % columnCount
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
