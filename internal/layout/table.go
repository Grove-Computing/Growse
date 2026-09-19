package layout

import (
	"strconv"
	"strings"

	"github.com/Grove-Computing/Growse/internal/dom"
	stylemodel "github.com/Grove-Computing/Growse/internal/style"
)

const maxTableSpan = 1000

type tableCell struct {
	node             *dom.Node
	row, column      int
	rowSpan, colSpan int
	minWidth         float32
}

type tableColumn struct {
	node  *dom.Node
	group *dom.Node
	style blockStyle
}

// flowChildren removes display:contents boxes while retaining their children at
// the exact position where the box would have participated in normal flow.
func (e *engine) flowChildren(node *dom.Node) []*dom.Node {
	var result []*dom.Node
	var appendChild func(*dom.Node)
	appendChild = func(child *dom.Node) {
		if child != nil && child.Type == dom.NodeElement && e.styleFor(child).display == stylemodel.DisplayContents {
			for _, grandchild := range child.Children {
				appendChild(grandchild)
			}
			return
		}
		result = append(result, child)
	}
	for _, child := range node.Children {
		appendChild(child)
	}
	return result
}

func (e *engine) addTable(node *dom.Node, tableStyle blockStyle, x, availableWidth, containingHeight float32, heightDefinite bool) {
	if !e.withinBudget(node.ID) {
		return
	}
	e.y += tableStyle.margin.Top
	x += tableStyle.margin.Left
	availableWidth -= tableStyle.margin.Left + tableStyle.margin.Right
	if availableWidth < 1 {
		availableWidth = 1
	}

	rows := e.tableRows(node)
	cells, columnCount := e.placeTableCells(rows)
	columns := e.tableColumns(node)
	columnCount = max(columnCount, len(columns))
	if columnCount == 0 {
		columnCount = 1
	}

	horizontalExtras := tableStyle.padding.Left + tableStyle.padding.Right + tableStyle.border.Left.Width + tableStyle.border.Right.Width
	tableWidth := availableWidth
	if resolved, ok := resolveSize(tableStyle.width, availableWidth, true); ok {
		tableWidth = resolved
		if tableStyle.boxSizing == stylemodel.BoxSizingContentBox {
			tableWidth += horizontalExtras
		}
	} else if intrinsic, ok := e.intrinsicKeywordSize(node, tableStyle.width, tableStyle, availableWidth, true); ok {
		tableWidth = intrinsic
	}
	tableWidth = constrainSize(tableWidth, tableStyle.minWidth, tableStyle.maxWidth, availableWidth, true)
	if intrinsic, ok := e.intrinsicKeywordSize(node, tableStyle.minWidth, tableStyle, availableWidth, true); ok {
		tableWidth = max(tableWidth, intrinsic)
	}
	if intrinsic, ok := e.intrinsicKeywordSize(node, tableStyle.maxWidth, tableStyle, availableWidth, true); ok {
		tableWidth = min(tableWidth, intrinsic)
	}
	if tableWidth > availableWidth && tableStyle.width.Kind == stylemodel.SizeAuto {
		tableWidth = availableWidth
	}
	contentWidth := max(tableWidth-horizontalExtras, float32(1))
	spacingX, spacingY := tableStyle.borderSpacingX, tableStyle.borderSpacingY
	if tableStyle.borderCollapse == stylemodel.BorderCollapseCollapse {
		spacingX, spacingY = 0, 0
	}
	columnSpace := spacingX * float32(columnCount+1)
	trackWidth := max(contentWidth-columnSpace, float32(columnCount))
	columnWidths := e.resolveTableColumnWidths(cells, columns, columnCount, trackWidth, containingHeight, heightDefinite, tableStyle.tableLayout)

	rowHeights := make([]float32, len(rows))
	for index := range cells {
		cell := &cells[index]
		if !e.withinBudget(cell.node.ID) {
			break
		}
		cellWidth := max(trackOffset(columnWidths, cell.column+cell.colSpan, spacingX)-trackOffset(columnWidths, cell.column, spacingX)-spacingX, float32(1))
		cellStyle := e.styleFor(cell.node)
		_, intrinsicHeight, _ := e.flexIntrinsicSizes(cell.node, cellStyle, flexAxis{horizontal: true}, cellWidth, cellWidth, containingHeight, heightDefinite)
		share := max(intrinsicHeight/float32(cell.rowSpan), cellStyle.lineHeight)
		for row := cell.row; row < min(cell.row+cell.rowSpan, len(rowHeights)); row++ {
			rowHeights[row] = max(rowHeights[row], share)
		}
	}
	for index := range rowHeights {
		if resolved, ok := resolveSize(e.styleFor(rows[index]).height, containingHeight, heightDefinite); ok {
			rowHeights[index] = max(rowHeights[index], resolved)
		}
		rowHeights[index] = max(rowHeights[index], float32(1))
	}
	rowBaselines := make([]float32, len(rowHeights))
	for _, cell := range cells {
		cellStyle := e.styleFor(cell.node)
		if cell.rowSpan == 1 && cellStyle.verticalAlign.Kind == stylemodel.VerticalAlignBaseline {
			rowBaselines[cell.row] = max(rowBaselines[cell.row], tableCellBaseline(cellStyle))
		}
	}

	boxTop := e.y
	topCaptions, bottomCaptions := e.tableCaptions(node)
	topCaptionHeight := e.renderTableCaptions(topCaptions, x, boxTop, tableWidth, containingHeight, heightDefinite)
	gridTop := boxTop + topCaptionHeight
	contentX := x + tableStyle.border.Left.Width + tableStyle.padding.Left
	contentY := gridTop + tableStyle.border.Top.Width + tableStyle.padding.Top
	verticalExtras := tableStyle.padding.Top + tableStyle.padding.Bottom + tableStyle.border.Top.Width + tableStyle.border.Bottom.Width
	rowSpace := spacingY * float32(len(rowHeights)+1)
	tableContentHeight := trackOffset(rowHeights, len(rowHeights), 0) + rowSpace
	tableGridHeight := tableContentHeight + verticalExtras
	if resolved, ok := resolveSize(tableStyle.height, containingHeight, heightDefinite); ok {
		tableGridHeight = resolved
		if tableStyle.boxSizing == stylemodel.BoxSizingContentBox {
			tableGridHeight += verticalExtras
		}
	}
	tableGridHeight = constrainSize(tableGridHeight, tableStyle.minHeight, tableStyle.maxHeight, containingHeight, heightDefinite)
	e.fitTableRows(rowHeights, max(tableGridHeight-verticalExtras-rowSpace, float32(len(rowHeights))))
	tableContentHeight = trackOffset(rowHeights, len(rowHeights), 0) + rowSpace
	tableGridHeight = max(tableGridHeight, tableContentHeight+verticalExtras)
	decorationIndex := -1
	if tableStyle.background != 0 || hasVisibleBorder(tableStyle.border) || len(tableStyle.boxShadows) != 0 || tableStyle.outline.Style != stylemodel.BorderNone {
		decorationIndex = len(e.tree.Decorations)
		e.tree.Decorations = append(e.tree.Decorations, Decoration{
			Order: e.nextOrder(), StackingID: e.stackingID, NodeID: node.ID,
			Rect:       Rect{X: x, Y: gridTop, Width: tableWidth},
			Background: tableStyle.background, Border: tableStyle.border, Padding: tableStyle.padding,
			Opacity: e.opacity * tableStyle.opacity, BoxShadows: append([]stylemodel.Shadow(nil), tableStyle.boxShadows...),
			Outline: tableStyle.outline, OutlineOffset: tableStyle.outlineOffset,
			Cursor: tableStyle.cursor, Transform: stylemodel.IdentityMatrix(), Hidden: tableStyle.hidden,
		})
	}
	for index := range cells {
		cell := &cells[index]
		cellX := contentX + spacingX + trackOffset(columnWidths, cell.column, spacingX)
		cellY := contentY + spacingY + trackOffset(rowHeights, cell.row, spacingY)
		cellWidth := max(trackOffset(columnWidths, min(cell.column+cell.colSpan, columnCount), spacingX)-trackOffset(columnWidths, cell.column, spacingX)-spacingX, float32(1))
		cellHeight := max(trackOffset(rowHeights, min(cell.row+cell.rowSpan, len(rowHeights)), spacingY)-trackOffset(rowHeights, cell.row, spacingY)-spacingY, float32(1))
		cellStyle := e.styleFor(cell.node)
		if tableStyle.borderCollapse == stylemodel.BorderCollapseCollapse {
			if cell.column > 0 {
				cellStyle.border.Left.Width = 0
			}
			if cell.row > 0 {
				cellStyle.border.Top.Width = 0
			}
		}
		if cell.rowSpan == 1 && cellStyle.verticalAlign.Kind == stylemodel.VerticalAlignBaseline {
			cellStyle.padding.Top += max(rowBaselines[cell.row]-tableCellBaseline(cellStyle), float32(0))
		}
		cellStyle.display = stylemodel.DisplayBlock
		e.renderGridItem(cell.node, cellStyle, cellX, cellY, cellWidth, cellHeight)
		e.tree.Bounds[cell.node.ID] = Rect{X: cellX, Y: cellY, Width: cellWidth, Height: cellHeight}
	}
	for rowIndex, row := range rows {
		rowRect := Rect{X: contentX + spacingX, Y: contentY + spacingY + trackOffset(rowHeights, rowIndex, spacingY), Width: max(contentWidth-2*spacingX, float32(1)), Height: rowHeights[rowIndex]}
		e.tree.Bounds[row.ID] = rowRect
		if row.Parent != nil && row.Parent != node {
			e.tree.Bounds[row.Parent.ID] = unionTableRect(e.tree.Bounds[row.Parent.ID], rowRect)
		}
	}
	gridBodyHeight := max(tableGridHeight-verticalExtras-spacingY*2, float32(1))
	for columnIndex := 0; columnIndex < min(len(columns), len(columnWidths)); columnIndex++ {
		column := columns[columnIndex]
		columnRect := Rect{X: contentX + spacingX + trackOffset(columnWidths, columnIndex, spacingX), Y: contentY + spacingY, Width: columnWidths[columnIndex], Height: gridBodyHeight}
		e.tree.Bounds[column.node.ID] = unionTableRect(e.tree.Bounds[column.node.ID], columnRect)
		if column.group != nil {
			e.tree.Bounds[column.group.ID] = unionTableRect(e.tree.Bounds[column.group.ID], columnRect)
		}
	}
	bottomCaptionHeight := e.renderTableCaptions(bottomCaptions, x, gridTop+tableGridHeight, tableWidth, containingHeight, heightDefinite)
	tableHeight := topCaptionHeight + tableGridHeight + bottomCaptionHeight
	e.tree.Bounds[node.ID] = Rect{X: x, Y: boxTop, Width: tableWidth, Height: tableHeight}
	if decorationIndex >= 0 {
		e.tree.Decorations[decorationIndex].Height = tableGridHeight
		e.tree.Decorations[decorationIndex].Radius = resolveBorderRadii(tableStyle.radius, tableWidth, tableGridHeight)
	}
	e.y = boxTop + tableHeight + tableStyle.margin.Bottom
}

