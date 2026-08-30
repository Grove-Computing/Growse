package paint

import (
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/css"
	"github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/layout"
	"github.com/Grove-Computing/Growse/internal/style"
)

func TestBrowserVisualPropertiesReachDisplayListAndStacking(t *testing.T) {
	document := dom.NewDocument()
	card := document.CreateElement("div", map[string]string{"class": "card", "data-mode": "dark"})
	if err := document.AppendChild(document.Root, card); err != nil {
		t.Fatal(err)
	}
	if err := document.AppendChild(card, document.CreateText("content")); err != nil {
		t.Fatal(err)
	}
	stylesheet, err := css.Parse(strings.NewReader(`
.card[data-mode="DARK" i] {
  display:block; width:120px; height:60px; padding:10px; overflow:hidden;
  background-image:conic-gradient(from 30deg, red, blue);
  background-origin:content-box; background-clip:padding-box;
  filter:brightness(120%); backdrop-filter:contrast(80%); opacity:.8; transform:translateX(2px);
}
.card::before { content:"before "; }
`))
	if err != nil {
		t.Fatal(err)
	}
	tree := layout.Build(document, style.Compute(document, stylesheet), 300)
	list := Build(tree)
	if len(list.CompositingLayers) < 2 {
		t.Fatalf("visual effects did not create a stacking layer: %#v", list.CompositingLayers)
	}
	foundBox, foundGenerated := false, false
	for _, command := range list.Commands {
		switch typed := command.(type) {
		case DrawBox:
			if typed.NodeID == card.ID {
				foundBox = len(typed.Layers) == 1 && typed.Layers[0].Image.Kind == style.BackgroundImageConicGradient &&
					typed.Layers[0].Origin == style.BackgroundBoxContent && typed.Layers[0].Clip == style.BackgroundBoxPadding &&
					typed.Padding.Left == 10 && len(typed.Filters) == 1 && len(typed.BackdropFilters) == 1
			}
		case DrawText:
			if typed.NodeID == card.ID && strings.HasPrefix(typed.Text, "before ") {
				foundGenerated = true
			}
		}
	}
	if !foundBox || !foundGenerated {
		t.Fatalf("visual display list box/generated = %v/%v: %#v", foundBox, foundGenerated, list.Commands)
	}
}
