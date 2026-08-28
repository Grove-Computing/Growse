package dom

import (
	"fmt"
	"strings"
	"testing"

	dommodel "github.com/Grove-Computing/Growse/internal/dom"
)

func TestDOMSafetyLimitsBoundNodesDepthAttributesAndStrings(t *testing.T) {
	t.Run("nodes", func(t *testing.T) {
		document := dommodel.NewDocument()
		api := New(document, nil, nil)
		var first *Element
		for count := 1; count < dommodel.MaxNodesPerDocument; count++ {
			created := api.CreateElement("div")
			if created == nil {
				t.Fatalf("CreateElement stopped at owned node %d", document.OwnedNodeCount())
			}
			if first == nil {
				first = created
			}
		}
		if document.OwnedNodeCount() != dommodel.MaxNodesPerDocument || api.CreateElement("div") != nil || api.CreateTextNode("x") != nil || first.SetText("x") {
			t.Fatalf("DOM node limit = owned:%d", document.OwnedNodeCount())
		}
	})

	t.Run("depth", func(t *testing.T) {
		document := dommodel.NewDocument()
		parent := document.Root
		for depth := 1; depth <= dommodel.MaxTreeDepth; depth++ {
			child := document.CreateElement("div", nil)
			if err := document.AppendChild(parent, child); err != nil {
				t.Fatalf("AppendChild depth %d: %v", depth, err)
			}
			parent = child
		}
		if err := document.AppendChild(parent, document.CreateElement("div", nil)); err == nil || !strings.Contains(err.Error(), "depth") {
			t.Fatalf("depth overflow error = %v", err)
		}
	})

	t.Run("attributes-and-strings", func(t *testing.T) {
		document := dommodel.NewDocument()
		element := document.CreateElement("div", nil)
		for index := range dommodel.MaxAttributesPerNode {
			if !document.SetAttribute(element.ID, fmt.Sprintf("data-%d", index), "value") {
				t.Fatalf("SetAttribute stopped at %d", index)
			}
		}
		if document.SetAttribute(element.ID, "overflow", "value") {
			t.Fatal("attribute count overflow was accepted")
		}
		stringElement := document.CreateElement("div", nil)
		large := strings.Repeat("x", dommodel.MaxDOMStringBytes)
		if !document.SetAttribute(stringElement.ID, "data-value", large) || document.SetAttribute(stringElement.ID, "data-value", large+"x") {
			t.Fatal("attribute string boundary was not enforced")
		}
		api := New(document, nil, nil)
		if api.CreateTextNode(large) == nil || api.CreateTextNode(large+"x") != nil {
			t.Fatal("DOM string boundary was not enforced")
		}
	})
}
