package forms

import (
	"testing"

	"github.com/Grove-Computing/Growse/internal/dom"
)

func TestNextFocusableUsesDOMOrderSkipsDisabledAndWraps(t *testing.T) {
	document := dom.NewDocument()
	first := document.CreateElement("input", nil)
	disabled := document.CreateElement("select", map[string]string{"disabled": ""})
	last := document.CreateElement("input", map[string]string{"type": "checkbox"})
	for _, node := range []*dom.Node{first, disabled, last} {
		appendNode(t, document, document.Root, node)
	}

	if got := FocusableControls(document); len(got) != 2 || got[0] != first.ID || got[1] != last.ID {
		t.Fatalf("focusable controls = %v", got)
	}
	if got := NextFocusable(document, first.ID, false); got != last.ID {
		t.Fatalf("forward = %d", got)
	}
	if got := NextFocusable(document, first.ID, true); got != last.ID {
		t.Fatalf("reverse wrap = %d", got)
	}
}
