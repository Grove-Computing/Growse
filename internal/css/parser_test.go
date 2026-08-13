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
	if got, want := len(stylesheet.Rules), 4; got != want {
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

func TestParseRejectsMalformedCombinators(t *testing.T) {
	stylesheet, err := Parse(strings.NewReader(`body >> p, main + ~ a { color: red }`))
	if err != nil {
		t.Fatal(err)
	}
	if len(stylesheet.Rules) != 0 {
		t.Fatalf("rule count = %d, want 0", len(stylesheet.Rules))
	}
}

func TestParseCombinators(t *testing.T) {
	stylesheet, err := Parse(strings.NewReader(`main article .note > p + span ~ a[href] { color: red }`))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(stylesheet.Rules), 1; got != want {
		t.Fatalf("rule count = %d, want %d", got, want)
	}
	selector := stylesheet.Rules[0].Selectors[0]
	wantCombinators := []Combinator{
		CombinatorDescendant, CombinatorDescendant, CombinatorChild,
		CombinatorAdjacentSibling, CombinatorGeneralSibling,
	}
	if !reflect.DeepEqual(selector.Combinators, wantCombinators) {
		t.Fatalf("combinators = %v, want %v", selector.Combinators, wantCombinators)
	}
	if got, want := len(selector.Compounds), 6; got != want {
		t.Fatalf("compound count = %d, want %d", got, want)
	}
	if got, want := selector.Specificity(), [3]int{0, 2, 5}; got != want {
		t.Fatalf("specificity = %v, want %v", got, want)
	}
}

func TestParseStructuralPseudoClasses(t *testing.T) {
	stylesheet, err := Parse(strings.NewReader(`
:root, :empty, p:first-child, p:last-child, p:only-child,
p:first-of-type, p:last-of-type, p:only-of-type,
p:nth-child(2n+1), p:nth-last-child(even),
p:nth-of-type(-n + 3), p:nth-last-of-type(2), p:not(.hidden) { color: red }
`))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(stylesheet.Rules), 1; got != want {
		t.Fatalf("rule count = %d, want %d", got, want)
	}
	selectors := stylesheet.Rules[0].Selectors
	if got, want := len(selectors), 13; got != want {
		t.Fatalf("selector count = %d, want %d", got, want)
	}
	wantKinds := []PseudoClassKind{
		PseudoRoot, PseudoEmpty, PseudoFirstChild, PseudoLastChild, PseudoOnlyChild,
		PseudoFirstOfType, PseudoLastOfType, PseudoOnlyOfType, PseudoNthChild,
		PseudoNthLastChild, PseudoNthOfType, PseudoNthLastOfType, PseudoNot,
	}
	for index, want := range wantKinds {
		pseudos := selectors[index].Compounds[0].Pseudos
		if len(pseudos) != 1 || pseudos[0].Kind != want {
			t.Fatalf("selector %d pseudos = %#v, want kind %v", index, pseudos, want)
		}
	}
	if got := selectors[8].Compounds[0].Pseudos[0]; got.A != 2 || got.B != 1 {
		t.Fatalf("2n+1 = (%d, %d), want (2, 1)", got.A, got.B)
	}
	if got := selectors[10].Compounds[0].Pseudos[0]; got.A != -1 || got.B != 3 {
		t.Fatalf("-n+3 = (%d, %d), want (-1, 3)", got.A, got.B)
	}
	if got, want := selectors[12].Specificity(), [3]int{0, 1, 1}; got != want {
		t.Fatalf(":not specificity = %v, want %v", got, want)
	}
}

func TestParseRejectsInvalidStructuralPseudoClasses(t *testing.T) {
	stylesheet, err := Parse(strings.NewReader(`
p:nth-child(wat) { color: red }
p:not(.one.two) { color: red }
p:not(:not(.one)) { color: red }
`))
	if err != nil {
		t.Fatal(err)
	}
	if got := len(stylesheet.Rules); got != 0 {
		t.Fatalf("rule count = %d, want 0", got)
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

func TestParseInlineDeclarations(t *testing.T) {
	declarations, err := ParseDeclarations(`color: red; broken; color: blue !important; margin: 4px`)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(declarations), 3; got != want {
		t.Fatalf("declaration count = %d, want %d", got, want)
	}
	if declarations[1].Property != "color" || declarations[1].Value.Raw != "blue" || !declarations[1].Important {
		t.Fatalf("important declaration = %#v", declarations[1])
	}
}

func TestParsePreservesCustomPropertyNameCase(t *testing.T) {
	stylesheet, err := Parse(strings.NewReader(`p { --Brand: red; --brand: blue; color: var(--Brand) }`))
	if err != nil {
		t.Fatal(err)
	}
	declarations := stylesheet.Rules[0].Declarations
	if got, want := []string{declarations[0].Property, declarations[1].Property, declarations[2].Property},
		[]string{"--Brand", "--brand", "color"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("properties = %v, want %v", got, want)
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

func TestParseInteractionAndFormStatePseudoClasses(t *testing.T) {
	stylesheet, err := Parse(strings.NewReader(`
a:link, input:focus, input:enabled, input:disabled, input:checked { color: red }
`))
	if err != nil {
		t.Fatal(err)
	}
	selectors := stylesheet.Rules[0].Selectors
	wantKinds := []PseudoClassKind{PseudoLink, PseudoFocus, PseudoEnabled, PseudoDisabled, PseudoChecked}
	if got, want := len(selectors), len(wantKinds); got != want {
		t.Fatalf("selector count = %d, want %d", got, want)
	}
	for index, want := range wantKinds {
		pseudos := selectors[index].Compounds[0].Pseudos
		if len(pseudos) != 1 || pseudos[0].Kind != want {
			t.Fatalf("selector %d pseudos = %#v, want kind %v", index, pseudos, want)
		}
		if got, want := selectors[index].Specificity(), [3]int{0, 1, 1}; got != want {
			t.Fatalf("selector %d specificity = %v, want %v", index, got, want)
		}
	}
}

func TestParseBeforeAndAfterPseudoElements(t *testing.T) {
	stylesheet, err := Parse(strings.NewReader(`p.note::before, main > p::after { content: "marker" }`))
	if err != nil {
		t.Fatal(err)
	}
	selectors := stylesheet.Rules[0].Selectors
	if got, want := len(selectors), 2; got != want {
		t.Fatalf("selector count = %d, want %d", got, want)
	}
	if got := selectors[0].Compounds[0].PseudoElement; got != PseudoElementBefore {
		t.Fatalf("first pseudo-element = %v, want before", got)
	}
	if got := selectors[1].Compounds[1].PseudoElement; got != PseudoElementAfter {
		t.Fatalf("second pseudo-element = %v, want after", got)
	}
	if got, want := selectors[0].Specificity(), [3]int{0, 1, 2}; got != want {
		t.Fatalf("specificity = %v, want %v", got, want)
	}
}

func TestSelectorSpecificityCountsEveryComponent(t *testing.T) {
	stylesheet, err := Parse(strings.NewReader(`
main#app.card[lang]:hover:first-child:not(.skip)::before { content: "x" }
body #dialog > *.action { color: red }
* { color: black }
`))
	if err != nil {
		t.Fatal(err)
	}
	wants := [][3]int{{1, 5, 2}, {1, 1, 1}, {0, 0, 0}}
	for index, want := range wants {
		if got := stylesheet.Rules[index].Selectors[0].Specificity(); got != want {
			t.Fatalf("selector %d specificity = %v, want %v", index, got, want)
		}
	}
}

func TestDecodeStringHandlesCSSEscapes(t *testing.T) {
	got, ok := DecodeString(`"go\000070 her"`)
	if !ok || got != "gopher" {
		t.Fatalf("DecodeString() = (%q, %v), want (gopher, true)", got, ok)
	}
}

func TestParseIgnoresUnsupportedHoverSelectors(t *testing.T) {
	stylesheet, err := Parse(strings.NewReader(`
:hover, div:hover:hover, button:active, button:hover::before { color: red }
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
