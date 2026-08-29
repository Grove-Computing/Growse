package css

import (
	"strings"
	"testing"
)

func TestParseLevel4FunctionalSelectorsForgivingListsAndSpecificity(t *testing.T) {
	stylesheet, err := Parse(strings.NewReader(`
.card:is(.featured, #hero, :future-pseudo) { color: red }
.card:where(#hero, .featured) { color: blue }
article:not(.draft > span, #archived) { color: green }
section:has(> .badge, :future-pseudo) { color: purple }
:scope > main { color: black }
.valid, :future-pseudo { color: orange }
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(stylesheet.Rules) != 5 {
		t.Fatalf("rules = %d, want 5; normal invalid list must be discarded", len(stylesheet.Rules))
	}
	wants := [][3]int{{1, 1, 0}, {0, 1, 0}, {1, 0, 1}, {0, 1, 1}, {0, 1, 1}}
	for index, want := range wants {
		if got := stylesheet.Rules[index].Selectors[0].Specificity(); got != want {
			t.Errorf("selector %d specificity = %v, want %v", index, got, want)
		}
	}
	isPseudo := stylesheet.Rules[0].Selectors[0].Compounds[0].Pseudos[0]
	if len(isPseudo.Selectors) != 2 {
		t.Fatalf(":is forgiving list = %#v", isPseudo.Selectors)
	}
	hasPseudo := stylesheet.Rules[3].Selectors[0].Compounds[0].Pseudos[0]
	if len(hasPseudo.Selectors) != 1 || hasPseudo.Selectors[0].Compounds[0].Pseudos[0].Kind != PseudoRelativeScope {
		t.Fatalf(":has relative selector = %#v", hasPseudo.Selectors)
	}
}

func TestParseRootHostSelectorListKeepsTailwindThemeRule(t *testing.T) {
	stylesheet, err := Parse(strings.NewReader(`
:root,:host { --spacing: .25rem; --color-slate-900: #0f172a }
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(stylesheet.Rules) != 1 {
		t.Fatalf("rules = %d, want 1", len(stylesheet.Rules))
	}
	rule := stylesheet.Rules[0]
	if len(rule.Selectors) != 2 {
		t.Fatalf("selectors = %#v, want :root and :host", rule.Selectors)
	}
	if got := rule.Selectors[0].Compounds[0].Pseudos[0].Kind; got != PseudoRoot {
		t.Fatalf("first pseudo = %v, want :root", got)
	}
	if got := rule.Selectors[1].Compounds[0].Pseudos[0].Kind; got != PseudoHost {
		t.Fatalf("second pseudo = %v, want :host", got)
	}
}
