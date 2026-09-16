package layout

import (
	"math"
	"sort"

	"github.com/Grove-Computing/Growse/internal/dom"
	stylemodel "github.com/Grove-Computing/Growse/internal/style"
)

func initializeStickyConstraints(tree *Tree, styles stylemodel.Map) {
	if tree == nil {
		return
	}
	tree.StickyConstraints = make(map[dom.NodeID]StickyConstraint)
	if tree.ScrollOffsets == nil {
		tree.ScrollOffsets = make(map[dom.NodeID]ScrollOffset)
	}
	for nodeID, computed := range styles {
		if computed.Position != stylemodel.PositionSticky {
			continue
		}
		normal, exists := tree.Bounds[nodeID]
		if !exists {
			continue
		}
		containingBlock := Rect{Width: tree.Width, Height: max(tree.Height, tree.ViewportHeight)}
		containingBlockID := dom.NodeID(0)
		for ancestor := tree.Parents[nodeID]; ancestor != 0; ancestor = tree.Parents[ancestor] {
			if bounds, bounded := tree.Bounds[ancestor]; bounded {
				containingBlock = paddingBoxForNode(tree, ancestor, bounds)
				containingBlockID = ancestor
				break
			}
		}
		tree.StickyConstraints[nodeID] = StickyConstraint{
			Normal: normal, ContainingBlock: containingBlock, ContainingBlockID: containingBlockID,
			XScrollContainer: nearestScrollContainer(tree, styles, nodeID, true),
			YScrollContainer: nearestScrollContainer(tree, styles, nodeID, false),
			Inset:            computed.Inset,
		}
	}
}

func nearestScrollContainer(tree *Tree, styles stylemodel.Map, nodeID dom.NodeID, horizontal bool) dom.NodeID {
	for ancestor := tree.Parents[nodeID]; ancestor != 0; ancestor = tree.Parents[ancestor] {
		computed, exists := styles[ancestor]
		if !exists {
			continue
		}
		overflow := computed.OverflowY
		if horizontal {
			overflow = computed.OverflowX
		}
		if isScrollContainerOverflow(overflow) {
			return ancestor
		}
	}
	return 0
}

func isScrollContainerOverflow(value stylemodel.Overflow) bool {
	return value == stylemodel.OverflowHidden || value == stylemodel.OverflowAuto || value == stylemodel.OverflowScroll
}

func initializeScrollContainers(tree *Tree, styles stylemodel.Map) {
	if tree == nil {
		return
	}
	tree.ScrollContainers = make(map[dom.NodeID]ScrollContainer)
	for nodeID, computed := range styles {
		if !isScrollContainerOverflow(computed.OverflowX) && !isScrollContainerOverflow(computed.OverflowY) {
			continue
		}
		bounds, bounded := tree.Bounds[nodeID]
		if !bounded {
			continue
		}
		viewport := paddingBoxForNode(tree, nodeID, bounds)
		maximumX, maximumY := viewport.X+viewport.Width, viewport.Y+viewport.Height
		for descendantID, descendantBounds := range tree.Bounds {
			if descendantID == nodeID || !isDescendantOf(tree, descendantID, nodeID) {
				continue
			}
			maximumX = max(maximumX, descendantBounds.X+descendantBounds.Width)
			maximumY = max(maximumY, descendantBounds.Y+descendantBounds.Height)
		}
		scrollWidth := max(viewport.Width, maximumX-viewport.X)
		scrollHeight := max(viewport.Height, maximumY-viewport.Y)
		if !isScrollContainerOverflow(computed.OverflowX) {
			scrollWidth = viewport.Width
		}
		if !isScrollContainerOverflow(computed.OverflowY) {
			scrollHeight = viewport.Height
		}
		tree.ScrollContainers[nodeID] = ScrollContainer{
			NodeID: nodeID, Viewport: viewport, ScrollWidth: scrollWidth, ScrollHeight: scrollHeight,
			OverflowX: computed.OverflowX, OverflowY: computed.OverflowY,
		}
	}
}

