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
