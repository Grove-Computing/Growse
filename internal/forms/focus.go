package forms

import (
	"strconv"
	"strings"

	"github.com/Grove-Computing/Growse/internal/dom"
)

// IsSubmitButton reports whether a button performs form submission.
func IsSubmitButton(node *dom.Node) bool {
	if node == nil || node.Type != dom.NodeElement {
		return false
	}
	typeValue, exists := node.Attribute("type")
	typeValue = strings.ToLower(strings.TrimSpace(typeValue))
	if node.TagName == "button" {
		return !exists || typeValue == "" || typeValue == "submit"
	}
	return node.TagName == "input" && typeValue == "submit"
}

// FocusableControls returns enabled form controls in DOM order.
func FocusableControls(document *dom.Document) []dom.NodeID {
	if document == nil {
		return nil
	}
	var result []dom.NodeID
	forEachElement(document.Root, func(node *dom.Node) {
		if Disabled(node) || !isFocusableControl(node) {
			return
		}
		if raw, exists := node.Attribute("tabindex"); exists {
			if value, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil && value < 0 {
				return
			}
		}
		result = append(result, node.ID)
	})
	return result
}

// NextFocusable returns the next or previous control, wrapping at the ends.
func NextFocusable(document *dom.Document, current dom.NodeID, reverse bool) dom.NodeID {
	controls := FocusableControls(document)
	if len(controls) == 0 {
		return 0
	}
	currentIndex := -1
	for index, nodeID := range controls {
		if nodeID == current {
			currentIndex = index
			break
		}
	}
	if reverse {
		if currentIndex <= 0 {
			return controls[len(controls)-1]
		}
		return controls[currentIndex-1]
	}
	if currentIndex < 0 || currentIndex == len(controls)-1 {
		return controls[0]
	}
	return controls[currentIndex+1]
}

func isFocusableControl(node *dom.Node) bool {
	if node == nil || node.Type != dom.NodeElement {
		return false
	}
	if IsEditableTextControl(node) || isLabelable(node) && (node.TagName == "select" || node.TagName == "button") {
		return true
	}
	_, checkable := CheckableState(node)
	return checkable || IsSubmitButton(node)
}