func isDescendantOf(tree *Tree, nodeID, ancestorID dom.NodeID) bool {
	for current := tree.Parents[nodeID]; current != 0; current = tree.Parents[current] {
		if current == ancestorID {
			return true
		}
	}
	return false
}

func paddingBoxForNode(tree *Tree, nodeID dom.NodeID, bounds Rect) Rect {
	for _, decoration := range tree.Decorations {
		if decoration.NodeID != nodeID {
			continue
		}
		return Rect{
			X:      bounds.X + decoration.Border.Left.Width,
			Y:      bounds.Y + decoration.Border.Top.Width,
			Width:  max(bounds.Width-decoration.Border.Left.Width-decoration.Border.Right.Width, float32(0)),
			Height: max(bounds.Height-decoration.Border.Top.Width-decoration.Border.Bottom.Width, float32(0)),
		}
	}
	return bounds
}

func applyInitialStickyOffsets(tree *Tree, _ stylemodel.Map) {
	for _, nodeID := range stickyNodeIDs(tree) {
		repositionSticky(tree, nodeID)
	}
}

// ApplyScrollOffset repositions viewport-attached and viewport-sticky subtrees.
// Normal-flow fragments remain in document coordinates; the UI scrolls them by
// list offset while fixed and sticky geometry is updated for paint/hit sharing.
func ApplyScrollOffset(tree *Tree, styles stylemodel.Map, scrollX, scrollY float32) []dom.NodeID {
	if tree == nil || tree.ScrollX == scrollX && tree.ScrollY == scrollY {
		return nil
	}
	if tree.StickyConstraints == nil {
		initializeStickyConstraints(tree, styles)
	}
	dirtySet := make(map[dom.NodeID]struct{})
	for _, nodeID := range positionedNodeIDs(styles, stylemodel.PositionFixed) {
		if _, bounded := tree.Bounds[nodeID]; !bounded || transformedContainingBlock(tree, styles, nodeID) != 0 || positionedAncestor(tree, styles, nodeID, stylemodel.PositionFixed) != 0 {
			continue
		}
		translateNodeSubtree(tree, nodeID, scrollX-tree.ScrollX, scrollY-tree.ScrollY)
		dirtySet[nodeID] = struct{}{}
	}
	tree.ScrollX, tree.ScrollY = scrollX, scrollY
	for _, nodeID := range stickyNodeIDs(tree) {
		constraint := tree.StickyConstraints[nodeID]
		if constraint.XScrollContainer != 0 && constraint.YScrollContainer != 0 {
			continue
		}
		if repositionSticky(tree, nodeID) {
			dirtySet[nodeID] = struct{}{}
		}
	}
	UpdateCompositingLayers(tree, styles)
	return sortedNodeSet(dirtySet)
}

// ApplyScrollContainerOffset moves a nested scroll container's contents and
// recomputes every sticky box attached to either of its physical axes.
func ApplyScrollContainerOffset(tree *Tree, styles stylemodel.Map, containerID dom.NodeID, scrollX, scrollY float32) []dom.NodeID {
	if tree == nil || containerID == 0 {
		return nil
	}
	if tree.ScrollOffsets == nil {
		tree.ScrollOffsets = make(map[dom.NodeID]ScrollOffset)
	}
	if tree.StickyConstraints == nil {
		initializeStickyConstraints(tree, styles)
	}
	if tree.ScrollContainers == nil {
		initializeScrollContainers(tree, styles)
	}
	container, exists := tree.ScrollContainers[containerID]
	if !exists {
		return nil
	}
	if isScrollContainerOverflow(container.OverflowX) {
		scrollX = min(max(scrollX, float32(0)), max(container.ScrollWidth-container.Viewport.Width, float32(0)))
	} else {
		scrollX = 0
	}
	if isScrollContainerOverflow(container.OverflowY) {
		scrollY = min(max(scrollY, float32(0)), max(container.ScrollHeight-container.Viewport.Height, float32(0)))
	} else {
		scrollY = 0
	}
	old := tree.ScrollOffsets[containerID]
	if old.X == scrollX && old.Y == scrollY {
		return nil
	}
	dirtySet := translateScrollDescendants(tree, styles, containerID, old.X-scrollX, old.Y-scrollY)
	tree.ScrollOffsets[containerID] = ScrollOffset{X: scrollX, Y: scrollY}
	container.Offset = tree.ScrollOffsets[containerID]
	tree.ScrollContainers[containerID] = container
	for _, nodeID := range stickyNodeIDs(tree) {
		constraint := tree.StickyConstraints[nodeID]
		if constraint.XScrollContainer != containerID && constraint.YScrollContainer != containerID {
			continue
		}
		if repositionSticky(tree, nodeID) {
			dirtySet[nodeID] = struct{}{}
		}
	}
	UpdateCompositingLayers(tree, styles)
	return sortedNodeSet(dirtySet)
}

