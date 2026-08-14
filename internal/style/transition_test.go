package style

import (
	"strings"
	"testing"
	"time"

	"github.com/Grove-Computing/Growse/internal/animation"
	"github.com/Grove-Computing/Growse/internal/css"
	"github.com/Grove-Computing/Growse/internal/dom"
)

func TestComputedTransitionLonghandsMatchLists(t *testing.T) {
	computed := transitionTestStyle(t, `
transition-property: opacity, transform, color;
transition-duration: 200ms, 1s;
transition-delay: -50ms;
transition-timing-function: linear, cubic-bezier(0.42, 0, 1, 1), steps(4, end);
`)
	if got := len(computed.Transitions); got != 3 {
		t.Fatalf("transitions = %d, want 3", got)
	}
	assertTransition(t, computed.Transitions[0], "opacity", 200*time.Millisecond, -50*time.Millisecond)
	assertTransition(t, computed.Transitions[1], "transform", time.Second, -50*time.Millisecond)
	assertTransition(t, computed.Transitions[2], "color", 200*time.Millisecond, -50*time.Millisecond)
	if _, ok := computed.Transitions[0].Timing.Easing.(animation.Linear); !ok {
		t.Fatalf("first easing = %T, want Linear", computed.Transitions[0].Timing.Easing)
	}
	if _, ok := computed.Transitions[1].Timing.Easing.(animation.CubicBezier); !ok {
		t.Fatalf("second easing = %T, want CubicBezier", computed.Transitions[1].Timing.Easing)
	}
	if _, ok := computed.Transitions[2].Timing.Easing.(animation.Steps); !ok {
		t.Fatalf("third easing = %T, want Steps", computed.Transitions[2].Timing.Easing)
	}
}

func TestComputedTransitionShorthandAndLonghandOverride(t *testing.T) {
	computed := transitionTestStyle(t, `
transition: opacity 200ms ease-in -50ms, transform 1s steps(4, jump-end);
transition-duration: 300ms;
`)
	if got := len(computed.Transitions); got != 2 {
		t.Fatalf("transitions = %d, want 2", got)
	}
	assertTransition(t, computed.Transitions[0], "opacity", 300*time.Millisecond, -50*time.Millisecond)
	assertTransition(t, computed.Transitions[1], "transform", 300*time.Millisecond, 0)
}

func TestComputedTransitionNoneAndInvalidDuration(t *testing.T) {
	if got := transitionTestStyle(t, `transition: none`).Transitions; len(got) != 0 {
		t.Fatalf("transition:none = %#v, want no transitions", got)
	}
	computed := transitionTestStyle(t, `transition-property: opacity; transition-duration: -1s`)
	assertTransition(t, computed.Transitions[0], "opacity", 0, 0)
}

func TestStartTransitionsFromComputedValueChange(t *testing.T) {
	document := dom.NewDocument()
	html := document.CreateElement("html", nil)
	item := document.CreateElement("div", map[string]string{"class": "box"})
	document.AppendChild(document.Root, html)
	document.AppendChild(html, item)
	stylesheet, err := css.Parse(strings.NewReader(`
.box { opacity: 0.2; color: red; transition: opacity 1s linear, color 1s linear }
.box:hover { opacity: 0.8; color: blue }
`))
	if err != nil {
		t.Fatal(err)
	}
	before := Compute(document, stylesheet)
	after := ComputeWithState(document, stylesheet, InteractionState{Hovered: map[dom.NodeID]bool{item.ID: true}})

	started := StartTransitions(before, after)
	if got := len(started); got != 2 {
		t.Fatalf("started transitions = %d, want opacity and color", got)
	}
	if started[0].NodeID != item.ID || started[0].Property != "opacity" || started[0].From.Number != 0.2 || started[0].To.Number != 0.8 {
		t.Fatalf("opacity transition = %#v", started[0])
	}
	if started[1].Property != "color" || started[1].From.Color == started[1].To.Color {
		t.Fatalf("color transition = %#v", started[1])
	}
	if got := StartTransitions(after, after); len(got) != 0 {
		t.Fatalf("unchanged styles started %d transitions", len(got))
	}
}

