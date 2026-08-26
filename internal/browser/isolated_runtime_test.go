package browser

import (
	"context"
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
