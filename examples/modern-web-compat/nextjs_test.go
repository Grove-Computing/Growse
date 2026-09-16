package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
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
	for attribute, want := range map[string]string{
		"data-build-id":            "growse-v0.17.0-nextjs",
		"data-framework-build":     "Next.js 16.3.3 / React 19.2.8",
		"data-upstream-entrypoint": "upstream-export/_next/static/chunks/34xz_oa9zbnuv.js",
	} {
		if got, _ := root.Attribute(attribute); got != want {
			t.Errorf("%s = %q, want %q", attribute, got, want)
		}
	}
	if requests.count("/_next/static/chunks/app.mjs") != 1 || requests.count("/_next/static/chunks/counter.chunk.mjs") != 1 {
		t.Fatalf("Next.js chunk requests = %#v", requests.paths)
	}
	if requests.count("/_next/static/chunks/upstream-contract.mjs") != 1 {
		t.Fatalf("Next.js upstream build contract requests = %#v", requests.paths)
	}
	foundChunkDiagnostic := false
	for _, context := range page.RuntimeDiagnostics() {
		for _, diagnostic := range context.Diagnostics {
			if diagnostic.Category == "resource/module" && diagnostic.State == "loaded" &&
				strings.Contains(diagnostic.Subject, "counter.chunk.mjs") && diagnostic.Initiator == "module-graph" {
				foundChunkDiagnostic = true
			}
		}
	}
	if !foundChunkDiagnostic {
		t.Fatalf("DevTools did not expose the dynamic chunk: %+v", page.RuntimeDiagnostics())
	}
	counter := fixtureNode(t, page, "next-counter")
	counterStyle, _ := page.ComputedStyles.For(counter)
	if counterStyle.Color != 0xffffffff || counterStyle.BackgroundColor != 0x2563ebff {
		t.Fatalf("counter visual style = color:%08x background:%08x", counterStyle.Color, counterStyle.BackgroundColor)
	}

	if !engine.DispatchClick(counter.ID, 0, 0) {
		t.Fatal("counter Event was not handled")
	}
	if got := fixtureNode(t, page, "next-count").TextContent(); got != "1" {
		t.Fatalf("counter state = %q", got)
	}
	if class, _ := root.Attribute("class"); class != "interactive" {
		t.Fatalf("Next.js class mutation = %q", class)
	}
	if style, _ := fixtureNode(t, page, "next-count").Attribute("style"); !strings.Contains(style, "color: rgb(37,99,235)") {
		t.Fatalf("Next.js style mutation = %q", style)
	}
	if !engine.DispatchClick(fixtureNode(t, page, "next-dialog-toggle").ID, 0, 0) {
		t.Fatal("dialog Event was not handled")
	}
	dialog := fixtureNode(t, page, "next-dialog")
	if _, hidden := dialog.Attribute("hidden"); hidden || fixtureNode(t, page, "next-dialog-state").TextContent() != "open:focused" {
		t.Fatal("Next.js dialog did not open and receive focus")
	}
	if !engine.DispatchClick(fixtureNode(t, page, "next-menu-toggle").ID, 0, 0) {
		t.Fatal("menu Event was not handled")
	}
	if _, hidden := fixtureNode(t, page, "next-menu").Attribute("hidden"); hidden {
		t.Fatal("Next.js menu remained hidden")
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
	if _, err := engine.Back(context.Background()); err != nil {
		t.Fatalf("history back traversal: %v", err)
	}
	waitForFixtureText(t, engine, mutations, "next-route", "/next/")
	if engine.Page().URL.Path != "/next/" || fixtureNode(t, engine.Page(), "__next").ID != rootID {
		t.Fatal("Next.js history traversal replaced SSR identity or lost popstate")
	}
	if engine.Page().RuntimeError != "" || len(engine.Page().ScriptErrors) != 0 {
		t.Fatalf("Next.js fixture runtime errors = %q / %v", engine.Page().RuntimeError, engine.Page().ScriptErrors)
	}
}

