package layout

import (
	"strings"
	"testing"

	"github.com/saku0512/growse/internal/css"
	"github.com/saku0512/growse/internal/dom"
	stylemodel "github.com/saku0512/growse/internal/style"
)

func TestBuildGridEstablishesFormattingContexts(t *testing.T) {
	document := dom.NewDocument()
	grid := document.CreateElement("div", map[string]string{"class": "grid"})
	first := document.CreateElement("span", map[string]string{"class": "item"})
	second := document.CreateElement("span", map[string]string{"class": "item"})
	paragraph := document.CreateElement("p", nil)
	inlineGrid := document.CreateElement("span", map[string]string{"class": "inline-grid"})
	inlineItem := document.CreateElement("span", map[string]string{"class": "inline-item"})
	appendNodes(t, document,
		[2]*dom.Node{document.Root, grid},
		[2]*dom.Node{grid, first}, [2]*dom.Node{first, document.CreateText("first")},
		[2]*dom.Node{grid, second}, [2]*dom.Node{second, document.CreateText("second")},
		[2]*dom.Node{document.Root, paragraph}, [2]*dom.Node{paragraph, document.CreateText("before ")},
		[2]*dom.Node{paragraph, inlineGrid}, [2]*dom.Node{inlineGrid, inlineItem},
		[2]*dom.Node{inlineItem, document.CreateText("grid")}, [2]*dom.Node{paragraph, document.CreateText(" after")},
	)
	stylesheet, err := css.Parse(strings.NewReader(`
.grid { display:grid; width:240px }
.item { display:inline; height:20px; background-color:#ddd }
.inline-grid { display:inline-grid }
.inline-item { height:20px; background-color:#ccc }
`))
	if err != nil {
		t.Fatal(err)
	}
	tree := Build(document, stylemodel.Compute(document, stylesheet), 500)
	firstRect, secondRect := decorationForNode(t, tree, first.ID), decorationForNode(t, tree, second.ID)
	if firstRect.Width != 240 || secondRect.Width != 240 || secondRect.Y <= firstRect.Y {
		t.Fatalf("grid items were not independently blockified: first=%#v second=%#v", firstRect.Rect, secondRect.Rect)
	}
	line := boxForNode(t, tree, paragraph.ID)
	inlineRect := decorationForNode(t, tree, inlineItem.ID)
	if inlineRect.X <= line.X || inlineRect.Y < line.Y || inlineRect.Y >= line.Y+line.Height {
		t.Fatalf("inline-grid did not participate atomically in inline flow: line=(%v,%v,%v,%v) item=%#v", line.X, line.Y, line.Width, line.Height, inlineRect.Rect)
	}
}
