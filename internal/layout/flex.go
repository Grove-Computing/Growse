package layout

import (
	"sort"
	"strings"

	"github.com/Grove-Computing/Growse/internal/dom"
	stylemodel "github.com/Grove-Computing/Growse/internal/style"
)

type flexAxis struct {
	horizontal bool
	reverse    bool
	crossFlip  bool
}

func axisFor(direction stylemodel.FlexDirection, wrap stylemodel.FlexWrap) flexAxis {
	axis := flexAxis{horizontal: direction == stylemodel.FlexDirectionRow || direction == stylemodel.FlexDirectionRowReverse}
	axis.reverse = direction == stylemodel.FlexDirectionRowReverse || direction == stylemodel.FlexDirectionColumnReverse
	axis.crossFlip = wrap == stylemodel.FlexWrapReverse
	return axis
}

type flexItem struct {
	index        int
	order        int
	base         float32
	hypothetical float32
	minimum      float32
	maximum      float32
	grow         float32
	shrink       float32
	marginStart  float32
	marginEnd    float32
	target       float32
	frozen       bool
}

type flexLine struct {
	items []*flexItem
}

func orderFlexItems(items []*flexItem) {
	sort.SliceStable(items, func(left, right int) bool {
		if items[left].order != items[right].order {
			return items[left].order < items[right].order
		}
		return items[left].index < items[right].index
	})
}

func formFlexLines(items []*flexItem, available, gap float32, wrap stylemodel.FlexWrap) []flexLine {
	if len(items) == 0 {
		return nil
	}
	if wrap == stylemodel.FlexNoWrap || available < 0 {
		return []flexLine{{items: append([]*flexItem(nil), items...)}}
	}
	lines := []flexLine{{}}
	used := float32(0)
	for _, item := range items {
		required := item.hypothetical + item.marginStart + item.marginEnd
		if len(lines[len(lines)-1].items) != 0 {
			required += gap
		}
		if len(lines[len(lines)-1].items) != 0 && used+required > available {
			lines = append(lines, flexLine{})
			used = 0
			required = item.hypothetical + item.marginStart + item.marginEnd
		}
		lines[len(lines)-1].items = append(lines[len(lines)-1].items, item)
		used += required
	}
	return lines
}

// resolveFlexibleLengths distributes free space and repeatedly freezes items
// which violate their min/max constraints. The final item receives any binary
// floating-point remainder so the line sum remains deterministic.
func resolveFlexibleLengths(line *flexLine, available, gap float32) {
	if line == nil || len(line.items) == 0 {
		return
	}
	itemSpace := available - gap*float32(len(line.items)-1)
	sumHypothetical := float32(0)
	for _, item := range line.items {
		itemSpace -= item.marginStart + item.marginEnd
		item.target = item.base
		item.frozen = false
		sumHypothetical += item.hypothetical
	}
	growing := sumHypothetical < itemSpace
	for _, item := range line.items {
		factor := item.shrink
		if growing {
			factor = item.grow
		}
		if factor == 0 || growing && item.base > item.hypothetical || !growing && item.base < item.hypothetical {
			item.target = item.hypothetical
			item.frozen = true
		}
	}

	for iteration := 0; iteration <= len(line.items); iteration++ {
		used, weight := float32(0), float32(0)
		var unfrozen []*flexItem
		for _, item := range line.items {
			used += item.target
			if item.frozen {
				continue
			}
			factor := item.grow
			if !growing {
				factor = item.shrink * item.base
			}
			weight += factor
			unfrozen = append(unfrozen, item)
		}
		if len(unfrozen) == 0 {
			break
		}
		remaining := itemSpace - used
		assigned := float32(0)
		violations := false
		for index, item := range unfrozen {
			share := float32(0)
			if weight > 0 {
				factor := item.grow
				if !growing {
					factor = item.shrink * item.base
				}
				if index == len(unfrozen)-1 {
					share = remaining - assigned
				} else {
					share = remaining * factor / weight
					assigned += share
				}
			}
			candidate := item.target + share
			clamped := max(item.minimum, candidate)
			if item.maximum >= 0 {
				clamped = min(item.maximum, clamped)
			}
			item.target = clamped
			if clamped != candidate {
				item.frozen = true
				violations = true
			}
		}
		if !violations {
			for _, item := range unfrozen {
				item.frozen = true
			}
			break
		}
	}
}