func repositionSticky(tree *Tree, nodeID dom.NodeID) bool {
	constraint, exists := tree.StickyConstraints[nodeID]
	current, bounded := tree.Bounds[nodeID]
	if !exists || !bounded {
		return false
	}
	flowX, flowY := current.X-constraint.AppliedX, current.Y-constraint.AppliedY
	xScrollport := stickyScrollport(tree, constraint.XScrollContainer)
	yScrollport := stickyScrollport(tree, constraint.YScrollContainer)
	containingBlock := constraint.ContainingBlock
	if constraint.ContainingBlockID != 0 {
		if bounds, ok := tree.Bounds[constraint.ContainingBlockID]; ok {
			containingBlock = paddingBoxForNode(tree, constraint.ContainingBlockID, bounds)
		}
	}
	targetX := stickyAxisPosition(flowX, current.Width, xScrollport.X, xScrollport.Width, containingBlock.X, containingBlock.Width, constraint.Inset.Left, constraint.Inset.Right)
	targetY := stickyAxisPosition(flowY, current.Height, yScrollport.Y, yScrollport.Height, containingBlock.Y, containingBlock.Height, constraint.Inset.Top, constraint.Inset.Bottom)
	dx, dy := targetX-current.X, targetY-current.Y
	if dx != 0 || dy != 0 {
		translateNodeSubtree(tree, nodeID, dx, dy)
	}
	constraint.AppliedX = targetX - flowX
	constraint.AppliedY = targetY - flowY
	tree.StickyConstraints[nodeID] = constraint
	return dx != 0 || dy != 0
}

func stickyScrollport(tree *Tree, containerID dom.NodeID) Rect {
	if containerID == 0 {
		return Rect{X: tree.ScrollX, Y: tree.ScrollY, Width: tree.Width, Height: tree.ViewportHeight}
	}
	if bounds, ok := tree.Bounds[containerID]; ok {
		return paddingBoxForNode(tree, containerID, bounds)
	}
	return Rect{}
}

func stickyAxisPosition(flowStart, size, scrollStart, scrollSize, containerStart, containerSize float32, startInset, endInset stylemodel.SizeValue) float32 {
	start, hasStart := resolveSize(startInset, scrollSize, scrollSize > 0)
	end, hasEnd := resolveSize(endInset, scrollSize, scrollSize > 0)
	if !hasStart && !hasEnd {
		return flowStart
	}
	lower, upper := float32(math.Inf(-1)), float32(math.Inf(1))
	if hasStart {
		lower = scrollStart + start
	}
	if hasEnd {
		upper = scrollStart + scrollSize - end - size
	}
	target := flowStart
	if lower <= upper {
		target = min(max(target, lower), upper)
	} else if hasStart {
		// Oversized sticky boxes keep the start inset deterministic; the end
		// inset becomes effectively negative as required by the sticky view rect.
		target = max(target, lower)
	} else {
		target = min(target, upper)
	}
	containerEnd := containerStart + containerSize - size
	if containerEnd < containerStart {
		containerEnd = containerStart
	}
	return min(max(target, containerStart), containerEnd)
}

