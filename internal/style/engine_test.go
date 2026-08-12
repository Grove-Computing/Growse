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

func appendNode(t *testing.T, document *dom.Document, parent, child *dom.Node) {
	t.Helper()
	if err := document.AppendChild(parent, child); err != nil {
		t.Fatal(err)
	}
}
