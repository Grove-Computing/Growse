package style

import (
	"strings"
	"testing"
	"time"

	"github.com/Grove-Computing/Growse/internal/animation"
	"github.com/Grove-Computing/Growse/internal/css"
	"github.com/Grove-Computing/Growse/internal/dom"
)

func TestAnimatedCascadeOrdersTransitionImportantAndAnimation(t *testing.T) {
	document := dom.NewDocument()
	target := document.CreateElement("div", nil)
	appendNode(t, document, document.Root, target)
	stylesheet, err := css.Parse(strings.NewReader(`
div {
  color: red !important;
  opacity: 0.2 !important;
  background-color: black;
  transform: translateX(10px);
}
`))
	if err != nil {
		t.Fatal(err)
	}
	underlying, _ := Compute(document, stylesheet).For(target)
	animations := AnimatedValues{
		"color":            {Kind: TransitionColor, Color: 0x0000ffff},
		"opacity":          {Kind: TransitionNumber, Number: 0.8},
		"background-color": {Kind: TransitionColor, Color: 0x00ff00ff},
		"transform": {Kind: TransitionTransform, Transform: []TransformFunction{{
			Kind: TransformTranslate, X: LengthPercentage{Pixels: 30},
		}}},
	}

	withoutTransition := ApplyAnimatedCascade(underlying, animations, nil)
	if withoutTransition.Color != 0xff0000ff || withoutTransition.Opacity != 0.2 {
		t.Fatalf("Animation overrode !important: color=%#08x opacity=%v", withoutTransition.Color, withoutTransition.Opacity)
	}
	if withoutTransition.BackgroundColor != 0x00ff00ff || withoutTransition.Transform[0].X.Pixels != 30 {
		t.Fatalf("Animation did not override normal author style: %#v", withoutTransition)
	}

	withTransition := ApplyAnimatedCascade(underlying, animations, AnimatedValues{
		"opacity": {Kind: TransitionNumber, Number: 0.5},
		"color":   {Kind: TransitionColor, Color: 0xffff00ff},
	})
	if withTransition.Opacity != 0.5 || withTransition.Color != 0xffff00ff {
		t.Fatalf("Transition did not override Animation and !important: color=%#08x opacity=%v", withTransition.Color, withTransition.Opacity)
	}
	if underlying.Opacity != 0.2 || underlying.Color != 0xff0000ff || underlying.BackgroundColor != 0x000000ff || underlying.Transform[0].X.Pixels != 10 {
		t.Fatalf("underlying computed style was modified: %#v", underlying)
	}
}

func TestComputedStyleTracksExpandedImportantProperties(t *testing.T) {
	computed := transitionTestStyle(t, `border-color: red !important; outline-color: blue`)
	for _, property := range []string{"border-top-color", "border-right-color", "border-bottom-color", "border-left-color"} {
		if !computed.Important(property) {
			t.Fatalf("%s was not marked important", property)
		}
	}
	if computed.Important("outline-color") {
		t.Fatal("normal outline-color was marked important")
	}
}

func TestAnimationEndUsesFillModeOrUnderlyingValue(t *testing.T) {
	start := time.Unix(100, 0)
	timing, _ := animation.NewTiming(time.Second, 0, animation.Linear{})
	underlying := ComputedStyle{Opacity: 0.25, Color: 0xff0000ff}
	effect := AnimatedValues{
		"opacity": {Kind: TransitionNumber, Number: 0.9},
		"color":   {Kind: TransitionColor, Color: 0x0000ffff},
	}

	withoutFill := CSSAnimation{
		Name: "fade", Timing: timing, Iterations: 1, FillMode: AnimationFillNone,
	}.Sample(start, start.Add(time.Second))
	got := ApplyAnimationSample(underlying, effect, withoutFill)
	if got.Opacity != underlying.Opacity || got.Color != underlying.Color {
		t.Fatalf("no-fill result = opacity:%v color:%#08x, want underlying", got.Opacity, got.Color)
	}

	withFill := CSSAnimation{
		Name: "fade", Timing: timing, Iterations: 1, FillMode: AnimationFillForwards,
	}.Sample(start, start.Add(time.Second))
	got = ApplyAnimationSample(underlying, effect, withFill)
	if got.Opacity != 0.9 || got.Color != 0x0000ffff {
		t.Fatalf("forwards-fill result = opacity:%v color:%#08x, want final effect", got.Opacity, got.Color)
	}
	if underlying.Opacity != 0.25 || underlying.Color != 0xff0000ff {
		t.Fatalf("underlying style changed = %#v", underlying)
	}
}
