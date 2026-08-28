package browser

import (
	"context"
	"crypto/sha512"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Grove-Computing/Growse/internal/forms"
	"github.com/Grove-Computing/Growse/internal/network"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
	"github.com/Grove-Computing/Growse/internal/runtime/isolated"
	"github.com/Grove-Computing/Growse/internal/serviceworker"
	storagecore "github.com/Grove-Computing/Growse/internal/storage"
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

func TestBrowserRefreshesDynamicInlineStyleFromIsolatedJavaScriptWorker(t *testing.T) {
	pageURL := mustParseURL(t, "https://app.example/dynamic-style.html")
	loader := &requestRouteLoader{routeLoader: routeLoader{responses: map[string]*network.Response{
		pageURL.String(): {
			URL: pageURL, StatusCode: http.StatusOK, ContentType: "text/html",
			Body: []byte(`<p id="target">styled</p><script>
				var style = document.createElement("style");
				style.textContent = "#target { color: red; font-size: 29px; }";
				document.getElementById("target").appendChild(style);
			</script>`),
		},
	}}}
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
	for time.Now().Before(deadline) {
		target, ok := page.Document.GetElementByID("target")
		if ok {
			computed := page.ComputedStyles[target.ID]
			if computed.Color == 0xff0000ff && computed.FontSize == 29 {
				return
			}
		}
		time.Sleep(time.Millisecond)
	}
	target, _ := page.Document.GetElementByID("target")
	t.Fatalf("isolated dynamic style = %#v, runtimeError=%q", page.ComputedStyles[target.ID], page.RuntimeError)
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

func TestBrowserRegistersAndControlsServiceWorkerThroughIsolatedRuntime(t *testing.T) {
	pageURL := mustParseURL(t, "https://app.example/app/page.html")
	workerURL := mustParseURL(t, "https://app.example/app/sw.js")
	loader := &requestRouteLoader{routeLoader: routeLoader{responses: map[string]*network.Response{
		pageURL.String(): {
			URL: pageURL, StatusCode: http.StatusOK, ContentType: "text/html",
			Body: []byte(`<p id="result">waiting</p><script>
				navigator.serviceWorker.register("/app/sw.js").then(function(registration) {
					return navigator.serviceWorker.ready.then(function(ready) {
						return Promise.all([
							navigator.serviceWorker.getRegistration(),
							navigator.serviceWorker.getRegistrations()
						]).then(function(values) {
							const before = [registration.active.state, ready.active.state, values[1].length,
								navigator.serviceWorker.controller.state].join("|");
							return registration.unregister().then(function(removed) {
								document.getElementById("result").textContent = before + "|" + removed + "|" +
									(navigator.serviceWorker.controller === null);
							});
						});
					});
			});
			</script>`),
		},
		workerURL.String(): {
			URL: workerURL, StatusCode: http.StatusOK, ContentType: "text/javascript",
			Body: []byte(`
				self.addEventListener("install", event => event.waitUntil(self.skipWaiting()));
				self.addEventListener("activate", event => event.waitUntil(clients.claim()));`),
		},
	}}}
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
	if got := result.TextContent(); got != "activated|activated|1|activated|true|true" {
		t.Fatalf("Service Worker lifecycle result = %q, runtimeError=%q", got, page.RuntimeError)
	}
	foundWorkerRequest := false
	for _, request := range loader.requests {
		if request.Kind == network.RequestServiceWorker && request.URL.String() == workerURL.String() && request.Credentials == network.CredentialsInclude {
			foundWorkerRequest = true
		}
	}
	if !foundWorkerRequest {
		t.Fatalf("Service Worker request was not brokered: %#v", loader.requests)
	}
}

func TestBrowserUsesActiveServiceWorkerForNavigationResourceAndFetch(t *testing.T) {
	installURL := mustParseURL(t, "https://app.example/app/install.html")
	workerURL := mustParseURL(t, "https://app.example/app/sw.js")
	controlledURL := mustParseURL(t, "https://app.example/app/controlled.html")
	loader := &requestRouteLoader{routeLoader: routeLoader{responses: map[string]*network.Response{
		installURL.String(): {
			URL: installURL, StatusCode: http.StatusOK, ContentType: "text/html",
			Body: []byte(`<p id="result">installing</p><script>
				navigator.serviceWorker.register("/app/sw.js").then(function() {
					document.getElementById("result").textContent = "installed";
				});
			</script>`),
		},
		workerURL.String(): {
			URL: workerURL, StatusCode: http.StatusOK, ContentType: "text/javascript",
			Body: []byte(`
				self.addEventListener("install", () => self.skipWaiting());
				self.addEventListener("activate", () => clients.claim());
				self.addEventListener("fetch", event => {
					if (event.request.url.endsWith("/controlled.html")) {
						event.respondWith(new Response('<p id="result">navigation</p><script src="/app/app.js"></' + 'script><script>fetch("/app/api").then(r => r.text()).then(value => console.log("service-worker-" + value));</' + 'script>', {headers: {"Content-Type": "text/html"}}));
					} else if (event.request.url.endsWith("/app.js")) {
						event.respondWith(Promise.resolve(new Response('document.getElementById("result").textContent += "|resource";', {headers: {"Content-Type": "text/javascript"}})));
					} else if (event.request.url.endsWith("/api")) {
						event.respondWith(new Response("fetch", {headers: {"Content-Type": "text/plain"}}));
					}
				});`),
		},
	}}}
	browserState := NewWithEngineFactory(loader, func(engine runtimemodel.Engine) runtimemodel.Runtime { return isolated.New(engine) })
	t.Cleanup(func() { _ = browserState.Close() })
	if _, err := browserState.SetEngine(context.Background(), runtimemodel.EngineJavaScript); err != nil {
		t.Fatal(err)
	}
	installPage, err := browserState.Navigate(context.Background(), installURL.String())
	if err != nil {
		t.Fatal(err)
	}
	installResult, _ := installPage.Document.GetElementByID("result")
	if got := installResult.TextContent(); got != "installed" {
		t.Fatalf("installation result = %q", got)
	}
	page, err := browserState.Navigate(context.Background(), controlledURL.String())
	if err != nil {
		t.Fatal(err)
	}
	result, _ := page.Document.GetElementByID("result")
	if got := result.TextContent(); got != "navigation|resource" || page.RuntimeError != "" {
		t.Fatalf("controlled page = text:%q runtimeError:%q scriptErrors:%v", got, page.RuntimeError, page.ScriptErrors)
	}
	deadline := time.Now().Add(2 * time.Second)
	for len(page.DevTools.Console()) == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	records := page.DevTools.Console()
	if len(records) != 1 || records[0].Message != "service-worker-fetch" {
		t.Fatalf("controlled Fetch console = %#v", records)
	}
	for _, request := range loader.requests {
		if request.URL.String() == controlledURL.String() || request.URL.Path == "/app/app.js" || request.URL.Path == "/app/api" {
			t.Fatalf("controlled request reached network fallback: %#v", request)
		}
	}
}

func TestBrowserRestoresNavigationFromServiceWorkerCacheStorage(t *testing.T) {
	installURL := mustParseURL(t, "https://cache.example/app/install.html")
	workerURL := mustParseURL(t, "https://cache.example/app/sw.js")
	offlineURL := mustParseURL(t, "https://cache.example/app/offline.html")
	loader := &requestRouteLoader{routeLoader: routeLoader{responses: map[string]*network.Response{
		installURL.String(): {
			URL: installURL, StatusCode: http.StatusOK, ContentType: "text/html",
			Body: []byte(`<script>navigator.serviceWorker.register("/app/sw.js")</script>`),
		},
		workerURL.String(): {
			URL: workerURL, StatusCode: http.StatusOK, ContentType: "text/javascript",
			Body: []byte(`
				self.addEventListener("install", event => {
					event.waitUntil(caches.open("offline-v1").then(cache => cache.put(
						"/app/offline.html",
						new Response('<p id="result">cache-storage</p>', {headers: {"Content-Type": "text/html"}})
					)));
					self.skipWaiting();
				});
				self.addEventListener("activate", event => event.waitUntil(clients.claim()));
				self.addEventListener("fetch", event => event.respondWith(
					caches.match(event.request).then(response => response || fetch(event.request))
				));`),
		},
	}}}
	browserState := NewWithEngineFactory(loader, func(engine runtimemodel.Engine) runtimemodel.Runtime { return isolated.New(engine) })
	t.Cleanup(func() { _ = browserState.Close() })
	if _, err := browserState.SetEngine(context.Background(), runtimemodel.EngineJavaScript); err != nil {
		t.Fatal(err)
	}
	if _, err := browserState.Navigate(context.Background(), installURL.String()); err != nil {
		t.Fatal(err)
	}
	page, err := browserState.Navigate(context.Background(), offlineURL.String())
	if err != nil {
		t.Fatal(err)
	}
	result, _ := page.Document.GetElementByID("result")
	if got := result.TextContent(); got != "cache-storage" {
		t.Fatalf("cached navigation text = %q", got)
	}
	for _, request := range loader.requests {
		if request.URL.String() == offlineURL.String() {
			t.Fatalf("cached navigation reached the network: %#v", request)
		}
	}
}

func TestBrowsersShareServiceWorkerProfileAcrossTabs(t *testing.T) {
	installURL := mustParseURL(t, "https://tabs.example/app/install.html")
	workerURL := mustParseURL(t, "https://tabs.example/app/sw.js")
	controlledURL := mustParseURL(t, "https://tabs.example/app/controlled.html")
	loader := &requestRouteLoader{routeLoader: routeLoader{responses: map[string]*network.Response{
		installURL.String(): {
			URL: installURL, StatusCode: http.StatusOK, ContentType: "text/html",
			Body: []byte(`<script>navigator.serviceWorker.register("/app/sw.js")</script>`),
		},
		workerURL.String(): {
			URL: workerURL, StatusCode: http.StatusOK, ContentType: "text/javascript",
			Body: []byte(`
				self.addEventListener("install", () => self.skipWaiting());
				self.addEventListener("activate", () => clients.claim());
				self.addEventListener("fetch", event => event.respondWith(new Response('<p id="result">shared-profile</p>', {headers: {"Content-Type": "text/html"}})));`),
		},
	}}}
	profile := serviceworker.NewManager()
	newTab := func() *Browser {
		return NewWithEngineFactoryAndStorageAndServiceWorkers(loader, func(engine runtimemodel.Engine) runtimemodel.Runtime {
			return isolated.New(engine)
		}, storagecore.NewManager(), profile)
	}
	first := newTab()
	second := newTab()
	t.Cleanup(func() { _ = first.Close(); _ = second.Close(); _ = profile.Close() })
	if _, err := first.SetEngine(context.Background(), runtimemodel.EngineJavaScript); err != nil {
		t.Fatal(err)
	}
	if _, err := second.SetEngine(context.Background(), runtimemodel.EngineJavaScript); err != nil {
		t.Fatal(err)
	}
	if _, err := first.Navigate(context.Background(), installURL.String()); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	page, err := second.Navigate(context.Background(), controlledURL.String())
	if err != nil {
		t.Fatal(err)
	}
	result, _ := page.Document.GetElementByID("result")
	if got := result.TextContent(); got != "shared-profile" {
		t.Fatalf("shared Service Worker profile result = %q", got)
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

func TestIsolatedJavaScriptEventCancellationBlocksBrowserClickDefault(t *testing.T) {
	pageURL := mustParseURL(t, "https://event.example/page")
	loader := &routeLoader{responses: map[string]*network.Response{
		pageURL.String(): {
			URL: pageURL, StatusCode: 200, ContentType: "text/html",
			Body: []byte(`<label id="label" for="choice"><span id="target">toggle</span></label><input id="choice" type="checkbox">
				<script>
				var labelElement = document.getElementById("label");
				var targetElement = document.getElementById("target");
				labelElement.addEventListener("click", function (event) {
					if (event.target !== targetElement || event.currentTarget !== labelElement || event.eventPhase !== 1) throw new Error("event metadata mismatch");
					event.preventDefault();
				}, {capture: true});
				</script>`),
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
	target, _ := page.Document.GetElementByID("target")
	checkbox, _ := page.Document.GetElementByID("choice")
	if !browserState.DispatchClick(target.ID, 0, 0) || forms.CurrentChecked(checkbox) || page.FocusTarget != 0 || page.RuntimeError != "" {
		t.Fatalf("isolated prevented click = checked:%t focus:%d runtimeError:%q", forms.CurrentChecked(checkbox), page.FocusTarget, page.RuntimeError)
	}
}