func tableCellBaseline(style blockStyle) float32 {
	_, ascent := usedLineMetrics(inlineRun{style: style})
	return style.border.Top.Width + style.padding.Top + ascent
}

func (e *engine) resolveTableColumnWidths(cells []tableCell, columns []tableColumn, count int, available, containingHeight float32, heightDefinite bool, algorithm stylemodel.TableLayout) []float32 {
	widths := make([]float32, count)
	specified := make([]bool, count)
	for index := 0; index < min(len(columns), count); index++ {
		if width, ok := resolveSize(columns[index].style.width, available, true); ok {
			widths[index], specified[index] = max(width, float32(1)), true
		}
	}
	for index := range cells {
		cell := &cells[index]
		cellStyle := e.styleFor(cell.node)
		width, explicit := resolveSize(cellStyle.width, available, true)
		if !explicit && algorithm == stylemodel.TableLayoutAuto {
			width, _, _ = e.flexIntrinsicSizes(cell.node, cellStyle, flexAxis{horizontal: true}, available, available, containingHeight, heightDefinite)
		}
		if algorithm == stylemodel.TableLayoutFixed && cell.row != 0 || width <= 0 {
			continue
		}
		cell.minWidth = max(width/float32(cell.colSpan), float32(1))
		for column := cell.column; column < min(cell.column+cell.colSpan, count); column++ {
			if algorithm == stylemodel.TableLayoutAuto || !specified[column] {
				widths[column] = max(widths[column], cell.minWidth)
				if explicit {
					specified[column] = true
				}
			}
		}
	}
	normalizeTableTracks(widths, specified, available)
	return widths
}

