package dom

import (
	"reflect"
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

func TestGetAndSetAttribute(t *testing.T) {
	document := dommodel.NewDocument()
	node := document.CreateElement("div", map[string]string{"id": "item", "class": "before"})
	if err := document.AppendChild(document.Root, node); err != nil {
		t.Fatal(err)
	}
	mutations := 0
	element := New(document, events.NewDispatcher(), func() { mutations++ }).GetElementByID("item")

	if got, ok := element.GetAttribute(" CLASS "); !ok || got != "before" {
		t.Fatalf("GetAttribute(class) = (%q, %v), want (before, true)", got, ok)
	}
	if !element.SetAttribute("DATA-ID", "42") {
		t.Fatal("SetAttribute(data-id) = false, want true")
	}
	if got, ok := element.GetAttribute("data-id"); !ok || got != "42" {
		t.Fatalf("GetAttribute(data-id) = (%q, %v), want (42, true)", got, ok)
	}
	if element.SetAttribute("data-id", "42") {
		t.Fatal("SetAttribute(data-id) = true for unchanged value")
	}
	if got, want := mutations, 1; got != want {
		t.Fatalf("mutation count = %d, want %d", got, want)
	}
}

func TestAttributeRejectsInvalidNameAndRemovedElement(t *testing.T) {
	document := dommodel.NewDocument()
	node := document.CreateElement("div", map[string]string{"id": "item"})
	if err := document.AppendChild(document.Root, node); err != nil {
		t.Fatal(err)
	}
	element := New(document, events.NewDispatcher(), nil).GetElementByID("item")
	for _, name := range []string{"", "data id", "<id>"} {
		if element.SetAttribute(name, "value") {
			t.Fatalf("SetAttribute(%q) = true, want false", name)
		}
		if value, ok := element.GetAttribute(name); ok || value != "" {
			t.Fatalf("GetAttribute(%q) = (%q, %v), want empty false", name, value, ok)
		}
	}
	if !element.Remove() {
		t.Fatal("Remove() = false, want true")
	}
	if element.SetAttribute("id", "new") {
		t.Fatal("SetAttribute() = true for removed element")
	}
	if value, ok := element.GetAttribute("id"); ok || value != "" {
		t.Fatalf("GetAttribute() = (%q, %v) for removed element", value, ok)
	}
}

func TestAddAndRemoveClass(t *testing.T) {
	document := dommodel.NewDocument()
	node := document.CreateElement("li", map[string]string{"id": "item", "class": "todo active"})
	if err := document.AppendChild(document.Root, node); err != nil {
		t.Fatal(err)
	}
	mutations := 0
	element := New(document, events.NewDispatcher(), func() { mutations++ }).GetElementByID("item")

	if !element.AddClass("completed") {
		t.Fatal("AddClass(completed) = false, want true")
	}
	if element.AddClass("completed") {
		t.Fatal("duplicate AddClass(completed) = true, want false")
	}
	if !element.RemoveClass("active") {
		t.Fatal("RemoveClass(active) = false, want true")
	}
	if element.RemoveClass("missing") {
		t.Fatal("RemoveClass(missing) = true, want false")
	}
	if got, ok := element.GetAttribute("class"); !ok || got != "todo completed" {
		t.Fatalf("class = (%q, %v), want (todo completed, true)", got, ok)
	}
	if got, want := mutations, 2; got != want {
		t.Fatalf("mutation count = %d, want %d", got, want)
	}
}

func TestClassOperationsRejectInvalidClassName(t *testing.T) {
	api := New(dommodel.NewDocument(), events.NewDispatcher(), nil)
	element := api.CreateElement("div")
	for _, className := range []string{"", "two classes", ".class", "<class>"} {
		if element.AddClass(className) {
			t.Fatalf("AddClass(%q) = true, want false", className)
		}
		if element.RemoveClass(className) {
			t.Fatalf("RemoveClass(%q) = true, want false", className)
		}
	}
}

func TestValueAndSetValueForTextInput(t *testing.T) {
	document := dommodel.NewDocument()
	input := document.CreateElement("input", map[string]string{"id": "input", "type": "text", "value": "before"})
	if err := document.AppendChild(document.Root, input); err != nil {
		t.Fatal(err)
	}
	mutations := 0
	element := New(document, events.NewDispatcher(), func() { mutations++ }).GetElementByID("input")

	if got, want := element.Value(), "before"; got != want {
		t.Fatalf("Value() = %q, want %q", got, want)
	}
	if !element.SetValue("after") {
		t.Fatal("SetValue() = false, want true")
	}
	if got, want := element.Value(), "after"; got != want {
		t.Fatalf("Value() = %q, want %q", got, want)
	}
	if element.SetValue("after") {
		t.Fatal("SetValue() = true for unchanged value")
	}
	if got, want := mutations, 1; got != want {
		t.Fatalf("mutation count = %d, want %d", got, want)
	}
}

func TestValueAPIIgnoresNonTextInput(t *testing.T) {
	document := dommodel.NewDocument()
	checkbox := document.CreateElement("input", map[string]string{"id": "checkbox", "type": "checkbox", "value": "on"})
	paragraph := document.CreateElement("p", map[string]string{"id": "paragraph", "value": "text"})
	for _, node := range []*dommodel.Node{checkbox, paragraph} {
		if err := document.AppendChild(document.Root, node); err != nil {
			t.Fatal(err)
		}
	}
	api := New(document, events.NewDispatcher(), nil)
	for _, id := range []string{"checkbox", "paragraph"} {
		element := api.GetElementByID(id)
		if got := element.Value(); got != "" {
			t.Fatalf("%s.Value() = %q, want empty", id, got)
		}
		if element.SetValue("changed") {
			t.Fatalf("%s.SetValue() = true, want false", id)
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

func TestOnClickEventProvidesPublicEventData(t *testing.T) {
	document := dommodel.NewDocument()
	input := document.CreateElement("input", map[string]string{"id": "query", "value": "gopher"})
	if err := document.AppendChild(document.Root, input); err != nil {
		t.Fatal(err)
	}
	dispatcher := events.NewDispatcher()
	element := New(document, dispatcher, nil).GetElementByID("query")
	var received Event
	element.OnClickEvent(func(event Event) { received = event })

	if !dispatcher.Dispatch(events.Event{Type: events.Click, Target: input.ID, X: 12, Y: 34}) {
		t.Fatal("click event was not handled")
	}
	if received.Type != "click" || received.TargetID != "query" || received.Value != "gopher" || received.X != 12 || received.Y != 34 {
		t.Fatalf("event = %#v, want click data", received)
	}
}

func TestOnClickEventRejectsNilHandlerAndDetachedElement(t *testing.T) {
	document := dommodel.NewDocument()
	dispatcher := events.NewDispatcher()
	api := New(document, dispatcher, nil)
	detached := api.CreateElement("button")
	detached.OnClickEvent(func(Event) {})
	detached.OnClickEvent(nil)
	if dispatcher.Dispatch(events.Event{Type: events.Click, Target: detached.id}) {
		t.Fatal("detached element registered a click handler")
	}
}

func TestOnMouseEnterAndLeaveProvidePublicEventData(t *testing.T) {
	document := dommodel.NewDocument()
	button := document.CreateElement("button", map[string]string{"id": "save"})
	if err := document.AppendChild(document.Root, button); err != nil {
		t.Fatal(err)
	}
	dispatcher := events.NewDispatcher()
	element := New(document, dispatcher, nil).GetElementByID("save")
	var received []Event
	element.OnMouseEnter(func(event Event) { received = append(received, event) })
	element.OnMouseLeave(func(event Event) { received = append(received, event) })

	dispatcher.Dispatch(events.Event{Type: events.MouseEnter, Target: button.ID, X: 12, Y: 34})
	dispatcher.Dispatch(events.Event{Type: events.MouseLeave, Target: button.ID, X: 56, Y: 78})

	want := []Event{
		{Type: "mouseenter", TargetID: "save", X: 12, Y: 34},
		{Type: "mouseleave", TargetID: "save", X: 56, Y: 78},
	}
	if !reflect.DeepEqual(received, want) {
		t.Fatalf("hover events = %#v, want %#v", received, want)
	}
}

func TestRemoveClearsHoverEventListeners(t *testing.T) {
	document := dommodel.NewDocument()
	button := document.CreateElement("button", map[string]string{"id": "temporary"})
	if err := document.AppendChild(document.Root, button); err != nil {
		t.Fatal(err)
	}
	dispatcher := events.NewDispatcher()
	element := New(document, dispatcher, nil).GetElementByID("temporary")
	called := false
	element.OnMouseEnter(func(Event) { called = true })
	element.OnMouseLeave(func(Event) { called = true })
	if !element.Remove() {
		t.Fatal("Remove() = false, want true")
	}

	if dispatcher.Dispatch(events.Event{Type: events.MouseEnter, Target: button.ID}) ||
		dispatcher.Dispatch(events.Event{Type: events.MouseLeave, Target: button.ID}) || called {
		t.Fatal("removed element retained a hover event listener")
	}
}

func TestOnInputProvidesUpdatedValue(t *testing.T) {
	document := dommodel.NewDocument()
	input := document.CreateElement("input", map[string]string{"id": "query", "value": "before"})
	if err := document.AppendChild(document.Root, input); err != nil {
		t.Fatal(err)
	}
	dispatcher := events.NewDispatcher()
	element := New(document, dispatcher, nil).GetElementByID("query")
	var received Event
	element.OnInput(func(event Event) { received = event })
	if !document.SetAttribute(input.ID, "value", "after") {
		t.Fatal("SetAttribute(value) = false, want true")
	}

	if !dispatcher.Dispatch(events.Event{Type: events.Input, Target: input.ID, Value: "after"}) {
		t.Fatal("input event was not handled")
	}
	if received.Type != "input" || received.TargetID != "query" || received.Value != "after" {
		t.Fatalf("event = %#v, want updated input data", received)
	}
}

func TestOnChangeProvidesCommittedValue(t *testing.T) {
	document := dommodel.NewDocument()
	input := document.CreateElement("input", map[string]string{"id": "query", "value": "committed"})
	if err := document.AppendChild(document.Root, input); err != nil {
		t.Fatal(err)
	}
	dispatcher := events.NewDispatcher()
	element := New(document, dispatcher, nil).GetElementByID("query")
	var received Event
	element.OnChange(func(event Event) { received = event })

	if !dispatcher.Dispatch(events.Event{Type: events.Change, Target: input.ID, Value: "committed"}) {
		t.Fatal("change event was not handled")
	}
	if received.Type != "change" || received.TargetID != "query" || received.Value != "committed" {
		t.Fatalf("event = %#v, want committed input data", received)
	}
}

func TestOnSubmitProvidesFormTarget(t *testing.T) {
	document := dommodel.NewDocument()
	form := document.CreateElement("form", map[string]string{"id": "todo-form"})
	if err := document.AppendChild(document.Root, form); err != nil {
		t.Fatal(err)
	}
	dispatcher := events.NewDispatcher()
	element := New(document, dispatcher, nil).GetElementByID("todo-form")
	var received Event
	element.OnSubmit(func(event Event) { received = event })

	if !dispatcher.Dispatch(events.Event{Type: events.Submit, Target: form.ID}) {
		t.Fatal("submit event was not handled")
	}
	if received.Type != "submit" || received.TargetID != "todo-form" {
		t.Fatalf("event = %#v, want form submit data", received)
	}
}
