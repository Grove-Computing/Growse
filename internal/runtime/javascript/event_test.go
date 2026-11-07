package javascript

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/events"
	"github.com/Grove-Computing/Growse/internal/forms"
	htmlparser "github.com/Grove-Computing/Growse/internal/html"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
	"github.com/dop251/goja"
)

func TestBrowserInputIsVisibleToReactStyleValueTracker(t *testing.T) {
	document, err := htmlparser.Parse(strings.NewReader(`<input id="query" value="hoge">`))
	if err != nil {
		t.Fatal(err)
	}
	target, _ := document.GetElementByID("query")
	dispatcher := events.NewDispatcher()
	runtime := New()
	t.Cleanup(func() { _ = runtime.Stop() })
	source := `
		var input = document.getElementById("query");
		if (!("oninput" in document)) throw new Error("input events are not advertised");
		if (input.type !== "text") throw new Error("input type default is not text");
		var descriptor = Object.getOwnPropertyDescriptor(input.constructor.prototype, "value");
		var trackerValue = input.value;
		Object.defineProperty(input, "value", {
			configurable: true,
			get: function () { return descriptor.get.call(this); },
			set: function (value) {
				trackerValue = String(value);
				descriptor.set.call(this, value);
			}
		});
		input._valueTracker = {
			getValue: function () { return trackerValue; },
			setValue: function (value) { trackerValue = String(value); }
		};
		var received = "";
		document.addEventListener("input", function (event) {
			var nextValue = event.target.value;
			if (event.target._valueTracker.getValue() !== nextValue && event.isTrusted) {
				event.target._valueTracker.setValue(nextValue);
				received = nextValue;
			}
		});`
	startJavaScriptRuntime(t, runtime, source, runtimemodel.Environment{Document: document, Events: dispatcher})

	if !runtime.DispatchDOMEvent(events.Event{Type: events.Input, Target: target.ID, Value: "golang"}) {
		t.Fatal("DispatchDOMEvent() = false, want true")
	}
	var received string
	if err := runtime.runSync(context.Background(), func(vm *goja.Runtime) error {
		received = vm.Get("received").String()
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if received != "golang" {
		t.Fatalf("React-style tracker received = %q, want golang", received)
	}
	if value := forms.CurrentValue(target); value != "golang" {
		t.Fatalf("input value = %q, want golang", value)
	}
}

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

func TestInitialInlineEventHandlerAttributesExecuteWithElementReceiver(t *testing.T) {
	document, err := htmlparser.Parse(strings.NewReader(`<button id="target" onload="this.setAttribute('loaded', event.type)"></button>`))
	if err != nil {
		t.Fatal(err)
	}
	runtime := New()
	t.Cleanup(func() { _ = runtime.Stop() })
	source := `document.getElementById("target").dispatchEvent(new Event("load"));`
	startJavaScriptRuntime(t, runtime, source, runtimemodel.Environment{Document: document, Events: events.NewDispatcher()})
	target, _ := document.GetElementByID("target")
	if value, _ := target.Attribute("loaded"); value != "load" {
		t.Fatalf("inline load handler value = %q, want load", value)
	}
}

func TestLifecycleEventHandlerPropertiesRunAndCanBeReplaced(t *testing.T) {
	document, err := htmlparser.Parse(strings.NewReader(`<main></main>`))
	if err != nil {
		t.Fatal(err)
	}
	runtime := New()
	t.Cleanup(func() { _ = runtime.Stop() })
	source := `
		var lifecycle = [];
		function removed() { lifecycle.push("removed"); }
		document.onreadystatechange = removed;
		document.onreadystatechange = function (event) {
			lifecycle.push(event.type + ":" + document.readyState + ":" + (this === document));
		};
		window.onload = function (event) {
			lifecycle.push(event.type + ":" + document.readyState + ":" + (this === window));
		};`
	startJavaScriptRuntime(t, runtime, source, runtimemodel.Environment{Document: document, Events: events.NewDispatcher()})

	var lifecycle []string
	if err := runtime.runSync(context.Background(), func(vm *goja.Runtime) error {
		return vm.ExportTo(vm.Get("lifecycle"), &lifecycle)
	}); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"readystatechange:interactive:true",
		"readystatechange:complete:true",
		"load:complete:true",
	}
	if !reflect.DeepEqual(lifecycle, want) {
		t.Fatalf("lifecycle handlers = %v, want %v", lifecycle, want)
	}
}

func TestJavaScriptEventConstructorsDispatchOncePassiveAndImmediateStop(t *testing.T) {
	document, err := htmlparser.Parse(strings.NewReader(`<main id="parent"><button id="target">Run</button></main>`))
	if err != nil {
		t.Fatal(err)
	}
	var message string
	runtime := New()
	t.Cleanup(func() { _ = runtime.Stop() })
	source := `
		var parentElement = document.getElementById("parent");
		var target = document.getElementById("target");
		var calls = [];
		parentElement.addEventListener("hydrate", function (event) {
			calls.push("capture:" + event.eventPhase + ":" + (event.target === target) + ":" + (event.currentTarget === parentElement));
		}, { capture: true, once: true });
		target.addEventListener("hydrate", function (event) {
			event.preventDefault();
			calls.push("passive:" + event.defaultPrevented);
		}, { passive: true });
		target.addEventListener("hydrate", function (event) {
			calls.push("immediate:" + event.detail.step);
			event.stopImmediatePropagation();
		});
		target.addEventListener("hydrate", function () { calls.push("late"); });
		parentElement.addEventListener("hydrate", function () { calls.push("bubble"); });
		var first = new CustomEvent("hydrate", { bubbles: true, cancelable: true, detail: { step: 1 } });
		var firstResult = target.dispatchEvent(first);
		var secondResult = target.dispatchEvent(new CustomEvent("hydrate", { bubbles: true, cancelable: true, detail: { step: 2 } }));
		var cancel = function (event) { event.preventDefault(); };
		target.addEventListener("cancel-me", cancel);
		var canceled = new Event("cancel-me", { cancelable: true });
		var cancelResult = target.dispatchEvent(canceled);
		target.removeEventListener("cancel-me", cancel);
		var removedResult = target.dispatchEvent(new Event("cancel-me"));
		var documentCalls = 0;
		document.addEventListener("document-event", function () { documentCalls++; }, { once: true });
		document.dispatchEvent(new Event("document-event"));
		document.dispatchEvent(new Event("document-event"));
		var detached = document.createElement("button");
		var detachedCalls = 0;
		detached.addEventListener("detached-event", function () { detachedCalls++; });
		detached.dispatchEvent(new Event("detached-event"));
		var mouse = new MouseEvent("click", { clientX: 12, clientY: 34, bubbles: true });
		var keyboard = new KeyboardEvent("keydown", { key: "Enter", code: "Enter", repeat: true });
		console.log([
			calls.join(","), firstResult, secondResult, !cancelResult, canceled.defaultPrevented,
			removedResult, documentCalls, detachedCalls, first instanceof CustomEvent, first instanceof Event,
			mouse instanceof MouseEvent, mouse.clientX, mouse.clientY,
			keyboard instanceof KeyboardEvent, keyboard.key, keyboard.code, keyboard.repeat,
			first.eventPhase, first.currentTarget === null, first.target === target
		].join("|"));`
	environment := runtimemodel.Environment{
		Document: document, Events: events.NewDispatcher(), ConsoleRecord: func(_, value string) { message = value },
	}
	if err := runtime.Load(context.Background(), []runtimemodel.Script{javaScript(source)}, environment); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := "capture:1:true:true,passive:false,immediate:1,passive:false,immediate:2|true|true|true|true|true|1|1|true|true|true|12|34|true|Enter|Enter|true|0|true|true"
	if message != want {
		t.Fatalf("JavaScript Event result = %q, want %q", message, want)
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
