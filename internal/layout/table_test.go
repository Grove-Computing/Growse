package layout_test

import (
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/css"
	"github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/layout"
	"github.com/Grove-Computing/Growse/internal/paint"
	"github.com/Grove-Computing/Growse/internal/style"
)

func TestTableFormattingAndDisplayContentsSharePaintAndHitGeometry(t *testing.T) {
	document := dom.NewDocument()
	table := document.CreateElement("table", map[string]string{"class": "matrix"})
	tbody := document.CreateElement("tbody", nil)
	firstRow := document.CreateElement("tr", nil)
	secondRow := document.CreateElement("tr", nil)
	wrapper := document.CreateElement("span", map[string]string{"class": "contents"})
	wide := document.CreateElement("td", map[string]string{"id": "wide", "colspan": "2"})
	tall := document.CreateElement("td", map[string]string{"id": "tall", "rowspan": "2"})
	left := document.CreateElement("td", map[string]string{"id": "left"})
	right := document.CreateElement("td", map[string]string{"id": "right"})
	appendTableNodes(t, document,
		[2]*dom.Node{document.Root, table}, [2]*dom.Node{table, tbody},
		[2]*dom.Node{tbody, firstRow}, [2]*dom.Node{firstRow, wrapper},
		[2]*dom.Node{wrapper, wide}, [2]*dom.Node{wide, document.CreateText("wide heading")},
		[2]*dom.Node{firstRow, tall}, [2]*dom.Node{tall, document.CreateText("two rows")},
		[2]*dom.Node{tbody, secondRow}, [2]*dom.Node{secondRow, left},
		[2]*dom.Node{left, document.CreateText("left")}, [2]*dom.Node{secondRow, right},
		[2]*dom.Node{right, document.CreateText("right")},
	)
	stylesheet, err := css.Parse(strings.NewReader(`
.matrix { width: 360px; background:#eee; border:1px solid #111 }
.contents { display:contents }
td { background:#def; border:1px solid #369; padding:4px }
`))
	if err != nil {
		t.Fatal(err)
	}
	computed := style.ComputeWithEnvironment(document, stylesheet, style.InteractionState{}, style.Environment{BrowserDefaults: true})
	tree := layout.Build(document, computed, 500)

	wideRect, wideOK := tree.Bounds[wide.ID]
	tallRect, tallOK := tree.Bounds[tall.ID]
	leftRect, leftOK := tree.Bounds[left.ID]
	rightRect, rightOK := tree.Bounds[right.ID]
	if !wideOK || !tallOK || !leftOK || !rightOK {
		t.Fatalf("table cell bounds missing: %#v", tree.Bounds)
	}
	if wideRect.Width <= leftRect.Width || tallRect.Height <= leftRect.Height || rightRect.X <= leftRect.X {
		t.Fatalf("span geometry wide=%#v tall=%#v left=%#v right=%#v", wideRect, tallRect, leftRect, rightRect)
	}
	if _, exists := tree.Bounds[wrapper.ID]; exists {
		t.Fatal("display:contents generated a principal layout box")
	}
	if hit, ok := layout.HitTest(tree, rightRect.X+2, rightRect.Y+2); !ok || hit != right.ID {
		t.Fatalf("table hit = (%d, %v), want right cell %d", hit, ok, right.ID)
	}
	displayList := paint.Build(tree)
	foundRight := false
	for _, command := range displayList.Commands {
		if box, ok := command.(paint.DrawBox); ok && box.NodeID == right.ID {
			foundRight = true
			break
		}
	}
	if !foundRight {
		t.Fatal("right cell geometry was not connected to the display list")
	}
}

func appendTableNodes(t *testing.T, document *dom.Document, edges ...[2]*dom.Node) {
	t.Helper()
	for _, edge := range edges {
		if err := document.AppendChild(edge[0], edge[1]); err != nil {
			t.Fatal(err)
		}
	}
}