func normalizeTableTracks(tracks []float32, specified []bool, available float32) {
	total, flexible := float32(0), 0
	for index, width := range tracks {
		total += width
		if !specified[index] {
			flexible++
		}
	}
	if total < available {
		count := flexible
		if count == 0 {
			count = len(tracks)
		}
		if count == 0 {
			return
		}
		extra := (available - total) / float32(count)
		for index := range tracks {
			if flexible == 0 || !specified[index] {
				tracks[index] += extra
			}
		}
		return
	}
	if total > available && total > 0 {
		scale := available / total
		for index := range tracks {
			tracks[index] = max(tracks[index]*scale, float32(1))
		}
	}
}

func (e *engine) fitTableRows(rows []float32, available float32) {
	if len(rows) == 0 {
		return
	}
	total := trackOffset(rows, len(rows), 0)
	if total < available {
		extra := (available - total) / float32(len(rows))
		for index := range rows {
			rows[index] += extra
		}
	} else if total > available && total > 0 {
		scale := available / total
		for index := range rows {
			rows[index] = max(rows[index]*scale, float32(1))
		}
	}
}

func unionTableRect(current, next Rect) Rect {
	if current.Width <= 0 || current.Height <= 0 {
		return next
	}
	left, top := min(current.X, next.X), min(current.Y, next.Y)
	right, bottom := max(current.X+current.Width, next.X+next.Width), max(current.Y+current.Height, next.Y+next.Height)
	return Rect{X: left, Y: top, Width: right - left, Height: bottom - top}
}

