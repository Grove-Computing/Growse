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