type flexLayoutItem struct {
	algorithm      *flexItem
	node           *dom.Node
	style          blockStyle
	crossSize      float32
	crossStart     float32
	crossEnd       float32
	mainAutoStart  bool
	mainAutoEnd    bool
	crossAutoStart bool
	crossAutoEnd   bool
	crossAutoSize  bool
	baseline       float32
}

func (e *engine) addFlexChildren(container *dom.Node, containerStyle blockStyle, x, width, containingHeight float32, heightDefinite bool) {
	axis := axisFor(containerStyle.flexDirection, containerStyle.flexWrap)
	availableMain := width
	if !axis.horizontal {
		availableMain = -1
		if heightDefinite {
			availableMain = containingHeight
		}
	}
	mainGap := containerStyle.columnGap.Resolve(width)
	crossGap := containerStyle.rowGap.Resolve(containingHeight)
	if !axis.horizontal {
		mainGap, crossGap = containerStyle.rowGap.Resolve(containingHeight), containerStyle.columnGap.Resolve(width)
	}

	items, byAlgorithm := e.collectFlexItems(container, axis, availableMain, width, containingHeight, heightDefinite)
	if len(items) == 0 {
		return
	}
	algorithms := make([]*flexItem, 0, len(items))
	for _, item := range items {
		algorithms = append(algorithms, item.algorithm)
	}
	orderFlexItems(algorithms)
	if availableMain < 0 {
		availableMain = mainGap * float32(max(len(algorithms)-1, 0))
		for _, item := range algorithms {
			availableMain += item.hypothetical + item.marginStart + item.marginEnd
		}
	}
	lines := formFlexLines(algorithms, availableMain, mainGap, containerStyle.flexWrap)
	lineCrossSizes := make([]float32, len(lines))
	for lineIndex := range lines {
		resolveFlexibleLengths(&lines[lineIndex], availableMain, mainGap)
		for _, algorithm := range lines[lineIndex].items {
			item := byAlgorithm[algorithm]
			lineCrossSizes[lineIndex] = max(lineCrossSizes[lineIndex], item.crossSize+item.crossStart+item.crossEnd)
		}
	}
	totalCross := crossGap * float32(max(len(lines)-1, 0))
	for _, size := range lineCrossSizes {
		totalCross += size
	}
	availableCross := width
	if axis.horizontal {
		availableCross = totalCross
		if heightDefinite {
			availableCross = containingHeight
		}
	}
	if len(lines) == 1 && containerStyle.flexWrap == stylemodel.FlexNoWrap {
		lineCrossSizes[0] = max(lineCrossSizes[0], availableCross)
		totalCross = lineCrossSizes[0]
	}
	crossOffset, crossSpacing := alignContentSpacing(containerStyle.alignContent, availableCross-totalCross, len(lines))
	if containerStyle.alignContent == stylemodel.AlignStretch && availableCross > totalCross && len(lines) > 0 {
		extra := (availableCross - totalCross) / float32(len(lines))
		for index := range lineCrossSizes {
			lineCrossSizes[index] += extra
		}
		totalCross = availableCross
		crossOffset = 0
	}

	crossCursor := crossOffset
	lineIndices := make([]int, len(lines))
	for index := range lineIndices {
		lineIndices[index] = index
	}
	if axis.crossFlip {
		for left, right := 0, len(lineIndices)-1; left < right; left, right = left+1, right-1 {
			lineIndices[left], lineIndices[right] = lineIndices[right], lineIndices[left]
		}
	}
	for _, lineIndex := range lineIndices {
		line := &lines[lineIndex]
		lineUsed, autoMargins := mainLineUsage(line, mainGap, byAlgorithm)
		freeMain := max(availableMain-lineUsed, float32(0))
		autoShare := float32(0)
		if autoMargins > 0 {
			autoShare = freeMain / float32(autoMargins)
		}
		mainOffset, distributedGap := justifySpacing(containerStyle.justifyContent, freeMain, len(line.items), autoMargins > 0)
		mainCursor := mainOffset
		if axis.reverse {
			mainCursor = availableMain - mainOffset
		}
		lineBaseline := float32(0)
		if axis.horizontal {
			for _, algorithm := range line.items {
				item := byAlgorithm[algorithm]
				lineBaseline = max(lineBaseline, item.crossStart+item.baseline)
			}
		}
		for _, algorithm := range line.items {
			item := byAlgorithm[algorithm]
			mainStart, mainEnd := algorithm.marginStart, algorithm.marginEnd
			if item.mainAutoStart {
				mainStart = autoShare
			}
			if item.mainAutoEnd {
				mainEnd = autoShare
			}
			usedCrossStart, usedCrossEnd := item.crossStart, item.crossEnd
			freeCross := max(lineCrossSizes[lineIndex]-item.crossSize-usedCrossStart-usedCrossEnd, float32(0))
			crossAutoCount := 0
			if item.crossAutoStart {
				crossAutoCount++
			}
			if item.crossAutoEnd {
				crossAutoCount++
			}
			if crossAutoCount > 0 {
				share := freeCross / float32(crossAutoCount)
				if item.crossAutoStart {
					usedCrossStart = share
				}
				if item.crossAutoEnd {
					usedCrossEnd = share
				}
			}
			alignment := item.style.alignSelf
			if alignment == stylemodel.AlignAuto {
				alignment = containerStyle.alignItems
			}
			if axis.crossFlip {
				alignment = flipCrossAlignment(alignment)
			}
			if alignment == stylemodel.AlignStretch && item.crossAutoSize && crossAutoCount == 0 {
				item.crossSize = max(lineCrossSizes[lineIndex]-usedCrossStart-usedCrossEnd, float32(0))
				freeCross = 0
			}
			crossPosition := usedCrossStart
			if crossAutoCount == 0 {
				switch alignment {
				case stylemodel.AlignFlexEnd:
					crossPosition += freeCross
				case stylemodel.AlignCenter:
					crossPosition += freeCross / 2
				case stylemodel.AlignBaseline:
					if axis.horizontal {
						crossPosition = max(lineBaseline-item.baseline, usedCrossStart)
					}
				}
			}
			var itemX, itemY float32
			if axis.horizontal {
				if axis.reverse {
					mainCursor -= mainEnd + algorithm.target
					itemX = x + mainCursor
					mainCursor -= mainStart + mainGap + distributedGap
				} else {
					mainCursor += mainStart
					itemX = x + mainCursor
					mainCursor += algorithm.target + mainEnd + mainGap + distributedGap
				}
				itemY = e.y + crossCursor + crossPosition
			} else {
				if axis.reverse {
					mainCursor -= mainEnd + algorithm.target
					itemY = e.y + mainCursor
					mainCursor -= mainStart + mainGap + distributedGap
				} else {
					mainCursor += mainStart
					itemY = e.y + mainCursor
					mainCursor += algorithm.target + mainEnd + mainGap + distributedGap
				}
				itemX = x + crossCursor + crossPosition
			}
			e.renderFlexItem(item, axis, itemX, itemY, algorithm.target, item.crossSize)
		}
		crossCursor += lineCrossSizes[lineIndex] + crossGap + crossSpacing
	}
	if axis.horizontal {
		e.y += totalCross
	} else {
		e.y += availableMain
	}
}

