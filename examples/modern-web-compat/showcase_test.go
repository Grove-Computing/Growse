package main

import (
	"context"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/browser"
	"github.com/Grove-Computing/Growse/internal/devtools"
	"github.com/Grove-Computing/Growse/internal/network"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
	"github.com/Grove-Computing/Growse/internal/runtime/javascript"
	"github.com/Grove-Computing/Growse/internal/style"
)

func TestModernWebCompatibilityShowcaseRunsEntirelyLocally(t *testing.T) {
	server := httptest.NewServer(modernWebCompatibilityHandler())
	defer server.Close()
	response, err := server.Client().Get(server.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	landing, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	for _, route := range []string{"/next/", "/svelte/", "/tailwind/", "/diagnostics/"} {
		if !strings.Contains(string(landing), route) {
			t.Fatalf("showcase landing is missing route %s", route)
		}
	}
	if strings.Contains(string(landing), "https://") || strings.Contains(string(landing), "http://") {
		t.Fatal("showcase landing references the Public Internet")
	}

	engine := browser.NewWithEngineFactory(network.NewClientWithLimits(server.Client(), 8<<20), func(selected runtimemodel.Engine) runtimemodel.Runtime {
		if selected == runtimemodel.EngineJavaScript {
			return javascript.New()
		}
		return nil
	})
	defer engine.Close()
	mutations := make(chan struct{}, 128)
	engine.SetOnMutation(func() {
		select {
		case mutations <- struct{}{}:
		default:
		}
	})

	goPage, err := engine.Navigate(context.Background(), server.URL+"/next/")
	if err != nil {
		t.Fatal(err)
	}
	if goPage.Engine != runtimemodel.EngineGo || fixtureNode(t, goPage, "next-hydration-marker").TextContent() != "not hydrated" {
		t.Fatal("showcase did not start with SSR-only Go Engine")
	}
	_, err = engine.SetEngine(context.Background(), runtimemodel.EngineJavaScript)
	if err != nil {
		t.Fatal(err)
	}
	waitForFixtureText(t, engine, mutations, "next-hydration-marker", "hydrated")
	jsPage := engine.Page()
	if len(jsPage.Fonts) != 1 || !jsPage.Fonts[0].Decoded {
		t.Fatalf("showcase Web Font = %+v errors=%v", jsPage.Fonts, jsPage.FontErrors)
	}
	imageNode := fixtureNode(t, jsPage, "next-image")
	if resource := jsPage.ImageResources[imageNode.ID]; !resource.Loaded || resource.Error != "" {
		t.Fatalf("showcase picture/image = %+v errors=%v", resource, jsPage.ImageErrors)
	}
	svgNode := fixtureNode(t, jsPage, "next-svg")
	if resource := jsPage.ImageResources[svgNode.ID]; !resource.Loaded || resource.IntrinsicWidth != 80 || resource.IntrinsicHeight != 48 {
		t.Fatalf("showcase inline SVG = %+v", resource)
	}
	if !engine.DispatchClick(fixtureNode(t, jsPage, "next-counter").ID, 0, 0) || fixtureNode(t, jsPage, "next-count").TextContent() != "1" {
		t.Fatal("showcase hydrated interaction failed")
	}
	if !engine.DispatchClick(fixtureNode(t, jsPage, "next-navigation").ID, 0, 0) || engine.Page().URL.Path != "/next/about" {
		t.Fatal("showcase client Navigation failed")
	}

	if _, err := engine.Navigate(context.Background(), server.URL+"/tailwind/"); err != nil {
		t.Fatal(err)
	}
	engine.UpdateViewport(1024, 720)
	grid := fixtureNode(t, engine.Page(), "tailwind-grid")
	wide, _ := engine.Page().ComputedStyles.For(grid)
	engine.UpdateViewport(640, 720)
	narrow, _ := engine.Page().ComputedStyles.For(grid)
	if wide.Display != style.DisplayGrid || narrow.Display != style.DisplayBlock {
		t.Fatalf("showcase responsive state = wide:%v narrow:%v", wide.Display, narrow.Display)
	}

	if _, err := engine.Navigate(context.Background(), server.URL+"/diagnostics/"); err != nil {
		t.Fatal(err)
	}
	waitForFixtureText(t, engine, mutations, "chunk-state", "chunk failure isolated")
	diagnosticPage := engine.Page()
	if !engine.DispatchClick(fixtureNode(t, diagnosticPage, "hydration-error").ID, 0, 0) {
		t.Fatal("hydration failure control was not handled")
	}
	if !engine.DispatchClick(fixtureNode(t, diagnosticPage, "observer-error").ID, 0, 0) {
		t.Fatal("observer failure control was not handled")
	}
	contexts := diagnosticPage.RuntimeDiagnostics()
	for _, reason := range []string{"chunk", "hydration", "observer"} {
		if !showcaseHasDiagnostic(contexts, "runtime", "error", reason) {
			t.Errorf("showcase DevTools is missing %s error: %+v", reason, contexts)
		}
	}
	if !showcaseHasDiagnostic(contexts, "font", "fallback", "decode") || !showcaseHasDiagnostic(contexts, "image", "fallback", "decode") {
		t.Fatalf("showcase DevTools fallback diagnostics = %+v font=%v image=%v", contexts, diagnosticPage.FontErrors, diagnosticPage.ImageErrors)
	}
}

func showcaseHasDiagnostic(contexts []devtools.RuntimeContext, category, state, reason string) bool {
	for _, context := range contexts {
		for _, diagnostic := range context.Diagnostics {
			if diagnostic.Category == category && diagnostic.State == state && diagnostic.Reason == reason {
				return true
			}
		}
	}
	return false
}
