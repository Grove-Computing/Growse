package layout

import (
	"strings"

	"github.com/Grove-Computing/Growse/internal/dom"
	stylemodel "github.com/Grove-Computing/Growse/internal/style"
)

type verticalInlineItem struct {
	run        inlineRun
	text       string
	advance    float32
	cross      float32
	baseline   float32
	atomic     bool
	forceBreak bool
}

// addVerticalInlineRuns implements the bounded upright-glyph subset declared
// by v0.19.0. Each glyph advances on the physical Y axis; columns progress on
// the physical X axis according to vertical-rl or vertical-lr. Complex bidi,
// text-orientation variants, ruby, and tate-chu-yoko remain deliberately out
// of scope, but every emitted rectangle is shared by paint and hit testing.
func (e *engine) addVerticalInlineRuns(nodeID dom.NodeID, tag string, runs []inlineRun, container blockStyle, x, width float32) {
	inlineLimit, definite := resolveSize(container.height, e.viewportHeight, e.viewportHeight > 0)
	if definite && container.boxSizing == stylemodel.BoxSizingBorderBox {
		inlineLimit -= container.padding.Top + container.padding.Bottom + container.border.Top.Width + container.border.Bottom.Width
	}
	if inlineLimit <= 0 {
		inlineLimit = max(e.viewportHeight-e.y, max(width, float32(160)))
	}
	inlineLimit = min(max(inlineLimit, float32(1)), float32(32768))

	items := e.verticalInlineItems(runs, width)
	if len(items) == 0 {
		return
	}
	columns := make([][]verticalInlineItem, 1)
	used := []float32{0}
	for _, item := range items {
		column := len(columns) - 1
		if item.forceBreak {
			if len(columns[column]) != 0 {
				columns = append(columns, nil)
				used = append(used, 0)
			}
			continue
		}
		if len(columns[column]) != 0 && used[column]+item.advance > inlineLimit {
			columns = append(columns, nil)
			used = append(used, 0)
			column++
		}
		columns[column] = append(columns[column], item)
		used[column] += item.advance
	}
	if len(columns[len(columns)-1]) == 0 {
		columns = columns[:len(columns)-1]
		used = used[:len(used)-1]
	}
	if len(columns) == 0 {
		return
	}

	crossSizes := make([]float32, len(columns))
	for columnIndex, column := range columns {
		for _, item := range column {
			crossSizes[columnIndex] = max(crossSizes[columnIndex], item.cross)
		}
		crossSizes[columnIndex] = max(crossSizes[columnIndex], container.lineHeight, container.fontSize)
	}
	baseY := e.y
	totalCross := float32(0)
	for _, cross := range crossSizes {
		totalCross += cross
	}
	boxX := x
	if container.writingMode == stylemodel.WritingModeVerticalRL {
		boxX = x + width - totalCross
	}
	box := Box{
		Order: e.nextOrder(), StackingID: e.stackingID, NodeID: nodeID, Tag: tag,
		X: boxX, Y: baseY, Width: totalCross, FontSize: container.fontSize, Bold: container.bold,
		FontFamilies: append([]string(nil), container.fontFamilies...), FontStyle: container.fontStyle, FontStretch: container.fontStretch,
		LetterSpacing: container.letterSpacing, WordSpacing: container.wordSpacing, Color: container.color,
		Cursor: container.cursor, Opacity: e.opacity, Decoration: container.decoration, DecorationColor: container.decorationColor,
		TextShadows: append([]stylemodel.Shadow(nil), container.textShadows...), Transform: stylemodel.IdentityMatrix(), Hidden: container.hidden,
		Clip: cloneRect(e.clip), Clips: cloneClipRegions(e.clips), WritingMode: container.writingMode, Direction: container.direction,
	}
	blockOffset := float32(0)
	maxInlineUsed := float32(0)
	for columnIndex, column := range columns {
		cross := crossSizes[columnIndex]
		columnX := x + blockOffset
		if container.writingMode == stylemodel.WritingModeVerticalRL {
			columnX = x + width - blockOffset - cross
			blockOffset += cross
		} else {
			blockOffset += cross
		}
		columnItems := append([]verticalInlineItem(nil), column...)
		columnY := baseY
		if container.direction == stylemodel.DirectionRTL {
			reverseVerticalItems(columnItems)
			columnY += max(inlineLimit-used[columnIndex], float32(0))
		}
		maxInlineUsed = max(maxInlineUsed, used[columnIndex])
		e.appendVerticalColumn(&box, columnItems, columnX, columnY, cross, width)
	}
	if len(box.Runs) != 0 {
		box.Text = joinVerticalText(box.Runs)
		box.Height = maxInlineUsed
		if definite {
			box.Height = inlineLimit
		}
		box.Baseline = box.Runs[0].Baseline
		e.tree.Boxes = append(e.tree.Boxes, box)
	}
	if definite {
		e.y = baseY + inlineLimit
	} else {
		e.y = baseY + maxInlineUsed
	}
}

