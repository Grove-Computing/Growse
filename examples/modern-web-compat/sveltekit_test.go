package main

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/browser"
	"github.com/Grove-Computing/Growse/internal/network"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
	"github.com/Grove-Computing/Growse/internal/runtime/javascript"
)

func TestSvelteKitSSRFixtureHydratesAndEnhancesForm(t *testing.T) {
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

	page, err := engine.Navigate(context.Background(), server.URL+"/svelte/")
	if err != nil {
		t.Fatal(err)
	}
	rootID := fixtureNode(t, page, "svelte").ID
	waitForFixtureText(t, engine, mutations, "svelte-hydration-marker", "hydrated")
	page = engine.Page()
	if fixtureNode(t, page, "svelte").ID != rootID {
		t.Fatal("SvelteKit hydration replaced the SSR root")
	}
	root := fixtureNode(t, page, "svelte")
	for attribute, want := range map[string]string{
		"data-framework-build":      "SvelteKit 2.70.3 / Svelte 5.57.0",
		"data-upstream-entrypoint":  "upstream-export/_app/immutable/entry/start.DKsUhvBt.js",
		"data-upstream-application": "upstream-export/_app/immutable/entry/app.BIgHAFAw.js",
	} {
		if got, _ := root.Attribute(attribute); got != want {
			t.Errorf("%s = %q, want %q", attribute, got, want)
		}
	}
	if requests.count("/_app/immutable/entry/start.mjs") != 1 || requests.count("/_app/immutable/nodes/app.mjs") != 1 {
		t.Fatalf("SvelteKit Module requests = %#v", requests.paths)
	}
	if requests.count("/_app/immutable/upstream-contract.mjs") != 1 {
		t.Fatalf("SvelteKit upstream build contract requests = %#v", requests.paths)
	}

	if !engine.DispatchClick(fixtureNode(t, page, "svelte-reactive").ID, 0, 0) {
		t.Fatal("reactive update Event was not handled")
	}
	if got := fixtureNode(t, page, "svelte-state").TextContent(); got != "reactive:1" {
		t.Fatalf("reactive state = %q", got)
	}
	if class, _ := root.Attribute("class"); class != "interactive" {
		t.Fatalf("SvelteKit class mutation = %q", class)
	}
	if style, _ := fixtureNode(t, page, "svelte-state").Attribute("style"); !strings.Contains(style, "color: rgb(124,58,237)") {
		t.Fatalf("SvelteKit style mutation = %q", style)
	}
	if !engine.SetInputValue(fixtureNode(t, page, "svelte-name").ID, "Growse") {
		t.Fatal("form input was not updated")
	}
	if !engine.DispatchClick(fixtureNode(t, page, "svelte-submit").ID, 0, 0) {
		t.Fatal("enhanced form submit was not handled")
	}
	if got := fixtureNode(t, page, "svelte-form-result").TextContent(); got != "enhanced:Growse" {
		t.Fatalf("enhanced form result = %q", got)
	}
	if !engine.DispatchClick(fixtureNode(t, page, "svelte-dialog-toggle").ID, 0, 0) {
		t.Fatal("dialog Event was not handled")
	}
	if _, hidden := fixtureNode(t, page, "svelte-dialog").Attribute("hidden"); hidden || fixtureNode(t, page, "svelte-dialog-state").TextContent() != "open:focused" {
		t.Fatal("SvelteKit dialog did not open and receive focus")
	}
	if !engine.DispatchClick(fixtureNode(t, page, "svelte-menu-toggle").ID, 0, 0) {
		t.Fatal("menu Event was not handled")
	}
	if _, hidden := fixtureNode(t, page, "svelte-menu").Attribute("hidden"); hidden {
		t.Fatal("SvelteKit menu remained hidden")
	}
	if !engine.DispatchClick(fixtureNode(t, page, "svelte-navigation").ID, 0, 0) {
		t.Fatal("client Navigation Event was not handled")
	}
	if got := engine.Page().URL.Path; got != "/svelte/about" {
		t.Fatalf("client Navigation path = %q", got)
	}
	if _, err := engine.Back(context.Background()); err != nil {
		t.Fatalf("history back traversal: %v", err)
	}
	waitForFixtureText(t, engine, mutations, "svelte-route", "/svelte/")
	if engine.Page().URL.Path != "/svelte/" || fixtureNode(t, engine.Page(), "svelte").ID != rootID {
		t.Fatal("SvelteKit history traversal replaced SSR identity or lost popstate")
	}
	if engine.Page().RuntimeError != "" || len(engine.Page().ScriptErrors) != 0 {
		t.Fatalf("SvelteKit fixture runtime errors = %q / %v", engine.Page().RuntimeError, engine.Page().ScriptErrors)
	}
}
