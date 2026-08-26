package javascript

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/events"
	htmlparser "github.com/Grove-Computing/Growse/internal/html"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
	"github.com/dop251/goja"
)

func TestEventListenersReceiveSupportedEventsOnPageQueue(t *testing.T) {
	document, err := htmlparser.Parse(strings.NewReader(`<input id="target" value="initial">`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	target, _ := document.GetElementByID("target")
	dispatcher := events.NewDispatcher()
	runtime := New()
	t.Cleanup(func() { _ = runtime.Stop() })
	source := `
		var received = [];
		var target = document.getElementById("target");
		["click", "input", "change", "submit", "reset", "focus", "blur", "mouseenter", "mouseleave"].forEach(function (type) {
			target.addEventListener(type, function (event) {
				if (event.target !== target) { throw new Error("wrong target"); }
				received.push(event.type + ":" + event.value + ":" + event.clientX + ":" + event.clientY);
			});
		});`
	startJavaScriptRuntime(t, runtime, source, runtimemodel.Environment{Document: document, Events: dispatcher})

	types := []events.Type{
		events.Click, events.Input, events.Change, events.Submit, events.Reset,
		events.Focus, events.Blur, events.MouseEnter, events.MouseLeave,
	}
	for _, eventType := range types {
		event := events.Event{Type: eventType, Target: target.ID, Value: "typed", X: 12, Y: 34}
		if !runtime.DispatchPageEvent(func() bool { return dispatcher.Dispatch(event) }) {
			t.Fatalf("DispatchPageEvent(%q) = false, want true", eventType)
		}
	}

	var received []string
	if err := runtime.runSync(context.Background(), func(vm *goja.Runtime) error {
		return vm.ExportTo(vm.Get("received"), &received)
	}); err != nil {
		t.Fatalf("read received events: %v", err)
	}
	if len(received) != len(types) {
		t.Fatalf("received events = %v, want %d", received, len(types))
	}
	if got, want := received[0], "click:initial:12:34"; got != want {
		t.Fatalf("click event = %q, want %q", got, want)
	}
}

func TestEventPreventDefaultDuplicateAndExceptionIsolation(t *testing.T) {
	document, err := htmlparser.Parse(strings.NewReader(`<form id="target"></form>`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	target, _ := document.GetElementByID("target")
	dispatcher := events.NewDispatcher()
	var records [][2]string
	runtime := New()
	t.Cleanup(func() { _ = runtime.Stop() })
	environment := runtimemodel.Environment{
		Document: document,
		Events:   dispatcher,
		ConsoleRecord: func(level, message string) {
			records = append(records, [2]string{level, message})
		},
	}
	source := `
		var calls = 0;
		var target = document.getElementById("target");
		function duplicate(event) { calls += 1; event.preventDefault(); throw new Error("listener failure"); }
		target.addEventListener("submit", duplicate);
		target.addEventListener("submit", duplicate);
		target.addEventListener("submit", function () { calls += 10; });`
	startJavaScriptRuntime(t, runtime, source, environment)

	event := events.Cancelable(events.Submit, target.ID)
	if !runtime.DispatchPageEvent(func() bool { return dispatcher.Dispatch(event) }) {
		t.Fatal("DispatchPageEvent() = false, want true")
	}
	if !event.DefaultPrevented() {
		t.Fatal("JavaScript preventDefault() did not cancel the browser default action")
	}
	var calls int64
	if err := runtime.runSync(context.Background(), func(vm *goja.Runtime) error {
		calls = vm.Get("calls").ToInteger()
		return nil
	}); err != nil {
		t.Fatalf("read calls: %v", err)
	}
	if calls != 11 {
		t.Fatalf("listener calls = %d, want 11 (duplicate ignored and later listener continued)", calls)
	}
	if len(records) != 1 || records[0][0] != "error" || !strings.Contains(records[0][1], "listener failure") {
		t.Fatalf("exception records = %v, want one isolated listener error", records)
	}
}

func TestEventListenerLimitAndPageClose(t *testing.T) {
	document, err := htmlparser.Parse(strings.NewReader(`<button id="target">click</button>`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	target, _ := document.GetElementByID("target")
	dispatcher := events.NewDispatcher()
	runtime := New()
	runtime.maxListeners = 1
	var records [][2]string
	source := `
		var target = document.getElementById("target");
		target.addEventListener("click", function () {});
		target.addEventListener("click", function () {});`
	if err := runtime.Load(context.Background(), []runtimemodel.Script{javaScript(source)}, runtimemodel.Environment{
		Document: document, Events: dispatcher,
		ConsoleRecord: func(level, message string) { records = append(records, [2]string{level, message}) },
	}); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("contained Start() error = %v", err)
	}
	if len(records) != 1 || !strings.Contains(records[0][1], "event listener limit exceeded") {
		t.Fatalf("event listener limit records = %v", records)
	}
	if err := runtime.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if dispatcher.Dispatch(events.Event{Type: events.Click, Target: target.ID}) {
		t.Fatal("Page close retained a JavaScript Event listener")
	}
	if runtime.DispatchPageEvent(func() bool { return true }) {
		t.Fatal("closed Runtime delivered a Page event")
	}
}

func TestEventPropagationMetadataRemovalAndCancellation(t *testing.T) {
	document, err := htmlparser.Parse(strings.NewReader(`<main id="parent"><button id="target">click</button></main>`))
	if err != nil {
		t.Fatal(err)
	}
	target, _ := document.GetElementByID("target")
	dispatcher := events.NewDispatcher()
	runtime := New()
	t.Cleanup(func() { _ = runtime.Stop() })
	var records []string
	source := `
		var order = [];
		var parentElement = document.getElementById("parent");
		var target = document.getElementById("target");
		function removed() { order.push("removed"); }
		target.addEventListener("click", removed);
		target.removeEventListener("click", removed);
		parentElement.addEventListener("click", function (event) {
			order.push("capture:" + event.eventPhase + ":" + (event.target === target) + ":" + (event.currentTarget === parentElement));
		}, {capture: true});
		target.addEventListener("click", function (event) {
			order.push("target:" + event.eventPhase + ":" + event.bubbles + ":" + event.cancelable + ":" + event.defaultPrevented);
			event.preventDefault();
			event.stopPropagation();
			order.push("prevented:" + event.defaultPrevented);
		});
		target.addEventListener("click", function () { order.push("same-target"); });
		parentElement.addEventListener("click", function () { order.push("bubble"); });`
	startJavaScriptRuntime(t, runtime, source, runtimemodel.Environment{
		Document: document, Events: dispatcher,
		ConsoleRecord: func(_, message string) { records = append(records, message) },
	})
	event := events.Cancelable(events.Click, target.ID)
	if !runtime.DispatchPageEvent(func() bool { return dispatcher.DispatchTree(document, event) }) || !event.DefaultPrevented() {
		t.Fatalf("cancelable propagated event was not handled/prevented: %v", records)
	}
	var order []string
	if err := runtime.runSync(context.Background(), func(vm *goja.Runtime) error { return vm.ExportTo(vm.Get("order"), &order) }); err != nil {
		t.Fatal(err)
	}
	want := []string{"capture:1:true:true", "target:2:true:true:false", "prevented:true", "same-target"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("JavaScript propagation order = %v, want %v", order, want)
	}
}

func startJavaScriptRuntime(t *testing.T, runtime *Runtime, source string, environment runtimemodel.Environment) {
	t.Helper()
	if err := runtime.Load(context.Background(), []runtimemodel.Script{javaScript(source)}, environment); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
}
