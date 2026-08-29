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

func TestParseTailwindEscapedUtilityClassNames(t *testing.T) {
	tests := []struct {
		selector string
		want     string
	}{
		{`.sm\:grid-cols-2`, `sm:grid-cols-2`},
		{`.hover\:bg-slate-900:hover`, `hover:bg-slate-900`},
		{`.w-\[calc\(100\%-1rem\)\]`, `w-[calc(100%-1rem)]`},
		{`.\31 0\/12`, `10/12`},
	}
	for _, test := range tests {
		t.Run(test.want, func(t *testing.T) {
			selectors := ParseSelectorList(test.selector)
			if len(selectors) != 1 || len(selectors[0].Compounds) != 1 || len(selectors[0].Compounds[0].Classes) != 1 {
				t.Fatalf("ParseSelectorList(%q) = %#v", test.selector, selectors)
			}
			if got := selectors[0].Compounds[0].Classes[0]; got != test.want {
				t.Fatalf("class = %q, want %q", got, test.want)
			}
		})
	}
	for _, invalid := range []string{`.broken\`, ".broken\\\nclass"} {
		if got := ParseSelectorList(invalid); len(got) != 0 {
			t.Fatalf("invalid escape %q parsed as %#v", invalid, got)
		}
	}
}
