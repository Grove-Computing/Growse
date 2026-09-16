package layout

import (
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/css"
	"github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/style"
)

func TestWritingModeAndDirectionSurviveLayoutTree(t *testing.T) {
	t.Parallel()
	document := dom.NewDocument()
	container := document.CreateElement("div", map[string]string{"class": "vertical"})
	text := document.CreateText("縦書き")
	if err := document.AppendChild(document.Root, container); err != nil {
		t.Fatal(err)
	}
	if err := document.AppendChild(container, text); err != nil {
		t.Fatal(err)
	}
	stylesheet, err := css.Parse(strings.NewReader(`.vertical { display:block; writing-mode:vertical-rl; direction:rtl; background:red }`))
	if err != nil {
		t.Fatal(err)
	}
	tree := Build(document, style.Compute(document, stylesheet), 400)

	decorationFound := false
	for _, decoration := range tree.Decorations {
		if decoration.NodeID != container.ID {
			continue
		}
		decorationFound = true
		if decoration.WritingMode != style.WritingModeVerticalRL || decoration.Direction != style.DirectionRTL {
			t.Fatalf("decoration writing mode/direction = %v/%v", decoration.WritingMode, decoration.Direction)
		}
	}
	if !decorationFound {
		t.Fatal("container decoration is missing")
	}
	boxFound := false
	for _, box := range tree.Boxes {
		if box.NodeID != container.ID {
			continue
		}
		boxFound = true
		if box.WritingMode != style.WritingModeVerticalRL || box.Direction != style.DirectionRTL {
			t.Fatalf("box writing mode/direction = %v/%v", box.WritingMode, box.Direction)
		}
		if len(box.Runs) == 0 || box.Runs[0].WritingMode != style.WritingModeVerticalRL || box.Runs[0].Direction != style.DirectionRTL {
			t.Fatalf("text run writing metadata = %#v", box.Runs)
		}
	}
	if !boxFound {
		t.Fatal("container text box is missing")
	}
}
