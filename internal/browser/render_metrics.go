package browser

// RenderEvent identifies one bounded rendering-pipeline counter update.
type RenderEvent uint8

const (
	RenderImagePaintHit RenderEvent = iota
	RenderImagePaintMiss
	RenderImagePaintEviction
	RenderLayoutBuild
	RenderDisplayListBuild
	RenderDisplayListReuse
	RenderCompositeFrame
	RenderPaintFrame
	RenderLayoutFrame
)

// RenderRebuildReason classifies why the cached layout/display list could not
// be reused. Values are deliberately fixed-size so diagnostics never retain a
// DOM node, stylesheet, or animation payload.
type RenderRebuildReason uint8

const (
	RenderRebuildInitial RenderRebuildReason = iota
	RenderRebuildPage
	RenderRebuildStyle
	RenderRebuildViewport
	RenderRebuildScroll
	RenderRebuildAnimation
)

// RenderMetrics is a payload-free snapshot for performance regression tests
// and DevTools. Counters are monotonic within one Page generation.
type RenderMetrics struct {
	ImageResourceHits      uint64
	ImageResourceMisses    uint64
	ImageResourceEvictions uint64
	ImagePaintHits         uint64
	ImagePaintMisses       uint64
	ImagePaintEvictions    uint64
	LayoutBuilds           uint64
	DisplayListBuilds      uint64
	DisplayListReuses      uint64
	LayoutFragmentReuses   uint64
	DisplayCommandReuses   uint64
	CompositeFrames        uint64
	PaintFrames            uint64
	LayoutFrames           uint64
	InitialRebuilds        uint64
	PageRebuilds           uint64
	StyleRebuilds          uint64
	ViewportRebuilds       uint64
	ScrollRebuilds         uint64
	AnimationRebuilds      uint64
	ScheduledFrames        uint64
	CoalescedFrames        uint64
}

// RecordRenderReuse counts stable fragments and commands retained across one
// incremental revision. Values saturate to keep diagnostics monotonic.
func (p *Page) RecordRenderReuse(fragments, commands int) {
	if p == nil {
		return
	}
	p.renderMu.Lock()
	p.renderMetrics.LayoutFragmentReuses = saturatingAdd(p.renderMetrics.LayoutFragmentReuses, fragments)
	p.renderMetrics.DisplayCommandReuses = saturatingAdd(p.renderMetrics.DisplayCommandReuses, commands)
	p.renderMu.Unlock()
}

func saturatingAdd(current uint64, amount int) uint64 {
	if amount <= 0 || current == ^uint64(0) {
		return current
	}
	addition := uint64(amount)
	if addition > ^uint64(0)-current {
		return ^uint64(0)
	}
	return current + addition
}

// RecordRenderEvent records UI-owned work without retaining DOM or resource
// payloads. Overflow saturates instead of wrapping diagnostics.
func (p *Page) RecordRenderEvent(event RenderEvent) {
	if p == nil {
		return
	}
	p.renderMu.Lock()
	target := (*uint64)(nil)
	switch event {
	case RenderImagePaintHit:
		target = &p.renderMetrics.ImagePaintHits
	case RenderImagePaintMiss:
		target = &p.renderMetrics.ImagePaintMisses
	case RenderImagePaintEviction:
		target = &p.renderMetrics.ImagePaintEvictions
	case RenderLayoutBuild:
		target = &p.renderMetrics.LayoutBuilds
	case RenderDisplayListBuild:
		target = &p.renderMetrics.DisplayListBuilds
	case RenderDisplayListReuse:
		target = &p.renderMetrics.DisplayListReuses
	case RenderCompositeFrame:
		target = &p.renderMetrics.CompositeFrames
	case RenderPaintFrame:
		target = &p.renderMetrics.PaintFrames
	case RenderLayoutFrame:
		target = &p.renderMetrics.LayoutFrames
	}
	if target != nil && *target != ^uint64(0) {
		*target++
	}
	p.renderMu.Unlock()
}

// RecordRenderRebuild records one cache miss without retaining its input.
func (p *Page) RecordRenderRebuild(reason RenderRebuildReason) {
	if p == nil {
		return
	}
	p.renderMu.Lock()
	target := (*uint64)(nil)
	switch reason {
	case RenderRebuildInitial:
		target = &p.renderMetrics.InitialRebuilds
	case RenderRebuildPage:
		target = &p.renderMetrics.PageRebuilds
	case RenderRebuildStyle:
		target = &p.renderMetrics.StyleRebuilds
	case RenderRebuildViewport:
		target = &p.renderMetrics.ViewportRebuilds
	case RenderRebuildScroll:
		target = &p.renderMetrics.ScrollRebuilds
	case RenderRebuildAnimation:
		target = &p.renderMetrics.AnimationRebuilds
	}
	if target != nil && *target != ^uint64(0) {
		*target++
	}
	p.renderMu.Unlock()
}

// RenderMetricsSnapshot returns current Page and resource-cache counters.
func (p *Page) RenderMetricsSnapshot() RenderMetrics {
	if p == nil {
		return RenderMetrics{}
	}
	p.renderMu.Lock()
	result := p.renderMetrics
	p.renderMu.Unlock()
	p.imageMu.Lock()
	imageCache := p.imageCache
	p.imageMu.Unlock()
	if imageCache != nil {
		stats := imageCache.statsSnapshot()
		result.ImageResourceHits = stats.hits
		result.ImageResourceMisses = stats.misses
		result.ImageResourceEvictions = stats.evictions
	}
	return result
}
