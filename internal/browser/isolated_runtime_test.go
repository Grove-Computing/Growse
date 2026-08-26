package browser

import (
	"context"
	"crypto/sha512"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

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
			_, _ = response.Write([]byte(`import { value } from "./dependency.js"; document.getElementById("result").textContent = value;`))
		case "/dependency.js":
			_, _ = response.Write([]byte(`export const value = "module graph executed";`))
		default:
			http.NotFound(response, request)
		}
	}))
	defer moduleServer.Close()
	pageServer := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "text/html")
		_, _ = response.Write([]byte(`<p id="result">idle</p><script type="module" src="` + moduleServer.URL + `/main.js"></script>`))
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
