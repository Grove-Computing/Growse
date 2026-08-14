package style

import (
	"strings"
	"testing"

	"github.com/saku0512/growse/internal/css"
	"github.com/saku0512/growse/internal/dom"
)

func TestComputeAppliesCascadeAndInheritance(t *testing.T) {
	document := dom.NewDocument()
	body := document.CreateElement("body", nil)
	p := document.CreateElement("p", map[string]string{"id": "message", "class": "lead note"})
	span := document.CreateElement("span", nil)
	appendNode(t, document, document.Root, body)
	appendNode(t, document, body, p)
	appendNode(t, document, p, span)

	stylesheet, err := css.Parse(strings.NewReader(`
p { color: red; font-size: 18px }
.lead { color: blue }
#message { color: #123456; font-weight: bold }
p.note { color: green !important }
`))
	if err != nil {
		t.Fatal(err)
	}

	computed := Compute(document, stylesheet)
	pStyle, ok := computed.For(p)
	if !ok {
		t.Fatal("paragraph has no computed style")
	}
	if got, want := pStyle.Color, uint32(0x008000ff); got != want {
		t.Fatalf("paragraph color = %#x, want %#x", got, want)
	}
	if pStyle.FontSize != 18 || !pStyle.Bold() {
		t.Fatalf("paragraph style = %#v, want 18px bold", pStyle)
	}
	spanStyle, _ := computed.For(span)
	if spanStyle.Color != pStyle.Color || spanStyle.FontSize != pStyle.FontSize {
		t.Fatalf("span style = %#v, want inherited %#v", spanStyle, pStyle)
	}
}

func TestComputeResolvesDisplayMarginAndPadding(t *testing.T) {
	document := dom.NewDocument()
	div := document.CreateElement("div", map[string]string{"class": "box"})
	appendNode(t, document, document.Root, div)

	stylesheet, err := css.Parse(strings.NewReader(`
.box {
  display: none;
  margin-left: 99px;
  margin: 1px 2px 3px 4px;
  padding: 5px 6px 7px;
  padding-right: 8px;
}
`))
	if err != nil {
		t.Fatal(err)
	}

	computed, ok := Compute(document, stylesheet).For(div)
	if !ok {
		t.Fatal("div has no computed style")
	}
	if computed.Display != DisplayNone {
		t.Fatalf("display = %v, want none", computed.Display)
	}
	if got, want := computed.Margin, (Edges{Top: 1, Right: 2, Bottom: 3, Left: 4}); got != want {
		t.Fatalf("margin = %#v, want %#v", got, want)
	}
	if got, want := computed.Padding, (Edges{Top: 5, Right: 8, Bottom: 7, Left: 6}); got != want {
		t.Fatalf("padding = %#v, want %#v", got, want)
	}
}

func TestComputeWithStateAppliesHoverToTargetAndAncestor(t *testing.T) {
	document := dom.NewDocument()
	button := document.CreateElement("button", map[string]string{"id": "save", "class": "action"})
	label := document.CreateElement("span", map[string]string{"id": "label"})
	appendNode(t, document, document.Root, button)
	appendNode(t, document, button, label)
	stylesheet, err := css.Parse(strings.NewReader(`
button { color: black; background-color: white; padding: 1px }
button:hover { color: blue; padding: 4px }
#save:hover { background-color: red }
span:hover { font-size: 22px }
`))
	if err != nil {
		t.Fatal(err)
	}

	normal, _ := Compute(document, stylesheet).For(button)
	if normal.Color != 0x000000ff || normal.BackgroundColor != 0xffffffff || normal.Padding.Top != 1 {
		t.Fatalf("normal button style = %#v", normal)
	}
	hovered := InteractionState{Hovered: map[dom.NodeID]bool{button.ID: true, label.ID: true}}
	computed := ComputeWithState(document, stylesheet, hovered)
	buttonStyle, _ := computed.For(button)
	labelStyle, _ := computed.For(label)
	if buttonStyle.Color != 0x0000ffff || buttonStyle.BackgroundColor != 0xff0000ff || buttonStyle.Padding.Top != 4 {
		t.Fatalf("hovered button style = %#v", buttonStyle)
	}
	if labelStyle.FontSize != 22 {
		t.Fatalf("hovered label font size = %v, want 22", labelStyle.FontSize)
	}
	if classes, _ := button.Attribute("class"); classes != "action" {
		t.Fatalf("hover state changed class attribute to %q", classes)
	}
}

func TestHoverCascadeUsesPseudoClassSpecificity(t *testing.T) {
	document := dom.NewDocument()
	button := document.CreateElement("button", map[string]string{"class": "action"})
	appendNode(t, document, document.Root, button)
	stylesheet, err := css.Parse(strings.NewReader(`
.action { color: red }
button:hover { color: blue }
.action:hover { color: green }
`))
	if err != nil {
		t.Fatal(err)
	}
	computed, _ := ComputeWithState(document, stylesheet, InteractionState{
		Hovered: map[dom.NodeID]bool{button.ID: true},
	}).For(button)
	if got, want := computed.Color, uint32(0x008000ff); got != want {
		t.Fatalf("hover cascade color = %#x, want %#x", got, want)
	}
}

func TestComputeMatchesUniversalAndCompoundSelectors(t *testing.T) {
	document := dom.NewDocument()
	article := document.CreateElement("article", map[string]string{
		"id": "story", "class": "card featured",
	})
	other := document.CreateElement("article", map[string]string{"class": "card"})
	appendNode(t, document, document.Root, article)
	appendNode(t, document, document.Root, other)
	stylesheet, err := css.Parse(strings.NewReader(`
* { font-size: 18px }
article.card.featured#story { color: red }
`))
	if err != nil {
		t.Fatal(err)
	}
	computed := Compute(document, stylesheet)
	articleStyle, _ := computed.For(article)
	otherStyle, _ := computed.For(other)
	if articleStyle.FontSize != 18 || otherStyle.FontSize != 18 {
		t.Fatalf("universal font sizes = (%v, %v), want 18", articleStyle.FontSize, otherStyle.FontSize)
	}
	if articleStyle.Color != 0xff0000ff || otherStyle.Color == 0xff0000ff {
		t.Fatalf("compound colors = (%#x, %#x)", articleStyle.Color, otherStyle.Color)
	}
}

