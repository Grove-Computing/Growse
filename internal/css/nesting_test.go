package css

import (
	"strings"
	"testing"
)

func TestParseCSSNestingExpandsParentSelectorsAndNestedGroups(t *testing.T) {
	stylesheet, err := Parse(strings.NewReader(`
.card, .panel {
  color: red;
  & > .title, &:hover { color: blue }
  @media (min-width: 600px) { & .badge { color: green } }
  @supports (display: grid) { .implicit { display: grid } }
  background-color: black;
}
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(stylesheet.Rules) != 4 {
		t.Fatalf("rules = %d, want 4", len(stylesheet.Rules))
	}
	parent := stylesheet.Rules[0]
	if len(parent.Declarations) != 2 || len(parent.Selectors) != 2 {
		t.Fatalf("parent rule = %#v", parent)
	}
	if got := len(stylesheet.Rules[1].Selectors); got != 4 {
		t.Fatalf("explicit nested selector expansion = %d, want 4", got)
	}
	mediaNested := stylesheet.Rules[2]
	if len(mediaNested.Selectors) != 2 || len(mediaNested.Media) != 1 {
		t.Fatalf("nested media rule = %#v", mediaNested)
	}
	supportsNested := stylesheet.Rules[3]
	if len(supportsNested.Selectors) != 2 || len(supportsNested.Supports) != 1 {
		t.Fatalf("implicit nested supports rule = %#v", supportsNested)
	}
}

func TestCSSNestingDepthIsBoundedLocally(t *testing.T) {
	var source strings.Builder
	for index := 0; index < maxNestingDepth+2; index++ {
		source.WriteString(".level { ")
	}
	source.WriteString("color: red;")
	for index := 0; index < maxNestingDepth+2; index++ {
		source.WriteString(" }")
	}
	source.WriteString(".safe { color: blue }")
	stylesheet, err := Parse(strings.NewReader(source.String()))
	if err != nil {
		t.Fatal(err)
	}
	foundSafe := false
	for _, rule := range stylesheet.Rules {
		for _, selector := range rule.Selectors {
			foundSafe = foundSafe || selector.Class == "safe"
		}
	}
	if !foundSafe {
		t.Fatal("nesting overflow invalidated the following safe rule")
	}
}
