package browser

import (
	"image"
	"strings"
	"testing"
	"time"

	animationmodel "github.com/Grove-Computing/Growse/internal/animation"
	"github.com/Grove-Computing/Growse/internal/css"
	"github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/layout"
	"github.com/Grove-Computing/Growse/internal/style"
)

func TestFrameSchedulerCoalescesCSSRAFImageAndScroll(t *testing.T) {
	document := dom.NewDocument()
	animated := document.CreateElement("div", map[string]string{"id": "animated"})
	transitioned := document.CreateElement("div", nil)
	imageNode := document.CreateElement("img", nil)
	for _, node := range []*dom.Node{animated, transitioned, imageNode} {
		if err := document.AppendChild(document.Root, node); err != nil {
			t.Fatal(err)
		}
	}
	stylesheet, err := css.Parse(strings.NewReader(`
@keyframes pulse { from { opacity: .2; } to { opacity: .8; } }
#animated { animation: pulse 1s linear infinite; }
`))
	if err != nil {
		t.Fatal(err)
	}
	computed := style.Compute(document, stylesheet)
	start := time.Unix(100, 0)
	animations := style.NewAnimationRegistry()
	animations.Reconcile(computed, start)
	timing, err := animationmodel.NewTiming(time.Second, 0, animationmodel.Linear{})
	if err != nil {
		t.Fatal(err)
	}
	transitions := style.NewTransitionRegistry()
	previous := computed[transitioned.ID]
	next := previous
	previous.Opacity = 0
	next.Opacity = 1
	next.Transitions = []style.Transition{{Property: "opacity", Timing: timing}}
	computed[transitioned.ID] = next
	transitions.Reconcile(style.Map{transitioned.ID: previous}, style.Map{transitioned.ID: next}, start)
	frames := []image.Image{image.NewNRGBA(image.Rect(0, 0, 1, 1)), image.NewNRGBA(image.Rect(0, 0, 1, 1))}
	page := &Page{
		Document: document, Stylesheet: stylesheet, ComputedStyles: computed, Animations: animations, Transitions: transitions,
		ImageResources: map[dom.NodeID]layout.ImageResource{imageNode.ID: {URL: "animated.gif"}},
		Images:         map[string]image.Image{"animated.gif": frames[0]},
		AnimatedImages: map[dom.NodeID]*animatedImagePlayer{imageNode.ID: {data: &animatedImageData{frames: frames, delays: []time.Duration{100 * time.Millisecond, 100 * time.Millisecond}}, started: start}},
		ViewportWidth:  320, ViewportHeight: 240,
	}
	tree := &layout.Tree{Width: 320, Height: 240, Bounds: map[dom.NodeID]layout.Rect{
		animated.ID: {Width: 20, Height: 20}, transitioned.ID: {Y: 30, Width: 20, Height: 20}, imageNode.ID: {Y: 60, Width: 20, Height: 20},
	}}
	rafCalls := 0
	plan := page.ScheduleFrame(start.Add(150*time.Millisecond), tree, FrameRequest{
		AnimationFramePending: true, RunAnimationFrame: func() { rafCalls++ }, ScrollPending: true,
	})
	wantSources := FrameSourceCSSAnimation | FrameSourceCSSTransition | FrameSourceAnimationFrame | FrameSourceAnimatedImage | FrameSourceScroll
	if plan.Sources != wantSources || plan.Damage != style.AnimationDamagePaint || rafCalls != 1 || len(plan.ImageNodes) != 1 {
		t.Fatalf("frame plan=%#v rafCalls=%d", plan, rafCalls)
	}
	metrics := page.RenderMetricsSnapshot()
	if metrics.ScheduledFrames != 1 || metrics.CoalescedFrames != 1 {
		t.Fatalf("scheduler metrics = %#v", metrics)
	}
}