func TestComputeMatchesAttributeSelectors(t *testing.T) {
	document := dom.NewDocument()
	input := document.CreateElement("input", map[string]string{
		"disabled": "", "type": "text", "class": "field item", "lang": "ja-JP",
		"data-url": "https://example.test/docs/guide.pdf",
	})
	other := document.CreateElement("input", map[string]string{
		"type": "number", "class": "field", "lang": "en",
		"data-url": "http://example.test/help.txt",
	})
	appendNode(t, document, document.Root, input)
	appendNode(t, document, document.Root, other)
	stylesheet, err := css.Parse(strings.NewReader(`
input[disabled][type="text"] { color: red }
[class~="item"][lang|="ja"] { font-size: 21px }
[data-url^="https"][data-url$=".pdf"][data-url*="/docs/"] { font-weight: bold }
`))
	if err != nil {
		t.Fatal(err)
	}
	computed := Compute(document, stylesheet)
	matched, _ := computed.For(input)
	unmatched, _ := computed.For(other)
	if matched.Color != 0xff0000ff || matched.FontSize != 21 || !matched.Bold() {
		t.Fatalf("matched style = %#v", matched)
	}
	if unmatched.Color == 0xff0000ff || unmatched.FontSize == 21 || unmatched.Bold() {
		t.Fatalf("unmatched style = %#v", unmatched)
	}
}

func TestComputeMatchesCombinators(t *testing.T) {
	document := dom.NewDocument()
	main := document.CreateElement("main", nil)
	section := document.CreateElement("section", nil)
	heading := document.CreateElement("h1", nil)
	space := document.CreateText("between")
	direct := document.CreateElement("p", map[string]string{"class": "direct"})
	general := document.CreateElement("span", map[string]string{"class": "general"})
	outside := document.CreateElement("p", map[string]string{"class": "direct general"})
	appendNode(t, document, document.Root, main)
	appendNode(t, document, main, section)
	appendNode(t, document, section, heading)
	appendNode(t, document, section, space)
	appendNode(t, document, section, direct)
	appendNode(t, document, section, general)
	appendNode(t, document, document.Root, outside)

	stylesheet, err := css.Parse(strings.NewReader(`
main .direct { color: red }
section > .direct { font-size: 21px }
h1 + .direct { font-weight: bold }
h1 ~ .general { background-color: blue }
`))
	if err != nil {
		t.Fatal(err)
	}
	computed := Compute(document, stylesheet)
	directStyle, _ := computed.For(direct)
	generalStyle, _ := computed.For(general)
	outsideStyle, _ := computed.For(outside)
	if directStyle.Color != 0xff0000ff || directStyle.FontSize != 21 || !directStyle.Bold() {
		t.Fatalf("descendant/child/adjacent style = %#v", directStyle)
	}
	if generalStyle.BackgroundColor != 0x0000ffff {
		t.Fatalf("general sibling style = %#v", generalStyle)
	}
	if outsideStyle.Color == 0xff0000ff || outsideStyle.FontSize == 21 || outsideStyle.BackgroundColor == 0x0000ffff {
		t.Fatalf("outside style = %#v", outsideStyle)
	}
}

func TestMatchesStructuralPseudoClasses(t *testing.T) {
	document := dom.NewDocument()
	html := document.CreateElement("html", nil)
	body := document.CreateElement("body", nil)
	parent := document.CreateElement("div", nil)
	first := document.CreateElement("p", nil)
	space := document.CreateText(" ")
	onlyType := document.CreateElement("span", nil)
	second := document.CreateElement("p", nil)
	last := document.CreateElement("p", map[string]string{"class": "excluded"})
	onlyParent := document.CreateElement("section", nil)
	onlyChild := document.CreateElement("em", nil)
	appendNode(t, document, document.Root, html)
	appendNode(t, document, html, body)
	appendNode(t, document, body, parent)
	appendNode(t, document, parent, first)
	appendNode(t, document, parent, space)
	appendNode(t, document, parent, onlyType)
	appendNode(t, document, parent, second)
	appendNode(t, document, parent, last)
	appendNode(t, document, body, onlyParent)
	appendNode(t, document, onlyParent, onlyChild)

	tests := []struct {
		selector string
		node     *dom.Node
		want     bool
	}{
		{":root", html, true}, {":root", body, false},
		{":empty", first, true}, {":empty", parent, false},
		{"p:first-child", first, true}, {"p:last-child", last, true},
		{"em:only-child", onlyChild, true}, {"span:only-of-type", onlyType, true},
		{"p:first-of-type", first, true}, {"p:last-of-type", last, true},
		{"p:nth-child(odd)", second, true}, {"p:nth-last-child(2)", second, true},
		{"p:nth-of-type(2n)", second, true}, {"p:nth-last-of-type(2)", second, true},
		{"p:nth-of-type(-n+2)", last, false},
		{"p:not(.excluded)", first, true}, {"p:not(.excluded)", last, false},
	}
	for _, test := range tests {
		t.Run(test.selector, func(t *testing.T) {
			selector := parseTestSelector(t, test.selector)
			if got := matches(test.node, selector, InteractionState{}); got != test.want {
				t.Fatalf("matches(%q) = %v, want %v", test.selector, got, test.want)
			}
		})
	}
}

