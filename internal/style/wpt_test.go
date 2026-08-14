package style

import (
	"strings"
	"testing"

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
