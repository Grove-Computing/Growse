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
