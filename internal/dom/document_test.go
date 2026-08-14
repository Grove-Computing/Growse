package dom

import (
	"slices"
	"testing"
)

func TestNodeByIDReturnsDocumentNode(t *testing.T) {
	document := NewDocument()
	element := document.CreateElement("a", nil)

	got, ok := document.NodeByID(element.ID)
	if !ok || got != element {
		t.Fatalf("NodeByID(%d) = (%p, %v), want (%p, true)", element.ID, got, ok, element)
	}
	if got, ok := document.NodeByID(999); ok || got != nil {
		t.Fatalf("NodeByID(999) = (%p, %v), want (nil, false)", got, ok)
	}
}

func TestDocumentBuildsTreeAndIndexesID(t *testing.T) {
	document := NewDocument()
	body := document.CreateElement("BODY", nil)
	heading := document.CreateElement("h1", map[string]string{"id": "title"})
	text := document.CreateText("Hello")

	for _, edge := range []struct{ parent, child *Node }{
		{document.Root, body},
		{body, heading},
		{heading, text},
	} {
		if err := document.AppendChild(edge.parent, edge.child); err != nil {
			t.Fatalf("AppendChild() error = %v", err)
		}
	}

	if heading.Parent != body {
		t.Fatal("heading parent was not set")
	}
	if got, want := body.TagName, "body"; got != want {
		t.Fatalf("tag name = %q, want %q", got, want)
	}
	if got, ok := document.GetElementByID("title"); !ok || got != heading {
		t.Fatalf("GetElementByID() = (%p, %v), want (%p, true)", got, ok, heading)
	}
	if got, want := heading.TextContent(), "Hello"; got != want {
		t.Fatalf("TextContent() = %q, want %q", got, want)
	}
	if document.Root.ID == body.ID || body.ID == heading.ID || heading.ID == text.ID {
		t.Fatal("node IDs are not unique")
	}
}

func TestAppendChildRejectsNodeFromAnotherDocument(t *testing.T) {
	first := NewDocument()
	second := NewDocument()

	if err := first.AppendChild(first.Root, second.CreateElement("p", nil)); err == nil {
		t.Fatal("AppendChild() error = nil for a foreign node")
	}
}

func TestIsConnectedDistinguishesAttachedAndDetachedNodes(t *testing.T) {
	document := NewDocument()
	attached := document.CreateElement("main", nil)
	detached := document.CreateElement("section", nil)
	if err := document.AppendChild(document.Root, attached); err != nil {
		t.Fatal(err)
	}

	if !document.IsConnected(document.Root) || !document.IsConnected(attached) {
		t.Fatal("root and attached element should be connected")
	}
	if document.IsConnected(detached) {
		t.Fatal("detached element should not be connected")
	}
	if document.IsConnected(NewDocument().Root) {
		t.Fatal("foreign document root should not be connected")
	}
}

func TestAppendChildIndexesDetachedSubtreeWhenAttached(t *testing.T) {
	document := NewDocument()
	parent := document.CreateElement("section", map[string]string{"id": "parent"})
	child := document.CreateElement("p", map[string]string{"id": "child"})
	if err := document.AppendChild(parent, child); err != nil {
		t.Fatal(err)
	}
	if _, ok := document.GetElementByID("child"); ok {
		t.Fatal("detached subtree must not appear in the id index")
	}
	if err := document.AppendChild(document.Root, parent); err != nil {
		t.Fatal(err)
	}
	if got, ok := document.GetElementByID("child"); !ok || got != child {
		t.Fatalf("GetElementByID(child) = (%p, %v), want (%p, true)", got, ok, child)
	}
}

func TestRemoveDeletesConnectedSubtreeAndIndexes(t *testing.T) {
	document := NewDocument()
	parent := document.CreateElement("main", map[string]string{"id": "parent"})
	child := document.CreateElement("section", map[string]string{"id": "child"})
	grandchild := document.CreateElement("span", map[string]string{"id": "grandchild"})
	for _, edge := range [][2]*Node{
		{document.Root, parent},
		{parent, child},
		{child, grandchild},
	} {
		if err := document.AppendChild(edge[0], edge[1]); err != nil {
			t.Fatal(err)
		}
	}

	removed, ok := document.Remove(child.ID)
	if !ok {
		t.Fatal("Remove() = false, want true")
	}
	if want := []NodeID{child.ID, grandchild.ID}; !slices.Equal(removed, want) {
		t.Fatalf("removed IDs = %v, want %v", removed, want)
	}
	if len(parent.Children) != 0 || child.Parent != nil {
		t.Fatalf("removed child remains attached: parent children=%v child parent=%v", parent.Children, child.Parent)
	}
	for _, id := range removed {
		if _, exists := document.NodeByID(id); exists {
			t.Fatalf("removed node %d remains indexed", id)
		}
	}
	for _, id := range []string{"child", "grandchild"} {
		if _, exists := document.GetElementByID(id); exists {
			t.Fatalf("removed element %q remains in id index", id)
		}
	}
}

func TestRemoveRejectsRootDetachedAndAlreadyRemovedNode(t *testing.T) {
	document := NewDocument()
	attached := document.CreateElement("main", nil)
	detached := document.CreateElement("section", nil)
	if err := document.AppendChild(document.Root, attached); err != nil {
		t.Fatal(err)
	}
	if _, ok := document.Remove(document.Root.ID); ok {
		t.Fatal("Remove(root) = true, want false")
	}
	if _, ok := document.Remove(detached.ID); ok {
		t.Fatal("Remove(detached) = true, want false")
	}
	if _, ok := document.Remove(attached.ID); !ok {
		t.Fatal("first Remove(attached) = false, want true")
	}
	if _, ok := document.Remove(attached.ID); ok {
		t.Fatal("second Remove(attached) = true, want false")
	}
}

