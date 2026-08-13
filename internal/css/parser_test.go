package css

import (
	"reflect"
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

func TestParseTypeUniversalAndCompoundSelectors(t *testing.T) {
	stylesheet, err := Parse(strings.NewReader(`
* { color: black }
article.card.featured { color: blue }
#app.primary.ready { color: green }
main#content.page.active { color: red }
`))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(stylesheet.Rules), 4; got != want {
		t.Fatalf("rule count = %d, want %d", got, want)
	}
	selectors := []Selector{
		stylesheet.Rules[0].Selectors[0], stylesheet.Rules[1].Selectors[0],
		stylesheet.Rules[2].Selectors[0], stylesheet.Rules[3].Selectors[0],
	}
	if selectors[0].Kind != SelectorUniversal || !selectors[0].Compounds[0].Universal {
		t.Fatalf("universal selector = %#v", selectors[0])
	}
	if got, want := selectors[1].Compounds[0].Classes, []string{"card", "featured"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("classes = %v, want %v", got, want)
	}
	if got, want := selectors[2].Specificity(), [3]int{1, 2, 0}; got != want {
		t.Fatalf("compound specificity = %v, want %v", got, want)
	}
	if got, want := selectors[3].Specificity(), [3]int{1, 2, 1}; got != want {
		t.Fatalf("typed compound specificity = %v, want %v", got, want)
	}
}

func TestParseAttributeSelectors(t *testing.T) {
	stylesheet, err := Parse(strings.NewReader(`
[disabled] { color: red }
input[type="text"] { color: red }
[class~="item"] { color: red }
[lang|="ja"] { color: red }
a[href^="https"][href$='.pdf'][href*="/docs/"] { color: red }
`))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(stylesheet.Rules), 5; got != want {
		t.Fatalf("rule count = %d, want %d", got, want)
	}
	wantMatchers := []AttributeMatcher{
		AttributePresent, AttributeExact, AttributeIncludes, AttributeDashMatch,
	}
	for index, want := range wantMatchers {
		attributes := stylesheet.Rules[index].Selectors[0].Compounds[0].Attributes
		if len(attributes) != 1 || attributes[0].Matcher != want {
			t.Fatalf("rule %d attributes = %#v, want matcher %v", index, attributes, want)
		}
	}
	last := stylesheet.Rules[4].Selectors[0]
	attributes := last.Compounds[0].Attributes
	if got, want := []AttributeMatcher{attributes[0].Matcher, attributes[1].Matcher, attributes[2].Matcher},
		[]AttributeMatcher{AttributePrefix, AttributeSuffix, AttributeSubstring}; !reflect.DeepEqual(got, want) {
		t.Fatalf("attribute matchers = %v, want %v", got, want)
	}
	if got, want := last.Specificity(), [3]int{0, 3, 1}; got != want {
		t.Fatalf("specificity = %v, want %v", got, want)
	}
}

func TestParseSelectorListKeepsCommaInsideAttributeValue(t *testing.T) {
	stylesheet, err := Parse(strings.NewReader(`[data-value="a,b"], p { color: red }`))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(stylesheet.Rules[0].Selectors), 2; got != want {
		t.Fatalf("selector count = %d, want %d", got, want)
	}
	attribute := stylesheet.Rules[0].Selectors[0].Compounds[0].Attributes[0]
	if got, want := attribute.Value, "a,b"; got != want {
		t.Fatalf("attribute value = %q, want %q", got, want)
	}
}

func TestParseBuildsTypedRuleSelectorAndValueModel(t *testing.T) {
	stylesheet, err := Parse(strings.NewReader(`main.card { width: calc(100% - 2rem); content: "hello"; background: url(image.png) }`))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(stylesheet.Rules), 1; got != want {
		t.Fatalf("rule count = %d, want %d", got, want)
	}
	rule := stylesheet.Rules[0]
	if rule.Kind != RuleStyle || len(rule.Selectors) != 1 || rule.Selectors[0].Kind != SelectorTagClass {
		t.Fatalf("typed rule and selector = %#v", rule)
	}
	if got, want := rule.Declarations[0].Value.Raw, "calc(100% - 2rem)"; got != want {
		t.Fatalf("raw value = %q, want %q", got, want)
	}
	var kinds []ComponentKind
	for _, component := range rule.Declarations[0].Value.Components {
		if component.Kind != ComponentWhitespace {
			kinds = append(kinds, component.Kind)
		}
	}
	wantKinds := []ComponentKind{
		ComponentFunction, ComponentPercentage, ComponentDelimiter, ComponentDimension, ComponentBlockEnd,
	}
	if !reflect.DeepEqual(kinds, wantKinds) {
		t.Fatalf("component kinds = %v, want %v", kinds, wantKinds)
	}
	if got := rule.Declarations[1].Value.Components[0].Kind; got != ComponentString {
		t.Fatalf("string component kind = %v, want %v", got, ComponentString)
	}
	if got := rule.Declarations[2].Value.Components[0].Kind; got != ComponentURL {
		t.Fatalf("URL component kind = %v, want %v", got, ComponentURL)
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

func TestParseRecoversFromInvalidDeclaration(t *testing.T) {
	stylesheet, err := Parse(strings.NewReader(`
p {
	color red;
	font-size: 18px;
	broken: ;
	color: blue;
}
h1 { font-weight: bold }
`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if got, want := len(stylesheet.Rules), 2; got != want {
		t.Fatalf("rule count = %d, want %d", got, want)
	}
	declarations := stylesheet.Rules[0].Declarations
	if got, want := len(declarations), 2; got != want {
		t.Fatalf("valid declaration count = %d, want %d", got, want)
	}
	if declarations[0].Property != "font-size" || declarations[1].Property != "color" {
		t.Fatalf("recovered declarations = %#v", declarations)
	}
}

func TestParseIgnoresUnknownAtRuleAndContinues(t *testing.T) {
	stylesheet, err := Parse(strings.NewReader(`
@growse-future example {
	.ignored { color: red }
}
p { color: blue }
`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if got, want := len(stylesheet.Rules), 1; got != want {
		t.Fatalf("rule count = %d, want %d", got, want)
	}
	if got, want := stylesheet.Rules[0].Selectors[0].Tag, "p"; got != want {
		t.Fatalf("remaining selector = %q, want %q", got, want)
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
