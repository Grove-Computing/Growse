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

func appendNode(t *testing.T, document *dom.Document, parent, child *dom.Node) {
	t.Helper()
	if err := document.AppendChild(parent, child); err != nil {
		t.Fatal(err)
	}
}
