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
