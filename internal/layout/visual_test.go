package layout

import (
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/css"
	"github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/style"
)

func TestListMarkersControlsAndBoundedEffectsReachLayout(t *testing.T) {
	document := dom.NewDocument()
	list := document.CreateElement("ol", nil)
	first, second := document.CreateElement("li", nil), document.CreateElement("li", nil)
	card := document.CreateElement("div", map[string]string{"class": "card"})
	check := document.CreateElement("input", map[string]string{"type": "checkbox", "checked": "", "class": "check"})
	huge := document.CreateElement("div", map[string]string{"class": "huge"})
	appendNodes(t, document,
		[2]*dom.Node{document.Root, list}, [2]*dom.Node{list, first}, [2]*dom.Node{first, document.CreateText("First")},
		[2]*dom.Node{list, second}, [2]*dom.Node{second, document.CreateText("Second")},
		[2]*dom.Node{document.Root, card}, [2]*dom.Node{document.Root, check}, [2]*dom.Node{document.Root, huge},
	)
	stylesheet, err := css.Parse(strings.NewReader(`
.card { width:100px; height:50px; background:red; filter:brightness(2) blur(4px); backdrop-filter:contrast(.8); mix-blend-mode:multiply; cursor:pointer }
.check { appearance:none; accent-color:#12ab34; cursor:crosshair }
.huge { width:5000px; height:5000px; filter:blur(5px); background:blue }
`))
	if err != nil {
		t.Fatal(err)
	}
	tree := Build(document, style.Compute(document, stylesheet), 6000)
	var firstText, secondText string
	var checkBox Box
	for _, box := range tree.Boxes {
		switch box.NodeID {
		case first.ID:
			for _, run := range box.Runs {
				firstText += run.Text
			}
		case second.ID:
			for _, run := range box.Runs {
				secondText += run.Text
			}
		case check.ID:
			checkBox = box
		}
	}
	if firstText != "1. First" || secondText != "2. Second" {
		t.Fatalf("ordered markers = %q / %q", firstText, secondText)
	}
	if !checkBox.Checkable || checkBox.Appearance != style.AppearanceNone || checkBox.AccentColor != 0x12ab34ff || checkBox.Cursor != style.CursorCrosshair {
		t.Fatalf("control visual state = %#v", checkBox)
	}
	cardDecoration, hugeDecoration := decorationForNode(t, tree, card.ID), decorationForNode(t, tree, huge.ID)
	if len(cardDecoration.Filters) != 2 || len(cardDecoration.BackdropFilters) != 1 || cardDecoration.BlendMode != style.BlendMultiply || cardDecoration.Cursor != style.CursorPointer {
		t.Fatalf("card effects = %#v", cardDecoration)
	}
	if len(hugeDecoration.Filters) != 0 || hugeDecoration.BlendMode != style.BlendNormal {
		t.Fatalf("unbounded effects were retained: %#v", hugeDecoration)
	}
}
