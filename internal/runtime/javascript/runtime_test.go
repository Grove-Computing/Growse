package javascript

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Grove-Computing/Growse/internal/events"
	htmlparser "github.com/Grove-Computing/Growse/internal/html"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
	"github.com/dop251/goja"
)

func TestRuntimeEvaluatesScriptsInDocumentOrder(t *testing.T) {
	runtime := New()
	t.Cleanup(func() { _ = runtime.Stop() })
	var records [][2]string
	environment := runtimemodel.Environment{ConsoleRecord: func(level, message string) {
		records = append(records, [2]string{level, message})
	}}
	scripts := []runtimemodel.Script{
		javaScript(`var order = ["first"];`),
		javaScript(`order.push("second"); console.log(order.join(","));`),
	}

	if err := runtime.Load(context.Background(), scripts, environment); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if got, want := records, [][2]string{{"log", "first,second"}}; !equalRecords(got, want) {
		t.Fatalf("console records = %v, want %v", got, want)
	}
}

func TestRuntimeContainsSyntaxAndRuntimeErrors(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{name: "syntax", source: `function (`},
		{name: "runtime", source: `throw new Error("page failure")`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runtime := New()
			t.Cleanup(func() { _ = runtime.Stop() })
			var records [][2]string
			environment := runtimemodel.Environment{ConsoleRecord: func(level, message string) { records = append(records, [2]string{level, message}) }}
			if err := runtime.Load(context.Background(), []runtimemodel.Script{javaScript(test.source)}, environment); err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if err := runtime.Start(context.Background()); err != nil {
				t.Fatalf("contained Start() error = %v", err)
			}
			if len(records) != 1 || records[0][0] != "error" {
				t.Fatalf("contained script records = %v", records)
			}
		})
	}

	good := New()
	t.Cleanup(func() { _ = good.Stop() })
	if err := good.Load(context.Background(), []runtimemodel.Script{javaScript(`console.log("still alive")`)}, runtimemodel.Environment{}); err != nil {
		t.Fatalf("independent Load() error = %v", err)
	}
	if err := good.Start(context.Background()); err != nil {
		t.Fatalf("independent Start() error = %v", err)
	}
}

func TestRuntimeOrdersClassicScriptsAndDocumentLifecycle(t *testing.T) {
	document, _ := htmlparser.Parse(strings.NewReader(`<p>page</p>`))
	var records [][2]string
	environment := runtimemodel.Environment{
		Document:      document,
		ConsoleRecord: func(level, message string) { records = append(records, [2]string{level, message}) },
	}
	scripts := []runtimemodel.Script{
		{Engine: runtimemodel.EngineJavaScript, Schedule: runtimemodel.ScriptDefer, DocumentOrder: 4, Source: `order.push("defer-2")`},
		{Engine: runtimemodel.EngineJavaScript, Schedule: runtimemodel.ScriptAsync, DocumentOrder: 6, FetchOrder: 2, Source: `order.push("async-slow")`},
		{Engine: runtimemodel.EngineJavaScript, Schedule: runtimemodel.ScriptParserBlocking, DocumentOrder: 1, Source: `
			var order = ["blocking-1"];
			document.addEventListener("readystatechange", function () { order.push("ready:" + document.readyState); });
			document.addEventListener("DOMContentLoaded", function (event) { order.push(event.type + ":" + document.readyState); });
			addEventListener("load", function () { order.push("load:" + document.readyState); console.log(order.join(",")); });`},
		{Engine: runtimemodel.EngineJavaScript, Schedule: runtimemodel.ScriptAsync, DocumentOrder: 5, FetchOrder: 1, Source: `order.push("async-fast")`},
		{Engine: runtimemodel.EngineJavaScript, Schedule: runtimemodel.ScriptParserBlocking, DocumentOrder: 2, Source: `order.push("blocking-2")`},
		{Engine: runtimemodel.EngineJavaScript, Schedule: runtimemodel.ScriptDefer, DocumentOrder: 3, Source: `order.push("defer-1")`},
	}
	runtime := New()
	t.Cleanup(func() { _ = runtime.Stop() })
	if err := runtime.Load(context.Background(), scripts, environment); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := "blocking-1,blocking-2,ready:interactive,defer-1,defer-2,DOMContentLoaded:interactive,async-fast,async-slow,ready:complete,load:complete"
	if len(records) != 1 || records[0] != [2]string{"log", want} {
		t.Fatalf("lifecycle records = %v, want %q", records, want)
	}
	if got := document.ReadyState(); got != "complete" {
		t.Fatalf("document.readyState = %q", got)
	}
}