func (e *engine) verticalInlineItems(runs []inlineRun, containingWidth float32) []verticalInlineItem {
	var result []verticalInlineItem
	for _, token := range tokenizeInlineRuns(transformInlineRuns(runs)) {
		if token.text == "\n" {
			result = append(result, verticalInlineItem{forceBreak: true})
			continue
		}
		if token.atomic {
			if token.image {
				token.width, token.height, token.baseline = e.resolveInlineImageSize(token, containingWidth)
			} else if token.flex {
				token.width, token.height, token.baseline = e.resolveInlineFlexSize(token.node, token.style, containingWidth)
			} else if token.grid {
				token.width, token.height, token.baseline = e.resolveInlineGridSize(token.node, token.style, containingWidth)
			} else {
				token.width, token.height = resolveAtomicSize(token, containingWidth)
				token.baseline = token.height
			}
			result = append(result, verticalInlineItem{run: token, advance: max(token.height, float32(1)), cross: max(token.width, float32(1)), baseline: token.baseline, atomic: true})
			continue
		}
		for _, character := range []rune(token.text) {
			text := string(character)
			glyphWidth, glyphHeight, ascent := measureStyledText(text, token.style)
			advance := max(glyphHeight+token.style.wordSpacing, token.style.fontSize+token.style.letterSpacing)
			result = append(result, verticalInlineItem{
				run: token, text: text, advance: max(advance, float32(1)),
				cross: max(glyphWidth, token.style.fontSize), baseline: ascent,
			})
		}
	}
	return result
}

func (e *engine) appendVerticalColumn(box *Box, items []verticalInlineItem, x, y, cross, containingWidth float32) {
	if len(items) == 0 || len(e.tree.Boxes) >= maxLineBoxes {
		if len(e.tree.Boxes) >= maxLineBoxes {
			e.tree.addFallback(box.NodeID, "line box limit exceeded")
		}
		return
	}
	advance := float32(0)
	for _, item := range items {
		itemY := y + advance
		if item.atomic {
			if item.run.node != nil {
				if item.run.image {
					e.renderInlineImage(item.run, x, itemY, containingWidth)
				} else if item.run.grid {
					e.renderInlineGrid(item.run, x, itemY)
				} else {
					layoutItem := &flexLayoutItem{node: item.run.node, style: item.run.style, crossSize: item.run.width}
					layoutItem.algorithm = &flexItem{target: item.run.height}
					e.renderFlexItem(layoutItem, flexAxis{horizontal: false}, x, itemY, item.run.height, item.run.width)
				}
			}
			advance += item.advance
			continue
		}
		run := TextRun{
			NodeID: item.run.nodeID, Tag: item.run.tag, Text: item.text, Width: item.advance,
			OffsetX: x - box.X, OffsetY: itemY - box.Y, CrossSize: cross,
			FontSize: item.run.style.fontSize, Bold: item.run.style.bold,
			FontFamilies: append([]string(nil), item.run.style.fontFamilies...), FontStyle: item.run.style.fontStyle, FontStretch: item.run.style.fontStretch,
			LetterSpacing: item.run.style.letterSpacing, WordSpacing: item.run.style.wordSpacing,
			Color: item.run.style.color, Background: item.run.style.background,
			Baseline: itemY + item.baseline, Decoration: item.run.style.decoration, DecorationColor: item.run.style.decorationColor,
			Opacity: item.run.opacity, WritingMode: box.WritingMode, Direction: box.Direction,
			TextShadows: append([]stylemodel.Shadow(nil), item.run.style.textShadows...),
		}
		box.Runs = append(box.Runs, run)
		advance += item.advance
	}
}

func joinVerticalText(runs []TextRun) string {
	var text strings.Builder
	for _, run := range runs {
		text.WriteString(run.Text)
	}
	return text.String()
}

func reverseVerticalItems(items []verticalInlineItem) {
	for left, right := 0, len(items)-1; left < right; left, right = left+1, right-1 {
		items[left], items[right] = items[right], items[left]
	}
}
