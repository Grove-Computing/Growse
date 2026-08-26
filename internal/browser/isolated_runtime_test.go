package browser

import (
	"context"
	"crypto/sha512"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Grove-Computing/Growse/internal/network"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
	"github.com/Grove-Computing/Growse/internal/runtime/isolated"
)

func TestBrowserSwitchesGoAndJavaScriptThroughIsolatedWorkers(t *testing.T) {
	pageURL := mustParseURL(t, "http://localhost:8080/isolated.html")
	loader := &routeLoader{responses: map[string]*network.Response{
		pageURL.String(): {
			URL: pageURL, StatusCode: 200, ContentType: "text/html",
			Body: []byte(`<p id="result">idle</p>
<script type="text/go">package main
import "growse/dom"
func main() { dom.GetElementByID("result").SetText("go isolated") }</script>
<script>document.getElementById("result").textContent = "javascript isolated";</script>`),
		},
	}}
	browser := NewWithEngineFactory(loader, func(engine runtimemodel.Engine) runtimemodel.Runtime {
		return isolated.New(engine)
	})
	t.Cleanup(func() { _ = browser.Close() })

	page, err := browser.Navigate(context.Background(), pageURL.String())
	if err != nil {
		t.Fatalf("Go Navigate() error = %v", err)
	}
	result, _ := page.Document.GetElementByID("result")
	if got := result.TextContent(); got != "go isolated" || !page.RuntimeStarted {
		t.Fatalf("Go isolated state = text:%q started:%t error:%q", got, page.RuntimeStarted, page.RuntimeError)
	}
	if !page.Sandbox.Ready || !page.Sandbox.ProcessBoundary || !page.Sandbox.BrokeredHostIO {
		t.Fatalf("Go sandbox status = %#v", page.Sandbox)
	}

	page, err = browser.SetEngine(context.Background(), runtimemodel.EngineJavaScript)
	if err != nil {
		t.Fatalf("JavaScript SetEngine() error = %v", err)
	}
	result, _ = page.Document.GetElementByID("result")
	if got := result.TextContent(); got != "javascript isolated" || !page.RuntimeStarted {
		t.Fatalf("JavaScript isolated state = text:%q started:%t error:%q", got, page.RuntimeStarted, page.RuntimeError)
	}
	if !page.Sandbox.Ready || len(page.Sandbox.Constraints) == 0 {
		t.Fatalf("JavaScript sandbox status = %#v", page.Sandbox)
	}
}

func TestBrowserExecutesExternalClassicJavaScriptOnOrdinaryHTTPSPage(t *testing.T) {
	pageURL := mustParseURL(t, "https://www.example.test/page")
	scriptURL := mustParseURL(t, "https://cdn.example.test/app.js")
	loader := &routeLoader{responses: map[string]*network.Response{
		pageURL.String(): {
			URL: pageURL, StatusCode: 200, ContentType: "text/html",
			Body: []byte(`<p id="result">idle</p><script src="https://cdn.example.test/app.js"></script>`),
		},
		scriptURL.String(): {
			URL: scriptURL, StatusCode: 200, ContentType: "text/javascript",
			Body: []byte(`
				if (window !== self || self !== globalThis || origin !== "https://www.example.test" || navigator.userAgent !== "Growse/0.14") {
					throw new Error("invalid Page global");
				}
				queueMicrotask(function () { document.getElementById("result").textContent = "external executed"; });`),
		},
	}}
	browserState := NewWithEngineFactory(loader, func(engine runtimemodel.Engine) runtimemodel.Runtime { return isolated.New(engine) })
	t.Cleanup(func() { _ = browserState.Close() })
	if _, err := browserState.SetEngine(context.Background(), runtimemodel.EngineJavaScript); err != nil {
		t.Fatal(err)
	}
	page, err := browserState.Navigate(context.Background(), pageURL.String())
	if err != nil {
		t.Fatal(err)
	}
	result, _ := page.Document.GetElementByID("result")
	if got := result.TextContent(); got != "external executed" || !page.RuntimeStarted || page.RuntimeError != "" {
		t.Fatalf("ordinary HTTPS JavaScript = text:%q started:%t error:%q", got, page.RuntimeStarted, page.RuntimeError)
	}
}

