package layout

import (
	"sort"

	"github.com/Grove-Computing/Growse/internal/dom"
	stylemodel "github.com/Grove-Computing/Growse/internal/style"
)

const (
	maxCompositingLayers  = 128
	maxLayerDamageRegions = 8
)

func buildCompositingLayers(tree *Tree, styles stylemodel.Map) {
	if tree == nil {
		return
	}
	nodeIDs := make([]dom.NodeID, 0, len(styles))
	for nodeID := range styles {
		nodeIDs = append(nodeIDs, nodeID)
	}
	sort.Slice(nodeIDs, func(left, right int) bool { return nodeIDs[left] < nodeIDs[right] })
	layerByNode := make(map[dom.NodeID]int)
	for _, nodeID := range nodeIDs {
		computed := styles[nodeID]
		reasons := compositingReasons(computed)
		bounds, bounded := tree.Bounds[nodeID]
		if reasons == 0 || !bounded || bounds.Width <= 0 || bounds.Height <= 0 {
			continue
		}
		if len(tree.CompositingLayers) >= maxCompositingLayers {
			tree.addFallback(nodeID, "compositing layer limit exceeded")
			break
		}
		parent := -1
		for ancestor := tree.Parents[nodeID]; ancestor != 0; ancestor = tree.Parents[ancestor] {
			if parentLayer, exists := layerByNode[ancestor]; exists {
				parent = parentLayer
				break
			}
		}
		layer := CompositingLayer{
			ID: len(tree.CompositingLayers), Parent: parent, NodeID: nodeID, Bounds: bounds,
			Reasons: reasons, Opacity: computed.Opacity, Transform: animatedTransform(computed, bounds), Damage: []Rect{bounds},
		}
		if reasons&(LayerClip|LayerScroll) != 0 {
			clip := bounds
			layer.Clip = &clip
		}
		layerByNode[nodeID] = layer.ID
		tree.CompositingLayers = append(tree.CompositingLayers, layer)
	}
}

func compositingReasons(computed stylemodel.ComputedStyle) LayerReason {
	var reasons LayerReason
	if len(computed.Transform) != 0 {
		reasons |= LayerTransform
	}
	if computed.Opacity < 1 {
		reasons |= LayerOpacity
	}
	if computed.OverflowX != stylemodel.OverflowVisible || computed.OverflowY != stylemodel.OverflowVisible {
		reasons |= LayerClip
		if computed.OverflowX == stylemodel.OverflowScroll || computed.OverflowY == stylemodel.OverflowScroll || computed.OverflowX == stylemodel.OverflowAuto || computed.OverflowY == stylemodel.OverflowAuto {
			reasons |= LayerScroll
		}
	}
	switch computed.Position {
	case stylemodel.PositionFixed:
		reasons |= LayerFixed
	case stylemodel.PositionSticky:
		reasons |= LayerSticky
	}
	if len(computed.Filters) != 0 || len(computed.BackdropFilters) != 0 {
		reasons |= LayerFilter
	}
	return reasons
}

// UpdateCompositingLayers computes bounded damage for changed composite
// properties without rebuilding layout or the display list.
func UpdateCompositingLayers(tree *Tree, styles stylemodel.Map) []Rect {
	if tree == nil {
		return nil
	}
	regions := make([]Rect, 0, len(tree.CompositingLayers))
	for index := range tree.CompositingLayers {
		layer := &tree.CompositingLayers[index]
		layer.Damage = layer.Damage[:0]
		computed, exists := styles[layer.NodeID]
		bounds, bounded := tree.Bounds[layer.NodeID]
		if !exists || !bounded {
			continue
		}
		transform := animatedTransform(computed, bounds)
		transformed := transformRect(bounds, transform)
		previous := transformRect(layer.Bounds, layer.Transform)
		if layer.Opacity != computed.Opacity || layer.Transform != transform || layer.Bounds != bounds {
			damage := unionRect(previous, transformed)
			layer.Damage = append(layer.Damage, damage)
			if len(regions) < maxCompositingLayers*maxLayerDamageRegions {
				regions = append(regions, damage)
			}
		}
		layer.Bounds, layer.Opacity, layer.Transform = bounds, computed.Opacity, transform
	}
	return regions
}

func transformRect(rect Rect, matrix stylemodel.Matrix) Rect {
	x1, y1 := matrix.TransformPoint(rect.X, rect.Y)
	x2, y2 := matrix.TransformPoint(rect.X+rect.Width, rect.Y)
	x3, y3 := matrix.TransformPoint(rect.X, rect.Y+rect.Height)
	x4, y4 := matrix.TransformPoint(rect.X+rect.Width, rect.Y+rect.Height)
	left, right := min(x1, x2, x3, x4), max(x1, x2, x3, x4)
	top, bottom := min(y1, y2, y3, y4), max(y1, y2, y3, y4)
	return Rect{X: left, Y: top, Width: right - left, Height: bottom - top}
}

func unionRect(left, right Rect) Rect {
	x, y := min(left.X, right.X), min(left.Y, right.Y)
	maximumX := max(left.X+left.Width, right.X+right.Width)
	maximumY := max(left.Y+left.Height, right.Y+right.Height)
	return Rect{X: x, Y: y, Width: maximumX - x, Height: maximumY - y}
}
