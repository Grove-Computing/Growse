package layout

import (
	"sort"

	"github.com/Grove-Computing/Growse/internal/dom"
	stylemodel "github.com/Grove-Computing/Growse/internal/style"
)

const (
	maxMultiColumnCount        = 32
	maxColumnBalanceIterations = 64
)

type columnGeometry struct {
	count int
	width float32
	gap   float32
}

type columnSourceRange struct {
	start float32
	end   float32
}

func usesMultiColumnLayout(style blockStyle) bool {
	return style.columnCount > 0 || style.columnWidth.Kind != stylemodel.SizeAuto
}

func resolveColumnGeometry(style blockStyle, availableWidth float32) columnGeometry {
	availableWidth = max(availableWidth, float32(1))
	gap := style.columnGap.Resolve(availableWidth)
	if style.columnGapNormal {
		gap = max(style.fontSize, float32(0))
	}
	gap = min(max(gap, float32(0)), availableWidth)
	requestedCount := style.columnCount
	requestedWidth, widthSpecified := resolveSize(style.columnWidth, availableWidth, true)
	if !widthSpecified || requestedWidth <= 0 {
		requestedWidth = availableWidth
	}
	count := requestedCount
	switch {
	case count < 1 && widthSpecified:
		count = max(int((availableWidth+gap)/(requestedWidth+gap)), 1)
	case count < 1:
		count = 1
	case widthSpecified:
		fitting := max(int((availableWidth+gap)/(requestedWidth+gap)), 1)
		count = min(count, fitting)
	}
	count = min(max(count, 1), maxMultiColumnCount)
	width := max((availableWidth-gap*float32(count-1))/float32(count), float32(1))
	return columnGeometry{count: count, width: width, gap: gap}
}

func (e *engine) addMultiColumnChildren(node *dom.Node, style blockStyle, contentX, contentWidth, containingHeight float32, heightDefinite bool) []*dom.Node {
	geometry := resolveColumnGeometry(style, contentWidth)
	children := e.flowChildren(node)
	positioned := make([]*dom.Node, 0)
	segment := make([]*dom.Node, 0, len(children))
	flushSegment := func() {
		if len(segment) == 0 {
			return
		}
		e.addColumnSegment(node, style, segment, geometry, contentX, containingHeight, heightDefinite)
		segment = segment[:0]
	}
	for _, child := range children {
		if child != nil && child.Type == dom.NodeElement {
			childStyle := e.styleFor(child)
			if childStyle.display == stylemodel.DisplayNone {
				continue
			}
			if childStyle.layoutPosition == stylemodel.PositionAbsolute || childStyle.layoutPosition == stylemodel.PositionFixed {
				flushSegment()
				positioned = append(positioned, child)
				continue
			}
			if childStyle.columnSpan == stylemodel.ColumnSpanAll && isBlockLevelDisplay(childStyle.display) {
				flushSegment()
				e.renderColumnWrapper(node, style, []*dom.Node{child}, contentX, contentWidth, containingHeight, heightDefinite)
				continue
			}
		}
		segment = append(segment, child)
	}
	flushSegment()
	return positioned
}

func (e *engine) addColumnSegment(node *dom.Node, style blockStyle, children []*dom.Node, geometry columnGeometry, contentX, containingHeight float32, heightDefinite bool) {
	segmentTop := e.y
	boxStart, decorationStart := len(e.tree.Boxes), len(e.tree.Decorations)
	e.renderColumnWrapper(node, style, children, contentX, geometry.width, containingHeight, heightDefinite)
	naturalEnd := e.y
	if naturalEnd <= segmentTop {
		return
	}
	ranges := e.planColumnRanges(children, style, boxStart, segmentTop, naturalEnd, geometry.count, containingHeight, heightDefinite)
	if len(ranges) == 0 {
		ranges = []columnSourceRange{{start: segmentTop, end: naturalEnd}}
	}
	usedHeight := float32(0)
	for _, source := range ranges {
		usedHeight = max(usedHeight, source.end-source.start)
	}
	e.fragmentColumnGeometry(node.ID, boxStart, decorationStart, ranges, geometry, contentX, segmentTop, usedHeight)
	e.addColumnRules(node.ID, style, geometry, contentX, segmentTop, usedHeight, len(ranges))
	e.y = segmentTop + usedHeight
}

