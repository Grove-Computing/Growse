package style

import (
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/css"
	"github.com/Grove-Computing/Growse/internal/dom"
)

func TestSupportsUsesImplementedDeclarationAndSelectorCapabilities(t *testing.T) {
	document := dom.NewDocument()
	target := document.CreateElement("div", map[string]string{"class": "target"})
	child := document.CreateElement("span", nil)
	appendNode(t, document, document.Root, target)
	appendNode(t, document, target, child)
	stylesheet, err := css.Parse(strings.NewReader(`
@supports (display: grid) and (not (display: ruby)) { .target { color: green } }
@supports (display: ruby) or (unknown-property: value) { .target { color: red } }
@supports selector(.target > span) { .target { background-color: blue } }
@supports selector(:future-pseudo) { .target { background-color: red } }
`))
	if err != nil {
		t.Fatal(err)
	}
	computed, _ := Compute(document, stylesheet).For(target)
	if computed.Color != 0x008000ff || computed.BackgroundColor != 0x0000ffff {
		t.Fatalf("supports result = color:%#08x background:%#08x", computed.Color, computed.BackgroundColor)
	}
}

func TestSupportsNeverClaimsUnknownFeature(t *testing.T) {
	for _, declaration := range []struct{ property, value string }{
		{"unknown-property", "value"}, {"display", "ruby"}, {"color", "future-color(1)"}, {"transform", "future(1)"},
	} {
		if supportsDeclaration(declaration.property, declaration.value) {
			t.Errorf("supportsDeclaration(%q, %q) = true", declaration.property, declaration.value)
		}
	}
}