func TestBrowserBridgesSameOriginIframeDOMThroughIsolatedWorker(t *testing.T) {
	pageURL := mustParseURL(t, "https://page.example/index.html")
	sameURL := mustParseURL(t, "https://page.example/frame.html")
	crossURL := mustParseURL(t, "https://other.example/frame.html")
	loader := &routeLoader{responses: map[string]*network.Response{
		pageURL.String(): {
			URL: pageURL, StatusCode: http.StatusOK, ContentType: "text/html",
			Body: []byte(`<iframe id="same" src="/frame.html"></iframe>
				<iframe id="cross" src="https://other.example/frame.html"></iframe>
				<script>
					const same = document.getElementById("same");
					const cross = document.getElementById("cross");
					same.contentDocument.getElementById("result").textContent = "changed through worker";
					console.log([same.contentWindow.location.origin, cross.contentDocument === null,
						typeof cross.contentWindow.document].join("|"));
				</script>`),
		},
		sameURL.String(): {
			URL: sameURL, StatusCode: http.StatusOK, ContentType: "text/html",
			Body: []byte(`<p id="result">before</p>`),
		},
		crossURL.String(): {
			URL: crossURL, StatusCode: http.StatusOK, ContentType: "text/html",
			Body: []byte(`<p id="secret">cross-origin</p>`),
		},
	}}
	browserState := NewWithEngineFactory(loader, func(engine runtimemodel.Engine) runtimemodel.Runtime { return isolated.New(engine) })
	t.Cleanup(func() { _ = browserState.Close() })
	if _, err := browserState.SetEngine(context.Background(), runtimemodel.EngineJavaScript); err != nil {
		t.Fatal(err)
	}
	page, err := browserState.Navigate(context.Background(), pageURL.String())
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Frames) != 2 || page.Frames[0].Page == nil {
		t.Fatalf("Frames = %#v", page.Frames)
	}
	result, _ := page.Frames[0].Page.Document.GetElementByID("result")
	if got := result.TextContent(); got != "changed through worker" {
		t.Fatalf("same-origin Frame text = %q", got)
	}
	records := page.DevTools.Console()
	if len(records) != 1 || records[0].Message != "https://page.example|true|undefined" {
		t.Fatalf("Frame access Console() = %#v", records)
	}
}

func TestBrowserRoutesPostMessageAcrossIsolatedIframeWorkers(t *testing.T) {
	pageURL := mustParseURL(t, "https://page.example/index.html")
	sameURL := mustParseURL(t, "https://page.example/same.html")
	crossURL := mustParseURL(t, "https://frame.example/cross.html")
	childScript := `<script>
		console.log([parent === top, frames.length].join("|"));
		addEventListener("message", function(event) {
			event.source.postMessage({from: location.pathname, value: event.data.value}, event.origin);
		});
	</script>`
	loader := &routeLoader{responses: map[string]*network.Response{
		pageURL.String(): {
			URL: pageURL, StatusCode: http.StatusOK, ContentType: "text/html",
			Body: []byte(`<p id="result">waiting</p>
				<iframe id="same" src="/same.html"></iframe>
				<iframe id="cross" src="https://frame.example/cross.html"></iframe>
				<script>
					const replies = [];
					const same = document.getElementById("same");
					const cross = document.getElementById("cross");
					console.log([parent === window, top === window, frames.length,
						frames[0] === same.contentWindow, frames[1] === cross.contentWindow].join("|"));
					addEventListener("message", function(event) {
						replies.push(event.origin + ":" + event.data.from + ":" + event.data.value);
						if (replies.length === 2) {
							document.getElementById("result").textContent = replies.sort().join("|");
							console.log("replies:done");
						}
					});
					same.contentWindow.postMessage({value: 1}, "https://page.example");
					try { cross.contentWindow.postMessage({value: 0}, "https://wrong.example"); }
					catch (error) { console.log("target:" + error.name); }
					cross.contentWindow.postMessage({value: 2}, "https://frame.example");
				</script>`),
		},
		sameURL.String():  {URL: sameURL, StatusCode: http.StatusOK, ContentType: "text/html", Body: []byte(childScript)},
		crossURL.String(): {URL: crossURL, StatusCode: http.StatusOK, ContentType: "text/html", Body: []byte(childScript)},
	}}
	browserState := NewWithEngineFactory(loader, func(engine runtimemodel.Engine) runtimemodel.Runtime { return isolated.New(engine) })
	t.Cleanup(func() { _ = browserState.Close() })
	if _, err := browserState.SetEngine(context.Background(), runtimemodel.EngineJavaScript); err != nil {
		t.Fatal(err)
	}
	page, err := browserState.Navigate(context.Background(), pageURL.String())
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for len(page.DevTools.Console()) < 3 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	result, _ := page.Document.GetElementByID("result")
	if got := result.TextContent(); got != "https://frame.example:/cross.html:2|https://page.example:/same.html:1" {
		t.Fatalf("postMessage replies = %q", got)
	}
	records := page.DevTools.Console()
	if len(records) != 3 || records[0].Message != "true|true|2|true|true" || records[1].Message != "target:SecurityError" || records[2].Message != "replies:done" {
		t.Fatalf("parent Window relationships = %#v", records)
	}
	for index, frame := range page.Frames {
		childRecords := frame.Page.DevTools.Console()
		if len(childRecords) != 1 || childRecords[0].Message != "true|0" {
			t.Fatalf("child %d Window relationships = %#v", index, childRecords)
		}
	}
}

