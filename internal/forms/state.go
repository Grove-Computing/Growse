package forms

import (
	"strings"

	"github.com/Grove-Computing/Growse/internal/dom"
)

// CurrentValue returns a control's live value without overwriting its default.
func CurrentValue(node *dom.Node) string {
	if node == nil {
		return ""
	}
	if node.ControlValueDirty {
		return node.ControlValue
	}
	if node.TagName == "textarea" {
		if value, exists := node.Attribute("value"); exists {
			return value
		}
		return node.TextContent()
	}
	value, _ := node.Attribute("value")
	return value
}

// SetCurrentValue changes browser-owned live state and marks it dirty.
func SetCurrentValue(node *dom.Node, value string) bool {
	if node == nil || node.ControlValueDirty && node.ControlValue == value || !node.ControlValueDirty && CurrentValue(node) == value {
		return false
	}
	node.ControlValue = value
	node.ControlValueDirty = true
	return true
}

// CurrentChecked returns checked state, preferring live state over the default attribute.
func CurrentChecked(node *dom.Node) bool {
	if node == nil {
		return false
	}
	if node.ControlCheckedDirty {
		return node.ControlChecked
	}
	_, checked := node.Attribute("checked")
	return checked
}

// Disabled reports whether the control itself or an ancestor fieldset is disabled.
func Disabled(node *dom.Node) bool {
	for current := node; current != nil; current = current.Parent {
		if current.Type != dom.NodeElement {
			continue
		}
		if _, disabled := current.Attribute("disabled"); disabled && (current == node || current.TagName == "fieldset") {
			return true
		}
	}
	return false
}

// ReadOnly reports whether an editable text control has the readonly attribute.
func ReadOnly(node *dom.Node) bool {
	if !IsEditableTextControl(node) {
		return false
	}
	_, readOnly := node.Attribute("readonly")
	return readOnly
}

// Reset restores descendants of a form to attribute-backed defaults.
func Reset(form *dom.Node) bool {
	if form == nil || form.Type != dom.NodeElement || form.TagName != "form" {
		return false
	}
	changed := false
	forEachElement(form, func(node *dom.Node) {
		if node.ControlValueDirty || node.ControlCheckedDirty {
			changed = true
		}
		node.ControlValue = ""
		node.ControlValueDirty = false
		node.ControlChecked = false
		node.ControlCheckedDirty = false
	})
	return changed
}

// LabeledControl resolves a label target using for=id or its first descendant control.
func LabeledControl(document *dom.Document, node *dom.Node) *dom.Node {
	if document == nil || node == nil {
		return nil
	}
	label := node
	for label != nil && (label.Type != dom.NodeElement || label.TagName != "label") {
		label = label.Parent
	}
	if label == nil {
		return nil
	}
	if targetID, exists := label.Attribute("for"); exists {
		if target, found := document.GetElementByID(strings.TrimSpace(targetID)); found && isLabelable(target) {
			return target
		}
		return nil
	}
	var result *dom.Node
	forEachElement(label, func(candidate *dom.Node) {
		if result == nil && candidate != label && isLabelable(candidate) {
			result = candidate
		}
	})
	return result
}

func isLabelable(node *dom.Node) bool {
	if node == nil || node.Type != dom.NodeElement {
		return false
	}
	switch node.TagName {
	case "button", "select", "textarea":
		return true
	case "input":
		typeValue, _ := node.Attribute("type")
		return !strings.EqualFold(strings.TrimSpace(typeValue), "hidden")
	default:
		return false
	}
}
