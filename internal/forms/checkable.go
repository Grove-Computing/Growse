package forms

import (
	"strings"

	"github.com/Grove-Computing/Growse/internal/dom"
)

// Checkable describes a checkbox or radio input's current state.
type Checkable struct {
	Kind    string
	Checked bool
}

// CheckableState resolves a checkbox or radio input.
func CheckableState(node *dom.Node) (Checkable, bool) {
	if node == nil || node.Type != dom.NodeElement || node.TagName != "input" {
		return Checkable{}, false
	}
	kind, _ := node.Attribute("type")
	kind = strings.ToLower(strings.TrimSpace(kind))
	if kind != "checkbox" && kind != "radio" {
		return Checkable{}, false
	}
	_, checked := node.Attribute("checked")
	return Checkable{Kind: kind, Checked: checked}, true
}

// ActivateCheckable applies the default activation behavior and returns the
// resulting checked state when it changed.
func ActivateCheckable(document *dom.Document, nodeID dom.NodeID) (bool, bool) {
	if document == nil {
		return false, false
	}
	node, ok := document.NodeByID(nodeID)
	if !ok || !document.IsConnected(node) {
		return false, false
	}
	state, ok := CheckableState(node)
	if !ok {
		return false, false
	}
	if state.Kind == "checkbox" {
		if state.Checked {
			return false, document.RemoveAttribute(node.ID, "checked")
		}
		return true, document.SetAttribute(node.ID, "checked", "")
	}
	if state.Checked {
		return true, false
	}
	name, _ := node.Attribute("name")
	owner := nearestForm(node)
	forEachElement(document.Root, func(candidate *dom.Node) {
		if candidate == node || nearestForm(candidate) != owner {
			return
		}
		candidateState, candidateOK := CheckableState(candidate)
		candidateName, _ := candidate.Attribute("name")
		if candidateOK && candidateState.Kind == "radio" && candidateName == name {
			document.RemoveAttribute(candidate.ID, "checked")
		}
	})
	return true, document.SetAttribute(node.ID, "checked", "")
}

func nearestForm(node *dom.Node) *dom.Node {
	for current := node; current != nil; current = current.Parent {
		if current.Type == dom.NodeElement && current.TagName == "form" {
			return current
		}
	}
	return nil
}

func forEachElement(node *dom.Node, visit func(*dom.Node)) {
	if node == nil {
		return
	}
	if node.Type == dom.NodeElement {
		visit(node)
	}
	for _, child := range node.Children {
		forEachElement(child, visit)
	}
}
