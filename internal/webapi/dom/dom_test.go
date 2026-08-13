package dom

import (
	"testing"

	dommodel "github.com/saku0512/growse/internal/dom"
	"github.com/saku0512/growse/internal/events"
)

func TestGetElementByIDReadsAndChangesText(t *testing.T) {
	document := dommodel.NewDocument()
	message := document.CreateElement("p", map[string]string{"id": "message"})
	text := document.CreateText("before")
	if err := document.AppendChild(document.Root, message); err != nil {
		t.Fatal(err)
	}
	if err := document.AppendChild(message, text); err != nil {
		t.Fatal(err)
	}
	mutations := 0
	api := New(document, events.NewDispatcher(), func() { mutations++ })

	element := api.GetElementByID("message")
	if element == nil || element.Text() != "before" {
		t.Fatalf("GetElementByID() = %#v, want element with text", element)
	}
	element.SetText("after")
	if got, want := message.TextContent(), "after"; got != want {
		t.Fatalf("TextContent() = %q, want %q", got, want)
	}
	if mutations != 1 {
		t.Fatalf("mutation count = %d, want 1", mutations)
	}
}

func TestGetElementByIDReturnsNilForUnknownElement(t *testing.T) {
	if element := New(dommodel.NewDocument(), events.NewDispatcher(), nil).GetElementByID("unknown"); element != nil {
		t.Fatalf("GetElementByID() = %#v, want nil", element)
	}
}

func TestQuerySelectorReadsFirstMatchingElement(t *testing.T) {
	document := dommodel.NewDocument()
	first := document.CreateElement("p", map[string]string{"class": "message"})
	second := document.CreateElement("p", map[string]string{"class": "message"})
	for _, node := range []*dommodel.Node{first, second} {
		if err := document.AppendChild(document.Root, node); err != nil {
			t.Fatal(err)
		}
	}
	if err := document.AppendChild(first, document.CreateText("first")); err != nil {
		t.Fatal(err)
	}
	if err := document.AppendChild(second, document.CreateText("second")); err != nil {
		t.Fatal(err)
	}

	element := New(document, events.NewDispatcher(), nil).QuerySelector("p.message")
	if element == nil || element.Text() != "first" {
		t.Fatalf("QuerySelector() = %#v, want first matching element", element)
	}
}

func TestQuerySelectorReturnsNilForUnsupportedSelector(t *testing.T) {
	api := New(dommodel.NewDocument(), events.NewDispatcher(), nil)
	if element := api.QuerySelector("main p"); element != nil {
		t.Fatalf("QuerySelector() = %#v, want nil", element)
	}
}

func TestCreateElementCreatesDetachedDocumentElement(t *testing.T) {
	document := dommodel.NewDocument()
	api := New(document, events.NewDispatcher(), nil)

	element := api.CreateElement("  LI  ")
	if element == nil {
		t.Fatal("CreateElement() = nil")
	}
	node, ok := document.NodeByID(element.id)
	if !ok {
		t.Fatal("created element is not owned by the document")
	}
	if got, want := node.TagName, "li"; got != want {
		t.Fatalf("TagName = %q, want %q", got, want)
	}
	if node.Parent != nil {
		t.Fatalf("Parent = %#v, want nil", node.Parent)
	}
	if got, want := document.ElementCount(), 0; got != want {
		t.Fatalf("ElementCount() = %d, want %d before attachment", got, want)
	}
}

func TestCreateElementRejectsInvalidTagName(t *testing.T) {
	api := New(dommodel.NewDocument(), events.NewDispatcher(), nil)
	for _, tagName := range []string{"", "   ", "div span", "<div>", "input/"} {
		if element := api.CreateElement(tagName); element != nil {
			t.Fatalf("CreateElement(%q) = %#v, want nil", tagName, element)
		}
	}
}

func TestOnClickRegistersHandler(t *testing.T) {
	document := dommodel.NewDocument()
	button := document.CreateElement("button", map[string]string{"id": "button"})
	if err := document.AppendChild(document.Root, button); err != nil {
		t.Fatal(err)
	}
	dispatcher := events.NewDispatcher()
	element := New(document, dispatcher, nil).GetElementByID("button")
	called := false
	element.OnClick(func() { called = true })

	if !dispatcher.Dispatch(events.Event{Type: events.Click, Target: button.ID}) || !called {
		t.Fatal("registered click handler was not called")
	}
}
