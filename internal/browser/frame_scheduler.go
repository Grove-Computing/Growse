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

type FrameTaskPriority uint8

const (
	FrameTaskInput FrameTaskPriority = iota
	FrameTaskChrome
	FrameTaskPage
)

const (
	maxFrameTasks         = 32
	maxFrameTaskBudget    = 8 * time.Millisecond
	backgroundFramePeriod = time.Second
	animationStormPeriod  = time.Second / 30
	maxAnimationsPerFrame = 128
)

type FrameTask struct {
	Priority FrameTaskPriority
	Cost     time.Duration
	Run      func()
}

type FrameRequest struct {
	Generation            uint64
	AnimationFramePending bool
	RunAnimationFrame     func()
	ScrollPending         bool
	Background            bool
	Tasks                 []FrameTask
}

type FramePlan struct {
	Generation    uint64
	Styles        style.Map
	Damage        style.AnimationDamage
	Sources       FrameSource
	ImageNodes    []dom.NodeID
	ExecutedTasks int
	DroppedTasks  int
	Throttled     bool
	Stale         bool
}

// ScheduleFrame samples all frame-producing subsystems at one timestamp and
// returns one coalesced renderer plan.
func (p *Page) ScheduleFrame(current time.Time, tree *layout.Tree, request FrameRequest) FramePlan {
	if p == nil {
		return FramePlan{}
	}
	plan := FramePlan{}
	p.frameScheduleMu.Lock()
	if p.frameGeneration == 0 {
		p.frameGeneration = 1
	}
	plan.Generation = p.frameGeneration
	if p.frameClosed || request.Generation != 0 && request.Generation != p.frameGeneration {
		p.frameScheduleMu.Unlock()
		plan.Styles, plan.Stale = p.ComputedStyles, true
		return plan
	}
	p.frameScheduleMu.Unlock()
	animationCount := 0
	if p.Animations != nil {
		animationCount += len(p.Animations.ActiveNodes(current))
	}
	if p.Transitions != nil {
		animationCount += len(p.Transitions.ActiveNodes(current))
	}
	p.frameScheduleMu.Lock()
	if p.frameClosed || p.frameGeneration != plan.Generation {
		p.frameScheduleMu.Unlock()
		plan.Styles, plan.Stale = p.ComputedStyles, true
		return plan
	}
	backgroundThrottled := request.Background && !p.lastFrame.IsZero() && current.Sub(p.lastFrame) < backgroundFramePeriod
	stormThrottled := animationCount > maxAnimationsPerFrame && !p.lastAnimationFrame.IsZero() && current.Sub(p.lastAnimationFrame) < animationStormPeriod
	if backgroundThrottled || stormThrottled {
		p.frameScheduleMu.Unlock()
		plan.Styles = p.ComputedStyles
		plan.Throttled = true
		p.recordThrottledFrame()
		return plan
	}
	p.lastFrame = current
	if animationCount != 0 {
		p.lastAnimationFrame = current
	}
	p.frameScheduleMu.Unlock()
	plan.ExecutedTasks, plan.DroppedTasks = p.runFrameTasks(request.Tasks)
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

// FrameGeneration returns the token accepted by the current Page lifecycle.
func (p *Page) FrameGeneration() uint64 {
	if p == nil {
		return 0
	}
	p.frameScheduleMu.Lock()
	if p.frameGeneration == 0 {
		p.frameGeneration = 1
	}
	result := p.frameGeneration
	p.frameScheduleMu.Unlock()
	return result
}

func (p *Page) cancelFrameLifecycle() {
	if p == nil {
		return
	}
	p.frameScheduleMu.Lock()
	p.frameGeneration++
	if p.frameGeneration == 0 {
		p.frameGeneration = 1
	}
	p.frameClosed = true
	p.lastFrame = time.Time{}
	p.lastAnimationFrame = time.Time{}
	p.frameScheduleMu.Unlock()
	if p.Animations != nil {
		p.Animations.Clear()
	}
	if p.Transitions != nil {
		p.Transitions.Clear()
	}
}

func (p *Page) runFrameTasks(tasks []FrameTask) (executed, dropped int) {
	remaining := maxFrameTaskBudget
	processed := 0
	counts := [3]int{}
	for _, priority := range []FrameTaskPriority{FrameTaskInput, FrameTaskChrome, FrameTaskPage} {
		for _, task := range tasks {
			if task.Priority != priority {
				continue
			}
			cost := max(task.Cost, time.Duration(0))
			if processed >= maxFrameTasks || cost > remaining {
				dropped++
				continue
			}
			processed++
			remaining -= cost
			if task.Run != nil {
				task.Run()
			}
			executed++
			if priority <= FrameTaskPage {
				counts[priority]++
			}
		}
	}
	p.renderMu.Lock()
	p.renderMetrics.InputTasks = saturatingAdd(p.renderMetrics.InputTasks, counts[FrameTaskInput])
	p.renderMetrics.ChromeTasks = saturatingAdd(p.renderMetrics.ChromeTasks, counts[FrameTaskChrome])
	p.renderMetrics.PageTasks = saturatingAdd(p.renderMetrics.PageTasks, counts[FrameTaskPage])
	p.renderMetrics.DroppedTasks = saturatingAdd(p.renderMetrics.DroppedTasks, dropped)
	p.renderMu.Unlock()
	return executed, dropped
}

func (p *Page) recordThrottledFrame() {
	p.renderMu.Lock()
	p.renderMetrics.ThrottledFrames = saturatingAdd(p.renderMetrics.ThrottledFrames, 1)
	p.renderMu.Unlock()
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
