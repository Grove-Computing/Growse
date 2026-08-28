package style

import (
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/css"
	"github.com/Grove-Computing/Growse/internal/dom"
)

func TestNestedSelectorsParticipateInCascadeAndMedia(t *testing.T) {
	document := dom.NewDocument()
	card := document.CreateElement("section", map[string]string{"class": "card"})
	title := document.CreateElement("h2", map[string]string{"class": "title"})
	badge := document.CreateElement("span", map[string]string{"class": "badge"})
	appendNode(t, document, document.Root, card)
	appendNode(t, document, card, title)
	appendNode(t, document, title, badge)
	stylesheet, err := css.Parse(strings.NewReader(`
.card {
  & > .title { color: blue }
  @media (min-width: 600px) { & .badge { background-color: green } }
  @supports (display: grid) { .badge { display: grid } }
}
`))
	if err != nil {
		t.Fatal(err)
	}
	computed := ComputeWithEnvironment(document, stylesheet, InteractionState{}, Environment{ViewportWidth: 800, ViewportHeight: 600})
	titleStyle, _ := computed.For(title)
	badgeStyle, _ := computed.For(badge)
	if titleStyle.Color != 0x0000ffff || badgeStyle.BackgroundColor != 0x008000ff || badgeStyle.Display != DisplayGrid {
		t.Fatalf("nested computed styles = title:%#v badge:%#v", titleStyle, badgeStyle)
	}
}
