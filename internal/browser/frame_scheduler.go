package browser

import (
	"time"

	"github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/layout"
	"github.com/Grove-Computing/Growse/internal/style"
)

// FrameSource is a bit set of work coalesced into one rendered frame.
type FrameSource uint8

const (
	FrameSourceCSSAnimation FrameSource = 1 << iota
	FrameSourceCSSTransition
	FrameSourceAnimationFrame
	FrameSourceAnimatedImage
	FrameSourceScroll
)

type FrameRequest struct {
	AnimationFramePending bool
	RunAnimationFrame     func()
	ScrollPending         bool
}

type FramePlan struct {
	Styles     style.Map
	Damage     style.AnimationDamage
	Sources    FrameSource
	ImageNodes []dom.NodeID
}

// ScheduleFrame samples all frame-producing subsystems at one timestamp and
// returns one coalesced renderer plan.
func (p *Page) ScheduleFrame(current time.Time, tree *layout.Tree, request FrameRequest) FramePlan {
	if p == nil {
		return FramePlan{}
	}
	plan := FramePlan{}
	if request.AnimationFramePending {
		plan.Sources |= FrameSourceAnimationFrame
		if request.RunAnimationFrame != nil {
			request.RunAnimationFrame()
		}
	}
	if p.Animations != nil && p.Animations.Active(current) {
		plan.Sources |= FrameSourceCSSAnimation
	}
	if p.Transitions != nil && p.Transitions.Active(current) {
		plan.Sources |= FrameSourceCSSTransition
	}
	plan.Styles, plan.Damage = p.AnimationFrame(current)
	plan.ImageNodes = p.AdvanceAnimatedImages(current, tree)
	if len(plan.ImageNodes) != 0 {
		plan.Sources |= FrameSourceAnimatedImage
		if plan.Damage < style.AnimationDamagePaint {
			plan.Damage = style.AnimationDamagePaint
		}
	}
	if request.ScrollPending {
		plan.Sources |= FrameSourceScroll
		if plan.Damage < style.AnimationDamageComposite {
			plan.Damage = style.AnimationDamageComposite
		}
	}
	p.recordScheduledFrame(plan.Sources)
	return plan
}

func (p *Page) ActiveAnimatedImagesInViewport(tree *layout.Tree) bool {
	if p == nil {
		return false
	}
	p.imageMu.Lock()
	defer p.imageMu.Unlock()
	for nodeID, player := range p.AnimatedImages {
		if player != nil && !player.closed && player.data != nil && !p.ReducedMotion && animatedImageVisible(p, tree, nodeID) {
			return true
		}
	}
	return false
}

func (p *Page) recordScheduledFrame(sources FrameSource) {
	p.renderMu.Lock()
	p.renderMetrics.ScheduledFrames = saturatingAdd(p.renderMetrics.ScheduledFrames, 1)
	if sources != 0 && sources&(sources-1) != 0 {
		p.renderMetrics.CoalescedFrames = saturatingAdd(p.renderMetrics.CoalescedFrames, 1)
	}
	p.renderMu.Unlock()
}
