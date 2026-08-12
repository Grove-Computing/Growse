package browser

import (
	"context"
	"testing"

	"github.com/saku0512/growse/internal/network"
	runtimemodel "github.com/saku0512/growse/internal/runtime"
	"github.com/saku0512/growse/internal/runtime/yaegi"
)

func TestCounterDemoIncrementsThroughWebGoClick(t *testing.T) {
	pageURL := mustParseURL(t, "http://localhost:8080/index.html")
	cssURL := mustParseURL(t, "http://localhost:8080/style.css")
	scriptURL := mustParseURL(t, "http://localhost:8080/app.go")
	loader := &routeLoader{responses: map[string]*network.Response{
		pageURL.String(): {
			URL: pageURL, StatusCode: 200, ContentType: "text/html",
			Body: []byte(`<!doctype html><html><head>
<link rel="stylesheet" href="/style.css"></head><body>
<button id="increment">+</button><p id="count">0</p>
<script type="text/go" src="/app.go"></script></body></html>`),
		},
		cssURL.String(): {
			URL: cssURL, StatusCode: 200, ContentType: "text/css",
			Body: []byte(`#count { font-size: 28px; font-weight: bold; }`),
		},
		scriptURL.String(): {
			URL: scriptURL, StatusCode: 200, ContentType: "text/go",
			Body: []byte(`package main
import (
	"growse/dom"
	"growse/strconv"
)
func main() {
	button := dom.GetElementByID("increment")
	output := dom.GetElementByID("count")
	count := 0
	button.OnClick(func() {
		count++
		output.SetText(strconv.Itoa(count))
	})
}`),
		},
	}}
	browser := NewWithRuntimeFactory(loader, func() runtimemodel.Runtime { return yaegi.New() })

	page, err := browser.Navigate(context.Background(), pageURL.String())
	if err != nil {
		t.Fatalf("Navigate() error = %v", err)
	}
	if !page.RuntimeStarted || page.RuntimeError != "" {
		t.Fatalf("runtime state = started:%v error:%q", page.RuntimeStarted, page.RuntimeError)
	}
	button, ok := page.Document.GetElementByID("increment")
	if !ok {
		t.Fatal("increment button was not found")
	}
	output, ok := page.Document.GetElementByID("count")
	if !ok {
		t.Fatal("count output was not found")
	}
	for click := 1; click <= 2; click++ {
		if !browser.DispatchClick(button.ID, 0, 0) {
			t.Fatalf("click %d was not handled", click)
		}
	}
	if got, want := output.TextContent(), "2"; got != want {
		t.Fatalf("counter text = %q, want %q", got, want)
	}
}
