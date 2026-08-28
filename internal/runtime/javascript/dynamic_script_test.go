package javascript

import (
	"context"
	"crypto/sha512"
	"encoding/base64"
	"net/http"
	"net/url"
	"strings"
	"sync"
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