func (e *engine) renderColumnWrapper(node *dom.Node, style blockStyle, children []*dom.Node, x, width, containingHeight float32, heightDefinite bool) {
	wrapperStyle := style
	wrapperStyle.display = stylemodel.DisplayFlowRoot
	wrapperStyle.layoutPosition = stylemodel.PositionStatic
	wrapperStyle.margin = stylemodel.Edges{}
	wrapperStyle.marginAuto = stylemodel.AutoEdges{}
	wrapperStyle.padding = stylemodel.Edges{}
	wrapperStyle.border = stylemodel.Borders{}
	wrapperStyle.background = 0
	wrapperStyle.image = stylemodel.BackgroundImage{}
	wrapperStyle.backgroundLayers = nil
	wrapperStyle.boxShadows = nil
	wrapperStyle.outline = stylemodel.BorderSide{}
	wrapperStyle.filters = nil
	wrapperStyle.backdropFilters = nil
	wrapperStyle.mixBlendMode = stylemodel.BlendNormal
	wrapperStyle.transform = nil
	wrapperStyle.opacity = 1
	wrapperStyle.width = stylemodel.SizeValue{Kind: stylemodel.SizeAuto}
	wrapperStyle.height = stylemodel.SizeValue{Kind: stylemodel.SizeAuto}
	wrapperStyle.minWidth = stylemodel.SizeValue{Kind: stylemodel.SizeAuto}
	wrapperStyle.minHeight = stylemodel.SizeValue{Kind: stylemodel.SizeAuto}
	wrapperStyle.maxWidth = stylemodel.SizeValue{Kind: stylemodel.SizeNone}
	wrapperStyle.maxHeight = stylemodel.SizeValue{Kind: stylemodel.SizeNone}
	wrapperStyle.columnCount = 0
	wrapperStyle.columnWidth = stylemodel.SizeValue{Kind: stylemodel.SizeAuto}
	wrapper := &dom.Node{ID: node.ID, Type: dom.NodeElement, TagName: node.TagName, Children: children}
	zero := float32(0)
	e.addBlock(wrapper, wrapperStyle, x, width, containingHeight, heightDefinite, &zero)
}

func (e *engine) planColumnRanges(children []*dom.Node, style blockStyle, boxStart int, top, end float32, requestedCount int, containingHeight float32, heightDefinite bool) []columnSourceRange {
	naturalHeight := end - top
	count := min(max(requestedCount, 1), maxMultiColumnCount)
	targetHeight := naturalHeight / float32(count)
	if style.columnFill == stylemodel.ColumnFillAuto && heightDefinite && containingHeight > 0 {
		targetHeight = containingHeight
		count = min(max(int((naturalHeight+targetHeight-0.0001)/targetHeight), 1), maxMultiColumnCount)
	}
	if targetHeight <= 0 {
		return []columnSourceRange{{start: top, end: end}}
	}

	candidates := e.columnBreakCandidates(boxStart, top, end)
	forced, avoided := e.columnBreakControls(children, top, end)
	boundaries := []float32{top}
	iterations := 0
	for column := 1; column < count && iterations < maxColumnBalanceIterations; column++ {
		ideal := top + targetHeight*float32(column)
		if ideal >= end {
			break
		}
		if boundary, ok := e.closestColumnBreak(candidates, ideal, boundaries[len(boundaries)-1], end, avoided); ok {
			boundaries = append(boundaries, boundary)
		} else {
			boundaries = append(boundaries, ideal)
			e.tree.addFallback(0, "multi-column balance used finite fallback")
		}
		iterations++
	}
	if naturalHeight > targetHeight*float32(maxMultiColumnCount) && style.columnFill == stylemodel.ColumnFillAuto {
		e.tree.addFallback(0, "multi-column count limit exceeded")
	}
	boundaries = append(boundaries, end)
	boundaries = append(boundaries, forced...)
	sort.Slice(boundaries, func(left, right int) bool { return boundaries[left] < boundaries[right] })
	boundaries = uniqueColumnBoundaries(boundaries, top, end)
	if len(boundaries)-1 > maxMultiColumnCount {
		boundaries = append(boundaries[:maxMultiColumnCount], end)
		e.tree.addFallback(0, "multi-column fragmentainer limit exceeded")
	}
	ranges := make([]columnSourceRange, 0, len(boundaries)-1)
	for index := 0; index+1 < len(boundaries); index++ {
		if boundaries[index+1]-boundaries[index] > 0.01 {
			ranges = append(ranges, columnSourceRange{start: boundaries[index], end: boundaries[index+1]})
		}
	}
	return ranges
}

func (e *engine) columnBreakCandidates(boxStart int, top, end float32) []float32 {
	result := []float32{top, end}
	for index := boxStart; index < len(e.tree.Boxes); index++ {
		box := e.tree.Boxes[index]
		if box.Y > top && box.Y < end {
			result = append(result, box.Y)
		}
		bottom := box.Y + box.Height
		if bottom > top && bottom < end {
			result = append(result, bottom)
		}
	}
	sort.Slice(result, func(left, right int) bool { return result[left] < result[right] })
	return uniqueColumnBoundaries(result, top, end)
}

