package yaegi

import (
	"context"
	"io"
	"strings"
	"testing"
	"testing/fstest"

	dommodel "github.com/saku0512/growse/internal/dom"
	"github.com/saku0512/growse/internal/events"
	runtimemodel "github.com/saku0512/growse/internal/runtime"
)

func TestRuntimeStartsMainAfterLoadingMultipleScripts(t *testing.T) {
	runtime := New()
	scripts := []runtimemodel.Script{
		{Source: "package main\nvar Started bool\nfunc markStarted() { Started = true }"},
		{Source: "package main\nfunc main() { markStarted() }"},
	}

	if err := runtime.Load(context.Background(), scripts, runtimemodel.Environment{}); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	packageSymbols, ok := runtime.interpreter.Symbols("page")["page"]
	if !ok {
		t.Fatal("page package symbols were not exported")
	}
	value, ok := packageSymbols["Started"]
	if !ok {
		t.Fatal("Started symbol was not exported")
	}
	if !value.Bool() {
		t.Fatal("main() was not invoked")
	}
}

func TestPortableFSNormalizesWindowsSeparators(t *testing.T) {
	filesystem := portableFS{FS: fstest.MapFS{
		"src/main/page/main.go": {Data: []byte("package main")},
	}}
	file, err := filesystem.Open(`src\main\page\main.go`)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer file.Close()
	content, err := io.ReadAll(file)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if got, want := string(content), "package main"; got != want {
		t.Fatalf("content = %q, want %q", got, want)
	}
}

func TestRuntimeExposesGrowseConsole(t *testing.T) {
	runtime := New()
	var messages []string
	scripts := []runtimemodel.Script{{Source: `package main
import "growse/console"
func main() { console.Log("Hello from Go", 42) }`}}
	environment := runtimemodel.Environment{ConsoleLog: func(message string) {
		messages = append(messages, message)
	}}

	if err := runtime.Load(context.Background(), scripts, environment); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if got, want := len(messages), 1; got != want {
		t.Fatalf("console message count = %d, want %d (%v)", got, want, messages)
	}
	if got, want := messages[0], "[WebGo] Hello from Go42"; got != want {
		t.Fatalf("console message = %q, want %q", got, want)
	}
}

func TestRuntimeExposesGrowseDOM(t *testing.T) {
	document := dommodel.NewDocument()
	message := document.CreateElement("p", map[string]string{"id": "message"})
	if err := document.AppendChild(document.Root, message); err != nil {
		t.Fatal(err)
	}
	if err := document.AppendChild(message, document.CreateText("before")); err != nil {
		t.Fatal(err)
	}
	mutations := 0
	runtime := New()
	scripts := []runtimemodel.Script{{Source: `package main
import "growse/dom"
func main() {
	element := dom.GetElementByID("message")
	if element != nil && element.Text() == "before" {
		element.SetText("after")
	}
}`}}
	environment := runtimemodel.Environment{
		Document: document,
		Events:   events.NewDispatcher(),
		OnMutation: func() {
			mutations++
		},
	}

	if err := runtime.Load(context.Background(), scripts, environment); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if got, want := message.TextContent(), "after"; got != want {
		t.Fatalf("TextContent() = %q, want %q", got, want)
	}
	if mutations != 1 {
		t.Fatalf("mutation count = %d, want 1", mutations)
	}
}

func TestRuntimeExposesQuerySelector(t *testing.T) {
	document := dommodel.NewDocument()
	message := document.CreateElement("p", map[string]string{"class": "message featured"})
	if err := document.AppendChild(document.Root, message); err != nil {
		t.Fatal(err)
	}
	if err := document.AppendChild(message, document.CreateText("before")); err != nil {
		t.Fatal(err)
	}
	runtime := New()
	scripts := []runtimemodel.Script{{Source: `package main
import "growse/dom"
func main() {
	element := dom.QuerySelector("p.featured")
	if element != nil {
		element.SetText("after")
	}
}`}}
	environment := runtimemodel.Environment{Document: document, Events: events.NewDispatcher()}

	if err := runtime.Load(context.Background(), scripts, environment); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if got, want := message.TextContent(), "after"; got != want {
		t.Fatalf("TextContent() = %q, want %q", got, want)
	}
}