func (e *engine) collectFlexItems(container *dom.Node, axis flexAxis, availableMain, width, height float32, heightDefinite bool) ([]*flexLayoutItem, map[*flexItem]*flexLayoutItem) {
	var items []*flexLayoutItem
	byAlgorithm := make(map[*flexItem]*flexLayoutItem)
	for index, node := range container.Children {
		if node.Type != dom.NodeElement && (node.Type != dom.NodeText || strings.TrimSpace(node.Text) == "") {
			continue
		}
		style := e.styleFor(node)
		if style.display == stylemodel.DisplayNone {
			continue
		}
		base, cross, minContent := e.flexIntrinsicSizes(node, style, axis, availableMain, width, height, heightDefinite)
		minimum, maximum := float32(0), float32(-1)
		minValue, maxValue := style.minWidth, style.maxWidth
		if !axis.horizontal {
			minValue, maxValue = style.minHeight, style.maxHeight
		}
		if minValue.Kind == stylemodel.SizeAuto && mainOverflow(style, axis) == stylemodel.OverflowVisible {
			minimum = minContent
			preferred := style.width
			if !axis.horizontal {
				preferred = style.height
			}
			if resolved, ok := resolveSize(preferred, max(availableMain, float32(0)), availableMain >= 0); ok {
				minimum = min(minimum, resolved)
			}
		} else if resolved, ok := resolveSize(minValue, max(availableMain, float32(0)), availableMain >= 0); ok {
			minimum = resolved
		}
		if resolved, ok := resolveSize(maxValue, max(availableMain, float32(0)), availableMain >= 0); ok {
			maximum = resolved
		}
		hypothetical := max(base, minimum)
		if maximum >= 0 {
			hypothetical = min(hypothetical, maximum)
		}
		mainStart, mainEnd := style.margin.Left, style.margin.Right
		crossStart, crossEnd := style.margin.Top, style.margin.Bottom
		mainAutoStart, mainAutoEnd := style.marginAuto.Left, style.marginAuto.Right
		crossAutoStart, crossAutoEnd := style.marginAuto.Top, style.marginAuto.Bottom
		crossAutoSize := style.height.Kind == stylemodel.SizeAuto
		if !axis.horizontal {
			mainStart, mainEnd = style.margin.Top, style.margin.Bottom
			crossStart, crossEnd = style.margin.Left, style.margin.Right
			mainAutoStart, mainAutoEnd = style.marginAuto.Top, style.marginAuto.Bottom
			crossAutoStart, crossAutoEnd = style.marginAuto.Left, style.marginAuto.Right
			crossAutoSize = style.width.Kind == stylemodel.SizeAuto
		}
		algorithm := &flexItem{
			index: index, order: style.order, base: base, hypothetical: hypothetical,
			minimum: minimum, maximum: maximum, grow: style.flexGrow, shrink: style.flexShrink,
			marginStart: mainStart, marginEnd: mainEnd,
		}
		_, _, ascent := measureText("Mg", style.fontSize, style.bold)
		item := &flexLayoutItem{
			algorithm: algorithm, node: node, style: style, crossSize: cross,
			crossStart: crossStart, crossEnd: crossEnd,
			mainAutoStart: mainAutoStart, mainAutoEnd: mainAutoEnd,
			crossAutoStart: crossAutoStart, crossAutoEnd: crossAutoEnd,
			crossAutoSize: crossAutoSize, baseline: ascent + style.border.Top.Width + style.padding.Top,
		}
		items = append(items, item)
		byAlgorithm[algorithm] = item
	}
	return items, byAlgorithm
}