func (e *engine) columnBreakControls(children []*dom.Node, top, end float32) (forced []float32, avoided []columnSourceRange) {
	for _, child := range children {
		if child == nil || child.Type != dom.NodeElement {
			continue
		}
		bounds, exists := e.tree.Bounds[child.ID]
		if !exists {
			continue
		}
		style := e.styleFor(child)
		if style.breakBefore == stylemodel.FragmentBreakColumn && bounds.Y > top && bounds.Y < end {
			forced = append(forced, bounds.Y)
		}
		bottom := bounds.Y + bounds.Height
		if style.breakAfter == stylemodel.FragmentBreakColumn && bottom > top && bottom < end {
			forced = append(forced, bottom)
		}
		if style.breakInside == stylemodel.FragmentBreakAvoid || style.breakInside == stylemodel.FragmentBreakAvoidColumn {
			avoided = append(avoided, columnSourceRange{start: bounds.Y, end: bottom})
		}
	}
	return forced, avoided
}

func (e *engine) closestColumnBreak(candidates []float32, ideal, previous, end float32, avoided []columnSourceRange) (float32, bool) {
	best, bestDistance := float32(0), float32(1<<30)
	for _, candidate := range candidates {
		if candidate <= previous+0.01 || candidate >= end-0.01 || columnBreakAvoided(candidate, avoided) || !e.columnBreakKeepsLines(candidate) {
			continue
		}
		distance := absFloat32(candidate - ideal)
		if distance < bestDistance || distance == bestDistance && candidate > best {
			best, bestDistance = candidate, distance
		}
	}
	return best, bestDistance < float32(1<<30)
}

func columnBreakAvoided(candidate float32, avoided []columnSourceRange) bool {
	for _, interval := range avoided {
		if candidate > interval.start+0.01 && candidate < interval.end-0.01 {
			return true
		}
	}
	return false
}

func (e *engine) columnBreakKeepsLines(candidate float32) bool {
	type counts struct{ before, after int }
	byNode := make(map[dom.NodeID]counts)
	for _, box := range e.tree.Boxes {
		if len(box.Runs) == 0 {
			continue
		}
		current := byNode[box.NodeID]
		if box.Y+box.Height <= candidate+0.01 {
			current.before++
		} else if box.Y >= candidate-0.01 {
			current.after++
		}
		byNode[box.NodeID] = current
	}
	for nodeID, current := range byNode {
		if current.before == 0 || current.after == 0 {
			continue
		}
		computed, ok := e.computed[nodeID]
		if !ok {
			continue
		}
		if current.before < max(computed.Orphans, 1) || current.after < max(computed.Widows, 1) {
			return false
		}
	}
	return true
}

func uniqueColumnBoundaries(values []float32, top, end float32) []float32 {
	result := make([]float32, 0, len(values))
	for _, value := range values {
		value = min(max(value, top), end)
		if len(result) == 0 || absFloat32(result[len(result)-1]-value) > 0.01 {
			result = append(result, value)
		}
	}
	return result
}

func (e *engine) fragmentColumnGeometry(owner dom.NodeID, boxStart, decorationStart int, ranges []columnSourceRange, geometry columnGeometry, contentX, visualTop, usedHeight float32) {
	originalBoxes := append([]Box(nil), e.tree.Boxes[boxStart:]...)
	originalDecorations := append([]Decoration(nil), e.tree.Decorations[decorationStart:]...)
	e.tree.Boxes = e.tree.Boxes[:boxStart]
	e.tree.Decorations = e.tree.Decorations[:decorationStart]
	movedNodes := make(map[dom.NodeID]struct{}, len(originalBoxes)+len(originalDecorations))
	for _, box := range originalBoxes {
		movedNodes[box.NodeID] = struct{}{}
	}
	for _, decoration := range originalDecorations {
		movedNodes[decoration.NodeID] = struct{}{}
	}
	fragmentBounds := make(map[dom.NodeID]Rect)
	for _, box := range originalBoxes {
		for column, source := range ranges {
			if !verticalRangesIntersect(box.Y, box.Y+box.Height, source.start, source.end) {
				continue
			}
			copy := cloneColumnBox(box)
			dx := float32(column) * (geometry.width + geometry.gap)
			dy := visualTop - source.start
			translateColumnBox(&copy, dx, dy, movedNodes)
			clipColumnBox(&copy, ClipRegion{Rect: Rect{X: contentX + dx, Y: visualTop, Width: geometry.width, Height: usedHeight}, NodeID: owner})
			e.tree.Boxes = append(e.tree.Boxes, copy)
			fragmentBounds[copy.NodeID] = unionTableRect(fragmentBounds[copy.NodeID], clippedVisualRect(copy.Rect(), copy.Clip))
		}
	}
	for _, decoration := range originalDecorations {
		for column, source := range ranges {
			if !verticalRangesIntersect(decoration.Y, decoration.Y+decoration.Height, source.start, source.end) {
				continue
			}
			copy := cloneColumnDecoration(decoration)
			dx := float32(column) * (geometry.width + geometry.gap)
			dy := visualTop - source.start
			translateColumnDecoration(&copy, dx, dy, movedNodes)
			clipColumnDecoration(&copy, ClipRegion{Rect: Rect{X: contentX + dx, Y: visualTop, Width: geometry.width, Height: usedHeight}, NodeID: owner})
			e.tree.Decorations = append(e.tree.Decorations, copy)
			fragmentBounds[copy.NodeID] = unionTableRect(fragmentBounds[copy.NodeID], clippedVisualRect(copy.Rect, copy.Clip))
		}
	}
	for nodeID, bounds := range fragmentBounds {
		if nodeID != owner {
			e.tree.Bounds[nodeID] = bounds
		}
	}
}