func TestRuntimeExposesCreateElement(t *testing.T) {
	document := dommodel.NewDocument()
	runtime := New()
	scripts := []runtimemodel.Script{{Source: `package main
import "growse/dom"
var Created *dom.Element
func main() { Created = dom.CreateElement("section") }`}}
	environment := runtimemodel.Environment{Document: document, Events: events.NewDispatcher()}

	if err := runtime.Load(context.Background(), scripts, environment); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	packageSymbols := runtime.interpreter.Symbols("page")["page"]
	created, ok := packageSymbols["Created"]
	if !ok || created.IsNil() {
		t.Fatal("CreateElement() did not return a WebGo element")
	}
	if got, want := document.NodeCount(), 1; got != want {
		t.Fatalf("NodeCount() = %d, want %d", got, want)
	}
	if got, want := document.ElementCount(), 0; got != want {
		t.Fatalf("ElementCount() = %d, want %d before attachment", got, want)
	}
}

func TestRuntimeAppendsCreatedElement(t *testing.T) {
	document := dommodel.NewDocument()
	list := document.CreateElement("ul", map[string]string{"id": "list"})
	if err := document.AppendChild(document.Root, list); err != nil {
		t.Fatal(err)
	}
	mutations := 0
	runtime := New()
	scripts := []runtimemodel.Script{{Source: `package main
import "growse/dom"
func main() {
	list := dom.GetElementByID("list")
	item := dom.CreateElement("li")
	item.SetText("created")
	list.AppendChild(item)
}`}}
	environment := runtimemodel.Environment{
		Document: document,
		Events:   events.NewDispatcher(),
		OnMutation: func() {
			mutations++
		},
	}

	if err := runtime.Load(context.Background(), scripts, environment); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if got, want := len(list.Children), 1; got != want {
		t.Fatalf("list child count = %d, want %d", got, want)
	}
	if got, want := list.Children[0].TextContent(), "created"; got != want {
		t.Fatalf("item text = %q, want %q", got, want)
	}
	if got, want := mutations, 2; got != want {
		t.Fatalf("mutation count = %d, want %d", got, want)
	}
}

func TestRuntimeRemovesElement(t *testing.T) {
	document := dommodel.NewDocument()
	item := document.CreateElement("li", map[string]string{"id": "item"})
	if err := document.AppendChild(document.Root, item); err != nil {
		t.Fatal(err)
	}
	mutations := 0
	runtime := New()
	scripts := []runtimemodel.Script{{Source: `package main
import "growse/dom"
func main() { dom.GetElementByID("item").Remove() }`}}
	environment := runtimemodel.Environment{
		Document: document,
		Events:   events.NewDispatcher(),
		OnMutation: func() {
			mutations++
		},
	}

	if err := runtime.Load(context.Background(), scripts, environment); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if _, ok := document.GetElementByID("item"); ok {
		t.Fatal("removed item remains in document")
	}
	if got, want := mutations, 1; got != want {
		t.Fatalf("mutation count = %d, want %d", got, want)
	}
}

func TestRuntimeGetsAndSetsAttributes(t *testing.T) {
	document := dommodel.NewDocument()
	item := document.CreateElement("li", map[string]string{"id": "item", "data-state": "before"})
	if err := document.AppendChild(document.Root, item); err != nil {
		t.Fatal(err)
	}
	runtime := New()
	scripts := []runtimemodel.Script{{Source: `package main
import "growse/dom"
func main() {
	item := dom.GetElementByID("item")
	value, ok := item.GetAttribute("data-state")
	if ok && value == "before" {
		item.SetAttribute("data-state", "after")
	}
}`}}
	environment := runtimemodel.Environment{Document: document, Events: events.NewDispatcher()}

	if err := runtime.Load(context.Background(), scripts, environment); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if got, ok := item.Attribute("data-state"); !ok || got != "after" {
		t.Fatalf("data-state = (%q, %v), want (after, true)", got, ok)
	}
}