func TestMatchesInteractionAndFormStatePseudoClasses(t *testing.T) {
	document := dom.NewDocument()
	link := document.CreateElement("a", map[string]string{"href": "/next"})
	anchor := document.CreateElement("a", nil)
	enabled := document.CreateElement("input", map[string]string{"type": "text"})
	disabled := document.CreateElement("input", map[string]string{"disabled": ""})
	checked := document.CreateElement("input", map[string]string{"type": "checkbox", "checked": ""})
	selected := document.CreateElement("option", map[string]string{"selected": ""})
	for _, node := range []*dom.Node{link, anchor, enabled, disabled, checked, selected} {
		appendNode(t, document, document.Root, node)
	}
	state := InteractionState{Hovered: map[dom.NodeID]bool{link.ID: true}, Focused: enabled.ID}
	tests := []struct {
		selector string
		node     *dom.Node
		want     bool
	}{
		{"a:link", link, true}, {"a:link", anchor, false},
		{"a:hover", link, true}, {"a:focus", link, false},
		{"input:focus", enabled, true}, {"input:enabled", enabled, true},
		{"input:disabled", enabled, false}, {"input:disabled", disabled, true},
		{"input:enabled", disabled, false}, {"input:checked", checked, true},
		{"option:checked", selected, true}, {"input:checked", enabled, false},
	}
	for _, test := range tests {
		t.Run(test.selector+test.node.TagName, func(t *testing.T) {
			selector := parseTestSelector(t, test.selector)
			if got := matches(test.node, selector, state); got != test.want {
				t.Fatalf("matches(%q) = %v, want %v", test.selector, got, test.want)
			}
		})
	}
}

func TestComputeGeneratesBeforeAndAfterStringContent(t *testing.T) {
	document := dom.NewDocument()
	paragraph := document.CreateElement("p", map[string]string{"class": "note"})
	appendNode(t, document, document.Root, paragraph)
	stylesheet, err := css.Parse(strings.NewReader(`
p::before { content: "default" }
.note::before { content: "Before "; }
.note::after { content: " After" !important; }
p::after { content: none; }
`))
	if err != nil {
		t.Fatal(err)
	}
	computed, _ := Compute(document, stylesheet).For(paragraph)
	if got, want := computed.BeforeContent, "Before "; got != want {
		t.Fatalf("before content = %q, want %q", got, want)
	}
	if got, want := computed.AfterContent, " After"; got != want {
		t.Fatalf("after content = %q, want %q", got, want)
	}
	if computed.Color != defaultTextColor {
		t.Fatalf("pseudo-element rule leaked into element color: %#x", computed.Color)
	}
}

func TestSelectorListUsesSpecificityOfMatchingSelector(t *testing.T) {
	document := dom.NewDocument()
	target := document.CreateElement("div", map[string]string{"class": "target"})
	appendNode(t, document, document.Root, target)
	stylesheet, err := css.Parse(strings.NewReader(`
#other, .target { color: red }
div.target { color: blue }
`))
	if err != nil {
		t.Fatal(err)
	}
	computed, _ := Compute(document, stylesheet).For(target)
	if got, want := computed.Color, uint32(0x0000ffff); got != want {
		t.Fatalf("selector-list cascade color = %#x, want %#x", got, want)
	}
}

func TestCascadeOrdersOriginImportanceSpecificityAndSource(t *testing.T) {
	document := dom.NewDocument()
	normalInline := document.CreateElement("p", map[string]string{
		"id": "normal", "class": "item", "style": "color: green; display: inline",
	})
	importantInline := document.CreateElement("p", map[string]string{
		"id": "important", "class": "item", "style": "color: green !important",
	})
	sourceOrder := document.CreateElement("p", map[string]string{"class": "ordered"})
	for _, node := range []*dom.Node{normalInline, importantInline, sourceOrder} {
		appendNode(t, document, document.Root, node)
	}
	stylesheet, err := css.Parse(strings.NewReader(`
p { display: none; }
.item { color: blue; }
#normal { color: red !important; }
#important { color: red !important; }
.ordered { color: red; }
.ordered { color: blue; }
`))
	if err != nil {
		t.Fatal(err)
	}
	computed := Compute(document, stylesheet)
	normalStyle, _ := computed.For(normalInline)
	importantStyle, _ := computed.For(importantInline)
	orderedStyle, _ := computed.For(sourceOrder)
	if normalStyle.Color != 0xff0000ff {
		t.Fatalf("author important vs inline normal color = %#x, want red", normalStyle.Color)
	}
	if normalStyle.Display != DisplayInline {
		t.Fatalf("inline style vs author/UA display = %v, want inline", normalStyle.Display)
	}
	if importantStyle.Color != 0x008000ff {
		t.Fatalf("inline important color = %#x, want green", importantStyle.Color)
	}
	if orderedStyle.Color != 0x0000ffff {
		t.Fatalf("source-order color = %#x, want blue", orderedStyle.Color)
	}
}

func TestComputeAppliesInlineStyleWithoutStylesheet(t *testing.T) {
	document := dom.NewDocument()
	paragraph := document.CreateElement("p", map[string]string{"style": "color: blue; font-size: 20px"})
	appendNode(t, document, document.Root, paragraph)
	computed, _ := Compute(document, nil).For(paragraph)
	if computed.Color != 0x0000ffff || computed.FontSize != 20 {
		t.Fatalf("inline-only style = %#v", computed)
	}
}

