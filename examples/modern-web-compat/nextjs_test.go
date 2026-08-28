package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/Grove-Computing/Growse/internal/browser"
	"github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/network"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
	"github.com/Grove-Computing/Growse/internal/runtime/javascript"
)

type fixtureRequestLog struct {
	mu    sync.Mutex
	paths []string
}

func (log *fixtureRequestLog) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		log.mu.Lock()
		log.paths = append(log.paths, request.URL.Path)
		log.mu.Unlock()
		next.ServeHTTP(response, request)
	})
}

func (log *fixtureRequestLog) count(path string) int {
	log.mu.Lock()
	defer log.mu.Unlock()
	count := 0
	for _, requested := range log.paths {
		if requested == path {
			count++
		}
	}
	return count
}

func TestNextJSSSRFixtureHydratesWithoutReplacingDOM(t *testing.T) {
	requests := &fixtureRequestLog{}
	server := httptest.NewServer(requests.middleware(modernWebCompatibilityHandler()))
	defer server.Close()

	engine := browser.NewWithEngineFactory(network.NewClientWithLimits(server.Client(), 4<<20), func(selected runtimemodel.Engine) runtimemodel.Runtime {
		if selected == runtimemodel.EngineJavaScript {
			return javascript.New()
		}
		return nil
	})
	defer engine.Close()
	if _, err := engine.SetEngine(context.Background(), runtimemodel.EngineJavaScript); err != nil {
		t.Fatal(err)
	}

	mutations := make(chan struct{}, 32)
	engine.SetOnMutation(func() {
		select {
		case mutations <- struct{}{}:
		default:
		}
	})
	page, err := engine.Navigate(context.Background(), server.URL+"/next/")
	if err != nil {
		t.Fatal(err)
	}
	root := fixtureNode(t, page, "__next")
	rootID := root.ID
	waitForFixtureText(t, engine, mutations, "next-hydration-marker", "hydrated")

	page = engine.Page()
	if hydratedRoot := fixtureNode(t, page, "__next"); hydratedRoot.ID != rootID {
		t.Fatalf("hydration replaced SSR root: before=%d after=%d", rootID, hydratedRoot.ID)
	}
	if value, _ := root.Attribute("data-bootstrap"); value != "loaded" {
		t.Fatalf("bootstrap marker = %q", value)
	}
	if requests.count("/_next/static/chunks/app.mjs") != 1 || requests.count("/_next/static/chunks/counter.chunk.mjs") != 1 {
		t.Fatalf("Next.js chunk requests = %#v", requests.paths)
	}

	if !engine.DispatchClick(fixtureNode(t, page, "next-counter").ID, 0, 0) {
		t.Fatal("counter Event was not handled")
	}
	if got := fixtureNode(t, page, "next-count").TextContent(); got != "1" {
		t.Fatalf("counter state = %q", got)
	}
	if !engine.DispatchClick(fixtureNode(t, page, "next-navigation").ID, 0, 0) {
		t.Fatal("client Navigation Event was not handled")
	}
	if got := engine.Page().URL.Path; got != "/next/about" {
		t.Fatalf("client Navigation path = %q", got)
	}
	if got := fixtureNode(t, engine.Page(), "next-route").TextContent(); got != "/next/about" {
		t.Fatalf("client Navigation content = %q", got)
	}
	if engine.Page().RuntimeError != "" || len(engine.Page().ScriptErrors) != 0 {
		t.Fatalf("Next.js fixture runtime errors = %q / %v", engine.Page().RuntimeError, engine.Page().ScriptErrors)
	}
}

func fixtureNode(t *testing.T, page *browser.Page, id string) *dom.Node {
	t.Helper()
	node, ok := page.Document.GetElementByID(id)
	if !ok {
		t.Fatalf("fixture node %q was not found", id)
	}
	return node
}

func waitForFixtureText(t *testing.T, engine *browser.Browser, mutations <-chan struct{}, id, want string) {
	t.Helper()
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	for {
		page := engine.Page()
		if node, ok := page.Document.GetElementByID(id); ok && node.TextContent() == want {
			return
		}
		select {
		case <-mutations:
		case <-deadline.C:
			t.Fatalf("fixture %s = %q, want %q; runtime=%q scripts=%v", id, fixtureNode(t, page, id).TextContent(), want, page.RuntimeError, page.ScriptErrors)
		}
	}
}
