package browser

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/Grove-Computing/Growse/internal/network"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
	runtimejavascript "github.com/Grove-Computing/Growse/internal/runtime/javascript"
)

type cachedDynamicResourceLoader struct {
	mu        sync.Mutex
	responses map[string]*network.Response
	cache     map[string]*network.Response
	requests  map[string]int
}

func (loader *cachedDynamicResourceLoader) Get(ctx context.Context, target *url.URL) (*network.Response, error) {
	return loader.Do(ctx, &network.Request{Method: http.MethodGet, URL: target})
}

func (loader *cachedDynamicResourceLoader) Do(_ context.Context, request *network.Request) (*network.Response, error) {
	if request == nil || request.URL == nil {
		return nil, errors.New("missing request URL")
	}
	key := request.URL.String()
	loader.mu.Lock()
	defer loader.mu.Unlock()
	if cached := loader.cache[key]; cached != nil {
		return cloneDynamicResourceResponse(cached), nil
	}
	response := loader.responses[key]
	if response == nil {
		return nil, errors.New("missing response for " + key)
	}
	loader.requests[key]++
	loader.cache[key] = cloneDynamicResourceResponse(response)
	return cloneDynamicResourceResponse(response), nil
}

func cloneDynamicResourceResponse(response *network.Response) *network.Response {
	copy := *response
	copy.Body = append([]byte(nil), response.Body...)
	if response.URL != nil {
		urlCopy := *response.URL
		copy.URL = &urlCopy
	}
	if response.Header != nil {
		copy.Header = response.Header.Clone()
	}
	return &copy
}

func TestJavaScriptDynamicStylesheetAndPreloadUpdateBrowserStyleRevision(t *testing.T) {
	pageURL := mustParseURL(t, "https://app.example/page")
	themeURL := mustParseURL(t, "https://app.example/theme.css")
	preloadedScriptURL := mustParseURL(t, "https://app.example/preloaded.js")
	pageSource := `<main id="host"><p id="target">styled</p></main><script>
		var host = document.getElementById("host");
		var target = document.getElementById("target");
		var inline = document.createElement("style");
		inline.textContent = "#target { color: red; font-size: 24px; }";
		host.appendChild(inline);

		var scriptPreload = document.createElement("link");
		scriptPreload.rel = "preload";
		scriptPreload.as = "script";
		scriptPreload.href = "/preloaded.js";
		host.appendChild(scriptPreload);

		var preload = document.createElement("link");
		preload.rel = "preload";
		preload.as = "style";
		preload.href = "/theme.css";
		preload.addEventListener("load", function () {
			var stylesheet = document.createElement("link");
			stylesheet.rel = "stylesheet";
			stylesheet.href = "/theme.css";
			stylesheet.addEventListener("load", function () {
				target.setAttribute("phase", "loaded");
				setTimeout(function () {
					inline.textContent = "#target { color: green; font-size: 26px; }";
					stylesheet.remove();
					target.setAttribute("phase", "removed");
				}, 100);
			});
			host.appendChild(stylesheet);
		});
		host.appendChild(preload);
	</script>`
	loader := &cachedDynamicResourceLoader{
		responses: map[string]*network.Response{
			pageURL.String():  {URL: pageURL, StatusCode: http.StatusOK, ContentType: "text/html", Body: []byte(pageSource)},
			themeURL.String(): {URL: themeURL, StatusCode: http.StatusOK, ContentType: "text/css", Body: []byte(`#target { font-size: 31px; background-color: blue; }`)},
			preloadedScriptURL.String(): {
				URL: preloadedScriptURL, StatusCode: http.StatusOK, ContentType: "text/javascript",
				Body: []byte(`document.getElementById("target").setAttribute("preload-executed", "yes")`),
			},
		},
		cache: make(map[string]*network.Response), requests: make(map[string]int),
	}
	browserState := NewWithEngineFactory(loader, func(engine runtimemodel.Engine) runtimemodel.Runtime {
		if engine == runtimemodel.EngineJavaScript {
			return runtimejavascript.New()
		}
		return nil
	})
	t.Cleanup(func() { _ = browserState.Close() })
	if _, err := browserState.SetEngine(context.Background(), runtimemodel.EngineJavaScript); err != nil {
		t.Fatal(err)
	}
	page, err := browserState.Navigate(context.Background(), pageURL.String())
	if err != nil {
		t.Fatal(err)
	}
	target, _ := page.Document.GetElementByID("target")
	initialRevision := page.StyleRevision
	waitForDynamicStylePhase(t, target, "loaded")
	loadedStyle := page.ComputedStyles[target.ID]
	if loadedStyle.Color != 0xff0000ff || loadedStyle.FontSize != 31 || loadedStyle.BackgroundColor != 0x0000ffff {
		t.Fatalf("loaded dynamic style = %#v", loadedStyle)
	}

	waitForDynamicStylePhase(t, target, "removed")
	removedStyle := page.ComputedStyles[target.ID]
	if removedStyle.Color != 0x008000ff || removedStyle.FontSize != 26 || removedStyle.BackgroundColor != 0x00000000 {
		t.Fatalf("changed/removed dynamic style = %#v", removedStyle)
	}
	if page.StyleRevision <= initialRevision {
		t.Fatalf("style revision = %d, want greater than %d", page.StyleRevision, initialRevision)
	}
	if _, executed := target.Attribute("preload-executed"); executed {
		t.Fatal("script preload executed its body")
	}
	loader.mu.Lock()
	defer loader.mu.Unlock()
	if loader.requests[themeURL.String()] != 1 || loader.requests[preloadedScriptURL.String()] != 1 {
		t.Fatalf("network requests = %v, want one warmed request per preload", loader.requests)
	}
}

func waitForDynamicStylePhase(t *testing.T, target interface{ Attribute(string) (string, bool) }, phase string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if value, _ := target.Attribute("phase"); value == phase {
			return
		}
		time.Sleep(time.Millisecond)
	}
	value, _ := target.Attribute("phase")
	t.Fatalf("dynamic style phase = %q, want %q", value, phase)
}