func TestComputeResolvesGlobalKeywordsByPropertyInheritance(t *testing.T) {
	document := dom.NewDocument()
	parent := document.CreateElement("div", map[string]string{"class": "parent"})
	child := document.CreateElement("p", map[string]string{"class": "child"})
	inheritedDisplay := document.CreateElement("span", map[string]string{"class": "inherit-display"})
	appendNode(t, document, document.Root, parent)
	appendNode(t, document, parent, child)
	appendNode(t, document, parent, inheritedDisplay)
	stylesheet, err := css.Parse(strings.NewReader(`
.parent {
  color: blue; background-color: red; font-size: 20px; font-weight: bold;
  display: block; margin: 1px 2px 3px 4px; padding: 8px;
}
.child {
  color: unset; background-color: inherit; font-size: initial; font-weight: inherit;
  display: initial; margin: inherit; padding: unset;
}
.inherit-display { display: inherit; }
`))
	if err != nil {
		t.Fatal(err)
	}
	computed := Compute(document, stylesheet)
	childStyle, _ := computed.For(child)
	if childStyle.Color != 0x0000ffff || childStyle.BackgroundColor != 0xff0000ff {
		t.Fatalf("global color values = %#v", childStyle)
	}
	if childStyle.FontSize != 16 || childStyle.FontWeight != 700 || childStyle.Display != DisplayInline {
		t.Fatalf("global font/display values = %#v", childStyle)
	}
	if got, want := childStyle.Margin, (Edges{Top: 1, Right: 2, Bottom: 3, Left: 4}); got != want {
		t.Fatalf("inherited margin = %#v, want %#v", got, want)
	}
	if childStyle.Padding != (Edges{}) {
		t.Fatalf("unset padding = %#v, want zero", childStyle.Padding)
	}
	displayStyle, _ := computed.For(inheritedDisplay)
	if displayStyle.Display != DisplayBlock {
		t.Fatalf("inherited display = %v, want block", displayStyle.Display)
	}
}

func TestComputeResolvesInheritedCustomPropertiesFallbackAndCycles(t *testing.T) {
	document := dom.NewDocument()
	parent := document.CreateElement("div", map[string]string{"class": "variables"})
	inherited := document.CreateElement("p", map[string]string{"class": "inherited"})
	caseSensitive := document.CreateElement("p", map[string]string{"class": "case-sensitive"})
	fallback := document.CreateElement("p", map[string]string{"class": "fallback"})
	nested := document.CreateElement("p", map[string]string{"class": "nested"})
	cycle := document.CreateElement("p", map[string]string{"class": "cycle"})
	reset := document.CreateElement("p", map[string]string{"class": "reset"})
	appendNode(t, document, document.Root, parent)
	for _, child := range []*dom.Node{inherited, caseSensitive, fallback, nested, cycle, reset} {
		appendNode(t, document, parent, child)
	}
	stylesheet, err := css.Parse(strings.NewReader(`
.variables {
  --Brand: red; --brand: blue; --space: 6px;
  --a: var(--b); --b: var(--a);
}
.inherited { color: var(--Brand); margin: var(--space); }
.case-sensitive { color: var(--brand); }
.fallback { color: var(--missing, green); }
.nested { color: var(--missing, var(--brand)); }
.cycle { color: var(--a, #123456); }
.reset { --brand: initial; color: var(--brand, green); }
`))
	if err != nil {
		t.Fatal(err)
	}
	computed := Compute(document, stylesheet)
	inheritedStyle, _ := computed.For(inherited)
	caseStyle, _ := computed.For(caseSensitive)
	fallbackStyle, _ := computed.For(fallback)
	nestedStyle, _ := computed.For(nested)
	cycleStyle, _ := computed.For(cycle)
	resetStyle, _ := computed.For(reset)
	if inheritedStyle.Color != 0xff0000ff || inheritedStyle.Margin != (Edges{Top: 6, Right: 6, Bottom: 6, Left: 6}) {
		t.Fatalf("inherited variables style = %#v", inheritedStyle)
	}
	if caseStyle.Color != 0x0000ffff || nestedStyle.Color != 0x0000ffff {
		t.Fatalf("case/nested variable colors = (%#x, %#x)", caseStyle.Color, nestedStyle.Color)
	}
	if fallbackStyle.Color != 0x008000ff || resetStyle.Color != 0x008000ff {
		t.Fatalf("fallback/reset colors = (%#x, %#x)", fallbackStyle.Color, resetStyle.Color)
	}
	if cycleStyle.Color != 0x123456ff {
		t.Fatalf("cycle fallback color = %#x, want #123456", cycleStyle.Color)
	}
}

func TestComputeResolvesFontAndViewportRelativeUnits(t *testing.T) {
	document := dom.NewDocument()
	html := document.CreateElement("html", nil)
	body := document.CreateElement("body", nil)
	rem := document.CreateElement("p", map[string]string{"class": "rem"})
	viewport := document.CreateElement("p", map[string]string{"class": "viewport"})
	percentage := document.CreateElement("p", map[string]string{"class": "percentage"})
	appendNode(t, document, document.Root, html)
	appendNode(t, document, html, body)
	appendNode(t, document, body, rem)
	appendNode(t, document, body, viewport)
	appendNode(t, document, body, percentage)
	stylesheet, err := css.Parse(strings.NewReader(`
html { font-size: 20px; }
.rem { font-size: 2rem; margin: 1em; }
.viewport { font-size: 10vw; padding: 5vh; }
.percentage { font-size: 150%; margin-left: 25%; }
`))
	if err != nil {
		t.Fatal(err)
	}
	computed := ComputeWithEnvironment(document, stylesheet, InteractionState{}, Environment{
		ViewportWidth: 800, ViewportHeight: 600, RootFontSize: 16,
	})
	remStyle, _ := computed.For(rem)
	viewportStyle, _ := computed.For(viewport)
	percentageStyle, _ := computed.For(percentage)
	if remStyle.FontSize != 40 || remStyle.Margin.Top != 40 {
		t.Fatalf("rem/em style = %#v", remStyle)
	}
	if viewportStyle.FontSize != 80 || viewportStyle.Padding.Top != 30 {
		t.Fatalf("viewport style = %#v", viewportStyle)
	}
	if percentageStyle.FontSize != 30 || percentageStyle.Margin.Left != 200 {
		t.Fatalf("percentage style = %#v", percentageStyle)
	}
}

