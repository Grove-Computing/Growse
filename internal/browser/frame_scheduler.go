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
	AnimationFramePending bool
	RunAnimationFrame     func()
	ScrollPending         bool
	Background            bool
	Tasks                 []FrameTask
}

type FramePlan struct {
	Styles        style.Map
	Damage        style.AnimationDamage
	Sources       FrameSource
	ImageNodes    []dom.NodeID
	ExecutedTasks int
	DroppedTasks  int
	Throttled     bool
}

// ScheduleFrame samples all frame-producing subsystems at one timestamp and
// returns one coalesced renderer plan.
func (p *Page) ScheduleFrame(current time.Time, tree *layout.Tree, request FrameRequest) FramePlan {
	if p == nil {
		return FramePlan{}
	}
	plan := FramePlan{}
	animationCount := 0
	if p.Animations != nil {
		animationCount += len(p.Animations.ActiveNodes(current))
	}
	if p.Transitions != nil {
		animationCount += len(p.Transitions.ActiveNodes(current))
	}
	p.frameScheduleMu.Lock()
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