func TestQuerySelectorReturnsFirstMatchingElement(t *testing.T) {
	document := NewDocument()
	body := document.CreateElement("body", nil)
	first := document.CreateElement("DIV", map[string]string{"id": "first", "class": "card featured"})
	second := document.CreateElement("div", map[string]string{"id": "second", "class": "card"})
	for _, edge := range [][2]*Node{
		{document.Root, body},
		{body, first},
		{body, second},
	} {
		if err := document.AppendChild(edge[0], edge[1]); err != nil {
			t.Fatal(err)
		}
	}

	tests := []struct {
		selector string
		want     *Node
	}{
		{selector: "div", want: first},
		{selector: "DIV", want: first},
		{selector: "#second", want: second},
		{selector: ".featured", want: first},
		{selector: "div.card", want: first},
		{selector: "  #first  ", want: first},
	}
	for _, test := range tests {
		t.Run(test.selector, func(t *testing.T) {
			got, ok := document.QuerySelector(test.selector)
			if !ok || got != test.want {
				t.Fatalf("QuerySelector(%q) = (%p, %v), want (%p, true)", test.selector, got, ok, test.want)
			}
		})
	}
}

func TestQuerySelectorRejectsUnsupportedOrUnknownSelector(t *testing.T) {
	document := NewDocument()
	child := document.CreateElement("div", map[string]string{"id": "message", "class": "card"})
	if err := document.AppendChild(document.Root, child); err != nil {
		t.Fatal(err)
	}

	for _, selector := range []string{"", "div span", "div#message", "div.card.featured", "*", "[id]", ":hover", "#unknown"} {
		t.Run(selector, func(t *testing.T) {
			if got, ok := document.QuerySelector(selector); ok || got != nil {
				t.Fatalf("QuerySelector(%q) = (%p, %v), want (nil, false)", selector, got, ok)
			}
		})
	}
}

func TestQuerySelectorIgnoresDetachedElement(t *testing.T) {
	document := NewDocument()
	document.CreateElement("p", map[string]string{"id": "detached"})

	if got, ok := document.QuerySelector("#detached"); ok || got != nil {
		t.Fatalf("QuerySelector() = (%p, %v), want (nil, false)", got, ok)
	}
}

func TestSetAttributeUpdatesValueAndIDIndex(t *testing.T) {
	document := NewDocument()
	element := document.CreateElement("div", map[string]string{"id": "before"})
	if err := document.AppendChild(document.Root, element); err != nil {
		t.Fatal(err)
	}

	if !document.SetAttribute(element.ID, "id", "after") {
		t.Fatal("SetAttribute() = false, want true")
	}
	if got, ok := element.Attribute("id"); !ok || got != "after" {
		t.Fatalf("Attribute(id) = (%q, %v), want (after, true)", got, ok)
	}
	if _, ok := document.GetElementByID("before"); ok {
		t.Fatal("old id remains indexed")
	}
	if got, ok := document.GetElementByID("after"); !ok || got != element {
		t.Fatalf("GetElementByID(after) = (%p, %v), want (%p, true)", got, ok, element)
	}
	if document.SetAttribute(element.ID, "id", "after") {
		t.Fatal("SetAttribute() = true for unchanged value")
	}
}

func TestSetAttributeInitializesAttributeMap(t *testing.T) {
	document := NewDocument()
	element := document.CreateElement("input", nil)
	if !document.SetAttribute(element.ID, "value", "hello") {
		t.Fatal("SetAttribute() = false, want true")
	}
	if got, ok := element.Attribute("value"); !ok || got != "hello" {
		t.Fatalf("Attribute(value) = (%q, %v), want (hello, true)", got, ok)
	}
}

func TestRemoveAttributeUpdatesElementAndIDIndex(t *testing.T) {
	document := NewDocument()
	element := document.CreateElement("input", map[string]string{"id": "choice", "checked": ""})
	if err := document.AppendChild(document.Root, element); err != nil {
		t.Fatal(err)
	}

	if !document.RemoveAttribute(element.ID, "checked") {
		t.Fatal("RemoveAttribute(checked) = false")
	}
	if _, exists := element.Attribute("checked"); exists || document.RemoveAttribute(element.ID, "checked") {
		t.Fatal("checked attribute was not removed exactly once")
	}
	if !document.RemoveAttribute(element.ID, "id") {
		t.Fatal("RemoveAttribute(id) = false")
	}
	if _, exists := document.GetElementByID("choice"); exists {
		t.Fatal("removed id remained indexed")
	}
}

func TestSetTextContentReplacesDescendantsAndIndexes(t *testing.T) {
	document := NewDocument()
	parent := document.CreateElement("p", map[string]string{"id": "message"})
	child := document.CreateElement("span", map[string]string{"id": "old"})
	text := document.CreateText("before")
	for _, edge := range [][2]*Node{{document.Root, parent}, {parent, child}, {child, text}} {
		if err := document.AppendChild(edge[0], edge[1]); err != nil {
			t.Fatal(err)
		}
	}

	if !document.SetTextContent(parent.ID, "after") {
		t.Fatal("SetTextContent() = false, want true")
	}
	if got, want := parent.TextContent(), "after"; got != want {
		t.Fatalf("TextContent() = %q, want %q", got, want)
	}
	if _, ok := document.NodeByID(child.ID); ok {
		t.Fatal("removed child remains in node index")
	}
	if _, ok := document.GetElementByID("old"); ok {
		t.Fatal("removed child remains in id index")
	}
	if got, want := len(parent.Children), 1; got != want || parent.Children[0].Type != NodeText {
		t.Fatalf("children = %#v, want one text node", parent.Children)
	}
}