func TestComputeAppliesCalcToFontAndBoxValues(t *testing.T) {
	document := dom.NewDocument()
	paragraph := document.CreateElement("p", nil)
	appendNode(t, document, document.Root, paragraph)
	stylesheet, err := css.Parse(strings.NewReader(`
p {
  font-size: calc(1rem + 4px);
  margin: calc(10% - 2px) calc(2 * 3px);
  padding: calc(1em / 2);
}
`))
	if err != nil {
		t.Fatal(err)
	}
	computed, _ := ComputeWithEnvironment(document, stylesheet, InteractionState{}, Environment{
		ViewportWidth: 800, ViewportHeight: 600, RootFontSize: 16,
	}).For(paragraph)
	if computed.FontSize != 20 {
		t.Fatalf("calculated font size = %v, want 20", computed.FontSize)
	}
	if got, want := computed.Margin, (Edges{Top: 78, Right: 6, Bottom: 78, Left: 6}); got != want {
		t.Fatalf("calculated margin = %#v, want %#v", got, want)
	}
	if computed.Padding != (Edges{Top: 10, Right: 10, Bottom: 10, Left: 10}) {
		t.Fatalf("calculated padding = %#v", computed.Padding)
	}
}

func TestComputeResolvesCurrentColorAndAlpha(t *testing.T) {
	document := dom.NewDocument()
	parent := document.CreateElement("div", map[string]string{"class": "parent"})
	child := document.CreateElement("p", map[string]string{"class": "child"})
	appendNode(t, document, document.Root, parent)
	appendNode(t, document, parent, child)
	stylesheet, err := css.Parse(strings.NewReader(`
.parent { color: rgba(10, 20, 30, 0.5); }
.child { color: currentColor; background-color: currentColor; }
`))
	if err != nil {
		t.Fatal(err)
	}
	computed, _ := Compute(document, stylesheet).For(child)
	if computed.Color != 0x0a141e80 || computed.BackgroundColor != computed.Color {
		t.Fatalf("currentColor style = %#v", computed)
	}
}

func TestComputeEvaluatesMediaQueries(t *testing.T) {
	document := dom.NewDocument()
	paragraph := document.CreateElement("p", nil)
	appendNode(t, document, document.Root, paragraph)
	stylesheet, err := css.Parse(strings.NewReader(`
p { color: black; }
@media screen and (min-width: 700px) and (max-height: 700px) {
  p { color: red; }
}
@media (orientation: landscape) and (min-resolution: 2dppx) {
  p { font-size: 20px; }
}
@media (prefers-color-scheme: dark) and (hover: hover) and (pointer: fine) {
  p { background-color: blue; }
}
@media only screen, print { p { padding: 4px; } }
@media print { p { color: green !important; } }
`))
	if err != nil {
		t.Fatal(err)
	}
	matched, _ := ComputeWithEnvironment(document, stylesheet, InteractionState{}, Environment{
		ViewportWidth: 800, ViewportHeight: 600, RootFontSize: 16,
		ResolutionDPI: 192, ColorScheme: "dark", Hover: true, Pointer: "fine",
	}).For(paragraph)
	if matched.Color != 0xff0000ff || matched.FontSize != 20 || matched.BackgroundColor != 0x0000ffff || matched.Padding.Top != 4 {
		t.Fatalf("matched media style = %#v", matched)
	}
	unmatched, _ := ComputeWithEnvironment(document, stylesheet, InteractionState{}, Environment{
		ViewportWidth: 500, ViewportHeight: 700, RootFontSize: 16,
		ResolutionDPI: 96, ColorScheme: "light", Hover: false, Pointer: "coarse",
	}).For(paragraph)
	if unmatched.Color != 0x000000ff || unmatched.FontSize == 20 || unmatched.BackgroundColor == 0x0000ffff {
		t.Fatalf("unmatched media style = %#v", unmatched)
	}
}

func TestComputeResolvesSizingAndBoxSizing(t *testing.T) {
	document := dom.NewDocument()
	box := document.CreateElement("div", nil)
	appendNode(t, document, document.Root, box)
	stylesheet, err := css.Parse(strings.NewReader(`
div {
  display: inline-block;
  width: 50%; height: 10vh;
  min-width: 200px; min-height: 40px;
  max-width: calc(100% - 20px); max-height: none;
  box-sizing: border-box;
}
`))
	if err != nil {
		t.Fatal(err)
	}
	computed, _ := ComputeWithEnvironment(document, stylesheet, InteractionState{}, Environment{
		ViewportWidth: 800, ViewportHeight: 600, RootFontSize: 16,
	}).For(box)
	if computed.Display != DisplayInlineBlock || computed.BoxSizing != BoxSizingBorderBox {
		t.Fatalf("display/box sizing = %#v", computed)
	}
	if computed.Width.Value.Percentage != 50 || computed.Height.Value.Pixels != 60 ||
		computed.MinWidth.Value.Pixels != 200 || computed.MaxWidth.Value.Percentage != 100 || computed.MaxWidth.Value.Pixels != -20 ||
		computed.MaxHeight.Kind != SizeNone {
		t.Fatalf("computed sizes = %#v", computed)
	}
}

func TestComputeExpandsBorderShorthands(t *testing.T) {
	document := dom.NewDocument()
	box := document.CreateElement("div", nil)
	appendNode(t, document, document.Root, box)
	stylesheet, err := css.Parse(strings.NewReader(`
div {
  color: blue;
  border: 2px solid currentColor;
  border-width: 1px 2px 3px 4px;
  border-right: thick dashed red;
  border-bottom-color: rgba(0, 255, 0, .5);
}
`))
	if err != nil {
		t.Fatal(err)
	}
	computed, _ := Compute(document, stylesheet).For(box)
	if computed.Border.Top != (BorderSide{Width: 1, Style: BorderSolid, Color: 0x0000ffff}) ||
		computed.Border.Right != (BorderSide{Width: 5, Style: BorderDashed, Color: 0xff0000ff}) ||
		computed.Border.Bottom != (BorderSide{Width: 3, Style: BorderSolid, Color: 0x00ff0080}) ||
		computed.Border.Left != (BorderSide{Width: 4, Style: BorderSolid, Color: 0x0000ffff}) {
		t.Fatalf("computed borders = %#v", computed.Border)
	}
}

