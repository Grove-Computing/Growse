package layout

import (
	"strings"

	"github.com/Grove-Computing/Growse/internal/dom"
	stylemodel "github.com/Grove-Computing/Growse/internal/style"
)

type gridLayoutItem struct {
	node                               *dom.Node
	style                              blockStyle
	rowStart, rowEnd, colStart, colEnd int
}

type gridSpanContribution struct {
	start, end int
	required   float32
}

type subgridContext struct {
	columns, rows         []float32
	columnGap, rowGap     float32
	columnLines, rowLines map[string][]int
}

// addGridChildren establishes the initial one-column grid formatting context.
// Track construction and placement extend this entry point without falling back
// to block/inline formatting for direct grid items.
func (e *engine) addGridChildren(container *dom.Node, containerStyle blockStyle, x, width, containingHeight float32, heightDefinite bool) {
	items := make([]gridLayoutItem, 0, len(container.Children))
	positioned := make([]gridLayoutItem, 0)
	for _, child := range container.Children {
		if child.Type == dom.NodeText {
			if text := normalizeWhitespace(child.Text); text != "" {
				anonymous := &dom.Node{ID: child.ID, Type: dom.NodeText, Text: text}
				items = append(items, gridLayoutItem{node: anonymous, style: e.styleFor(container)})
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
		item := gridLayoutItem{node: child, style: childStyle}
		if childStyle.layoutPosition == stylemodel.PositionAbsolute || childStyle.layoutPosition == stylemodel.PositionFixed {
			positioned = append(positioned, item)
			continue
		}
		items = append(items, item)
	}
	if len(items) == 0 && len(positioned) == 0 {
		return
	}
	columnGap := containerStyle.columnGap.Resolve(width)
	rowGap := containerStyle.rowGap.Resolve(containingHeight)
	inherited, hasInherited := e.subgrids[container.ID]
	columnLines, rowLines := containerStyle.gridColumnLines, containerStyle.gridRowLines
	if containerStyle.gridColumnsSubgrid && hasInherited && len(inherited.columns) != 0 {
		columnGap = inherited.columnGap
		columnLines = mergeGridLineMaps(inherited.columnLines, columnLines)
	}
	if containerStyle.gridRowsSubgrid && hasInherited && len(inherited.rows) != 0 {
		rowGap = inherited.rowGap
		rowLines = mergeGridLineMaps(inherited.rowLines, rowLines)
	}
	columnTemplate := expandAutoRepeatTracks(containerStyle.gridTemplateColumns, width, columnGap, len(items))
	rowTemplate := expandAutoRepeatTracks(containerStyle.gridTemplateRows, containingHeight, rowGap, len(items))
	columnCount := len(columnTemplate)
	if containerStyle.gridColumnsSubgrid && hasInherited && len(inherited.columns) != 0 {
		columnCount = len(inherited.columns)
	}
	for _, area := range containerStyle.gridTemplateAreas {
		columnCount = max(columnCount, area.ColumnEnd)
	}
	if columnCount == 0 {
		columnCount = 1
	}
	rowCount := len(rowTemplate)
	if containerStyle.gridRowsSubgrid && hasInherited && len(inherited.rows) != 0 {
		rowCount = len(inherited.rows)
	}
	occupied := make(map[[2]int]bool)
	for index := range items {
		item := &items[index]
		if area, ok := containerStyle.gridTemplateAreas[item.style.gridAreaName]; ok {
			item.rowStart, item.rowEnd, item.colStart, item.colEnd = area.RowStart, area.RowEnd, area.ColumnStart, area.ColumnEnd
		} else {
			item.colStart, item.colEnd = resolveGridAxis(item.style.gridColumn, columnLines, columnCount)
			item.rowStart, item.rowEnd = resolveGridAxis(item.style.gridRow, rowLines, max(rowCount, 1))
		}
		if containerStyle.gridColumnsSubgrid && hasInherited {
			item.colStart, item.colEnd = clampGridArea(item.colStart, item.colEnd, columnCount)
		} else {
			columnCount = max(columnCount, item.colEnd)
		}
		if containerStyle.gridRowsSubgrid && hasInherited {
			item.rowStart, item.rowEnd = clampGridArea(item.rowStart, item.rowEnd, rowCount)
		} else {
			rowCount = max(rowCount, item.rowEnd)
		}
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
		if colSpan > columnCount {
			if containerStyle.gridColumnsSubgrid && hasInherited {
				colSpan = columnCount
			} else {
				columnCount = colSpan
			}
		}
		if rowSpan > max(rowCount, 1) && containerStyle.gridRowsSubgrid && hasInherited {
			rowSpan = max(rowCount, 1)
		}
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
	columnSpans := make([]gridSpanContribution, 0)
	for _, item := range items {
		if item.style.gridColumnsSubgrid && e.addSubgridColumnContributions(item, columnLines, width, containingHeight, heightDefinite, columnMaxContent, columnMinContent, &columnSpans) {
			continue
		}
		maxContent, _, minContent := e.flexIntrinsicSizes(item.node, item.style, flexAxis{horizontal: true}, width, width, containingHeight, heightDefinite)
		horizontalMargin := item.style.margin.Left + item.style.margin.Right
		span := max(item.colEnd-item.colStart, 1)
		if span == 1 {
			columnMaxContent[item.colStart] = max(columnMaxContent[item.colStart], maxContent+horizontalMargin)
			columnMinContent[item.colStart] = max(columnMinContent[item.colStart], minContent+horizontalMargin)
		} else {
			columnSpans = append(columnSpans, gridSpanContribution{start: item.colStart, end: item.colEnd, required: max(maxContent, minContent) + horizontalMargin})
		}
	}
	columns := append([]float32(nil), inherited.columns...)
	if !containerStyle.gridColumnsSubgrid || !hasInherited || len(columns) == 0 {
		columns = resolveGridTracks(columnTemplate, containerStyle.gridAutoColumns, columnCount, width, true, columnGap, columnMinContent, columnMaxContent)
		for _, contribution := range columnSpans {
			growGridSpan(columns, contribution, columnGap, columnTemplate, containerStyle.gridAutoColumns)
		}
	}
	rowMaxContent := make([]float32, rowCount)
	rowSpans := make([]gridSpanContribution, 0)
	for _, item := range items {
		itemWidth := trackSpanSize(columns, item.colStart, item.colEnd, columnGap)
		if item.style.gridRowsSubgrid && e.addSubgridRowContributions(item, rowLines, itemWidth, containingHeight, heightDefinite, rowMaxContent, &rowSpans) {
			continue
		}
		_, intrinsicHeight, _ := e.flexIntrinsicSizes(item.node, item.style, flexAxis{horizontal: true}, itemWidth, itemWidth, containingHeight, heightDefinite)
		rowSpan := max(item.rowEnd-item.rowStart, 1)
		required := intrinsicHeight + item.style.margin.Top + item.style.margin.Bottom
		if rowSpan == 1 {
			rowMaxContent[item.rowStart] = max(rowMaxContent[item.rowStart], required)
		} else {
			rowSpans = append(rowSpans, gridSpanContribution{start: item.rowStart, end: item.rowEnd, required: required})
		}
	}
	rows := append([]float32(nil), inherited.rows...)
	if !containerStyle.gridRowsSubgrid || !hasInherited || len(rows) == 0 {
		rows = resolveGridTracks(rowTemplate, containerStyle.gridAutoRows, rowCount, containingHeight, heightDefinite, rowGap, rowMaxContent, rowMaxContent)
		for _, contribution := range rowSpans {
			growGridSpan(rows, contribution, rowGap, rowTemplate, containerStyle.gridAutoRows)
		}
	}
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
		if item.style.gridColumnsSubgrid || item.style.gridRowsSubgrid {
			e.subgrids[item.node.ID] = subgridContextForItem(item, columns, rows, columnGap, rowGap, columnLines, rowLines)
		}
		e.renderGridItem(item.node, item.style, itemX, itemY, itemWidth, itemHeight)
		delete(e.subgrids, item.node.ID)
	}
	for _, item := range positioned {
		if item.style.layoutPosition == stylemodel.PositionFixed {
			e.renderPositionedChild(item.node, item.style)
			continue
		}
		item.colStart, item.colEnd = resolveGridAxis(item.style.gridColumn, columnLines, len(columns))
		item.rowStart, item.rowEnd = resolveGridAxis(item.style.gridRow, rowLines, len(rows))
		if item.colStart < 0 {
			item.colStart = 0
		}
		if item.colEnd <= item.colStart {
			item.colEnd = item.colStart + 1
		}
		if item.rowStart < 0 {
			item.rowStart = 0
		}
		if item.rowEnd <= item.rowStart {
			item.rowEnd = item.rowStart + 1
		}
		item.colStart, item.colEnd = min(item.colStart, len(columns)), min(item.colEnd, len(columns))
		item.rowStart, item.rowEnd = min(item.rowStart, len(rows)), min(item.rowEnd, len(rows))
		static := &Rect{
			X: x + columnOffset + trackOffset(columns, item.colStart, columnGap),
			Y: startY + rowOffset + trackOffset(rows, item.rowStart, rowGap),
		}
		e.renderPositionedChildAt(item.node, item.style, static)
	}
	e.y = startY + rowOffset + trackSpanSize(rows, 0, len(rows), rowGap)
}

func (e *engine) addSubgridColumnContributions(item gridLayoutItem, parentLines map[string][]int, width, height float32, heightDefinite bool, maximum, minimum []float32, spans *[]gridSpanContribution) bool {
	count := item.colEnd - item.colStart
	if count <= 0 || item.node == nil || item.node.Type != dom.NodeElement {
		return false
	}
	names := mergeGridLineMaps(sliceGridLineMap(parentLines, item.colStart, item.colEnd), item.style.gridColumnLines)
	for _, child := range subgridItems(e, item.node, true, names, count) {
		maxContent, _, minContent := e.flexIntrinsicSizes(child.node, child.style, flexAxis{horizontal: true}, width, width, height, heightDefinite)
		start, end := item.colStart+child.colStart, item.colStart+child.colEnd
		requiredMax := maxContent + child.style.margin.Left + child.style.margin.Right
		requiredMin := minContent + child.style.margin.Left + child.style.margin.Right
		if end-start == 1 && start >= 0 && start < len(maximum) {
			maximum[start] = max(maximum[start], requiredMax)
			minimum[start] = max(minimum[start], requiredMin)
		} else {
			*spans = append(*spans, gridSpanContribution{start: start, end: end, required: max(requiredMax, requiredMin)})
		}
	}
	return true
}

func (e *engine) addSubgridRowContributions(item gridLayoutItem, parentLines map[string][]int, width, height float32, heightDefinite bool, maximum []float32, spans *[]gridSpanContribution) bool {
	count := item.rowEnd - item.rowStart
	if count <= 0 || item.node == nil || item.node.Type != dom.NodeElement {
		return false
	}
	names := mergeGridLineMaps(sliceGridLineMap(parentLines, item.rowStart, item.rowEnd), item.style.gridRowLines)
	for _, child := range subgridItems(e, item.node, false, names, count) {
		_, intrinsicHeight, _ := e.flexIntrinsicSizes(child.node, child.style, flexAxis{horizontal: true}, width, width, height, heightDefinite)
		start, end := item.rowStart+child.rowStart, item.rowStart+child.rowEnd
		required := intrinsicHeight + child.style.margin.Top + child.style.margin.Bottom
		if end-start == 1 && start >= 0 && start < len(maximum) {
			maximum[start] = max(maximum[start], required)
		} else {
			*spans = append(*spans, gridSpanContribution{start: start, end: end, required: required})
		}
	}
	return true
}

func subgridItems(e *engine, container *dom.Node, columns bool, named map[string][]int, count int) []gridLayoutItem {
	items := make([]gridLayoutItem, 0, len(container.Children))
	cursor := 0
	for _, child := range container.Children {
		if child.Type != dom.NodeElement && (child.Type != dom.NodeText || strings.TrimSpace(child.Text) == "") {
			continue
		}
		childStyle := e.styleFor(child)
		if childStyle.display == stylemodel.DisplayNone || childStyle.layoutPosition == stylemodel.PositionAbsolute || childStyle.layoutPosition == stylemodel.PositionFixed {
			continue
		}
		placement := childStyle.gridRow
		if columns {
			placement = childStyle.gridColumn
		}
		start, end := resolveGridAxis(placement, named, count)
		span := placementSpan(placement, start, end)
		if start < 0 {
			start = cursor
			end = start + span
		}
		start = min(max(start, 0), count)
		if start >= count {
			continue
		}
		end = min(max(end, start+1), count)
		cursor = end
		item := gridLayoutItem{node: child, style: childStyle}
		if columns {
			item.colStart, item.colEnd = start, end
		} else {
			item.rowStart, item.rowEnd = start, end
		}
		items = append(items, item)
	}
	return items
}

func subgridContextForItem(item gridLayoutItem, columns, rows []float32, columnGap, rowGap float32, columnLines, rowLines map[string][]int) subgridContext {
	context := subgridContext{}
	if item.style.gridColumnsSubgrid {
		start := min(max(item.colStart, 0), len(columns))
		end := min(max(item.colEnd, start), len(columns))
		context.columns = append([]float32(nil), columns[start:end]...)
		context.columnGap = columnGap
		context.columnLines = sliceGridLineMap(columnLines, start, end)
	}
	if item.style.gridRowsSubgrid {
		start := min(max(item.rowStart, 0), len(rows))
		end := min(max(item.rowEnd, start), len(rows))
		context.rows = append([]float32(nil), rows[start:end]...)
		context.rowGap = rowGap
		context.rowLines = sliceGridLineMap(rowLines, start, end)
	}
	return context
}

func clampGridArea(start, end, count int) (int, int) {
	if start < 0 {
		return start, end
	}
	start = min(start, max(count-1, 0))
	end = min(max(end, start+1), count)
	return start, end
}

func sliceGridLineMap(source map[string][]int, start, end int) map[string][]int {
	if len(source) == 0 || end < start {
		return nil
	}
	result := make(map[string][]int)
	for name, lines := range source {
		for _, line := range lines {
			if line >= start && line <= end {
				result[name] = append(result[name], line-start)
			}
		}
	}
	return result
}

func mergeGridLineMaps(base, additional map[string][]int) map[string][]int {
	if len(base) == 0 && len(additional) == 0 {
		return nil
	}
	result := make(map[string][]int, len(base)+len(additional))
	for name, lines := range base {
		result[name] = append([]int(nil), lines...)
	}
	for name, lines := range additional {
		for _, line := range lines {
			present := false
			for _, existing := range result[name] {
				if existing == line {
					present = true
					break
				}
			}
			if !present {
				result[name] = append(result[name], line)
			}
		}
	}
	return result
}

// growGridSpan applies a spanning item's intrinsic contribution after the
// non-spanning base sizes are known. Fixed tracks retain their authored size;
// the remaining contribution is shared by the intrinsic/flexible tracks in
// the span. This avoids losing pixels through the old per-track division.
func growGridSpan(tracks []float32, contribution gridSpanContribution, gap float32, explicit, implicit []stylemodel.GridTrackSize) {
	start, end := max(contribution.start, 0), min(contribution.end, len(tracks))
	if end <= start {
		return
	}
	shortfall := contribution.required - trackSpanSize(tracks, start, end, gap)
	if shortfall <= 0 {
		return
	}
	growable := make([]int, 0, end-start)
	for index := start; index < end; index++ {
		track := gridTrackAt(explicit, implicit, index)
		if track.Kind != stylemodel.GridTrackLength {
			growable = append(growable, index)
		}
	}
	if len(growable) == 0 {
		return
	}
	share := shortfall / float32(len(growable))
	for _, index := range growable {
		tracks[index] += share
	}
}

func gridTrackAt(explicit, implicit []stylemodel.GridTrackSize, index int) stylemodel.GridTrackSize {
	if index >= 0 && index < len(explicit) {
		return explicit[index]
	}
	if len(implicit) != 0 && index >= len(explicit) {
		return implicit[(index-len(explicit))%len(implicit)]
	}
	return stylemodel.GridTrackSize{Kind: stylemodel.GridTrackAuto}
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
	if end <= start {
		return 0
	}
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
	} else if isEditableTextControl(node) {
		e.addInput(node, style, 0, width, height, true)
	} else if isSelectControl(node) {
		e.addSelect(node, style, 0, width, height, true)
	} else if isCheckableControl(node) {
		e.addCheckable(node, style, 0, width, height, true)
	} else if isSubmitButtonControl(node) {
		e.addSubmitButton(node, style, 0, width, height, true)
	} else if isImageElement(node, e.images) {
		e.addImage(node, style, 0, width, height, true)
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
