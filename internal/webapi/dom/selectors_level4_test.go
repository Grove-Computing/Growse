package dom

import (
	"testing"

	dommodel "github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/events"
)

func TestElementSelectorAPIsUseLevel4SelectorsAndScopedQueries(t *testing.T) {
	document := dommodel.NewDocument()
	main := document.CreateElement("main", map[string]string{"id": "app"})
	card := document.CreateElement("article", map[string]string{"class": "card featured"})
	badge := document.CreateElement("span", map[string]string{"class": "badge"})
	for _, edge := range [][2]*dommodel.Node{{document.Root, main}, {main, card}, {card, badge}} {
		if err := document.AppendChild(edge[0], edge[1]); err != nil {
			t.Fatal(err)
		}
	}
	api := New(document, events.NewDispatcher(), nil)
	app := api.GetElementByID("app")
	cardElement := app.QuerySelector(":scope > .card")
	if cardElement == nil || !cardElement.Matches(".card:is(.featured, :future):has(> .badge)") {
		t.Fatalf("scoped Level 4 query = %#v", cardElement)
	}
	if scoped := api.QuerySelector(":scope"); scoped == nil || scoped.ID() != app.ID() {
		t.Fatalf("document :scope = %#v", scoped)
	}
	if len(app.QuerySelectorAll(":scope .badge")) != 1 {
		t.Fatal("scoped descendant query did not match badge")
	}
}