func TestComputeResolvesLineHeightAndWhiteSpace(t *testing.T) {
	document := dom.NewDocument()
	paragraph := document.CreateElement("p", nil)
	appendNode(t, document, document.Root, paragraph)
	stylesheet, err := css.Parse(strings.NewReader(`p { font-size: 20px; line-height: 1.5; white-space: pre-wrap; }`))
	if err != nil {
		t.Fatal(err)
	}
	computed, _ := Compute(document, stylesheet).For(paragraph)
	if computed.LineHeight != 30 || computed.WhiteSpace != WhiteSpacePreWrap {
		t.Fatalf("text style = %#v", computed)
	}
}

func TestComputeResolvesOverflowShorthandAndAxes(t *testing.T) {
	document := dom.NewDocument()
	box := document.CreateElement("div", nil)
	appendNode(t, document, document.Root, box)
	stylesheet, err := css.Parse(strings.NewReader(`div { overflow: hidden; overflow-x: scroll; }`))
	if err != nil {
		t.Fatal(err)
	}
	computed, _ := Compute(document, stylesheet).For(box)
	if computed.OverflowX != OverflowScroll || computed.OverflowY != OverflowHidden {
		t.Fatalf("overflow = (%v, %v)", computed.OverflowX, computed.OverflowY)
	}
}

func TestComputeResolvesSingleBackgroundLayerProperties(t *testing.T) {
	document := dom.NewDocument()
	gradient := document.CreateElement("div", map[string]string{"class": "gradient"})
	image := document.CreateElement("div", map[string]string{"class": "image"})
	appendNode(t, document, document.Root, gradient)
	appendNode(t, document, document.Root, image)
	stylesheet, err := css.Parse(strings.NewReader(`
.gradient {
  background-image: linear-gradient(to right, red 20%, rgba(0, 0, 255, .5));
  background-repeat: no-repeat;
  background-position: right bottom;
  background-size: 50% auto;
}
.image { background-image: url("images/card.png"); background-repeat: repeat-x; }
`))
	if err != nil {
		t.Fatal(err)
	}
	computed := Compute(document, stylesheet)
	gradientStyle, _ := computed.For(gradient)
	if gradientStyle.BackgroundImage.Kind != BackgroundImageLinearGradient || gradientStyle.BackgroundImage.GradientAngle != 90 {
		t.Fatalf("gradient = %#v", gradientStyle.BackgroundImage)
	}
	stops := gradientStyle.BackgroundImage.GradientStops
	if len(stops) != 2 || stops[0] != (GradientStop{Color: 0xff0000ff, Position: .2}) || stops[1] != (GradientStop{Color: 0x0000ff80, Position: 1}) {
		t.Fatalf("gradient stops = %#v", stops)
	}
	if gradientStyle.BackgroundRepeat.X || gradientStyle.BackgroundRepeat.Y || gradientStyle.BackgroundPos.X.Percentage != 100 || gradientStyle.BackgroundPos.Y.Percentage != 100 {
		t.Fatalf("background repeat/position = %#v / %#v", gradientStyle.BackgroundRepeat, gradientStyle.BackgroundPos)
	}
	if gradientStyle.BackgroundSize.Kind != BackgroundSizeExplicit || gradientStyle.BackgroundSize.Width.Value.Percentage != 50 || gradientStyle.BackgroundSize.Height.Kind != SizeAuto {
		t.Fatalf("background size = %#v", gradientStyle.BackgroundSize)
	}
	imageStyle, _ := computed.For(image)
	if imageStyle.BackgroundImage.Kind != BackgroundImageURL || imageStyle.BackgroundImage.URL != "images/card.png" || !imageStyle.BackgroundRepeat.X || imageStyle.BackgroundRepeat.Y {
		t.Fatalf("image background = %#v / %#v", imageStyle.BackgroundImage, imageStyle.BackgroundRepeat)
	}
}

func TestComputeResolvesRadiusDecorationAndOpacity(t *testing.T) {
	document := dom.NewDocument()
	box := document.CreateElement("div", nil)
	appendNode(t, document, document.Root, box)
	stylesheet, err := css.Parse(strings.NewReader(`
div {
  color: #123456;
  border-radius: 10px 20% 30px / 4px 8px;
  border-bottom-left-radius: 7px 9px;
  text-decoration-line: underline line-through;
  text-decoration-color: rgba(255, 0, 0, .5);
  opacity: .4;
}`))
	if err != nil {
		t.Fatal(err)
	}
	computed, _ := Compute(document, stylesheet).For(box)
	if computed.BorderRadius.TopLeft.X.Pixels != 10 || computed.BorderRadius.TopLeft.Y.Pixels != 4 ||
		computed.BorderRadius.TopRight.X.Percentage != 20 || computed.BorderRadius.BottomRight.X.Pixels != 30 ||
		computed.BorderRadius.BottomLeft.X.Pixels != 7 || computed.BorderRadius.BottomLeft.Y.Pixels != 9 {
		t.Fatalf("border radius = %#v", computed.BorderRadius)
	}
	if computed.TextDecoration != TextDecorationUnderline|TextDecorationLineThrough || computed.DecorationColor != 0xff000080 || computed.Opacity != .4 {
		t.Fatalf("decoration/opacity = %v / %#x / %v", computed.TextDecoration, computed.DecorationColor, computed.Opacity)
	}
}

