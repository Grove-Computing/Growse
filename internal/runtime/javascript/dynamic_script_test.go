package javascript

import (
	"context"
	"crypto/sha512"
	"encoding/base64"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Grove-Computing/Growse/internal/events"
	htmlparser "github.com/Grove-Computing/Growse/internal/html"
	"github.com/Grove-Computing/Growse/internal/network"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
	"github.com/dop251/goja"
)

func TestDynamicClassicScriptsSnapshotFetchAndExecuteExactlyOnce(t *testing.T) {
	document, err := htmlparser.Parse(strings.NewReader(`<main id="host"><output id="result"></output></main>`))
	if err != nil {
		t.Fatal(err)
	}
	baseURL, _ := url.Parse("https://app.example/page")
	classicSource := []byte(`document.getElementById("result").setAttribute("external", "executed");`)
	digest := sha512.Sum384(classicSource)
	integrity := "sha384-" + base64.StdEncoding.EncodeToString(digest[:])

	var requestsMu sync.Mutex
	var requests []*network.Request
	environment := runtimemodel.Environment{
		Document: document, Events: events.NewDispatcher(), BaseURL: baseURL,
		Fetch: func(_ context.Context, request *network.Request) (*network.Response, error) {
			requestsMu.Lock()
			copy := *request
			requests = append(requests, &copy)
			requestsMu.Unlock()
			body := classicSource
			if request.URL.Path == "/bad.js" {
				body = []byte(`throw new Error("must not execute")`)
			}
			return &network.Response{
				URL: request.URL, StatusCode: http.StatusOK, Status: "OK",
				ContentType: "text/javascript; charset=utf-8", Body: body,
			}, nil
		},
	}
	runtime := New()
	t.Cleanup(func() { _ = runtime.Stop() })
	source := `
		var host = document.getElementById("host");
		var result = document.getElementById("result");
		var detached = document.createElement("section");

		var inline = document.createElement("script");
		inline.text = "var value = document.getElementById('result'); value.setAttribute('inline', String(Number(value.getAttribute('inline') || '0') + 1));";
		detached.appendChild(inline);

		var external = document.createElement("script");
		external.src = "/classic.js";
		external.integrity = "` + integrity + `";
		external.crossOrigin = "anonymous";
		external.addEventListener("load", function () { result.setAttribute("load", "yes"); });
		external.addEventListener("error", function () { result.setAttribute("unexpected-error", "yes"); });
		detached.appendChild(external);

		var bad = document.createElement("script");
		bad.src = "/bad.js";
		bad.integrity = "sha384-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA";
		bad.crossOrigin = "anonymous";
		bad.addEventListener("error", function () { result.setAttribute("error", "yes"); });
		detached.appendChild(bad);

		host.appendChild(detached);
		external.src = "/changed-after-connect.js";
		detached.removeChild(external);
		detached.appendChild(external);
		detached.removeChild(inline);
		detached.appendChild(inline);`
	startJavaScriptRuntime(t, runtime, source, environment)

	deadline := time.Now().Add(2 * time.Second)
	for {
		var inline, external, loaded, failed string
		var unexpected bool
		if err := runtime.runSync(context.Background(), func(_ *goja.Runtime) error {
			result, _ := document.GetElementByID("result")
			inline, _ = result.Attribute("inline")
			external, _ = result.Attribute("external")
			loaded, _ = result.Attribute("load")
			failed, _ = result.Attribute("error")
			_, unexpected = result.Attribute("unexpected-error")
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if inline == "1" && external == "executed" && loaded == "yes" && failed == "yes" {
			if unexpected {
				t.Fatal("valid external script dispatched error")
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("dynamic results = inline:%q external:%q load:%q error:%q", inline, external, loaded, failed)
		}
		time.Sleep(time.Millisecond)
	}

	requestsMu.Lock()
	defer requestsMu.Unlock()
	if len(requests) != 2 {
		t.Fatalf("dynamic script requests = %d, want 2: %#v", len(requests), requests)
	}
	seen := make(map[string]int)
	for _, request := range requests {
		seen[request.URL.Path]++
		if request.Kind != network.RequestScript || request.Engine != "javascript" || !request.CORS || request.Credentials != network.CredentialsSameOrigin {
			t.Fatalf("dynamic script request policy = %#v", request)
		}
	}
	if seen["/classic.js"] != 1 || seen["/bad.js"] != 1 || seen["/changed-after-connect.js"] != 0 {
		t.Fatalf("snapshotted request paths = %v", seen)
	}
}

func TestDynamicModuleAndModulePreloadShareGraphAndEvaluateOnce(t *testing.T) {
	document, err := htmlparser.Parse(strings.NewReader(`<link rel="modulepreload" href="/shared.js"><main id="host"><output id="result"></output></main>`))
	if err != nil {
		t.Fatal(err)
	}
	baseURL, _ := url.Parse("https://app.example/page")
	sources := map[string]string{
		"/root.js":    `import { value } from "./shared.js"; globalThis.moduleOrder.push("root:" + value); const dynamic = await import("./dynamic.js"); document.getElementById("result").setAttribute("module", moduleOrder.join(",") + ":" + dynamic.value);`,
		"/shared.js":  `globalThis.moduleOrder = ["shared"]; export const value = "static";`,
		"/dynamic.js": `globalThis.moduleOrder.push("dynamic"); export const value = "loaded";`,
	}
	var fetchMu sync.Mutex
	fetches := make(map[string]int)
	environment := runtimemodel.Environment{
		Document: document, Events: events.NewDispatcher(), BaseURL: baseURL,
		Fetch: func(_ context.Context, request *network.Request) (*network.Response, error) {
			if request.Kind != network.RequestModule || !request.CORS || request.Credentials != network.CredentialsSameOrigin {
				t.Fatalf("module request policy = %#v", request)
			}
			fetchMu.Lock()
			fetches[request.URL.Path]++
			fetchMu.Unlock()
			return &network.Response{
				URL: request.URL, StatusCode: http.StatusOK, Status: "OK", ContentType: "text/javascript",
				Body: []byte(sources[request.URL.Path]),
			}, nil
		},
	}
	runtime := New()
	t.Cleanup(func() { _ = runtime.Stop() })
	loader := `
		var host = document.getElementById("host");
		var result = document.getElementById("result");
		function moduleScript() {
			var script = document.createElement("script");
			script.type = "module";
			script.src = "/root.js";
			script.crossOrigin = "anonymous";
			script.addEventListener("load", function () { result.setAttribute("module-loads", String(Number(result.getAttribute("module-loads") || "0") + 1)); });
			script.addEventListener("error", function () { result.setAttribute("module-error", "yes"); });
			return script;
		}
		host.appendChild(moduleScript());
		host.appendChild(moduleScript());`
	startJavaScriptRuntime(t, runtime, loader, environment)

	deadline := time.Now().Add(2 * time.Second)
	for {
		var module, loads string
		var failed bool
		if err := runtime.runSync(context.Background(), func(_ *goja.Runtime) error {
			result, _ := document.GetElementByID("result")
			module, _ = result.Attribute("module")
			loads, _ = result.Attribute("module-loads")
			_, failed = result.Attribute("module-error")
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if failed {
			t.Fatal("dynamic module dispatched error")
		}
		if module == "shared,root:static,dynamic:loaded" && loads == "2" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("dynamic module = %q, loads = %q", module, loads)
		}
		time.Sleep(time.Millisecond)
	}

	fetchMu.Lock()
	defer fetchMu.Unlock()
	for _, path := range []string{"/root.js", "/shared.js", "/dynamic.js"} {
		if fetches[path] != 1 {
			t.Fatalf("module fetches = %v; %s count = %d, want 1", fetches, path, fetches[path])
		}
	}
}

func TestStoppedRuntimeDropsDynamicResourcesHydrationCallbacksAndMutations(t *testing.T) {
	document, err := htmlparser.Parse(strings.NewReader(`<main id="host"><output id="result">idle</output></main>`))
	if err != nil {
		t.Fatal(err)
	}
	baseURL, _ := url.Parse("https://app.example/page")
	started := make(chan string, 3)
	returned := make(chan struct{}, 3)
	release := make(chan struct{})
	var mutations atomic.Int64
	var refreshes atomic.Int64
	var recordsMu sync.Mutex
	var records [][2]string
	environment := runtimemodel.Environment{
		Document: document, Events: events.NewDispatcher(), BaseURL: baseURL,
		OnMutation:    func() { mutations.Add(1) },
		RefreshStyles: func(context.Context) error { refreshes.Add(1); return nil },
		ConsoleRecord: func(level, message string) {
			recordsMu.Lock()
			records = append(records, [2]string{level, message})
			recordsMu.Unlock()
		},
		Fetch: func(_ context.Context, request *network.Request) (*network.Response, error) {
			started <- request.URL.Path
			<-release
			returned <- struct{}{}
			body, contentType := `document.getElementById("result").textContent = "stale-classic";`, "text/javascript"
			switch request.URL.Path {
			case "/module.js":
				body = `document.getElementById("result").textContent = "stale-module"; export const stale = true;`
			case "/style.css":
				body, contentType = `#result { color: red; }`, "text/css"
			}
			return &network.Response{URL: request.URL, StatusCode: http.StatusOK, ContentType: contentType, Body: []byte(body)}, nil
		},
	}
	runtime := New()
	source := `
		var host = document.getElementById("host");
		var result = document.getElementById("result");
		var classic = document.createElement("script");
		classic.src = "/classic.js";
		classic.addEventListener("load", function () { result.textContent = "stale-classic-load"; });
		host.appendChild(classic);
		var module = document.createElement("script");
		module.type = "module";
		module.src = "/module.js";
		module.addEventListener("load", function () { result.textContent = "stale-module-load"; });
		host.appendChild(module);
		var style = document.createElement("link");
		style.rel = "stylesheet";
		style.href = "/style.css";
		style.addEventListener("load", function () { result.textContent = "stale-style-load"; });
		host.appendChild(style);
		setTimeout(function () { result.textContent = "stale-hydration"; }, 25);`
	if err := runtime.Load(context.Background(), []runtimemodel.Script{javaScript(source)}, environment); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("dynamic resource fetch did not start")
		}
	}
	mutationsBeforeStop := mutations.Load()
	if err := runtime.Stop(); err != nil {
		t.Fatal(err)
	}
	close(release)
	for range 3 {
		select {
		case <-returned:
		case <-time.After(time.Second):
			t.Fatal("released resource fetch did not return")
		}
	}
	time.Sleep(40 * time.Millisecond)
	result, _ := document.GetElementByID("result")
	if result.TextContent() != "idle" || mutations.Load() != mutationsBeforeStop || refreshes.Load() != 0 {
		t.Fatalf("stale completion = text:%q mutations:%d/%d refreshes:%d", result.TextContent(), mutations.Load(), mutationsBeforeStop, refreshes.Load())
	}
	recordsMu.Lock()
	defer recordsMu.Unlock()
	if len(records) != 0 {
		t.Fatalf("stale completion console records = %v", records)
	}
}
