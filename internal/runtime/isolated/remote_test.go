package isolated

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Grove-Computing/Growse/internal/events"
	htmlparser "github.com/Grove-Computing/Growse/internal/html"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
	storagecore "github.com/Grove-Computing/Growse/internal/storage"
)

func TestIsolatedJavaScriptPreservesDOMEventStorageAndConsoleBehavior(t *testing.T) {
	document, err := htmlparser.Parse(strings.NewReader(`<button id="target">idle</button>`))
	if err != nil {
		t.Fatal(err)
	}
	target, _ := document.GetElementByID("target")
	local := storagecore.NewArea()
	var console [][2]string
	mutations := make(chan struct{}, 8)
	runtime := New(runtimemodel.EngineJavaScript)
	t.Cleanup(func() { _ = runtime.Stop() })
	environment := runtimemodel.Environment{
		Document: document, Events: events.NewDispatcher(), BaseURL: mustURL(t, "https://example.test/page"),
		LocalStorage: local, SessionStorage: storagecore.NewArea(), StorageSource: storagecore.MutationSource{ID: 7, URL: "https://example.test/page"},
		OnMutation:    func() { mutations <- struct{}{} },
		ConsoleRecord: func(level, message string) { console = append(console, [2]string{level, message}) },
	}
	source := `
		var target = document.getElementById("target");
		target.textContent = "started";
		localStorage.setItem("engine", "worker");
		console.log("isolated");
		target.addEventListener("click", function (event) {
			event.preventDefault();
			target.textContent = "clicked";
		});`
	if err := runtime.Load(context.Background(), []runtimemodel.Script{{Engine: runtimemodel.EngineJavaScript, SourceURL: environment.BaseURL, Source: source, Inline: true}}, environment); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if got := target.TextContent(); got != "started" {
		t.Fatalf("worker DOM text = %q", got)
	}
	if value, ok := local.Get("engine"); !ok || value != "worker" {
		t.Fatalf("worker localStorage = %q, %t", value, ok)
	}
	if len(console) != 1 || console[0] != [2]string{"log", "isolated"} {
		t.Fatalf("worker console = %#v", console)
	}
	event := events.Cancelable(events.Click, target.ID)
	if !runtime.DispatchDOMEvent(event) || !event.DefaultPrevented() {
		t.Fatal("isolated click listener did not handle and cancel event")
	}
	if got := target.TextContent(); got != "clicked" {
		t.Fatalf("worker event DOM text = %q", got)
	}
	if len(mutations) < 2 {
		t.Fatalf("worker mutation notifications = %d, want at least 2", len(mutations))
	}
}

func TestIsolatedGoRuntimeMutatesBrowserOwnedDocument(t *testing.T) {
	document, err := htmlparser.Parse(strings.NewReader(`<p id="result">idle</p>`))
	if err != nil {
		t.Fatal(err)
	}
	result, _ := document.GetElementByID("result")
	runtime := New(runtimemodel.EngineGo)
	t.Cleanup(func() { _ = runtime.Stop() })
	source := `package main
import "growse/dom"
func main() { dom.GetElementByID("result").SetText("go worker") }`
	environment := runtimemodel.Environment{Document: document, Events: events.NewDispatcher(), BaseURL: mustURL(t, "http://localhost/page")}
	if err := runtime.Load(context.Background(), []runtimemodel.Script{{Engine: runtimemodel.EngineGo, SourceURL: environment.BaseURL, Source: source, Inline: true}}, environment); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if got := result.TextContent(); got != "go worker" {
		t.Fatalf("isolated Go DOM text = %q", got)
	}
}

func TestIsolatedRuntimeStopTerminatesWorkerAndRejectsCallbacks(t *testing.T) {
	document, _ := htmlparser.Parse(strings.NewReader(`<button id="target">idle</button>`))
	runtime := New(runtimemodel.EngineJavaScript)
	environment := runtimemodel.Environment{Document: document, Events: events.NewDispatcher(), BaseURL: mustURL(t, "https://example.test/")}
	script := runtimemodel.Script{Engine: runtimemodel.EngineJavaScript, SourceURL: environment.BaseURL, Source: `setTimeout(function () { document.getElementById("target").textContent = "late"; }, 20);`, Inline: true}
	if err := runtime.Load(context.Background(), []runtimemodel.Script{script}, environment); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Stop(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(40 * time.Millisecond)
	target, _ := document.GetElementByID("target")
	if got := target.TextContent(); got != "idle" || runtime.DispatchDOMEvent(events.Event{Type: events.Click, Target: target.ID}) {
		t.Fatalf("stopped worker state = text:%q event:%t", got, runtime.DispatchDOMEvent(events.Event{Type: events.Click, Target: target.ID}))
	}
}

func mustURL(t *testing.T, value string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(value)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}
