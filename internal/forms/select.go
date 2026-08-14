// Package forms implements browser-owned HTML form state and serialization.
package forms

import (
	"strings"

	"github.com/Grove-Computing/Growse/internal/dom"
)

// Option is one flattened option in DOM order.
type Option struct {
	NodeID   dom.NodeID
	Value    string
	Label    string
	Disabled bool
}

// SelectOptions returns option descendants in DOM order.
func SelectOptions(selectNode *dom.Node) []Option {
	if selectNode == nil || selectNode.Type != dom.NodeElement || selectNode.TagName != "select" {
		return nil
	}
	var result []Option
	var walk func(*dom.Node, bool)
	walk = func(node *dom.Node, groupDisabled bool) {
		if node == nil {
			return
		}
		if node.Type == dom.NodeElement && node.TagName == "optgroup" {
			_, disabled := node.Attribute("disabled")
			groupDisabled = groupDisabled || disabled
		}
		if node.Type == dom.NodeElement && node.TagName == "option" {
			value, exists := node.Attribute("value")
			label := strings.TrimSpace(node.TextContent())
			if !exists {
				value = label
			}
			_, disabled := node.Attribute("disabled")
			result = append(result, Option{NodeID: node.ID, Value: value, Label: label, Disabled: disabled || groupDisabled})
			return
		}
		for _, child := range node.Children {
			walk(child, groupDisabled)
		}
	}
	for _, child := range selectNode.Children {
		walk(child, false)
	}
	return result
}

// SelectedIndex resolves current value, selected content attribute, then the
// first option in that order. It returns -1 for an empty select.
func SelectedIndex(selectNode *dom.Node, options []Option) int {
	if len(options) == 0 {
		return -1
	}
	if selectNode.ControlValueDirty {
		current := selectNode.ControlValue
		for index, option := range options {
			if option.Value == current {
				return index
			}
		}
	}
	selectedNodes := make(map[dom.NodeID]bool)
	collectSelectedOptions(selectNode, selectedNodes)
	for index, option := range options {
		if selectedNodes[option.NodeID] {
			return index
		}
	}
	return 0
}

// SetSelectedValue updates a select to an enabled option value.
func SetSelectedValue(document *dom.Document, nodeID dom.NodeID, value string) bool {
	if document == nil {
		return false
	}
	selectNode, ok := document.NodeByID(nodeID)
	if !ok || !document.IsConnected(selectNode) || selectNode.TagName != "select" || Disabled(selectNode) {
		return false
	}
	for _, option := range SelectOptions(selectNode) {
		if option.Value == value && !option.Disabled {
			return SetCurrentValue(selectNode, value)
		}
	}
	return false
}

// NextEnabledValue returns the next enabled option, wrapping at the end.
func NextEnabledValue(options []Option, selected int) (string, bool) {
	for offset := 1; offset <= len(options); offset++ {
		index := (selected + offset) % len(options)
		if !options[index].Disabled {
			return options[index].Value, true
		}
	}
	return "", false
}

func collectSelectedOptions(node *dom.Node, selected map[dom.NodeID]bool) {
	if node == nil {
		return
	}
	if node.Type == dom.NodeElement && node.TagName == "option" {
		_, selectedAttribute := node.Attribute("selected")
		selected[node.ID] = selectedAttribute
	}
	for _, child := range node.Children {
		collectSelectedOptions(child, selected)
	}
}
