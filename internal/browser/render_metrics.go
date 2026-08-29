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
	CompositeFrames        uint64
	PaintFrames            uint64
	LayoutFrames           uint64
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
