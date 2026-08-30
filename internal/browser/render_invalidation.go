package browser

import (
	"reflect"

	"github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/style"
)

// RenderDamage is the least expensive renderer stage affected by a mutation.
type RenderDamage uint8

const (
	RenderDamageNone RenderDamage = iota
	RenderDamageStyle
	RenderDamageComposite
	RenderDamagePaint
	RenderDamageLayout
)

const maxRenderInvalidationNodes = 256

// RenderInvalidation is a payload-free, bounded mutation summary. LayoutRoots
// identify subtree roots rather than expanding every descendant.
type RenderInvalidation struct {
	Revision       uint64
	Damage         RenderDamage
	StyleNodes     []dom.NodeID
	LayoutRoots    []dom.NodeID
	PaintNodes     []dom.NodeID
	CompositeNodes []dom.NodeID
}

// RecordComputedStyleChanges classifies changed nodes without propagating
// dirtiness to unrelated siblings or the entire document.
func (p *Page) RecordComputedStyleChanges(previous, current style.Map) RenderInvalidation {
	if p == nil {
		return RenderInvalidation{}
	}
	result := RenderInvalidation{Revision: p.StyleRevision + 1}
	seen := make(map[dom.NodeID]bool, len(previous)+len(current))
	for nodeID, next := range current {
		seen[nodeID] = true
		old, existed := previous[nodeID]
		appendRenderDamage(&result, nodeID, classifyComputedStyleDamage(old, next, existed))
	}
	for nodeID := range previous {
		if !seen[nodeID] {
			appendRenderDamage(&result, nodeID, RenderDamageLayout)
		}
	}
	p.renderMu.Lock()
	p.renderDirty = cloneRenderInvalidation(result)
	p.renderMu.Unlock()
	return result
}

func classifyComputedStyleDamage(previous, current style.ComputedStyle, existed bool) RenderDamage {
	if !existed {
		return RenderDamageLayout
	}
	if reflect.DeepEqual(previous, current) {
		return RenderDamageNone
	}
	left, right := previous, current
	left.Cursor, right.Cursor = 0, 0
	left.CustomProperties, right.CustomProperties = nil, nil
	if reflect.DeepEqual(left, right) {
		return RenderDamageStyle
	}
	damage := style.ClassifyAnimationDamage(style.Map{1: previous}, style.Map{1: current})
	switch damage {
	case style.AnimationDamageComposite:
		return RenderDamageComposite
	case style.AnimationDamagePaint:
		return RenderDamagePaint
	default:
		return RenderDamageLayout
	}
}

func appendRenderDamage(result *RenderInvalidation, nodeID dom.NodeID, damage RenderDamage) {
	if result == nil || nodeID == 0 || damage == RenderDamageNone || len(result.StyleNodes) >= maxRenderInvalidationNodes {
		return
	}
	result.StyleNodes = append(result.StyleNodes, nodeID)
	if damage > result.Damage {
		result.Damage = damage
	}
	switch damage {
	case RenderDamageLayout:
		result.LayoutRoots = append(result.LayoutRoots, nodeID)
	case RenderDamagePaint:
		result.PaintNodes = append(result.PaintNodes, nodeID)
	case RenderDamageComposite:
		result.CompositeNodes = append(result.CompositeNodes, nodeID)
	}
}

func (p *Page) RenderInvalidationSnapshot() RenderInvalidation {
	if p == nil {
		return RenderInvalidation{}
	}
	p.renderMu.Lock()
	defer p.renderMu.Unlock()
	return cloneRenderInvalidation(p.renderDirty)
}

func cloneRenderInvalidation(source RenderInvalidation) RenderInvalidation {
	source.StyleNodes = append([]dom.NodeID(nil), source.StyleNodes...)
	source.LayoutRoots = append([]dom.NodeID(nil), source.LayoutRoots...)
	source.PaintNodes = append([]dom.NodeID(nil), source.PaintNodes...)
	source.CompositeNodes = append([]dom.NodeID(nil), source.CompositeNodes...)
	return source
}