func TestRuntimeAddsAndRemovesClasses(t *testing.T) {
	document := dommodel.NewDocument()
	item := document.CreateElement("li", map[string]string{"id": "item", "class": "todo pending"})
	if err := document.AppendChild(document.Root, item); err != nil {
		t.Fatal(err)
	}
	runtime := New()
	scripts := []runtimemodel.Script{{Source: `package main
import "growse/dom"
func main() {
	item := dom.GetElementByID("item")
	item.AddClass("completed")
	item.RemoveClass("pending")
}`}}
	environment := runtimemodel.Environment{Document: document, Events: events.NewDispatcher()}

	if err := runtime.Load(context.Background(), scripts, environment); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if got, ok := item.Attribute("class"); !ok || got != "todo completed" {
		t.Fatalf("class = (%q, %v), want (todo completed, true)", got, ok)
	}
}

func TestRuntimeGetsAndSetsInputValue(t *testing.T) {
	document := dommodel.NewDocument()
	input := document.CreateElement("input", map[string]string{"id": "input", "value": "before"})
	if err := document.AppendChild(document.Root, input); err != nil {
		t.Fatal(err)
	}
	runtime := New()
	scripts := []runtimemodel.Script{{Source: `package main
import "growse/dom"
func main() {
	input := dom.GetElementByID("input")
	if input.Value() == "before" {
		input.SetValue("after")
	}
}`}}
	environment := runtimemodel.Environment{Document: document, Events: events.NewDispatcher()}

	if err := runtime.Load(context.Background(), scripts, environment); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if got, ok := input.Attribute("value"); !ok || got != "after" {
		t.Fatalf("input value = (%q, %v), want (after, true)", got, ok)
	}
}

func TestRuntimeReceivesClickEventData(t *testing.T) {
	document := dommodel.NewDocument()
	button := document.CreateElement("button", map[string]string{"id": "save"})
	message := document.CreateElement("p", map[string]string{"id": "message"})
	for _, node := range []*dommodel.Node{button, message} {
		if err := document.AppendChild(document.Root, node); err != nil {
			t.Fatal(err)
		}
	}
	dispatcher := events.NewDispatcher()
	runtime := New()
	scripts := []runtimemodel.Script{{Source: `package main
import "growse/dom"
func main() {
	button := dom.GetElementByID("save")
	message := dom.GetElementByID("message")
	button.OnClickEvent(func(event dom.Event) {
		if event.Type == "click" && event.TargetID == "save" && event.X == 12 && event.Y == 34 {
			message.SetText("received")
		}
	})
}`}}
	environment := runtimemodel.Environment{Document: document, Events: dispatcher}

	if err := runtime.Load(context.Background(), scripts, environment); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if !dispatcher.Dispatch(events.Event{Type: events.Click, Target: button.ID, X: 12, Y: 34}) {
		t.Fatal("click event was not handled")
	}
	if got, want := message.TextContent(), "received"; got != want {
		t.Fatalf("message = %q, want %q", got, want)
	}
}

func TestRuntimeReceivesInputEvent(t *testing.T) {
	document := dommodel.NewDocument()
	input := document.CreateElement("input", map[string]string{"id": "query"})
	message := document.CreateElement("p", map[string]string{"id": "message"})
	for _, node := range []*dommodel.Node{input, message} {
		if err := document.AppendChild(document.Root, node); err != nil {
			t.Fatal(err)
		}
	}
	dispatcher := events.NewDispatcher()
	runtime := New()
	scripts := []runtimemodel.Script{{Source: `package main
import "growse/dom"
func main() {
	dom.GetElementByID("query").OnInput(func(event dom.Event) {
		if event.Type == "input" {
			dom.GetElementByID("message").SetText(event.Value)
		}
	})
}`}}
	environment := runtimemodel.Environment{Document: document, Events: dispatcher}

	if err := runtime.Load(context.Background(), scripts, environment); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if !document.SetAttribute(input.ID, "value", "gopher") {
		t.Fatal("SetAttribute(value) = false, want true")
	}
	if !dispatcher.Dispatch(events.Event{Type: events.Input, Target: input.ID, Value: "gopher"}) {
		t.Fatal("input event was not handled")
	}
	if got, want := message.TextContent(), "gopher"; got != want {
		t.Fatalf("message = %q, want %q", got, want)
	}
}