func stickyNodeIDs(tree *Tree) []dom.NodeID {
	ids := make([]dom.NodeID, 0, len(tree.StickyConstraints))
	for nodeID := range tree.StickyConstraints {
		ids = append(ids, nodeID)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func positionedNodeIDs(styles stylemodel.Map, position stylemodel.Position) []dom.NodeID {
	var ids []dom.NodeID
	for nodeID, computed := range styles {
		if computed.Position == position {
			ids = append(ids, nodeID)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func transformedContainingBlock(tree *Tree, styles stylemodel.Map, nodeID dom.NodeID) dom.NodeID {
	for ancestor := tree.Parents[nodeID]; ancestor != 0; ancestor = tree.Parents[ancestor] {
		if computed, ok := styles[ancestor]; ok && len(computed.Transform) != 0 {
			return ancestor
		}
	}
	return 0
}

func positionedAncestor(tree *Tree, styles stylemodel.Map, nodeID dom.NodeID, position stylemodel.Position) dom.NodeID {
	for ancestor := tree.Parents[nodeID]; ancestor != 0; ancestor = tree.Parents[ancestor] {
		if computed, ok := styles[ancestor]; ok && computed.Position == position {
			return ancestor
		}
	}
	return 0
}

func translateScrollDescendants(tree *Tree, styles stylemodel.Map, containerID dom.NodeID, dx, dy float32) map[dom.NodeID]struct{} {
	dirty := make(map[dom.NodeID]struct{})
	belongs := func(nodeID dom.NodeID) bool {
		for current := nodeID; current != 0 && current != containerID; current = tree.Parents[current] {
			if computed, ok := styles[current]; ok && computed.Position == stylemodel.PositionFixed {
				return false
			}
			if tree.Parents[current] == containerID {
				return true
			}
		}
		return false
	}
	for nodeID, bounds := range tree.Bounds {
		if belongs(nodeID) {
			bounds.X, bounds.Y = bounds.X+dx, bounds.Y+dy
			tree.Bounds[nodeID] = bounds
			dirty[nodeID] = struct{}{}
		}
	}
	for index := range tree.Boxes {
		if belongs(tree.Boxes[index].NodeID) {
			translateBoxForScroll(tree, &tree.Boxes[index], containerID, dx, dy)
		}
	}
	for index := range tree.Decorations {
		if belongs(tree.Decorations[index].NodeID) {
			translateDecorationForScroll(tree, &tree.Decorations[index], containerID, dx, dy)
		}
	}
	for nodeID, container := range tree.ScrollContainers {
		if nodeID != containerID && belongs(nodeID) {
			container.Viewport.X += dx
			container.Viewport.Y += dy
			tree.ScrollContainers[nodeID] = container
		}
	}
	return dirty
}

func translateBoxForScroll(tree *Tree, box *Box, containerID dom.NodeID, dx, dy float32) {
	box.X, box.Y, box.Baseline = box.X+dx, box.Y+dy, box.Baseline+dy
	box.ImageRect.X, box.ImageRect.Y = box.ImageRect.X+dx, box.ImageRect.Y+dy
	box.ImageClip.X, box.ImageClip.Y = box.ImageClip.X+dx, box.ImageClip.Y+dy
	box.Transform = translatedTransform(box.Transform, dx, dy)
	translateOwnedClipRegions(tree, box.Clips, containerID, dx, dy)
	if len(box.Clips) != 0 {
		box.Clip = intersectClipRegions(box.Clips)
	}
	for index := range box.Runs {
		box.Runs[index].Baseline += dy
	}
}

func translateDecorationForScroll(tree *Tree, decoration *Decoration, containerID dom.NodeID, dx, dy float32) {
	decoration.X, decoration.Y = decoration.X+dx, decoration.Y+dy
	decoration.Transform = translatedTransform(decoration.Transform, dx, dy)
	translateOwnedClipRegions(tree, decoration.Clips, containerID, dx, dy)
	if len(decoration.Clips) != 0 {
		decoration.Clip = intersectClipRegions(decoration.Clips)
	}
}

func translateOwnedClipRegions(tree *Tree, clips []ClipRegion, containerID dom.NodeID, dx, dy float32) {
	for index := range clips {
		owner := clips[index].NodeID
		if owner != 0 && owner != containerID && isDescendantOf(tree, owner, containerID) {
			clips[index].X += dx
			clips[index].Y += dy
		}
	}
}

func intersectClipRegions(clips []ClipRegion) *Rect {
	var result *Rect
	for _, region := range clips {
		result = intersectClip(result, region.Rect)
	}
	return result
}

func translatedTransform(matrix stylemodel.Matrix, dx, dy float32) stylemodel.Matrix {
	if matrix == (stylemodel.Matrix{}) {
		return matrix
	}
	matrix.E += dx - matrix.A*dx - matrix.C*dy
	matrix.F += dy - matrix.B*dx - matrix.D*dy
	return matrix
}

func sortedNodeSet(values map[dom.NodeID]struct{}) []dom.NodeID {
	result := make([]dom.NodeID, 0, len(values))
	for nodeID := range values {
		result = append(result, nodeID)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func translateNodeSubtree(tree *Tree, root dom.NodeID, dx, dy float32) {
	belongs := func(nodeID dom.NodeID) bool {
		for current := nodeID; current != 0; current = tree.Parents[current] {
			if current == root {
				return true
			}
		}
		return false
	}
	for nodeID, bounds := range tree.Bounds {
		if belongs(nodeID) {
			bounds.X, bounds.Y = bounds.X+dx, bounds.Y+dy
			tree.Bounds[nodeID] = bounds
		}
	}
	for index := range tree.Boxes {
		if belongs(tree.Boxes[index].NodeID) {
			translateBox(tree, &tree.Boxes[index], root, dx, dy)
		}
	}
	for index := range tree.Decorations {
		if belongs(tree.Decorations[index].NodeID) {
			translateDecoration(tree, &tree.Decorations[index], root, dx, dy)
		}
	}
}

func translateBox(tree *Tree, box *Box, root dom.NodeID, dx, dy float32) {
	box.X, box.Y, box.Baseline = box.X+dx, box.Y+dy, box.Baseline+dy
	box.ImageRect.X, box.ImageRect.Y = box.ImageRect.X+dx, box.ImageRect.Y+dy
	box.ImageClip.X, box.ImageClip.Y = box.ImageClip.X+dx, box.ImageClip.Y+dy
	box.Transform = translatedTransform(box.Transform, dx, dy)
	if len(box.Clips) == 0 && box.Clip != nil {
		box.Clip.X, box.Clip.Y = box.Clip.X+dx, box.Clip.Y+dy
	}
	translateSubtreeClipRegions(tree, box.Clips, root, dx, dy)
	if len(box.Clips) != 0 {
		box.Clip = intersectClipRegions(box.Clips)
	}
	for index := range box.Runs {
		box.Runs[index].Baseline += dy
	}
}

func translateDecoration(tree *Tree, decoration *Decoration, root dom.NodeID, dx, dy float32) {
	decoration.X, decoration.Y = decoration.X+dx, decoration.Y+dy
	decoration.Transform = translatedTransform(decoration.Transform, dx, dy)
	if len(decoration.Clips) == 0 && decoration.Clip != nil {
		decoration.Clip.X, decoration.Clip.Y = decoration.Clip.X+dx, decoration.Clip.Y+dy
	}
	translateSubtreeClipRegions(tree, decoration.Clips, root, dx, dy)
	if len(decoration.Clips) != 0 {
		decoration.Clip = intersectClipRegions(decoration.Clips)
	}
}

func translateSubtreeClipRegions(tree *Tree, clips []ClipRegion, root dom.NodeID, dx, dy float32) {
	for index := range clips {
		owner := clips[index].NodeID
		if owner == 0 || owner == root || isDescendantOf(tree, owner, root) {
			clips[index].X += dx
			clips[index].Y += dy
		}
	}
}