func TestRuntimeContinuesAfterIndependentScriptFailure(t *testing.T) {
	var records [][2]string
	runtime := New()
	t.Cleanup(func() { _ = runtime.Stop() })
	scripts := []runtimemodel.Script{
		javaScript(`throw new Error("first failed")`),
		javaScript(`console.log("second executed")`),
	}
	environment := runtimemodel.Environment{ConsoleRecord: func(level, message string) { records = append(records, [2]string{level, message}) }}
	if err := runtime.Load(context.Background(), scripts, environment); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || records[0][0] != "error" || !strings.Contains(records[0][1], "first failed") || records[1] != [2]string{"log", "second executed"} {
		t.Fatalf("independent script records = %v", records)
	}
}

func TestRuntimeInterruptsRunningScriptWhenPageIsCanceled(t *testing.T) {
	pageContext, cancelPage := context.WithCancel(context.Background())
	document, _ := htmlparser.Parse(strings.NewReader(`<p>page</p>`))
	var records [][2]string
	runtime := New()
	t.Cleanup(func() { _ = runtime.Stop() })
	source := `
		document.addEventListener("DOMContentLoaded", function () { console.log("stale DOMContentLoaded"); });
		addEventListener("load", function () { console.log("stale load"); });
		for (;;) {}`
	environment := runtimemodel.Environment{
		Document:      document,
		ConsoleRecord: func(level, message string) { records = append(records, [2]string{level, message}) },
	}
	if err := runtime.Load(pageContext, []runtimemodel.Script{javaScript(source)}, environment); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	finished := make(chan error, 1)
	go func() { finished <- runtime.Start(context.Background()) }()
	time.Sleep(10 * time.Millisecond)
	cancelPage()

	select {
	case err := <-finished:
		if err == nil || !strings.Contains(err.Error(), "context canceled") {
			t.Fatalf("Start() error = %v, want context canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Start() did not stop after Page cancellation")
	}
	if document.ReadyState() != "loading" || len(records) != 0 {
		t.Fatalf("canceled lifecycle = state:%q records:%v", document.ReadyState(), records)
	}
}

func TestRuntimeRejectsWrongEngineAndUseAfterStop(t *testing.T) {
	runtime := New()
	if err := runtime.Load(context.Background(), []runtimemodel.Script{{Engine: runtimemodel.EngineGo, Source: "package main"}}, runtimemodel.Environment{}); err == nil {
		t.Fatal("Load() error = nil, want engine mismatch")
	}
	if err := runtime.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if err := runtime.Load(context.Background(), []runtimemodel.Script{javaScript(`true`)}, runtimemodel.Environment{}); !errors.Is(err, errRuntimeStopped) {
		t.Fatalf("Load() error = %v, want errRuntimeStopped", err)
	}
}

func TestRuntimeDoesNotExposeProcessOrGoHostAPIs(t *testing.T) {
	runtime := New()
	t.Cleanup(func() { _ = runtime.Stop() })
	var message string
	environment := runtimemodel.Environment{ConsoleRecord: func(_, value string) { message = value }}
	source := `console.log([
		typeof process,
		typeof require,
		typeof Deno,
		typeof os,
		typeof fs,
		typeof Go,
		typeof GrowseGo
	].join(","))`
	if err := runtime.Load(context.Background(), []runtimemodel.Script{javaScript(source)}, environment); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if got, want := message, "undefined,undefined,undefined,undefined,undefined,undefined,undefined"; got != want {
		t.Fatalf("host globals = %q, want %q", got, want)
	}
}

func TestRuntimeSerializesConcurrentPageEvents(t *testing.T) {
	document, err := htmlparser.Parse(strings.NewReader(`<button id="counter">count</button>`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	button, ok := document.GetElementByID("counter")
	if !ok {
		t.Fatal("counter button was not parsed")
	}
	dispatcher := events.NewDispatcher()
	runtime := New()
	t.Cleanup(func() { _ = runtime.Stop() })
	environment := runtimemodel.Environment{Document: document, Events: dispatcher}
	source := `
		var eventCount = 0;
		document.getElementById("counter").addEventListener("click", function () {
			eventCount += 1;
		});`
	if err := runtime.Load(context.Background(), []runtimemodel.Script{javaScript(source)}, environment); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	const eventCount = 128
	var wait sync.WaitGroup
	wait.Add(eventCount)
	for range eventCount {
		go func() {
			defer wait.Done()
			if !runtime.DispatchPageEvent(func() bool {
				return dispatcher.Dispatch(events.Event{Type: events.Click, Target: button.ID})
			}) {
				t.Errorf("DispatchPageEvent() = false, want true")
			}
		}()
	}
	wait.Wait()

	var got int64
	if err := runtime.runSync(context.Background(), func(vm *goja.Runtime) error {
		got = vm.Get("eventCount").ToInteger()
		return nil
	}); err != nil {
		t.Fatalf("read eventCount: %v", err)
	}
	if got != eventCount {
		t.Fatalf("eventCount = %d, want %d", got, eventCount)
	}
}

func javaScript(source string) runtimemodel.Script {
	return runtimemodel.Script{Engine: runtimemodel.EngineJavaScript, Source: source, Inline: true}
}

func equalRecords(left, right [][2]string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