func (e *engine) tableCaptions(table *dom.Node) (top, bottom []*dom.Node) {
	for _, child := range e.flowChildren(table) {
		if child == nil || child.Type != dom.NodeElement {
			continue
		}
		style := e.styleFor(child)
		if style.display != stylemodel.DisplayTableCaption && child.TagName != "caption" {
			continue
		}
		if style.captionSide == stylemodel.CaptionSideBottom {
			bottom = append(bottom, child)
		} else {
			top = append(top, child)
		}
	}
	return top, bottom
}

func (e *engine) renderTableCaptions(captions []*dom.Node, x, y, width, containingHeight float32, heightDefinite bool) float32 {
	start := y
	for _, caption := range captions {
		style := e.styleFor(caption)
		_, height, _ := e.flexIntrinsicSizes(caption, style, flexAxis{horizontal: true}, width, width, containingHeight, heightDefinite)
		if resolved, ok := resolveSize(style.height, containingHeight, heightDefinite); ok {
			height = resolved
			if style.boxSizing == stylemodel.BoxSizingContentBox {
				height += style.padding.Top + style.padding.Bottom + style.border.Top.Width + style.border.Bottom.Width
			}
		}
		height = max(height, style.lineHeight, float32(1))
		captionWidth := max(width-style.margin.Left-style.margin.Right, float32(1))
		captionX, captionY := x+style.margin.Left, y+style.margin.Top
		e.renderGridItem(caption, style, captionX, captionY, captionWidth, height)
		e.tree.Bounds[caption.ID] = Rect{X: captionX, Y: captionY, Width: captionWidth, Height: height}
		y += style.margin.Top + height + style.margin.Bottom
	}
	return y - start
}

