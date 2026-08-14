package forms

import (
	"testing"

	"github.com/Grove-Computing/Growse/internal/dom"
)

func TestLiveValueAndCheckedStateStaySeparateFromDefaultsUntilReset(t *testing.T) {
	document := dom.NewDocument()
	form := document.CreateElement("form", nil)
	input := document.CreateElement("input", map[string]string{"value": "default"})
	checkbox := document.CreateElement("input", map[string]string{"type": "checkbox", "checked": ""})
	selectNode := document.CreateElement("select", nil)
	first := document.CreateElement("option", map[string]string{"value": "one", "selected": ""})
	second := document.CreateElement("option", map[string]string{"value": "two"})
	appendNode(t, document, document.Root, form)
	appendNode(t, document, form, input)
	appendNode(t, document, form, checkbox)
	appendNode(t, document, form, selectNode)
	appendNode(t, document, selectNode, first)
	appendNode(t, document, selectNode, second)

	if !SetCurrentValue(input, "edited") {
		t.Fatal("live value did not change")
	}
	if checked, changed := ActivateCheckable(document, checkbox.ID); !changed || checked {
		t.Fatalf("checkbox activation = (%v, %v)", checked, changed)
	}
	if !SetSelectedValue(document, selectNode.ID, "two") {
		t.Fatal("select live value did not change")
	}
	document.SetAttribute(input.ID, "value", "new-default")
	if CurrentValue(input) != "edited" || CurrentChecked(checkbox) || SelectedIndex(selectNode, SelectOptions(selectNode)) != 1 {
		t.Fatalf("dirty state was overwritten: value=%q checked=%v select=%d", CurrentValue(input), CurrentChecked(checkbox), SelectedIndex(selectNode, SelectOptions(selectNode)))
	}
	if !Reset(form) || CurrentValue(input) != "new-default" || !CurrentChecked(checkbox) || SelectedIndex(selectNode, SelectOptions(selectNode)) != 0 {
		t.Fatalf("reset state: value=%q checked=%v select=%d", CurrentValue(input), CurrentChecked(checkbox), SelectedIndex(selectNode, SelectOptions(selectNode)))
	}
}

func TestDisabledReadonlyAndLabelResolution(t *testing.T) {
	document := dom.NewDocument()
	fieldset := document.CreateElement("fieldset", map[string]string{"disabled": ""})
	nested := document.CreateElement("input", nil)
	readonly := document.CreateElement("input", map[string]string{"readonly": ""})
	checkbox := document.CreateElement("input", map[string]string{"id": "accept", "type": "checkbox"})
	label := document.CreateElement("label", map[string]string{"for": "accept"})
	labelText := document.CreateText("Accept")
	appendNode(t, document, document.Root, fieldset)
	appendNode(t, document, fieldset, nested)
	appendNode(t, document, document.Root, readonly)
	appendNode(t, document, document.Root, checkbox)
	appendNode(t, document, document.Root, label)
	appendNode(t, document, label, labelText)

	if !Disabled(nested) || !ReadOnly(readonly) {
		t.Fatalf("disabled=%v readonly=%v", Disabled(nested), ReadOnly(readonly))
	}
	if got := LabeledControl(document, labelText); got != checkbox {
		t.Fatalf("labeled control = %#v, want checkbox", got)
	}
}
