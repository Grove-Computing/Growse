package dom

import "testing"

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