func (e *engine) flexIntrinsicSizes(node *dom.Node, style blockStyle, axis flexAxis, availableMain, width, height float32, heightDefinite bool) (float32, float32, float32) {
	text := normalizeWhitespace(e.inlineText(node))
	textWidth, textHeight, _ := measureStyledText(text, style)
	minTextWidth := float32(0)
	for _, word := range strings.Fields(text) {
		wordWidth, _, _ := measureStyledText(word, style)
		minTextWidth = max(minTextWidth, wordWidth)
	}
	if textHeight <= 0 {
		textHeight = style.fontSize * 1.4
	}
	if isEditableTextControl(node) || isSelectControl(node) {
		textWidth, textHeight = inputWidth, inputHeight
		minTextWidth = inputWidth
	} else if isCheckableControl(node) {
		textWidth, textHeight = checkableSize, checkableSize
		minTextWidth = checkableSize
	} else if isSubmitButtonControl(node) {
		textWidth, textHeight = buttonWidth, inputHeight
		minTextWidth = buttonWidth
	} else if node != nil && node.TagName == "img" && e.images != nil {
		resource := e.images[node.ID]
		textWidth, textHeight = resource.IntrinsicWidth, resource.IntrinsicHeight
		if attribute, ok := imageDimensionAttribute(node, "width"); ok {
			textWidth = attribute
		}
		if attribute, ok := imageDimensionAttribute(node, "height"); ok {
			textHeight = attribute
		}
		textWidth, textHeight = max(textWidth, float32(16)), max(textHeight, float32(16))
		minTextWidth = textWidth
	}
	horizontalExtras := style.padding.Left + style.padding.Right + style.border.Left.Width + style.border.Right.Width
	verticalExtras := style.padding.Top + style.padding.Bottom + style.border.Top.Width + style.border.Bottom.Width
	intrinsicWidth, intrinsicHeight := textWidth+horizontalExtras, textHeight+verticalExtras
	if resolved, ok := resolveSize(style.width, width, true); ok {
		intrinsicWidth = resolved
		if style.boxSizing == stylemodel.BoxSizingContentBox {
			intrinsicWidth += horizontalExtras
		}
	}
	if resolved, ok := resolveSize(style.height, height, heightDefinite); ok {
		intrinsicHeight = resolved
		if style.boxSizing == stylemodel.BoxSizingContentBox {
			intrinsicHeight += verticalExtras
		}
	}
	base := intrinsicWidth
	if !axis.horizontal {
		base = intrinsicHeight
	}
	if style.flexBasis.Kind == stylemodel.FlexBasisLength && (style.flexBasis.Value.Percentage == 0 || availableMain >= 0) {
		base = style.flexBasis.Value.Resolve(max(availableMain, float32(0)))
	} else if style.flexBasis.Kind == stylemodel.FlexBasisContent {
		if axis.horizontal {
			base = textWidth + horizontalExtras
		} else {
			base = textHeight + verticalExtras
		}
	}
	if style.aspectRatio > 0 {
		if axis.horizontal && style.width.Kind == stylemodel.SizeAuto && style.height.Kind == stylemodel.SizeLength {
			base = intrinsicHeight * style.aspectRatio
		} else if !axis.horizontal && style.height.Kind == stylemodel.SizeAuto && style.width.Kind == stylemodel.SizeLength {
			base = intrinsicWidth / style.aspectRatio
		}
	}
	if axis.horizontal {
		return max(base, float32(0)), max(intrinsicHeight, float32(1)), max(minTextWidth+horizontalExtras, float32(0))
	}
	return max(base, float32(0)), max(intrinsicWidth, float32(1)), max(textHeight+verticalExtras, float32(0))
}

