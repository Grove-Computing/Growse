package css

import (
	"strings"
	"testing"
)

func TestParseSupportedRules(t *testing.T) {
	stylesheet, err := Parse(strings.NewReader(`
h1, .lead { color: #123456; font-size: 30px }
#app { color: blue !important; }
main.card { font-weight: 700 }
body > p { color: red }
`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if got, want := len(stylesheet.Rules), 3; got != want {
		t.Fatalf("rule count = %d, want %d", got, want)
	}
	if got, want := len(stylesheet.Rules[0].Selectors), 2; got != want {
		t.Fatalf("selector count = %d, want %d", got, want)
	}
	if got, want := stylesheet.Rules[1].Selectors[0].Specificity(), [3]int{1, 0, 0}; got != want {
		t.Fatalf("specificity = %v, want %v", got, want)
	}
	if !stylesheet.Rules[1].Declarations[0].Important {
		t.Fatal("!important was not parsed")
	}
}

func TestParseIgnoresUnsupportedSelectors(t *testing.T) {
	stylesheet, err := Parse(strings.NewReader(`body > p { color: red }`))
	if err != nil {
		t.Fatal(err)
	}
	if len(stylesheet.Rules) != 0 {
		t.Fatalf("rule count = %d, want 0", len(stylesheet.Rules))
	}
}

func TestParseSupportedHoverSelectors(t *testing.T) {
	stylesheet, err := Parse(strings.NewReader(`
button:hover, #save:hover, .todo:hover, li.todo:hover { color: red }
`))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(stylesheet.Rules), 1; got != want {
		t.Fatalf("rule count = %d, want %d", got, want)
	}
	selectors := stylesheet.Rules[0].Selectors
	if got, want := len(selectors), 4; got != want {
		t.Fatalf("selector count = %d, want %d", got, want)
	}
	for index, selector := range selectors {
		if !selector.Hover {
			t.Fatalf("selector %d does not require hover: %#v", index, selector)
		}
	}
	if got, want := selectors[0].Specificity(), [3]int{0, 1, 1}; got != want {
		t.Fatalf("button:hover specificity = %v, want %v", got, want)
	}
	if got, want := selectors[1].Specificity(), [3]int{1, 1, 0}; got != want {
		t.Fatalf("#save:hover specificity = %v, want %v", got, want)
	}
	if got, want := selectors[2].Specificity(), [3]int{0, 2, 0}; got != want {
		t.Fatalf(".todo:hover specificity = %v, want %v", got, want)
	}
	if got, want := selectors[3].Specificity(), [3]int{0, 2, 1}; got != want {
		t.Fatalf("li.todo:hover specificity = %v, want %v", got, want)
	}
}

func TestParseIgnoresUnsupportedHoverSelectors(t *testing.T) {
	stylesheet, err := Parse(strings.NewReader(`
:hover, div:hover:hover, main button:hover, button:active, button:hover::before { color: red }
p { color: blue }
`))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(stylesheet.Rules), 1; got != want {
		t.Fatalf("rule count = %d, want %d", got, want)
	}
	if got, want := stylesheet.Rules[0].Selectors[0].Tag, "p"; got != want {
		t.Fatalf("remaining selector = %q, want %q", got, want)
	}
}