func verticalRangesIntersect(firstStart, firstEnd, secondStart, secondEnd float32) bool {
	return firstEnd > secondStart+0.01 && firstStart < secondEnd-0.01
}

func cloneColumnBox(box Box) Box {
	box.Runs = append([]TextRun(nil), box.Runs...)
	box.Clips = cloneClipRegions(box.Clips)
	box.Clip = cloneRect(box.Clip)
	return box
}

func cloneColumnDecoration(decoration Decoration) Decoration {
	decoration.Clips = cloneClipRegions(decoration.Clips)
	decoration.Clip = cloneRect(decoration.Clip)
	decoration.Layers = cloneBackgroundLayers(decoration.Layers)
	decoration.BoxShadows = append([]stylemodel.Shadow(nil), decoration.BoxShadows...)
	return decoration
}

func translateColumnBox(box *Box, dx, dy float32, movedNodes map[dom.NodeID]struct{}) {
	box.X += dx
	box.Y += dy
	box.Baseline += dy
	box.Transform = translatedTransform(box.Transform, dx, dy)
	if box.Image {
		box.ImageRect.X += dx
		box.ImageRect.Y += dy
		box.ImageClip.X += dx
		box.ImageClip.Y += dy
	}
	for index := range box.Runs {
		box.Runs[index].Baseline += dy
	}
	translateOwnedColumnClips(box.Clips, dx, dy, movedNodes)
}

func translateColumnDecoration(decoration *Decoration, dx, dy float32, movedNodes map[dom.NodeID]struct{}) {
	decoration.X += dx
	decoration.Y += dy
	decoration.Transform = translatedTransform(decoration.Transform, dx, dy)
	translateOwnedColumnClips(decoration.Clips, dx, dy, movedNodes)
}

func translateOwnedColumnClips(clips []ClipRegion, dx, dy float32, movedNodes map[dom.NodeID]struct{}) {
	for index := range clips {
		if _, moves := movedNodes[clips[index].NodeID]; clips[index].NodeID == 0 || moves {
			clips[index].X += dx
			clips[index].Y += dy
		}
	}
}

func clipColumnBox(box *Box, column ClipRegion) {
	box.Clips = append(box.Clips, column)
	box.Clip = intersectClipRegions(box.Clips)
}

func clipColumnDecoration(decoration *Decoration, column ClipRegion) {
	decoration.Clips = append(decoration.Clips, column)
	decoration.Clip = intersectClipRegions(decoration.Clips)
}

func clippedVisualRect(rect Rect, clip *Rect) Rect {
	if clip == nil {
		return rect
	}
	intersection := intersectClip(&rect, *clip)
	if intersection == nil {
		return Rect{}
	}
	return *intersection
}

func (e *engine) addColumnRules(nodeID dom.NodeID, style blockStyle, geometry columnGeometry, x, y, height float32, actualColumns int) {
	if style.columnRule.Style == stylemodel.BorderNone || style.columnRule.Width <= 0 || actualColumns < 2 || height <= 0 {
		return
	}
	for column := 1; column < actualColumns; column++ {
		center := x + float32(column)*geometry.width + float32(column-1)*geometry.gap + geometry.gap/2
		width := min(style.columnRule.Width, geometry.gap)
		e.tree.Decorations = append(e.tree.Decorations, Decoration{
			Order: e.nextOrder(), StackingID: e.stackingID, NodeID: nodeID,
			Rect: Rect{X: center - width/2, Y: y, Width: width, Height: height}, Background: style.columnRule.Color,
			Opacity: e.opacity, Clip: cloneRect(e.clip), Clips: cloneClipRegions(e.clips), Cursor: style.cursor,
			Transform: stylemodel.IdentityMatrix(), Hidden: style.hidden,
		})
	}
}
