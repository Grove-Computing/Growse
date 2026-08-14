package forms

import (
	"testing"

	"github.com/Grove-Computing/Growse/internal/dom"
)

func TestActivateCheckableTogglesCheckbox(t *testing.T) {
	document := dom.NewDocument()
	checkbox := document.CreateElement("input", map[string]string{"type": "checkbox"})
	appendNode(t, document, document.Root, checkbox)

	if checked, changed := ActivateCheckable(document, checkbox.ID); !changed || !checked {
		t.Fatalf("first activation = (%v, %v)", checked, changed)
	}
	if checked, changed := ActivateCheckable(document, checkbox.ID); !changed || checked {
		t.Fatalf("second activation = (%v, %v)", checked, changed)
	}
}

func TestActivateCheckableKeepsOneRadioCheckedPerFormAndName(t *testing.T) {
	document := dom.NewDocument()
	leftForm := document.CreateElement("form", nil)
	rightForm := document.CreateElement("form", nil)
	first := document.CreateElement("input", map[string]string{"type": "radio", "name": "size", "checked": ""})
	second := document.CreateElement("input", map[string]string{"type": "radio", "name": "size"})
	otherForm := document.CreateElement("input", map[string]string{"type": "radio", "name": "size", "checked": ""})
	appendNode(t, document, document.Root, leftForm)
	appendNode(t, document, document.Root, rightForm)
	appendNode(t, document, leftForm, first)
	appendNode(t, document, leftForm, second)
	appendNode(t, document, rightForm, otherForm)

	if checked, changed := ActivateCheckable(document, second.ID); !changed || !checked {
		t.Fatalf("radio activation = (%v, %v)", checked, changed)
	}
	if _, checked := first.Attribute("checked"); checked {
		t.Fatal("previous radio in the group remained checked")
	}
	if _, checked := second.Attribute("checked"); !checked {
		t.Fatal("activated radio was not checked")
	}
	if _, checked := otherForm.Attribute("checked"); !checked {
		t.Fatal("radio in another form was unexpectedly unchecked")
	}
}
