package dom

import "testing"

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