func mainOverflow(style blockStyle, axis flexAxis) stylemodel.Overflow {
	if axis.horizontal {
		return style.overflowX
	}
	return style.overflowY
}

func (e *engine) renderFlexItem(item *flexLayoutItem, axis flexAxis, x, y, mainSize, crossSize float32) {
	style := item.style
	style.margin = stylemodel.Edges{}
	style.marginAuto = stylemodel.AutoEdges{}
	style.boxSizing = stylemodel.BoxSizingBorderBox
	if axis.horizontal {
		style.width = pixelSize(mainSize)
		style.height = pixelSize(crossSize)
	} else {
		style.width = pixelSize(crossSize)
		style.height = pixelSize(mainSize)
	}
	startBoxes, startDecorations := len(e.tree.Boxes), len(e.tree.Decorations)
	savedY, savedClip := e.y, e.clip
	e.y, e.clip = 0, nil
	outerWidth, outerHeight := crossSize, mainSize
	if axis.horizontal {
		outerWidth, outerHeight = mainSize, crossSize
	}
	if item.node.Type == dom.NodeText {
		e.addText(item.node.ID, "text", normalizeWhitespace(item.node.Text), style, 0, outerWidth)
	} else if isEditableTextControl(item.node) {
		e.addInput(item.node, style, 0, outerWidth, outerHeight, true)
	} else if isSelectControl(item.node) {
		e.addSelect(item.node, style, 0, outerWidth, outerHeight, true)
	} else if isCheckableControl(item.node) {
		e.addCheckable(item.node, style, 0, outerWidth, outerHeight, true)
	} else if isSubmitButtonControl(item.node) {
		e.addSubmitButton(item.node, style, 0, outerWidth, outerHeight, true)
	} else if item.node.TagName == "img" && e.images != nil {
		e.addImage(item.node, style, 0, outerWidth, outerHeight, true)
	} else {
		if style.display == stylemodel.DisplayInlineFlex {
			style.display = stylemodel.DisplayFlex
		} else if style.display == stylemodel.DisplayInlineGrid {
			style.display = stylemodel.DisplayGrid
		} else if style.display != stylemodel.DisplayFlex && style.display != stylemodel.DisplayGrid {
			style.display = stylemodel.DisplayBlock
		}
		e.addBlock(item.node, style, 0, outerWidth, outerHeight, true, nil)
	}
	e.y, e.clip = savedY, savedClip
	translateFlexGeometry(e.tree, startBoxes, startDecorations, x, y, savedClip)
}

