package style

import (
	"math"
	"strings"
	"testing"
	"time"

	animationmodel "github.com/Grove-Computing/Growse/internal/animation"
	"github.com/Grove-Computing/Growse/internal/css"
	"github.com/Grove-Computing/Growse/internal/dom"
)

func TestComputedAnimationLonghandsAndListMatching(t *testing.T) {
	computed := animationTestStyle(t, `
animation-name: pulse, slide;
animation-duration: 200ms, 1s;
animation-timing-function: linear;
animation-delay: -50ms;
animation-iteration-count: 2, infinite;
animation-direction: alternate;
animation-fill-mode: both;
animation-play-state: running, paused;
`)
	if len(computed.Animations) != 2 {
		t.Fatalf("animations = %d, want 2", len(computed.Animations))
	}
	first, second := computed.Animations[0], computed.Animations[1]
	if first.Name != "pulse" || first.Timing.Duration != 200*time.Millisecond || first.Timing.Delay != -50*time.Millisecond || first.Iterations != 2 || first.Direction != AnimationAlternate || first.FillMode != AnimationFillBoth || first.PlayState != AnimationRunning {
		t.Fatalf("first animation = %#v", first)
	}
	if second.Name != "slide" || second.Timing.Duration != time.Second || !math.IsInf(second.Iterations, 1) || second.PlayState != AnimationPaused {
		t.Fatalf("second animation = %#v", second)
	}
	if _, ok := first.Timing.Easing.(animationmodel.Linear); !ok {
		t.Fatalf("easing = %T, want Linear", first.Timing.Easing)
	}
}

func TestComputedAnimationShorthandKeywordPrecedence(t *testing.T) {
	for _, value := range []string{"3s none backwards", "1s ease-in 2 reverse both paused pulse"} {
		if _, valid := parseAnimationShorthand(value); !valid {
			t.Fatalf("parseAnimationShorthand(%q) is invalid", value)
		}
	}
	computed := animationTestStyle(t, `animation: 3s none backwards, 1s ease-in 2 reverse both paused pulse`)
	if len(computed.Animations) != 2 {
		t.Fatalf("animations = %d, want 2", len(computed.Animations))
	}
	first := computed.Animations[0]
	if first.Name != "backwards" || first.FillMode != AnimationFillNone || first.Timing.Duration != 3*time.Second {
		t.Fatalf("keyword precedence animation = %#v", first)
	}
	second := computed.Animations[1]
	if second.Name != "pulse" || second.Iterations != 2 || second.Direction != AnimationReverse || second.FillMode != AnimationFillBoth || second.PlayState != AnimationPaused {
		t.Fatalf("full shorthand animation = %#v", second)
	}
}

func TestComputedAnimationRejectsNegativeDuration(t *testing.T) {
	computed := animationTestStyle(t, `animation-name: pulse; animation-duration: -1s`)
	if computed.Animations[0].Timing.Duration != 0 {
		t.Fatalf("negative duration = %v, want initial zero", computed.Animations[0].Timing.Duration)
	}
}

func TestCSSAnimationAppliesDelayIterationsDirectionAndFill(t *testing.T) {
	start := time.Unix(100, 0)
	timing, _ := animationmodel.NewTiming(time.Second, 200*time.Millisecond, animationmodel.Linear{})
	item := CSSAnimation{
		Name: "pulse", Timing: timing, Iterations: 2,
		Direction: AnimationAlternate, FillMode: AnimationFillBoth,
	}

	before := item.Sample(start, start.Add(100*time.Millisecond))
	if before.Phase != animationmodel.PhaseBefore || !before.Applies || before.Progress != 0 {
		t.Fatalf("before sample = %#v", before)
	}
	first := item.Sample(start, start.Add(700*time.Millisecond))
	if first.Phase != animationmodel.PhaseActive || first.Iteration != 0 || first.Progress != 0.5 {
		t.Fatalf("first iteration sample = %#v", first)
	}
	second := item.Sample(start, start.Add(1700*time.Millisecond))
	if second.Iteration != 1 || second.Progress != 0.5 {
		t.Fatalf("alternate iteration sample = %#v", second)
	}
	after := item.Sample(start, start.Add(2200*time.Millisecond))
	if after.Phase != animationmodel.PhaseAfter || !after.Applies || after.Iteration != 1 || after.Progress != 0 {
		t.Fatalf("forwards fill sample = %#v", after)
	}
}