func (e *engine) tableColumns(table *dom.Node) []tableColumn {
	var result []tableColumn
	appendColumn := func(node, group *dom.Node) {
		style := e.styleFor(node)
		span := tableSpan(node, "span")
		for offset := 0; offset < span && len(result) < maxTableSpan; offset++ {
			result = append(result, tableColumn{node: node, group: group, style: style})
		}
	}
	for _, child := range e.flowChildren(table) {
		if child == nil || child.Type != dom.NodeElement {
			continue
		}
		style := e.styleFor(child)
		switch {
		case style.display == stylemodel.DisplayTableColumn || child.TagName == "col":
			appendColumn(child, nil)
		case style.display == stylemodel.DisplayTableColumnGroup || child.TagName == "colgroup":
			found := false
			for _, column := range e.flowChildren(child) {
				if column == nil || column.Type != dom.NodeElement {
					continue
				}
				columnStyle := e.styleFor(column)
				if columnStyle.display == stylemodel.DisplayTableColumn || column.TagName == "col" {
					appendColumn(column, child)
					found = true
				}
			}
			if !found {
				appendColumn(child, child)
			}
		}
	}
	return result
}

func (e *engine) tableRows(table *dom.Node) []*dom.Node {
	var rows []*dom.Node
	var visit func(*dom.Node)
	visit = func(node *dom.Node) {
		if node == nil || node.Type != dom.NodeElement {
			return
		}
		style := e.styleFor(node)
		if style.display == stylemodel.DisplayNone {
			return
		}
		if node != table && style.display == stylemodel.DisplayTable {
			return
		}
		if node != table && (style.display == stylemodel.DisplayTableRow || node.TagName == "tr") {
			rows = append(rows, node)
			return
		}
		for _, child := range node.Children {
			visit(child)
		}
	}
	visit(table)
	return rows
}

func (e *engine) placeTableCells(rows []*dom.Node) ([]tableCell, int) {
	occupied := make(map[[2]int]bool)
	var cells []tableCell
	columnCount := 0
	for rowIndex, row := range rows {
		column := 0
		for _, child := range e.flowChildren(row) {
			if child == nil || child.Type != dom.NodeElement {
				continue
			}
			style := e.styleFor(child)
			if style.display == stylemodel.DisplayNone || style.display != stylemodel.DisplayTableCell && child.TagName != "td" && child.TagName != "th" {
				continue
			}
			for occupied[[2]int{rowIndex, column}] {
				column++
			}
			rowSpan, colSpan := tableSpan(child, "rowspan"), tableSpan(child, "colspan")
			for rowOffset := 0; rowOffset < rowSpan; rowOffset++ {
				for columnOffset := 0; columnOffset < colSpan; columnOffset++ {
					occupied[[2]int{rowIndex + rowOffset, column + columnOffset}] = true
				}
			}
			cells = append(cells, tableCell{node: child, row: rowIndex, column: column, rowSpan: rowSpan, colSpan: colSpan})
			column += colSpan
			columnCount = max(columnCount, column)
		}
	}
	return cells, columnCount
}

func tableSpan(node *dom.Node, name string) int {
	raw, ok := node.Attribute(name)
	if !ok {
		return 1
	}
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value < 1 {
		return 1
	}
	return min(value, maxTableSpan)
}
