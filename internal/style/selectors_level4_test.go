package style

import (
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/css"
	"github.com/Grove-Computing/Growse/internal/dom"
)

func TestLevel4SelectorMatchingHasScopeIsWhereAndComplexNot(t *testing.T) {
	document := dom.NewDocument()
	main := document.CreateElement("main", map[string]string{"id": "app"})
	card := document.CreateElement("article", map[string]string{"class": "card featured"})
	badge := document.CreateElement("span", map[string]string{"class": "badge"})
	sibling := document.CreateElement("aside", map[string]string{"class": "next"})
	appendNode(t, document, document.Root, main)
	appendNode(t, document, main, card)
	appendNode(t, document, card, badge)
	appendNode(t, document, main, sibling)
	stylesheet, err := css.Parse(strings.NewReader(`
:scope { background-color: blue }
.card:is(.featured, :future-pseudo):has(> .badge) { color: green }
.card:not(.draft > span, #archived) { font-size: 20px }
.card:where(#impossible, .featured) { width: 120px }
.card:has(+ .next) { height: 30px }
`))
	if err != nil {
		t.Fatal(err)
	}
	computed := Compute(document, stylesheet)
	mainStyle, _ := computed.For(main)
	cardStyle, _ := computed.For(card)
	if mainStyle.BackgroundColor != 0x0000ffff {
		t.Fatalf(":scope style = %#08x", mainStyle.BackgroundColor)
	}
	if cardStyle.Color != 0x008000ff || cardStyle.FontSize != 20 || cardStyle.Width.Value.Pixels != 120 || cardStyle.Height.Value.Pixels != 30 {
		t.Fatalf("Level 4 matched style = %#v", cardStyle)
	}
}