func TestCSSAnimationNegativeDelayAndNoFill(t *testing.T) {
	start := time.Unix(100, 0)
	timing, _ := animationmodel.NewTiming(time.Second, -1500*time.Millisecond, animationmodel.Linear{})
	item := CSSAnimation{Name: "slide", Timing: timing, Iterations: 2, Direction: AnimationNormal, FillMode: AnimationFillNone}

	active := item.Sample(start, start)
	if active.Phase != animationmodel.PhaseActive || active.Iteration != 1 || active.Progress != 0.5 || !active.Applies {
		t.Fatalf("negative-delay sample = %#v", active)
	}
	after := item.Sample(start, start.Add(500*time.Millisecond))
	if after.Phase != animationmodel.PhaseAfter || after.Applies {
		t.Fatalf("no-fill after sample = %#v", after)
	}
}

func TestCSSAnimationReverseFractionalFinalProgress(t *testing.T) {
	start := time.Unix(100, 0)
	timing, _ := animationmodel.NewTiming(time.Second, 0, animationmodel.Linear{})
	item := CSSAnimation{Name: "reverse", Timing: timing, Iterations: 1.25, Direction: AnimationReverse, FillMode: AnimationFillForwards}
	after := item.Sample(start, start.Add(2*time.Second))
	if !after.Applies || after.Iteration != 1 || after.Progress != 0.75 {
		t.Fatalf("fractional reverse final sample = %#v", after)
	}
}

func TestRunningAnimationPausesAndResumesFromHeldProgress(t *testing.T) {
	start := time.Unix(100, 0)
	timing, _ := animationmodel.NewTiming(time.Second, 0, animationmodel.Linear{})
	running := NewRunningAnimation(CSSAnimation{
		Name: "pulse", Timing: timing, Iterations: 1, FillMode: AnimationFillForwards, PlayState: AnimationRunning,
	}, start)

	running.Pause(start.Add(250 * time.Millisecond))
	running.Pause(start.Add(400 * time.Millisecond))
	if !running.Paused() {
		t.Fatal("animation is not paused")
	}
	if got := running.Sample(start.Add(750 * time.Millisecond)).Progress; got != 0.25 {
		t.Fatalf("progress while paused = %v, want 0.25", got)
	}

	running.Resume(start.Add(750 * time.Millisecond))
	running.Resume(start.Add(800 * time.Millisecond))
	if running.Paused() {
		t.Fatal("animation remains paused")
	}
	if got := running.Sample(start.Add(time.Second)).Progress; got != 0.5 {
		t.Fatalf("progress after resume = %v, want 0.5", got)
	}
}

func TestRunningAnimationCanStartPaused(t *testing.T) {
	start := time.Unix(100, 0)
	timing, _ := animationmodel.NewTiming(time.Second, 0, animationmodel.Linear{})
	running := NewRunningAnimation(CSSAnimation{
		Name: "pulse", Timing: timing, Iterations: 1, PlayState: AnimationPaused,
	}, start)
	if got := running.Sample(start.Add(time.Second)).Progress; got != 0 {
		t.Fatalf("initially paused progress = %v, want zero", got)
	}
	running.Resume(start.Add(time.Second))
	if got := running.Sample(start.Add(1250 * time.Millisecond)).Progress; got != 0.25 {
		t.Fatalf("resumed initially paused progress = %v, want 0.25", got)
	}
}