func (e *engine) resolveInlineFlexSize(node *dom.Node, containerStyle blockStyle, containingWidth float32) (float32, float32, float32) {
	axis := axisFor(containerStyle.flexDirection, containerStyle.flexWrap)
	mainSize, crossSize := float32(0), float32(0)
	itemCount := 0
	baseline := float32(0)
	for _, child := range node.Children {
		if child.Type != dom.NodeElement && (child.Type != dom.NodeText || strings.TrimSpace(child.Text) == "") {
			continue
		}
		childStyle := e.styleFor(child)
		if childStyle.display == stylemodel.DisplayNone {
			continue
		}
		main, cross, _ := e.flexIntrinsicSizes(child, childStyle, axis, containingWidth, containingWidth, 0, false)
		if axis.horizontal {
			mainSize += main + childStyle.margin.Left + childStyle.margin.Right
			crossSize = max(crossSize, cross+childStyle.margin.Top+childStyle.margin.Bottom)
		} else {
			mainSize += main + childStyle.margin.Top + childStyle.margin.Bottom
			crossSize = max(crossSize, cross+childStyle.margin.Left+childStyle.margin.Right)
		}
		if itemCount == 0 {
			_, _, ascent := measureText("Mg", childStyle.fontSize, childStyle.bold)
			baseline = ascent + childStyle.margin.Top + childStyle.border.Top.Width + childStyle.padding.Top
		}
		itemCount++
	}
	gap := containerStyle.columnGap.Resolve(containingWidth)
	if !axis.horizontal {
		gap = containerStyle.rowGap.Resolve(0)
	}
	if itemCount > 1 {
		mainSize += gap * float32(itemCount-1)
	}
	contentWidth, contentHeight := mainSize, crossSize
	if !axis.horizontal {
		contentWidth, contentHeight = crossSize, mainSize
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
	width = constrainSize(width, containerStyle.minWidth, containerStyle.maxWidth, containingWidth, true)
	height = constrainSize(height, containerStyle.minHeight, containerStyle.maxHeight, 0, false)
	baseline += containerStyle.border.Top.Width + containerStyle.padding.Top
	if baseline <= 0 || baseline > height {
		baseline = height
	}
	return max(width, float32(1)), max(height, float32(1)), baseline
}

func pixelSize(value float32) stylemodel.SizeValue {
	return stylemodel.SizeValue{Kind: stylemodel.SizeLength, Value: stylemodel.LengthPercentage{Pixels: max(value, float32(0))}}
}

func translateFlexGeometry(tree *Tree, boxStart, decorationStart int, x, y float32, parentClip *Rect) {
	translateClip := func(clip *Rect) *Rect {
		if clip != nil {
			clip.X += x
			clip.Y += y
		}
		if parentClip == nil {
			return clip
		}
		if clip == nil {
			return cloneRect(parentClip)
		}
		return intersectClip(parentClip, *clip)
	}
	for index := boxStart; index < len(tree.Boxes); index++ {
		tree.Boxes[index].X += x
		tree.Boxes[index].Y += y
		tree.Boxes[index].Baseline += y
		if tree.Boxes[index].Image {
			tree.Boxes[index].ImageRect.X += x
			tree.Boxes[index].ImageRect.Y += y
			tree.Boxes[index].ImageClip.X += x
			tree.Boxes[index].ImageClip.Y += y
		}
		for runIndex := range tree.Boxes[index].Runs {
			tree.Boxes[index].Runs[runIndex].Baseline += y
		}
		tree.Boxes[index].Clip = translateClip(tree.Boxes[index].Clip)
		for clipIndex := range tree.Boxes[index].Clips {
			tree.Boxes[index].Clips[clipIndex].X += x
			tree.Boxes[index].Clips[clipIndex].Y += y
		}
	}
	for index := decorationStart; index < len(tree.Decorations); index++ {
		tree.Decorations[index].X += x
		tree.Decorations[index].Y += y
		tree.Decorations[index].Clip = translateClip(tree.Decorations[index].Clip)
		for clipIndex := range tree.Decorations[index].Clips {
			tree.Decorations[index].Clips[clipIndex].X += x
			tree.Decorations[index].Clips[clipIndex].Y += y
		}
	}
}

func mainLineUsage(line *flexLine, gap float32, items map[*flexItem]*flexLayoutItem) (float32, int) {
	if line == nil {
		return 0, 0
	}
	used := gap * float32(max(len(line.items)-1, 0))
	autoMargins := 0
	for _, algorithm := range line.items {
		item := items[algorithm]
		used += algorithm.target
		if item.mainAutoStart {
			autoMargins++
		} else {
			used += algorithm.marginStart
		}
		if item.mainAutoEnd {
			autoMargins++
		} else {
			used += algorithm.marginEnd
		}
	}
	return used, autoMargins
}

func justifySpacing(alignment stylemodel.JustifyContent, free float32, itemCount int, hasAutoMargins bool) (float32, float32) {
	if free <= 0 || itemCount == 0 || hasAutoMargins {
		return 0, 0
	}
	switch alignment {
	case stylemodel.JustifyFlexEnd:
		return free, 0
	case stylemodel.JustifyCenter:
		return free / 2, 0
	case stylemodel.JustifySpaceBetween:
		if itemCount > 1 {
			return 0, free / float32(itemCount-1)
		}
	case stylemodel.JustifySpaceAround:
		spacing := free / float32(itemCount)
		return spacing / 2, spacing
	case stylemodel.JustifySpaceEvenly:
		spacing := free / float32(itemCount+1)
		return spacing, spacing
	}
	return 0, 0
}

func alignContentSpacing(alignment stylemodel.Align, free float32, lineCount int) (float32, float32) {
	if free <= 0 || lineCount == 0 {
		return 0, 0
	}
	switch alignment {
	case stylemodel.AlignFlexEnd:
		return free, 0
	case stylemodel.AlignCenter:
		return free / 2, 0
	case stylemodel.AlignSpaceBetween:
		if lineCount > 1 {
			return 0, free / float32(lineCount-1)
		}
	case stylemodel.AlignSpaceAround:
		spacing := free / float32(lineCount)
		return spacing / 2, spacing
	case stylemodel.AlignSpaceEvenly:
		spacing := free / float32(lineCount+1)
		return spacing, spacing
	}
	return 0, 0
}

func flipCrossAlignment(alignment stylemodel.Align) stylemodel.Align {
	if alignment == stylemodel.AlignFlexStart {
		return stylemodel.AlignFlexEnd
	}
	if alignment == stylemodel.AlignFlexEnd {
		return stylemodel.AlignFlexStart
	}
	return alignment
}