func TestStartTransitionsUsesLastMatchingEntryAndSkipsZeroDuration(t *testing.T) {
	previous := Map{1: {Opacity: 0}}
	zero, _ := animation.NewTiming(0, 0, animation.Linear{})
	oneSecond, _ := animation.NewTiming(time.Second, 0, animation.Linear{})
	next := Map{1: {Opacity: 1, Transitions: []Transition{
		{Property: "opacity", Timing: oneSecond},
		{Property: "all", Timing: zero},
	}}}
	if got := StartTransitions(previous, next); len(got) != 0 {
		t.Fatalf("last zero-duration match started %d transitions", len(got))
	}
}

func TestRunningTransitionInterruptsFromCurrentValueAndShortensReverse(t *testing.T) {
	start := time.Unix(100, 0)
	timing, err := animation.NewTiming(time.Second, 0, animation.Linear{})
	if err != nil {
		t.Fatal(err)
	}
	running := NewRunningTransition(StartedTransition{
		NodeID: 1, Property: "opacity",
		From:   TransitionValue{Kind: TransitionNumber, Number: 0},
		To:     TransitionValue{Kind: TransitionNumber, Number: 1},
		Timing: timing,
	}, start)

	current, active := running.Advance(start.Add(250 * time.Millisecond))
	if !active || current.Number != 0.25 {
		t.Fatalf("quarter sample = (%v, %v), want (0.25, active)", current.Number, active)
	}

	running.Interrupt(start.Add(250*time.Millisecond), TransitionValue{Kind: TransitionNumber, Number: 0}, timing)
	if running.From.Number != 0.25 || running.To.Number != 0 {
		t.Fatalf("reversed range = %v -> %v, want 0.25 -> 0", running.From.Number, running.To.Number)
	}
	if running.Timing.Duration != 250*time.Millisecond {
		t.Fatalf("reversed duration = %v, want 250ms", running.Timing.Duration)
	}

	current, active = running.Advance(start.Add(375 * time.Millisecond))
	if !active || current.Number != 0.125 {
		t.Fatalf("reversed midpoint = (%v, %v), want (0.125, active)", current.Number, active)
	}
	current, active = running.Advance(start.Add(500 * time.Millisecond))
	if active || current.Number != 0 {
		t.Fatalf("reversed end = (%v, %v), want (0, inactive)", current.Number, active)
	}
}

func TestRunningTransitionInterruptsTowardNewTargetAtFullDuration(t *testing.T) {
	start := time.Unix(100, 0)
	timing, _ := animation.NewTiming(time.Second, 0, animation.Linear{})
	running := NewRunningTransition(StartedTransition{
		From: TransitionValue{Kind: TransitionNumber, Number: 0},
		To:   TransitionValue{Kind: TransitionNumber, Number: 1}, Timing: timing,
	}, start)
	running.Interrupt(start.Add(400*time.Millisecond), TransitionValue{Kind: TransitionNumber, Number: 0.8}, timing)
	if running.From.Number != 0.4 || running.To.Number != 0.8 || running.Timing.Duration != time.Second {
		t.Fatalf("replacement transition = %v -> %v in %v", running.From.Number, running.To.Number, running.Timing.Duration)
	}
}

func transitionTestStyle(t *testing.T, declarations string) ComputedStyle {
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

func assertTransition(t *testing.T, transition Transition, property string, duration, delay time.Duration) {
	t.Helper()
	if transition.Property != property || transition.Timing.Duration != duration || transition.Timing.Delay != delay {
		t.Fatalf("transition = {property:%q duration:%v delay:%v}, want {property:%q duration:%v delay:%v}",
			transition.Property, transition.Timing.Duration, transition.Timing.Delay, property, duration, delay)
	}
}