func TestBrowserExecutesCORSAndIntegrityCheckedClassicJavaScript(t *testing.T) {
	scriptBody := []byte(`document.getElementById("result").textContent = "cors integrity";`)
	digest := sha512.Sum384(scriptBody)
	integrity := "sha384-" + base64.StdEncoding.EncodeToString(digest[:])
	allowedOrigin := ""
	scriptServer := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
		response.Header().Set("Content-Type", "text/javascript")
		_, _ = response.Write(scriptBody)
	}))
	defer scriptServer.Close()
	pageServer := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "text/html")
		_, _ = response.Write([]byte(`<p id="result">idle</p><script src="` + scriptServer.URL + `/app.js" crossorigin="anonymous" integrity="` + integrity + `"></script>`))
	}))
	defer pageServer.Close()
	allowedOrigin = pageServer.URL

	browserState := NewWithEngineFactory(network.NewClient(), func(engine runtimemodel.Engine) runtimemodel.Runtime { return isolated.New(engine) })
	t.Cleanup(func() { _ = browserState.Close() })
	if _, err := browserState.SetEngine(context.Background(), runtimemodel.EngineJavaScript); err != nil {
		t.Fatal(err)
	}
	page, err := browserState.Navigate(context.Background(), pageServer.URL)
	if err != nil {
		t.Fatal(err)
	}
	result, _ := page.Document.GetElementByID("result")
	if got := result.TextContent(); got != "cors integrity" || !page.RuntimeStarted || len(page.ScriptErrors) != 0 {
		t.Fatalf("CORS integrity JavaScript = text:%q started:%t errors:%v", got, page.RuntimeStarted, page.ScriptErrors)
	}
}

func TestBrowserExecutesCrossOriginModuleGraphInIsolatedWorker(t *testing.T) {
	allowedOrigin := ""
	moduleServer := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
		response.Header().Set("Content-Type", "text/javascript")
		switch request.URL.Path {
		case "/main.js":
			_, _ = response.Write([]byte(`const module = await import("fixture"); await new Promise(resolve => setTimeout(resolve, 5)); document.getElementById("result").textContent = module.value;`))
		case "/dependency.js":
			_, _ = response.Write([]byte(`export const value = "module graph executed";`))
		default:
			http.NotFound(response, request)
		}
	}))
	defer moduleServer.Close()
	pageServer := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "text/html")
		_, _ = response.Write([]byte(`<p id="result">idle</p><script type="importmap">{"imports":{"fixture":"` + moduleServer.URL + `/dependency.js"}}</script><script type="module" src="` + moduleServer.URL + `/main.js"></script>`))
	}))
	defer pageServer.Close()
	allowedOrigin = pageServer.URL

	browserState := NewWithEngineFactory(network.NewClient(), func(engine runtimemodel.Engine) runtimemodel.Runtime { return isolated.New(engine) })
	t.Cleanup(func() { _ = browserState.Close() })
	if _, err := browserState.SetEngine(context.Background(), runtimemodel.EngineJavaScript); err != nil {
		t.Fatal(err)
	}
	page, err := browserState.Navigate(context.Background(), pageServer.URL)
	if err != nil {
		t.Fatal(err)
	}
	result, _ := page.Document.GetElementByID("result")
	if got := result.TextContent(); got != "module graph executed" || !page.RuntimeStarted || page.RuntimeError != "" || len(page.ScriptErrors) != 0 {
		t.Fatalf("cross-origin module = text:%q started:%t runtimeError:%q scriptErrors:%v", got, page.RuntimeStarted, page.RuntimeError, page.ScriptErrors)
	}
	moduleRecords := 0
	for _, record := range page.DevTools.Network() {
		if record.Kind == "module" {
			moduleRecords++
		}
	}
	if moduleRecords != 2 {
		t.Fatalf("module Network records = %d, want 2", moduleRecords)
	}
}

