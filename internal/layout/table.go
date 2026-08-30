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
	e.y += tableStyle.margin.Top
	x += tableStyle.margin.Left
	availableWidth -= tableStyle.margin.Left + tableStyle.margin.Right
	if availableWidth < 1 {
		availableWidth = 1
	}

	rows := e.tableRows(node)
	cells, columnCount := e.placeTableCells(rows)
	if columnCount == 0 {
		columnCount = 1
	}
	columnWidths := make([]float32, columnCount)
	for index := range cells {
		cell := &cells[index]
		cellStyle := e.styleFor(cell.node)
		intrinsicWidth, _, _ := e.flexIntrinsicSizes(cell.node, cellStyle, flexAxis{horizontal: true}, availableWidth, availableWidth, containingHeight, heightDefinite)
		cell.minWidth = max(intrinsicWidth/float32(cell.colSpan), float32(1))
		for column := cell.column; column < min(cell.column+cell.colSpan, columnCount); column++ {
			columnWidths[column] = max(columnWidths[column], cell.minWidth)
		}
	}

	horizontalExtras := tableStyle.padding.Left + tableStyle.padding.Right + tableStyle.border.Left.Width + tableStyle.border.Right.Width
	tableWidth := availableWidth
	if resolved, ok := resolveSize(tableStyle.width, availableWidth, true); ok {
		tableWidth = resolved
		if tableStyle.boxSizing == stylemodel.BoxSizingContentBox {
			tableWidth += horizontalExtras
		}
	}
	tableWidth = constrainSize(tableWidth, tableStyle.minWidth, tableStyle.maxWidth, availableWidth, true)
	if tableWidth > availableWidth && tableStyle.width.Kind == stylemodel.SizeAuto {
		tableWidth = availableWidth
	}
	contentWidth := max(tableWidth-horizontalExtras, float32(1))
	totalMinimum := float32(0)
	for _, width := range columnWidths {
		totalMinimum += width
	}
	if totalMinimum <= 0 {
		for index := range columnWidths {
			columnWidths[index] = contentWidth / float32(columnCount)
		}
	} else if totalMinimum < contentWidth {
		extra := (contentWidth - totalMinimum) / float32(columnCount)
		for index := range columnWidths {
			columnWidths[index] += extra
		}
	} else if totalMinimum > contentWidth {
		scale := contentWidth / totalMinimum
		for index := range columnWidths {
			columnWidths[index] = max(columnWidths[index]*scale, float32(1))
		}
	}

	rowHeights := make([]float32, len(rows))
	for index := range cells {
		cell := &cells[index]
		cellWidth := trackOffset(columnWidths, cell.column+cell.colSpan, 0) - trackOffset(columnWidths, cell.column, 0)
		cellStyle := e.styleFor(cell.node)
		_, intrinsicHeight, _ := e.flexIntrinsicSizes(cell.node, cellStyle, flexAxis{horizontal: true}, cellWidth, cellWidth, containingHeight, heightDefinite)
		share := max(intrinsicHeight/float32(cell.rowSpan), cellStyle.lineHeight)
		for row := cell.row; row < min(cell.row+cell.rowSpan, len(rowHeights)); row++ {
			rowHeights[row] = max(rowHeights[row], share)
		}
	}
	for index := range rowHeights {
		rowHeights[index] = max(rowHeights[index], float32(1))
	}

	boxTop := e.y
	contentX := x + tableStyle.border.Left.Width + tableStyle.padding.Left
	contentY := boxTop + tableStyle.border.Top.Width + tableStyle.padding.Top
	decorationIndex := -1
	if tableStyle.background != 0 || hasVisibleBorder(tableStyle.border) || len(tableStyle.boxShadows) != 0 || tableStyle.outline.Style != stylemodel.BorderNone {
		decorationIndex = len(e.tree.Decorations)
		e.tree.Decorations = append(e.tree.Decorations, Decoration{
			Order: e.nextOrder(), StackingID: e.stackingID, NodeID: node.ID,
			Rect:       Rect{X: x, Y: boxTop, Width: tableWidth},
			Background: tableStyle.background, Border: tableStyle.border, Padding: tableStyle.padding,
			Opacity: e.opacity * tableStyle.opacity, BoxShadows: append([]stylemodel.Shadow(nil), tableStyle.boxShadows...),
			Outline: tableStyle.outline, OutlineOffset: tableStyle.outlineOffset,
			Cursor: tableStyle.cursor, Transform: stylemodel.IdentityMatrix(), Hidden: tableStyle.hidden,
		})
	}
	for index := range cells {
		cell := &cells[index]
		cellX := contentX + trackOffset(columnWidths, cell.column, 0)
		cellY := contentY + trackOffset(rowHeights, cell.row, 0)
		cellWidth := trackOffset(columnWidths, min(cell.column+cell.colSpan, columnCount), 0) - trackOffset(columnWidths, cell.column, 0)
		cellHeight := trackOffset(rowHeights, min(cell.row+cell.rowSpan, len(rowHeights)), 0) - trackOffset(rowHeights, cell.row, 0)
		cellStyle := e.styleFor(cell.node)
		cellStyle.display = stylemodel.DisplayBlock
		e.renderGridItem(cell.node, cellStyle, cellX, cellY, cellWidth, cellHeight)
		e.tree.Bounds[cell.node.ID] = Rect{X: cellX, Y: cellY, Width: cellWidth, Height: cellHeight}
	}
	tableContentHeight := trackOffset(rowHeights, len(rowHeights), 0)
	for rowIndex, row := range rows {
		e.tree.Bounds[row.ID] = Rect{X: contentX, Y: contentY + trackOffset(rowHeights, rowIndex, 0), Width: contentWidth, Height: rowHeights[rowIndex]}
	}
	verticalExtras := tableStyle.padding.Top + tableStyle.padding.Bottom + tableStyle.border.Top.Width + tableStyle.border.Bottom.Width
	tableHeight := tableContentHeight + verticalExtras
	if resolved, ok := resolveSize(tableStyle.height, containingHeight, heightDefinite); ok {
		tableHeight = resolved
		if tableStyle.boxSizing == stylemodel.BoxSizingContentBox {
			tableHeight += verticalExtras
		}
	}
	tableHeight = constrainSize(tableHeight, tableStyle.minHeight, tableStyle.maxHeight, containingHeight, heightDefinite)
	e.tree.Bounds[node.ID] = Rect{X: x, Y: boxTop, Width: tableWidth, Height: tableHeight}
	if decorationIndex >= 0 {
		e.tree.Decorations[decorationIndex].Height = tableHeight
		e.tree.Decorations[decorationIndex].Radius = resolveBorderRadii(tableStyle.radius, tableWidth, tableHeight)
	}
	e.y = boxTop + tableHeight + tableStyle.margin.Bottom
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