func TestNextJSDelayedResourcesKeepSSRInteractiveThroughIncrementalCommits(t *testing.T) {
	scriptStarted := make(chan struct{}, 1)
	imageStarted := make(chan struct{}, 1)
	scriptGate := make(chan struct{})
	imageGate := make(chan struct{})
	var releaseScriptOnce sync.Once
	var releaseImageOnce sync.Once
	releaseScript := func() { releaseScriptOnce.Do(func() { close(scriptGate) }) }
	releaseImage := func() { releaseImageOnce.Do(func() { close(imageGate) }) }
	defer releaseScript()
	defer releaseImage()

	fixture := modernWebCompatibilityHandler()
	delayed := http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		var started chan<- struct{}
		var gate <-chan struct{}
		switch request.URL.Path {
		case "/_next/static/chunks/app.mjs":
			started, gate = scriptStarted, scriptGate
		case "/assets/pixel.png":
			started, gate = imageStarted, imageGate
		}
		if gate != nil {
			select {
			case started <- struct{}{}:
			default:
			}
			select {
			case <-gate:
			case <-request.Context().Done():
				return
			}
		}
		fixture.ServeHTTP(response, request)
	})
	server := httptest.NewServer(delayed)
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
	mutations := make(chan struct{}, 64)
	engine.SetOnMutation(func() {
		select {
		case mutations <- struct{}{}:
		default:
		}
	})

	type navigationResult struct {
		page *browser.Page
		err  error
	}
	navigation := make(chan navigationResult, 1)
	go func() {
		page, err := engine.Navigate(context.Background(), server.URL+"/next/")
		navigation <- navigationResult{page: page, err: err}
	}()
	for name, started := range map[string]<-chan struct{}{"script": scriptStarted, "image": imageStarted} {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatalf("delayed %s request did not start", name)
		}
	}
	page := engine.Page()
	if page == nil {
		t.Fatal("initial SSR was not published while resources were delayed")
	}
	if got := fixtureNode(t, page, "next-ssr-marker").TextContent(); got != "SSR rendered" {
		t.Fatalf("initial SSR marker = %q", got)
	}
	if got := fixtureNode(t, page, "next-hydration-marker").TextContent(); got != "not hydrated" {
		t.Fatalf("hydration ran before delayed script was released: %q", got)
	}
	imageNode := fixtureNode(t, page, "next-image")
	if resource := page.ImageResources[imageNode.ID]; resource.Loaded || resource.Error != "" {
		t.Fatalf("delayed image settled in initial commit: %+v", resource)
	}

	releaseScript()
	select {
	case result := <-navigation:
		if result.err != nil || result.page != engine.Page() {
			t.Fatalf("completed Navigation = page:%p active:%p error:%v", result.page, engine.Page(), result.err)
		}
	case <-time.After(time.Second):
		t.Fatal("Navigation did not complete after delayed script was released")
	}
	waitForInspectedFixtureText(t, engine, mutations, "next-hydration-marker", "hydrated")
	page = engine.Page()
	if resource := page.ImageResources[imageNode.ID]; resource.Loaded || resource.Error != "" {
		t.Fatalf("image settled with the independent script commit: %+v", resource)
	}

	releaseImage()
	waitForFixtureImageSettled(t, engine, mutations, "next-image")
	page = engine.Page()
	if resource := page.ImageResources[imageNode.ID]; !resource.Loaded || resource.Error != "" {
		t.Fatalf("delayed image commit = %+v", resource)
	}
	if !engine.DispatchClick(fixtureNode(t, page, "next-navigation").ID, 0, 0) {
		t.Fatal("client Navigation Event was not handled after resource commits")
	}
	if engine.Page().URL.Path != "/next/about" || fixtureNode(t, engine.Page(), "next-route").TextContent() != "/next/about" {
		t.Fatalf("post-resource Navigation = %s / %q", engine.Page().URL.Path, fixtureNode(t, engine.Page(), "next-route").TextContent())
	}
}

func waitForInspectedFixtureText(t *testing.T, engine *browser.Browser, mutations <-chan struct{}, id, want string) {
	t.Helper()
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	for {
		got := ""
		if engine.InspectPage(func(page *browser.Page) bool {
			if node, ok := page.Document.GetElementByID(id); ok {
				got = node.TextContent()
			}
			return true
		}) && got == want {
			return
		}
		select {
		case <-mutations:
		case <-deadline.C:
			t.Fatalf("fixture %s = %q, want %q", id, got, want)
		}
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

func waitForFixtureImage(t *testing.T, engine *browser.Browser, mutations <-chan struct{}, id string) {
	t.Helper()
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	var findImage func(*dom.Node) *dom.Node
	findImage = func(node *dom.Node) *dom.Node {
		if node.Type == dom.NodeElement && (node.TagName == "img" || node.TagName == "svg") {
			return node
		}
		for _, child := range node.Children {
			if image := findImage(child); image != nil {
				return image
			}
		}
		return nil
	}
	for {
		page := engine.Page()
		container := fixtureNode(t, page, id)
		if image := findImage(container); image != nil && page.ImageResources[image.ID].Loaded {
			return
		}
		select {
		case <-mutations:
		case <-deadline.C:
			t.Fatalf("fixture %s image did not load; errors=%v", id, page.ImageErrors)
		}
	}
}

func waitForFixtureImageSettled(t *testing.T, engine *browser.Browser, mutations <-chan struct{}, id string) {
	t.Helper()
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	for {
		page := engine.Page()
		image := fixtureNode(t, page, id)
		if resource, ok := page.ImageResources[image.ID]; ok && (resource.Loaded || resource.Error != "") {
			return
		}
		select {
		case <-mutations:
		case <-deadline.C:
			t.Fatalf("fixture %s image did not settle; errors=%v", id, page.ImageErrors)
		}
	}
}
