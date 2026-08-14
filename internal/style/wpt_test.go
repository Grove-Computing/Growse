package style

import (
	"strings"
	"testing"
	"time"

	"github.com/Grove-Computing/Growse/internal/animation"
	"github.com/Grove-Computing/Growse/internal/css"
	"github.com/Grove-Computing/Growse/internal/dom"
)

// Adapted from WPT css/CSS2/cascade/specificity-001.xht at the revision
// recorded in docs/wpt.md. The original visual assertion is expressed as a
// computed-value assertion for Growse's renderer-independent style layer.
func TestWPTSpecificity001AttributeBeatsType(t *testing.T) {
	document := dom.NewDocument()
	target := document.CreateElement("div", map[string]string{"id": "id1"})
	appendNode(t, document, document.Root, target)
	stylesheet, err := css.Parse(strings.NewReader(`[id=id1] { color: green; } div { color: red; }`))
	if err != nil {
		t.Fatal(err)
	}
	computed, _ := Compute(document, stylesheet).For(target)
	if computed.Color != 0x008000ff {
		t.Fatalf("color = %#x, want green", computed.Color)
	}
}

// Adapted from WPT css/css-values/calc-catch-divide-by-0.html. CSS Values 4
// serializes infinity, while the v0.4.0 scope deliberately invalidates every
// non-finite result; both behaviors must complete without a crash.
func TestWPTCalcDivideByZeroIsRejectedWithoutCrash(t *testing.T) {
	for _, value := range []string{
		"calc(100px / 0)",
		"calc(100px / (0))",
		"calc(100px / (2 - 2))",
		"calc(100px * (1 / 0))",
	} {
		if resolved, ok := ResolveLength(value, LengthContext{}); ok {
			t.Fatalf("ResolveLength(%q) = %#v, true; want invalid", value, resolved)
		}
	}
}

// Adapted from WPT css/css-backgrounds/border-radius-001.xht. The source
// reftest asserts that border-radius: 0 is identical to square corners.
func TestWPTBorderRadius001ZeroProducesSquareCorners(t *testing.T) {
	document := dom.NewDocument()
	target := document.CreateElement("div", nil)
	appendNode(t, document, document.Root, target)
	stylesheet, err := css.Parse(strings.NewReader(`div { border-radius: 0; }`))
	if err != nil {
		t.Fatal(err)
	}
	computed, _ := Compute(document, stylesheet).For(target)
	if computed.BorderRadius != (BorderRadii{}) {
		t.Fatalf("border radius = %#v, want square corners", computed.BorderRadius)
	}
}

// Adapted from WPT css/css-animations/animation-important-with-transition.html
// at the revision recorded in docs/wpt.md. Web Animations objects are outside
// v0.7, so this checks their observable computed-value cascade instead.
func TestWPTAnimationImportantWithTransitionCascade(t *testing.T) {
	underlying := ComputedStyle{
		Color: 0xff0000ff, Opacity: 1,
		ImportantProperties: map[string]bool{"color": true, "opacity": true},
	}
	animated := AnimatedValues{
		"color":   {Kind: TransitionColor, Color: 0x0000ffff},
		"opacity": {Kind: TransitionNumber, Number: 0},
	}
	transitioned := AnimatedValues{"opacity": {Kind: TransitionNumber, Number: 0.5}}
	computed := ApplyAnimatedCascade(underlying, animated, transitioned)
	if computed.Color != 0xff0000ff || computed.Opacity != 0.5 {
		t.Fatalf("cascade result = color:%#08x opacity:%v", computed.Color, computed.Opacity)
	}
}

// Adapted from WPT css/css-animations/animation-fill-mode-001-manual.html.
// Its post-animation visual blue square is reduced to the underlying computed
// background-color after a no-fill animation finishes.
func TestWPTAnimationFillModeNoneRestoresUnderlyingColor(t *testing.T) {
	start := time.Unix(100, 0)
	timing, _ := animation.NewTiming(time.Second, 0, animation.Linear{})
	sample := (CSSAnimation{
		Name: "sample", Timing: timing, Iterations: 1, FillMode: AnimationFillNone,
	}).Sample(start, start.Add(time.Second))
	underlying := ComputedStyle{BackgroundColor: 0x0000ffff}
	computed := ApplyAnimationSample(underlying, AnimatedValues{
		"background-color": {Kind: TransitionColor, Color: 0x008000ff},
	}, sample)
	if computed.BackgroundColor != 0x0000ffff {
		t.Fatalf("post-animation color = %#08x, want underlying blue", computed.BackgroundColor)
	}
}

// Adapted from WPT css/css-transitions/transition-duration-shorthand.html.
// Growse substitutes supported paint-only properties for width and height and
// preserves the source assertion that the last matching zero duration wins.
func TestWPTTransitionDurationShorthandUsesLastMatchingProperty(t *testing.T) {
	long, _ := animation.NewTiming(100*time.Second, 0, animation.Linear{})
	zero, _ := animation.NewTiming(0, 0, animation.Linear{})
	previous := Map{1: {Opacity: 0, Color: 0xff0000ff}}
	next := Map{1: {
		Opacity: 1, Color: 0x0000ffff,
		Transitions: []Transition{{Property: "all", Timing: long}, {Property: "opacity", Timing: zero}},
	}}
	started := StartTransitions(previous, next)
	if len(started) != 1 || started[0].Property != "color" {
		t.Fatalf("started transitions = %#v, want only color", started)
	}
}
