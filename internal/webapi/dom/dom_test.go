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

func TestAppendChildAttachesCreatedElementAndNotifiesMutation(t *testing.T) {
	document := dommodel.NewDocument()
	listNode := document.CreateElement("ul", map[string]string{"id": "list"})
	if err := document.AppendChild(document.Root, listNode); err != nil {
		t.Fatal(err)
	}
	mutations := 0
	api := New(document, events.NewDispatcher(), func() { mutations++ })
	list := api.GetElementByID("list")
	item := api.CreateElement("li")

	if !list.AppendChild(item) {
		t.Fatal("AppendChild() = false, want true")
	}
	itemNode, ok := document.NodeByID(item.id)
	if !ok || itemNode.Parent != listNode || !document.IsConnected(itemNode) {
		t.Fatalf("created item was not attached: %#v", itemNode)
	}
	if got, want := mutations, 1; got != want {
		t.Fatalf("mutation count = %d, want %d", got, want)
	}
}

func TestAppendChildRejectsInvalidRelationshipsWithoutMutation(t *testing.T) {
	document := dommodel.NewDocument()
	parentNode := document.CreateElement("main", map[string]string{"id": "parent"})
	alreadyAttachedNode := document.CreateElement("p", map[string]string{"id": "attached"})
	for _, node := range []*dommodel.Node{parentNode, alreadyAttachedNode} {
		if err := document.AppendChild(document.Root, node); err != nil {
			t.Fatal(err)
		}
	}
	mutations := 0
	api := New(document, events.NewDispatcher(), func() { mutations++ })
	parent := api.GetElementByID("parent")
	detachedParent := api.CreateElement("section")
	detachedChild := api.CreateElement("span")
	foreignChild := New(dommodel.NewDocument(), events.NewDispatcher(), nil).CreateElement("span")
	alreadyAttached := api.GetElementByID("attached")

	for name, appendChild := range map[string]func() bool{
		"nil child":       func() bool { return parent.AppendChild(nil) },
		"detached parent": func() bool { return detachedParent.AppendChild(detachedChild) },
		"foreign child":   func() bool { return parent.AppendChild(foreignChild) },
		"connected child": func() bool { return parent.AppendChild(alreadyAttached) },
		"nil parent receiver": func() bool {
			var element *Element
			return element.AppendChild(detachedChild)
		},
	} {
		t.Run(name, func(t *testing.T) {
			if appendChild() {
				t.Fatal("AppendChild() = true, want false")
			}
		})
	}
	if mutations != 0 {
		t.Fatalf("mutation count = %d, want 0", mutations)
	}
}

func TestRemoveDeletesElementAndDescendants(t *testing.T) {
	document := dommodel.NewDocument()
	parentNode := document.CreateElement("main", map[string]string{"id": "parent"})
	childNode := document.CreateElement("section", map[string]string{"id": "child"})
	grandchildNode := document.CreateElement("button", map[string]string{"id": "grandchild"})
	for _, edge := range [][2]*dommodel.Node{
		{document.Root, parentNode},
		{parentNode, childNode},
		{childNode, grandchildNode},
	} {
		if err := document.AppendChild(edge[0], edge[1]); err != nil {
			t.Fatal(err)
		}
	}
	mutations := 0
	dispatcher := events.NewDispatcher()
	api := New(document, dispatcher, func() { mutations++ })
	child := api.GetElementByID("child")
	grandchild := api.GetElementByID("grandchild")
	called := false
	grandchild.OnClick(func() { called = true })

	if !child.Remove() {
		t.Fatal("Remove() = false, want true")
	}
	if child.Remove() {
		t.Fatal("second Remove() = true, want false")
	}
	if len(parentNode.Children) != 0 {
		t.Fatalf("parent child count = %d, want 0", len(parentNode.Children))
	}
	if dispatcher.Dispatch(events.Event{Type: events.Click, Target: grandchildNode.ID}) || called {
		t.Fatal("removed descendant click handler was invoked")
	}
	if got, want := mutations, 1; got != want {
		t.Fatalf("mutation count = %d, want %d", got, want)
	}
}

func TestRemoveRejectsDetachedElementWithoutMutation(t *testing.T) {
	mutations := 0
	api := New(dommodel.NewDocument(), events.NewDispatcher(), func() { mutations++ })
	if api.CreateElement("div").Remove() {
		t.Fatal("Remove(detached) = true, want false")
	}
	if mutations != 0 {
		t.Fatalf("mutation count = %d, want 0", mutations)
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
