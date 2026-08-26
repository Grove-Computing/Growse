package isolated

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/events"
	htmlparser "github.com/Grove-Computing/Growse/internal/html"
	"github.com/Grove-Computing/Growse/internal/network"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
	storagecore "github.com/Grove-Computing/Growse/internal/storage"
)

func TestIsolatedJavaScriptFetchUsesDocumentResourceBaseURL(t *testing.T) {
	documentURL := mustURL(t, "https://example.test/root/page.html")
	resourceBaseURL := mustURL(t, "https://example.test/assets/")
	requests := make(chan *network.Request, 1)
	runtime := New(runtimemodel.EngineJavaScript)
	t.Cleanup(func() { _ = runtime.Stop() })
	environment := runtimemodel.Environment{
		Document: dom.NewDocument(), Events: events.NewDispatcher(), BaseURL: documentURL, ResourceBaseURL: resourceBaseURL,
		Fetch: func(_ context.Context, request *network.Request) (*network.Response, error) {
			requests <- request
			return &network.Response{URL: request.URL, StatusCode: 200, Body: []byte("ok")}, nil
		},
	}
	script := runtimemodel.Script{Engine: runtimemodel.EngineJavaScript, SourceURL: documentURL, Source: `fetch("data.json")`, Inline: true}
	if err := runtime.Load(context.Background(), []runtimemodel.Script{script}, environment); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case request := <-requests:
		if got, want := request.URL.String(), "https://example.test/assets/data.json"; got != want {
			t.Fatalf("isolated Fetch URL = %q, want %q", got, want)
		}
	case <-time.After(time.Second):
		t.Fatal("isolated Fetch did not reach the Browser broker")
	}
}

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

func TestIsolatedRuntimeDoesNotExposeProcessFilesystemOrEnvironment(t *testing.T) {
	document, _ := htmlparser.Parse(strings.NewReader(`<p id="result">idle</p>`))
	var record string
	runtime := New(runtimemodel.EngineJavaScript)
	t.Cleanup(func() { _ = runtime.Stop() })
	environment := runtimemodel.Environment{
		Document: document, Events: events.NewDispatcher(), BaseURL: mustURL(t, "https://example.test/"),
		ConsoleRecord: func(_, message string) { record = message },
	}
	source := `console.log([typeof process, typeof require, typeof Deno, typeof os, typeof fs, typeof Go, typeof GrowseGo].join(","));`
	if err := runtime.Load(context.Background(), []runtimemodel.Script{{Engine: runtimemodel.EngineJavaScript, SourceURL: environment.BaseURL, Source: source}}, environment); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if want := "undefined,undefined,undefined,undefined,undefined,undefined,undefined"; record != want {
		t.Fatalf("isolated host globals = %q, want %q", record, want)
	}
}

func TestIsolatedGoRuntimeRejectsOSImports(t *testing.T) {
	document, _ := htmlparser.Parse(strings.NewReader(`<p>safe</p>`))
	runtime := New(runtimemodel.EngineGo)
	t.Cleanup(func() { _ = runtime.Stop() })
	environment := runtimemodel.Environment{Document: document, Events: events.NewDispatcher(), BaseURL: mustURL(t, "http://localhost/")}
	source := `package main
import "os"
func main() { _ = os.Getenv("HOME") }`
	if err := runtime.Load(context.Background(), []runtimemodel.Script{{Engine: runtimemodel.EngineGo, SourceURL: environment.BaseURL, Source: source}}, environment); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Start(context.Background()); err == nil || !strings.Contains(err.Error(), "os") {
		t.Fatalf("isolated Go OS import error = %v", err)
	}
}

func TestIsolatedRuntimeTimesOutBusyScriptAndReportsFailure(t *testing.T) {
	document, _ := htmlparser.Parse(strings.NewReader(`<p>safe</p>`))
	failures := make(chan error, 1)
	runtime := New(runtimemodel.EngineJavaScript)
	runtime.taskTimeout = 50 * time.Millisecond
	t.Cleanup(func() { _ = runtime.Stop() })
	environment := runtimemodel.Environment{
		Document: document, Events: events.NewDispatcher(), BaseURL: mustURL(t, "https://example.test/"),
		RuntimeFailure: func(err error) { failures <- err },
	}
	if err := runtime.Load(context.Background(), []runtimemodel.Script{{Engine: runtimemodel.EngineJavaScript, SourceURL: environment.BaseURL, Source: `for (;;) {}`}}, environment); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	err := runtime.Start(context.Background())
	if err == nil || !strings.Contains(err.Error(), "exceeded") || time.Since(started) > time.Second {
		t.Fatalf("busy script timeout = %v after %v", err, time.Since(started))
	}
	select {
	case failure := <-failures:
		if failure == nil {
			t.Fatal("worker failure callback received nil")
		}
	case <-time.After(time.Second):
		t.Fatal("worker timeout was not reported")
	}
}

func TestWorkerCrashDoesNotStopIndependentRuntime(t *testing.T) {
	newRunning := func(text string) (*Runtime, *dom.Node) {
		document, _ := htmlparser.Parse(strings.NewReader(`<p id="result">idle</p>`))
		result, _ := document.GetElementByID("result")
		runtime := New(runtimemodel.EngineJavaScript)
		environment := runtimemodel.Environment{Document: document, Events: events.NewDispatcher(), BaseURL: mustURL(t, "https://example.test/")}
		source := `document.getElementById("result").textContent = "` + text + `";`
		if err := runtime.Load(context.Background(), []runtimemodel.Script{{Engine: runtimemodel.EngineJavaScript, SourceURL: environment.BaseURL, Source: source}}, environment); err != nil {
			t.Fatal(err)
		}
		if err := runtime.Start(context.Background()); err != nil {
			t.Fatal(err)
		}
		return runtime, result
	}
	crashed, _ := newRunning("first")
	independent, result := newRunning("second")
	t.Cleanup(func() { _ = crashed.Stop(); _ = independent.Stop() })
	if crashed.command == nil || crashed.command.Process == nil {
		t.Fatal("first worker process is unavailable")
	}
	if err := crashed.command.Process.Kill(); err != nil {
		t.Fatalf("kill first worker: %v", err)
	}
	select {
	case <-crashed.peer.done:
	case <-time.After(time.Second):
		t.Fatal("crashed worker connection remained open")
	}
	if got := result.TextContent(); got != "second" {
		t.Fatalf("independent worker DOM = %q", got)
	}
	if !independent.DispatchPageEvent(func() bool { return true }) || independent.stopped {
		t.Fatal("independent worker stopped after peer crash")
	}
}

func TestWorkerSessionLimitFailsBeforeStartingProcess(t *testing.T) {
	previous := activeWorkers.Load()
	activeWorkers.Store(maxSessionWorkers)
	t.Cleanup(func() { activeWorkers.Store(previous) })
	if _, _, _, _, _, err := startWorkerProcess(); err == nil || !strings.Contains(err.Error(), "session limit") {
		t.Fatalf("worker session limit error = %v", err)
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