func TestAnimationStackExecutesMultipleAnimationsOnOneElement(t *testing.T) {
	start := time.Unix(100, 0)
	shortTiming, _ := animationmodel.NewTiming(time.Second, 0, animationmodel.Linear{})
	longTiming, _ := animationmodel.NewTiming(2*time.Second, 0, animationmodel.Linear{})
	stack := NewAnimationStack([]CSSAnimation{
		{Name: "fade", Timing: shortTiming, Iterations: 1, PlayState: AnimationRunning},
		{Name: "move", Timing: longTiming, Iterations: 1, PlayState: AnimationRunning},
		{Name: "none", Timing: longTiming, Iterations: 1, PlayState: AnimationRunning},
	}, start)
	if stack.Len() != 2 {
		t.Fatalf("executable animations = %d, want 2", stack.Len())
	}

	samples := stack.Sample(start.Add(500 * time.Millisecond))
	if len(samples) != 2 || samples[0].Name != "fade" || samples[1].Name != "move" {
		t.Fatalf("sample order = %#v", samples)
	}
	if samples[0].Sample.Progress != 0.5 || samples[1].Sample.Progress != 0.25 {
		t.Fatalf("sample progress = fade:%v move:%v, want 0.5 and 0.25", samples[0].Sample.Progress, samples[1].Sample.Progress)
	}
}

func TestAnimationRegistryReconcilesComputedStyleChanges(t *testing.T) {
	start := time.Unix(100, 0)
	timing, _ := animationmodel.NewTiming(time.Second, 0, animationmodel.Linear{})
	nodeID := dom.NodeID(7)
	registry := NewAnimationRegistry()
	registry.Reconcile(Map{nodeID: {Animations: []CSSAnimation{{
		Name: "pulse", Timing: timing, Iterations: math.Inf(1), PlayState: AnimationRunning,
	}}}}, start)

	registry.Reconcile(Map{nodeID: {Animations: []CSSAnimation{{
		Name: "pulse", Timing: timing, Iterations: math.Inf(1), PlayState: AnimationPaused,
	}}}}, start.Add(250*time.Millisecond))
	paused := registry.Sample(nodeID, start.Add(750*time.Millisecond))
	if len(paused) != 1 || paused[0].Sample.Progress != 0.25 {
		t.Fatalf("paused sample = %#v, want pulse at 0.25", paused)
	}

	registry.Reconcile(Map{nodeID: {Animations: []CSSAnimation{{
		Name: "pulse", Timing: timing, Iterations: math.Inf(1), PlayState: AnimationRunning,
	}}}}, start.Add(750*time.Millisecond))
	resumed := registry.Sample(nodeID, start.Add(time.Second))
	if len(resumed) != 1 || resumed[0].Sample.Progress != 0.5 {
		t.Fatalf("resumed sample = %#v, want pulse at 0.5", resumed)
	}

	registry.Reconcile(Map{nodeID: {Animations: []CSSAnimation{{
		Name: "spin", Timing: timing, Iterations: math.Inf(1), PlayState: AnimationRunning,
	}}}}, start.Add(time.Second))
	restarted := registry.Sample(nodeID, start.Add(1250*time.Millisecond))
	if len(restarted) != 1 || restarted[0].Name != "spin" || restarted[0].Sample.Progress != 0.25 {
		t.Fatalf("restarted sample = %#v, want spin at 0.25", restarted)
	}

	registry.Reconcile(Map{}, start.Add(1250*time.Millisecond))
	if registry.Count(nodeID) != 0 {
		t.Fatalf("removed element animation count = %d, want zero", registry.Count(nodeID))
	}
}

func animationTestStyle(t *testing.T, declarations string) ComputedStyle {
	t.Helper()
	document := dom.NewDocument()
	html := document.CreateElement("html", nil)
	item := document.CreateElement("div", nil)
	document.AppendChild(document.Root, html)
	document.AppendChild(html, item)
	stylesheet, err := css.Parse(strings.NewReader("div{" + declarations + "}"))
	if err != nil {
		t.Fatal(err)
	}
	computed, ok := Compute(document, stylesheet).For(item)
	if !ok {
		t.Fatal("computed style is missing")
	}
	return computed
}