func TestComputeResolvesFlexContainerAndItemProperties(t *testing.T) {
	document := dom.NewDocument()
	container := document.CreateElement("section", nil)
	item := document.CreateElement("article", nil)
	appendNode(t, document, document.Root, container)
	appendNode(t, document, container, item)
	stylesheet, err := css.Parse(strings.NewReader(`
section {
  display: inline-flex;
  flex-flow: column-reverse wrap-reverse;
  justify-content: space-evenly;
  align-items: baseline;
  align-content: space-around;
  gap: 12px 5%;
}
article {
  order: -2;
  flex: 2 3 content;
  align-self: center;
  margin: auto 4px 6px auto;
  aspect-ratio: 16 / 9;
}`))
	if err != nil {
		t.Fatal(err)
	}
	computed := Compute(document, stylesheet)
	containerStyle, _ := computed.For(container)
	if containerStyle.Display != DisplayInlineFlex || containerStyle.FlexDirection != FlexDirectionColumnReverse || containerStyle.FlexWrap != FlexWrapReverse {
		t.Fatalf("flex container model = %#v", containerStyle)
	}
	if containerStyle.JustifyContent != JustifySpaceEvenly || containerStyle.AlignItems != AlignBaseline || containerStyle.AlignContent != AlignSpaceAround {
		t.Fatalf("flex alignment = %v / %v / %v", containerStyle.JustifyContent, containerStyle.AlignItems, containerStyle.AlignContent)
	}
	if containerStyle.RowGap.Pixels != 12 || containerStyle.ColumnGap.Percentage != 5 {
		t.Fatalf("flex gap = %#v / %#v", containerStyle.RowGap, containerStyle.ColumnGap)
	}
	itemStyle, _ := computed.For(item)
	if itemStyle.Order != -2 || itemStyle.FlexGrow != 2 || itemStyle.FlexShrink != 3 || itemStyle.FlexBasis.Kind != FlexBasisContent {
		t.Fatalf("flex item sizing = %#v", itemStyle)
	}
	if itemStyle.AlignSelf != AlignCenter || !itemStyle.MarginAuto.Top || !itemStyle.MarginAuto.Left || itemStyle.Margin.Right != 4 || itemStyle.Margin.Bottom != 6 {
		t.Fatalf("flex item alignment/margin = %#v", itemStyle)
	}
	if itemStyle.AspectRatio != float32(16.0/9.0) {
		t.Fatalf("aspect ratio = %v", itemStyle.AspectRatio)
	}
}

func TestComputeUsesFlexInitialValuesAndLonghands(t *testing.T) {
	document := dom.NewDocument()
	item := document.CreateElement("div", map[string]string{"style": `display:flex; flex-grow:1.5; flex-shrink:0; flex-basis:25%; row-gap:normal; column-gap:8px`})
	appendNode(t, document, document.Root, item)
	computed, _ := Compute(document, nil).For(item)
	if computed.Display != DisplayFlex || computed.FlexDirection != FlexDirectionRow || computed.FlexWrap != FlexNoWrap {
		t.Fatalf("flex initial container values = %#v", computed)
	}
	if computed.FlexGrow != 1.5 || computed.FlexShrink != 0 || computed.FlexBasis.Kind != FlexBasisLength || computed.FlexBasis.Value.Percentage != 25 {
		t.Fatalf("flex longhands = %#v", computed)
	}
	if computed.RowGap != (LengthPercentage{}) || computed.ColumnGap.Pixels != 8 || computed.AlignSelf != AlignAuto {
		t.Fatalf("gap/alignment initial values = %#v", computed)
	}
	if computed.MinWidth.Kind != SizeAuto || computed.MinHeight.Kind != SizeAuto {
		t.Fatalf("automatic minimum sizes = %#v / %#v", computed.MinWidth, computed.MinHeight)
	}
}

func TestComputeGridDisplayValues(t *testing.T) {
	document := dom.NewDocument()
	grid := document.CreateElement("div", map[string]string{"class": "grid"})
	inlineGrid := document.CreateElement("span", map[string]string{"class": "inline-grid"})
	appendNode(t, document, document.Root, grid)
	appendNode(t, document, document.Root, inlineGrid)
	stylesheet, err := css.Parse(strings.NewReader(`.grid { display:grid } .inline-grid { display:inline-grid }`))
	if err != nil {
		t.Fatal(err)
	}
	computed := Compute(document, stylesheet)
	gridStyle, _ := computed.For(grid)
	inlineGridStyle, _ := computed.For(inlineGrid)
	if gridStyle.Display != DisplayGrid || inlineGridStyle.Display != DisplayInlineGrid {
		t.Fatalf("grid displays = (%v, %v)", gridStyle.Display, inlineGridStyle.Display)
	}
}

func TestComputeGridExplicitAndImplicitTracks(t *testing.T) {
	document := dom.NewDocument()
	grid := document.CreateElement("div", map[string]string{"style": "display:grid; grid-template-columns:100px 25%; grid-template-rows:30px; grid-auto-columns:12px; grid-auto-rows:40px 50px"})
	appendNode(t, document, document.Root, grid)
	computed, _ := Compute(document, nil).For(grid)
	if len(computed.GridTemplateColumns) != 2 || computed.GridTemplateColumns[0].Value.Pixels != 100 || computed.GridTemplateColumns[1].Value.Percentage != 25 {
		t.Fatalf("explicit columns = %#v", computed.GridTemplateColumns)
	}
	if len(computed.GridTemplateRows) != 1 || computed.GridTemplateRows[0].Value.Pixels != 30 {
		t.Fatalf("explicit rows = %#v", computed.GridTemplateRows)
	}
	if len(computed.GridAutoColumns) != 1 || len(computed.GridAutoRows) != 2 || computed.GridAutoRows[1].Value.Pixels != 50 {
		t.Fatalf("implicit track patterns = columns %#v rows %#v", computed.GridAutoColumns, computed.GridAutoRows)
	}
}

