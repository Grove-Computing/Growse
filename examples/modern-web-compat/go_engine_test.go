package main

import (
	"context"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Grove-Computing/Growse/internal/browser"
	"github.com/Grove-Computing/Growse/internal/network"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
)

func TestFrameworkFixturesKeepJavaScriptDisabledInGoEngine(t *testing.T) {
	tests := []struct {
		name, route, rootID, ssrID, hydrationID string
	}{
		{name: "Next.js", route: "/next/", rootID: "__next", ssrID: "next-ssr-marker", hydrationID: "next-hydration-marker"},
		{name: "SvelteKit", route: "/svelte/", rootID: "svelte", ssrID: "svelte-ssr-marker", hydrationID: "svelte-hydration-marker"},
		{name: "Tailwind", route: "/tailwind/", rootID: "tailwind-root"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requests := &fixtureRequestLog{}
			server := httptest.NewServer(requests.middleware(modernWebCompatibilityHandler()))
			defer server.Close()
			var runtimeCreations atomic.Int32
			engine := browser.NewWithEngineFactory(network.NewClientWithLimits(server.Client(), 4<<20), func(runtimemodel.Engine) runtimemodel.Runtime {
				runtimeCreations.Add(1)
				return nil
			})
			defer engine.Close()
			var hydrationMutations atomic.Int32
			engine.SetOnMutation(func() { hydrationMutations.Add(1) })

			page, err := engine.Navigate(context.Background(), server.URL+test.route)
			if err != nil {
				t.Fatal(err)
			}
			if page.Engine != runtimemodel.EngineGo || engine.Engine() != runtimemodel.EngineGo {
				t.Fatalf("default engine = page:%q tab:%q", page.Engine, engine.Engine())
			}
			if len(page.Scripts) != 0 || page.RuntimeStarted || runtimeCreations.Load() != 0 {
				t.Fatalf("Go Engine script state = scripts:%v started:%t runtime creations:%d", page.Scripts, page.RuntimeStarted, runtimeCreations.Load())
			}
			if page.RuntimeError != "" || len(page.ScriptErrors) != 0 || hydrationMutations.Load() != 0 {
				t.Fatalf("Go Engine JavaScript side effects = runtime:%q scripts:%v mutations:%d", page.RuntimeError, page.ScriptErrors, hydrationMutations.Load())
			}
			root := fixtureNode(t, page, test.rootID)
			if _, hydrated := root.Attribute("data-hydrated"); hydrated {
				t.Fatal("Go Engine fixture gained a hydration marker")
			}
			if test.ssrID != "" && fixtureNode(t, page, test.ssrID).TextContent() != "SSR rendered" {
				t.Fatal("Go Engine did not preserve SSR HTML")
			}
			if test.hydrationID != "" && fixtureNode(t, page, test.hydrationID).TextContent() != "not hydrated" {
				t.Fatal("Go Engine mutated the hydration marker")
			}
			requests.mu.Lock()
			defer requests.mu.Unlock()
			for _, path := range requests.paths {
				if strings.HasSuffix(path, ".mjs") || strings.HasSuffix(path, ".js") || strings.Contains(path, "/chunks/") || strings.Contains(path, "/entry/") {
					t.Fatalf("Go Engine requested JavaScript resource %q; all requests=%v", path, requests.paths)
				}
			}
		})
	}
}
