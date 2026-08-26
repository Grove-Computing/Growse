package javascript

import (
	"context"
	"net/url"
	"strings"
	"testing"

	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
)

func TestPageGlobalsShareIdentityAndExposeStableNavigatorAndOrigin(t *testing.T) {
	pageURL, _ := url.Parse("https://user:secret@www.example.test:8443/page")
	var records [][2]string
	runtime := New()
	t.Cleanup(func() { _ = runtime.Stop() })
	source := `console.log([
		window === self,
		self === globalThis,
		navigator.userAgent,
		navigator.platform,
		navigator.language,
		navigator.languages.join(","),
		navigator.hardwareConcurrency,
		navigator.onLine,
		navigator.webdriver,
		origin
	].join("|"))`
	environment := runtimemodel.Environment{
		BaseURL:       pageURL,
		ConsoleRecord: func(level, message string) { records = append(records, [2]string{level, message}) },
	}
	if err := runtime.Load(context.Background(), []runtimemodel.Script{javaScript(source)}, environment); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := "true|true|Growse/0.14||en-US|en-US|1|true|false|https://www.example.test:8443"
	if len(records) != 1 || records[0] != [2]string{"log", want} {
		t.Fatalf("page globals = %v, want %q", records, want)
	}
}

func TestQueueMicrotaskRunsFIFOAtScriptCheckpointsAndContainsErrors(t *testing.T) {
	var records [][2]string
	runtime := New()
	t.Cleanup(func() { _ = runtime.Stop() })
	scripts := []runtimemodel.Script{
		javaScript(`
			var order = ["script-1"];
			queueMicrotask(function () { order.push("micro-1"); queueMicrotask(function () { order.push("micro-3"); }); });
			queueMicrotask(function () { order.push("micro-2"); throw new Error("microtask failed"); });`),
		javaScript(`order.push("script-2"); console.log(order.join(","));`),
	}
	environment := runtimemodel.Environment{ConsoleRecord: func(level, message string) { records = append(records, [2]string{level, message}) }}
	if err := runtime.Load(context.Background(), scripts, environment); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || records[0][0] != "error" || !strings.Contains(records[0][1], "microtask failed") ||
		records[1] != [2]string{"log", "script-1,micro-1,micro-2,micro-3,script-2"} {
		t.Fatalf("microtask records = %v", records)
	}
}

func TestPageGlobalsAndMicrotaskQueueDoNotCrossRuntimeBoundary(t *testing.T) {
	first := New()
	second := New()
	t.Cleanup(func() { _ = first.Stop(); _ = second.Stop() })
	if err := first.Load(context.Background(), []runtimemodel.Script{javaScript(`window.privateValue = "first"`)}, runtimemodel.Environment{}); err != nil {
		t.Fatal(err)
	}
	if err := first.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	var record string
	environment := runtimemodel.Environment{ConsoleRecord: func(_, message string) { record = message }}
	if err := second.Load(context.Background(), []runtimemodel.Script{javaScript(`console.log(typeof privateValue)`)}, environment); err != nil {
		t.Fatal(err)
	}
	if err := second.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if record != "undefined" {
		t.Fatalf("cross-runtime global = %q", record)
	}
}

func TestQueueMicrotaskEnforcesPendingLimit(t *testing.T) {
	var records [][2]string
	runtime := New()
	runtime.maxMicrotasks = 1
	t.Cleanup(func() { _ = runtime.Stop() })
	source := `queueMicrotask(function () { console.log("first ran"); }); queueMicrotask(function () {});`
	environment := runtimemodel.Environment{ConsoleRecord: func(level, message string) { records = append(records, [2]string{level, message}) }}
	if err := runtime.Load(context.Background(), []runtimemodel.Script{javaScript(source)}, environment); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || records[0] != [2]string{"log", "first ran"} || records[1][0] != "error" || !strings.Contains(records[1][1], "microtask queue limit") {
		t.Fatalf("microtask limit records = %v", records)
	}
}
