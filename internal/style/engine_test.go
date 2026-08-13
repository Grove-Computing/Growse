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