func TestComputeGridIntrinsicAndFlexibleTracks(t *testing.T) {
	document := dom.NewDocument()
	grid := document.CreateElement("div", map[string]string{"style": "display:grid; grid-template-columns:min-content max-content 2fr"})
	appendNode(t, document, document.Root, grid)
	computed, _ := Compute(document, nil).For(grid)
	tracks := computed.GridTemplateColumns
	if len(tracks) != 3 || tracks[0].Kind != GridTrackMinContent || tracks[1].Kind != GridTrackMaxContent || tracks[2].Kind != GridTrackFraction || tracks[2].Flex != 2 {
		t.Fatalf("intrinsic/flexible tracks = %#v", tracks)
	}
}

func TestComputeGridMinmaxFitContentAndRepeat(t *testing.T) {
	document := dom.NewDocument()
	grid := document.CreateElement("div", map[string]string{"style": "display:grid; grid-template-columns:repeat(2, 40px) minmax(60px, 1fr) fit-content(80px)"})
	appendNode(t, document, document.Root, grid)
	computed, _ := Compute(document, nil).For(grid)
	tracks := computed.GridTemplateColumns
	if len(tracks) != 4 || tracks[0].Value.Pixels != 40 || tracks[1].Value.Pixels != 40 {
		t.Fatalf("fixed repeat expansion = %#v", tracks)
	}
	if tracks[2].Kind != GridTrackFraction || tracks[2].Flex != 1 || !tracks[2].MinSet || tracks[2].MinKind != GridTrackLength || tracks[2].MinValue.Pixels != 60 {
		t.Fatalf("minmax track = %#v", tracks[2])
	}
	if tracks[3].Kind != GridTrackMaxContent || tracks[3].FitLimit == nil || tracks[3].FitLimit.Pixels != 80 {
		t.Fatalf("fit-content track = %#v", tracks[3])
	}
}

func TestComputeGridNamedLinesSpanAndAreas(t *testing.T) {
	document := dom.NewDocument()
	grid := document.CreateElement("div", map[string]string{"style": `display:grid; grid-template-columns:[left] 100px [middle] 100px [right]; grid-template-areas:"hero hero" "side main"`})
	item := document.CreateElement("div", map[string]string{"style": "grid-column:left / span 2; grid-row:1 / 2; grid-area:hero"})
	appendNode(t, document, document.Root, grid)
	appendNode(t, document, grid, item)
	computed := Compute(document, nil)
	containerStyle, _ := computed.For(grid)
	itemStyle, _ := computed.For(item)
	if len(containerStyle.GridColumnLines["left"]) != 1 || containerStyle.GridColumnLines["left"][0] != 0 || containerStyle.GridColumnLines["right"][0] != 2 {
		t.Fatalf("named lines = %#v", containerStyle.GridColumnLines)
	}
	if area := containerStyle.GridTemplateAreas["main"]; area != (GridArea{RowStart: 1, RowEnd: 2, ColumnStart: 1, ColumnEnd: 2}) {
		t.Fatalf("template area = %#v", area)
	}
	if itemStyle.GridColumn.Start.Name != "left" || itemStyle.GridColumn.End.Span != 2 || itemStyle.GridAreaName != "hero" {
		t.Fatalf("item placement = %#v / %q", itemStyle.GridColumn, itemStyle.GridAreaName)
	}
}

func TestComputeGridAutoFlowModes(t *testing.T) {
	document := dom.NewDocument()
	row := document.CreateElement("div", map[string]string{"style": "display:grid; grid-auto-flow:row"})
	columnDense := document.CreateElement("div", map[string]string{"style": "display:grid; grid-auto-flow:column dense"})
	appendNode(t, document, document.Root, row)
	appendNode(t, document, document.Root, columnDense)
	computed := Compute(document, nil)
	rowStyle, _ := computed.For(row)
	columnStyle, _ := computed.For(columnDense)
	if rowStyle.GridAutoFlow.Column || rowStyle.GridAutoFlow.Dense || !columnStyle.GridAutoFlow.Column || !columnStyle.GridAutoFlow.Dense {
		t.Fatalf("auto-flow modes = %#v / %#v", rowStyle.GridAutoFlow, columnStyle.GridAutoFlow)
	}
}

func TestComputeGridPlaceShorthands(t *testing.T) {
	document := dom.NewDocument()
	container := document.CreateElement("div", map[string]string{"style": "display:grid; place-content:flex-end center; place-items:center flex-end"})
	item := document.CreateElement("div", map[string]string{"style": "place-self:flex-start center"})
	appendNode(t, document, document.Root, container)
	appendNode(t, document, container, item)
	computed := Compute(document, nil)
	containerStyle, _ := computed.For(container)
	itemStyle, _ := computed.For(item)
	if containerStyle.AlignContent != AlignFlexEnd || containerStyle.JustifyContent != JustifyCenter || containerStyle.AlignItems != AlignCenter || containerStyle.JustifyItems != AlignFlexEnd {
		t.Fatalf("container place shorthands = %#v", containerStyle)
	}
	if itemStyle.AlignSelf != AlignFlexStart || itemStyle.JustifySelf != AlignCenter {
		t.Fatalf("item place-self = %#v", itemStyle)
	}
}

func parseTestSelector(t *testing.T, value string) css.Selector {
	t.Helper()
	stylesheet, err := css.Parse(strings.NewReader(value + " { color: red }"))
	if err != nil {
		t.Fatal(err)
	}
	if len(stylesheet.Rules) != 1 || len(stylesheet.Rules[0].Selectors) != 1 {
		t.Fatalf("selector %q was not parsed", value)
	}
	return stylesheet.Rules[0].Selectors[0]
}

func appendNode(t *testing.T, document *dom.Document, parent, child *dom.Node) {
	t.Helper()
	if err := document.AppendChild(parent, child); err != nil {
		t.Fatal(err)
	}
}
