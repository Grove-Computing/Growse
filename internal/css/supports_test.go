package css

import (
	"strings"
	"testing"
)

func TestParseSupportsDeclarationSelectorAndBooleanConditions(t *testing.T) {
	stylesheet, err := Parse(strings.NewReader(`
@supports (display: grid) and (not (display: ruby)) { .grid { color: green } }
@supports selector(.card > span) or (future-prop: value) { .card { color: blue } }
@supports selector(:future-pseudo) { .ignored { color: red } }
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(stylesheet.Rules) != 3 {
		t.Fatalf("rules = %d, want 3", len(stylesheet.Rules))
	}
	first := stylesheet.Rules[0].Supports[0]
	if first.Kind != SupportsAnd || len(first.Children) != 2 || first.Children[0].Kind != SupportsDeclaration || first.Children[1].Kind != SupportsNot {
		t.Fatalf("first condition = %#v", first)
	}
	second := stylesheet.Rules[1].Supports[0]
	if second.Kind != SupportsOr || second.Children[0].Kind != SupportsSelector {
		t.Fatalf("second condition = %#v", second)
	}
	if stylesheet.Rules[2].Supports[0].Kind != SupportsUnknown {
		t.Fatalf("unknown selector condition = %#v", stylesheet.Rules[2].Supports[0])
	}
}