func TestBrowserRejectsCrossOriginModuleDependencyWithoutCORS(t *testing.T) {
	moduleServer := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "text/javascript")
		_, _ = response.Write([]byte(`export const value = "must not execute";`))
	}))
	defer moduleServer.Close()
	pageServer := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "text/html")
		_, _ = response.Write([]byte(`<p id="result">idle</p><script type="module">import { value } from "` + moduleServer.URL + `/dependency.js"; document.getElementById("result").textContent = value;</script>`))
	}))
	defer pageServer.Close()

	browserState := NewWithEngineFactory(network.NewClient(), func(engine runtimemodel.Engine) runtimemodel.Runtime { return isolated.New(engine) })
	t.Cleanup(func() { _ = browserState.Close() })
	if _, err := browserState.SetEngine(context.Background(), runtimemodel.EngineJavaScript); err != nil {
		t.Fatal(err)
	}
	page, err := browserState.Navigate(context.Background(), pageServer.URL)
	if err != nil {
		t.Fatal(err)
	}
	result, _ := page.Document.GetElementByID("result")
	if got := result.TextContent(); got != "idle" || !page.RuntimeStarted || page.RuntimeError != "" {
		t.Fatalf("CORS-rejected module = text:%q started:%t runtimeError:%q", got, page.RuntimeStarted, page.RuntimeError)
	}
	foundCORSError := false
	for _, record := range page.DevTools.Network() {
		if record.Kind == "module" && record.ErrorCategory == "cors" {
			foundCORSError = true
		}
	}
	if !foundCORSError {
		t.Fatalf("module CORS failure was not recorded: %+v", page.DevTools.Network())
	}
}

func TestBrowserContinuesValidJavaScriptAfterIndependentLoadFailure(t *testing.T) {
	pageURL := mustParseURL(t, "https://www.example.test/page")
	loader := &routeLoader{responses: map[string]*network.Response{
		pageURL.String(): {
			URL: pageURL, StatusCode: 200, ContentType: "text/html",
			Body: []byte(`<p id="result">idle</p>
				<script src="/missing.js"></script>
				<script>document.getElementById("result").textContent = document.readyState;</script>`),
		},
	}}
	browserState := NewWithEngineFactory(loader, func(engine runtimemodel.Engine) runtimemodel.Runtime { return isolated.New(engine) })
	t.Cleanup(func() { _ = browserState.Close() })
	if _, err := browserState.SetEngine(context.Background(), runtimemodel.EngineJavaScript); err != nil {
		t.Fatal(err)
	}
	page, err := browserState.Navigate(context.Background(), pageURL.String())
	if err != nil {
		t.Fatal(err)
	}
	result, _ := page.Document.GetElementByID("result")
	if got := result.TextContent(); got != "loading" || !page.RuntimeStarted || page.RuntimeError != "" || len(page.ScriptErrors) != 1 {
		t.Fatalf("partial JavaScript load = text:%q started:%t runtimeError:%q scriptErrors:%v", got, page.RuntimeStarted, page.RuntimeError, page.ScriptErrors)
	}
	if page.Document.ReadyState() != "complete" {
		t.Fatalf("document.readyState = %q", page.Document.ReadyState())
	}
}
