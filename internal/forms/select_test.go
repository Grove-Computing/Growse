package forms

import (
	"testing"

	"github.com/Grove-Computing/Growse/internal/dom"
)

func TestSelectOptionsAndSelectionResolution(t *testing.T) {
	document := dom.NewDocument()
	selectNode := document.CreateElement("select", nil)
	first := document.CreateElement("option", map[string]string{"value": "one"})
	second := document.CreateElement("option", map[string]string{"selected": ""})
	third := document.CreateElement("option", map[string]string{"value": "disabled", "disabled": ""})
	appendNode(t, document, document.Root, selectNode)
	for _, item := range []struct {
		option *dom.Node
		label  string
	}{{first, "One"}, {second, "Two"}, {third, "Disabled"}} {
		appendNode(t, document, selectNode, item.option)
		appendNode(t, document, item.option, document.CreateText(item.label))
	}

	options := SelectOptions(selectNode)
	if len(options) != 3 || options[1].Value != "Two" || !options[2].Disabled {
		t.Fatalf("options = %#v", options)
	}
	if got := SelectedIndex(selectNode, options); got != 1 {
		t.Fatalf("selected index = %d, want 1", got)
	}
	if !SetSelectedValue(document, selectNode.ID, "one") || SelectedIndex(selectNode, options) != 0 {
		t.Fatal("enabled option was not selected")
	}
	if SetSelectedValue(document, selectNode.ID, "disabled") {
		t.Fatal("disabled option was selected")
	}
	if next, ok := NextEnabledValue(options, 0); !ok || next != "Two" {
		t.Fatalf("next enabled option = (%q, %v)", next, ok)
	}
}

func appendNode(t *testing.T, document *dom.Document, parent, child *dom.Node) {
	t.Helper()
	if err := document.AppendChild(parent, child); err != nil {
		t.Fatal(err)
	}
}