func TestRuntimeReceivesChangeEvent(t *testing.T) {
	document := dommodel.NewDocument()
	input := document.CreateElement("input", map[string]string{"id": "query", "value": "done"})
	message := document.CreateElement("p", map[string]string{"id": "message"})
	for _, node := range []*dommodel.Node{input, message} {
		if err := document.AppendChild(document.Root, node); err != nil {
			t.Fatal(err)
		}
	}
	dispatcher := events.NewDispatcher()
	runtime := New()
	scripts := []runtimemodel.Script{{Source: `package main
import "growse/dom"
func main() {
	dom.GetElementByID("query").OnChange(func(event dom.Event) {
		dom.GetElementByID("message").SetText(event.Type + ":" + event.Value)
	})
}`}}
	environment := runtimemodel.Environment{Document: document, Events: dispatcher}

	if err := runtime.Load(context.Background(), scripts, environment); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if !dispatcher.Dispatch(events.Event{Type: events.Change, Target: input.ID, Value: "done"}) {
		t.Fatal("change event was not handled")
	}
	if got, want := message.TextContent(), "change:done"; got != want {
		t.Fatalf("message = %q, want %q", got, want)
	}
}

func TestRuntimeDispatchesWebGoOnClick(t *testing.T) {
	document := dommodel.NewDocument()
	button := document.CreateElement("button", map[string]string{"id": "increment"})
	message := document.CreateElement("p", map[string]string{"id": "message"})
	for _, node := range []*dommodel.Node{button, message} {
		if err := document.AppendChild(document.Root, node); err != nil {
			t.Fatal(err)
		}
	}
	if err := document.AppendChild(message, document.CreateText("before")); err != nil {
		t.Fatal(err)
	}
	dispatcher := events.NewDispatcher()
	runtime := New()
	scripts := []runtimemodel.Script{{Source: `package main
import "growse/dom"
func main() {
	button := dom.GetElementByID("increment")
	message := dom.GetElementByID("message")
	button.OnClick(func() { message.SetText("clicked") })
}`}}
	environment := runtimemodel.Environment{Document: document, Events: dispatcher}

	if err := runtime.Load(context.Background(), scripts, environment); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if !dispatcher.Dispatch(events.Event{Type: events.Click, Target: button.ID}) {
		t.Fatal("click event was not handled")
	}
	if got, want := message.TextContent(), "clicked"; got != want {
		t.Fatalf("TextContent() = %q, want %q", got, want)
	}
}

func TestRuntimeLoadRequiresMainPackage(t *testing.T) {
	runtime := New()
	err := runtime.Load(context.Background(), []runtimemodel.Script{{
		Source: "package other\nfunc main() {}",
	}}, runtimemodel.Environment{})
	if err == nil || !strings.Contains(err.Error(), "want package main") {
		t.Fatalf("Load() error = %v, want package validation error", err)
	}
}

func TestRuntimeReportsMissingMain(t *testing.T) {
	runtime := New()
	err := runtime.Load(context.Background(), []runtimemodel.Script{{
		Source: "package main\nfunc helper() {}",
	}}, runtimemodel.Environment{})
	if err == nil || !strings.Contains(err.Error(), "exactly 1") {
		t.Fatalf("Load() error = %v, want missing main error", err)
	}
}

func TestRuntimeStopIsIdempotent(t *testing.T) {
	runtime := New()
	if err := runtime.Stop(); err != nil {
		t.Fatalf("first Stop() error = %v", err)
	}
	if err := runtime.Stop(); err != nil {
		t.Fatalf("second Stop() error = %v", err)
	}
}
