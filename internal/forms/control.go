package forms

import (
	"strings"

	"github.com/Grove-Computing/Growse/internal/dom"
)

var nonTextInputTypes = map[string]bool{
	"button": true, "checkbox": true, "color": true, "date": true,
	"file": true, "hidden": true, "radio": true, "range": true,
	"reset": true, "submit": true, "time": true,
}

// EditableTextControlType returns the normalized type of an editable text
// control. Unknown input types use the HTML text fallback.
func EditableTextControlType(node *dom.Node) (string, bool) {
	if node == nil || node.Type != dom.NodeElement {
		return "", false
	}
	if node.TagName == "textarea" {
		return "textarea", true
	}
	if node.TagName != "input" {
		return "", false
	}
	typeValue, exists := node.Attribute("type")
	typeValue = strings.ToLower(strings.TrimSpace(typeValue))
	if !exists || typeValue == "" {
		return "text", true
	}
	if nonTextInputTypes[typeValue] {
		return "", false
	}
	switch typeValue {
	case "text", "password", "email", "url", "number":
		return typeValue, true
	default:
		return "text", true
	}
}

// IsEditableTextControl reports whether users can edit a control as text.
func IsEditableTextControl(node *dom.Node) bool {
	_, ok := EditableTextControlType(node)
	return ok
}
