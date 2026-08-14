package style

import (
	"strings"
	"testing"
	"time"

	"github.com/Grove-Computing/Growse/internal/css"
	"github.com/Grove-Computing/Growse/internal/dom"
)

func TestAnimationRegistryCompositesKeyframesAtTimestamp(t *testing.T) {
	document := dom.NewDocument()
	target := document.CreateElement("div", nil)
	appendNode(t, document, document.Root, target)
	stylesheet, err := css.Parse(strings.NewReader(`
div { opacity: 0; color: red; transform: translateX(0); animation: 1s linear forwards showcase; }
@keyframes showcase {
  from { opacity: 0; color: red; transform: translateX(0); }
  to { opacity: 1; color: blue; transform: translateX(100px); }
}
`))
	if err != nil {
		t.Fatal(err)
	}
	underlying := Compute(document, stylesheet)
	start := time.Unix(100, 0)
	registry := NewAnimationRegistry()
	registry.Reconcile(underlying, start)
	animated := registry.AnimatedStyles(underlying, stylesheet, start.Add(500*time.Millisecond))
	computed := animated[target.ID]
	if computed.Opacity != 0.5 || computed.Color != 0x800080ff || len(computed.Transform) != 1 || computed.Transform[0].X.Pixels != 50 {
		t.Fatalf("keyframe midpoint = %#v", computed)
	}
	if underlying[target.ID].Opacity != 0 || underlying[target.ID].Color != 0xff0000ff || underlying[target.ID].Transform[0].X.Pixels != 0 {
		t.Fatalf("underlying style changed = %#v", underlying[target.ID])
	}
	if !registry.Active(start.Add(500*time.Millisecond)) || registry.Active(start.Add(time.Second)) {
		t.Fatal("finite animation active lifecycle is incorrect")
	}
}

func TestKeyframeMissingEndpointUsesUnderlyingValue(t *testing.T) {
	document := dom.NewDocument()
	target := document.CreateElement("div", nil)
	appendNode(t, document, document.Root, target)
	stylesheet, err := css.Parse(strings.NewReader(`
div { opacity: .2; animation: 1s linear forwards partial; }
@keyframes partial { 50% { opacity: 1; } }
`))
	if err != nil {
		t.Fatal(err)
	}
	underlying := Compute(document, stylesheet)
	start := time.Unix(100, 0)
	registry := NewAnimationRegistry()
	registry.Reconcile(underlying, start)
	firstQuarter := registry.AnimatedStyles(underlying, stylesheet, start.Add(250*time.Millisecond))[target.ID]
	lastQuarter := registry.AnimatedStyles(underlying, stylesheet, start.Add(750*time.Millisecond))[target.ID]
	if firstQuarter.Opacity != 0.6 || lastQuarter.Opacity != 0.6 {
		t.Fatalf("partial keyframes = first:%v last:%v, want 0.6", firstQuarter.Opacity, lastQuarter.Opacity)
	}
}
