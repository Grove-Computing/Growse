package layout

import (
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/css"
	"github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/style"
)

func TestStableFragmentsReuseUnrelatedSubtreeStorage(t *testing.T) {
	document := dom.NewDocument()
	first := document.CreateElement("p", map[string]string{"id": "first"})
	second := document.CreateElement("p", map[string]string{"id": "second"})
	for _, edge := range [][2]*dom.Node{{document.Root, first}, {first, document.CreateText("first")}, {document.Root, second}, {second, document.CreateText("second")}} {
		if err := document.AppendChild(edge[0], edge[1]); err != nil {
			t.Fatal(err)
		}
	}
	stylesheet, err := css.Parse(strings.NewReader(`#first { width: 100px; } #second { color: blue; }`))
	if err != nil {
		t.Fatal(err)
	}
	previous := Build(document, style.Compute(document, stylesheet), 800)
	current := Build(document, style.Compute(document, stylesheet), 800)
	if previous.Boxes[0].FragmentID == 0 || previous.Boxes[0].FragmentID != current.Boxes[0].FragmentID {
		t.Fatalf("fragment identity was not stable: %d / %d", previous.Boxes[0].FragmentID, current.Boxes[0].FragmentID)
	}
	reused := ReuseStableFragments(previous, current, map[dom.NodeID]bool{first.ID: true})
	if reused == 0 {
		t.Fatal("unrelated subtree fragments were not reused")
	}
	for index := range current.Boxes {
		if current.Boxes[index].NodeID == second.ID && len(current.Boxes[index].Runs) != 0 && &current.Boxes[index].Runs[0] != &previous.Boxes[index].Runs[0] {
			t.Fatal("unrelated text run storage was rebuilt")
		}
	}
}
