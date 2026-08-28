// Command growse-smoke runs the packaged External Web Platform fixture without
// opening a GUI. It is shipped in the Docker image as a release smoke test.
package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Grove-Computing/Growse/internal/browser"
	"github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/network"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
	"github.com/Grove-Computing/Growse/internal/runtime/isolated"
	"github.com/Grove-Computing/Growse/internal/serviceworker"
	storagecore "github.com/Grove-Computing/Growse/internal/storage"
	woff2 "github.com/pgaskin/go-woff2"
	"golang.org/x/image/font/gofont/goregular"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: growse-smoke <external-web-platform-directory>")
		os.Exit(2)
	}
	root := os.Args[1]
	if filepath.Base(filepath.Clean(root)) == "modern-web-compat" {
		if err := runModern(root); err != nil {
			fmt.Fprintln(os.Stderr, "Docker Modern Web Compatibility smoke failed:", err)
			os.Exit(1)
		}
		fmt.Println("Docker Modern Web Compatibility smoke passed: sandbox worker, SSR, hydration, interaction, navigation, CSS, image, font")
		return
	}
	if err := runExternal(root); err != nil {
		fmt.Fprintln(os.Stderr, "Docker Web Platform smoke failed:", err)
		os.Exit(1)
	}
	fmt.Println("Docker Web Platform smoke passed: sandbox worker, JavaScript, Module, WASM, iframe, Service Worker")
}

func runExternal(root string) error {
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		return fmt.Errorf("fixture directory is unavailable: %s", root)
	}
	cdn := httptest.NewServer(cdnHandler(root))
	defer cdn.Close()
	frameOrigin := httptest.NewServer(http.FileServer(http.Dir(root)))
	defer frameOrigin.Close()
	top := httptest.NewServer(topHandler(root, cdn.URL, frameOrigin.URL))
	defer top.Close()

	workers := serviceworker.NewManager()
	defer workers.Close()
	engine := browser.NewWithEngineFactoryAndStorageAndServiceWorkers(
		network.NewClientWithLimits(top.Client(), 4<<20),
		func(selected runtimemodel.Engine) runtimemodel.Runtime { return isolated.New(selected) },
		storagecore.NewManager(), workers,
	)
	defer engine.Close()
	if _, err := engine.SetEngine(context.Background(), runtimemodel.EngineJavaScript); err != nil {
		return err
	}
	mutations := make(chan struct{}, 128)
	engine.SetOnMutation(func() {
		select {
		case mutations <- struct{}{}:
		default:
		}
	})
	page, err := engine.Navigate(context.Background(), top.URL+"/app/?token=container-secret")
	if err != nil {
		return err
	}
	if err := waitUntilReady(engine, mutations); err != nil {
		return err
	}
	if !page.Sandbox.Ready || !page.Sandbox.ProcessBoundary || !page.Sandbox.BrokeredHostIO || !contains(page.Sandbox.Constraints, "linux:no-new-privileges") {
		return fmt.Errorf("top-level sandbox is not verified: %+v", page.Sandbox)
	}
	if len(page.Frames) != 3 {
		return fmt.Errorf("frame count = %d", len(page.Frames))
	}
	for _, frame := range page.Frames[:2] {
		if frame.Page == nil || !frame.Page.RuntimeStarted || !frame.Page.Sandbox.Ready {
			return fmt.Errorf("active frame sandbox is not ready: %+v", frame)
		}
	}
	sandboxFrame := page.Frames[2]
	if sandboxFrame.Page == nil || sandboxFrame.Page.RuntimeStarted || text(sandboxFrame.Page.Document, "sandbox-state") != "script blocked by sandbox" {
		return fmt.Errorf("sandboxed frame executed script: %+v", sandboxFrame)
	}
	kinds := make(map[string]int)
	for _, diagnostic := range page.RuntimeDiagnostics() {
		kinds[diagnostic.Kind]++
		if strings.Contains(diagnostic.URL, "container-secret") || strings.Contains(diagnostic.URL, "token=") {
			return errors.New("runtime diagnostic leaked fixture query")
		}
	}
	if kinds["page"] != 1 || kinds["frame"] != 3 || kinds["service-worker"] != 1 {
		return fmt.Errorf("runtime diagnostic contexts = %v", kinds)
	}
	return nil
}

func runModern(root string) error {
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		return fmt.Errorf("fixture directory is unavailable: %s", root)
	}
	server := httptest.NewServer(modernHandler(root))
	defer server.Close()
	engine := browser.NewWithEngineFactory(network.NewClientWithLimits(server.Client(), 4<<20), func(selected runtimemodel.Engine) runtimemodel.Runtime {
		return isolated.New(selected)
	})
	defer engine.Close()
	if _, err := engine.SetEngine(context.Background(), runtimemodel.EngineJavaScript); err != nil {
		return err
	}
	mutations := make(chan struct{}, 64)
	engine.SetOnMutation(func() {
		select {
		case mutations <- struct{}{}:
		default:
		}
	})
	page, err := engine.Navigate(context.Background(), server.URL+"/next/")
	if err != nil {
		return err
	}
	if err := waitForText(engine, mutations, "next-hydration-marker", "hydrated"); err != nil {
		return err
	}
	rootNode, ok := page.Document.GetElementByID("__next")
	if !ok {
		return errors.New("Next.js SSR root is missing")
	}
	if token, _ := rootNode.Attribute("data-ssr-token"); token != "next-ssr-root-v1" {
		return fmt.Errorf("Next.js SSR token = %q", token)
	}
	if hydrated, _ := rootNode.Attribute("data-hydrated"); hydrated != "true" {
		return fmt.Errorf("Next.js hydration marker = %q", hydrated)
	}
	if !page.Sandbox.Ready || !page.Sandbox.ProcessBoundary || !page.Sandbox.BrokeredHostIO {
		return fmt.Errorf("modern fixture sandbox is not verified: %+v", page.Sandbox)
	}
	if len(page.Fonts) == 0 || len(page.Images) == 0 {
		return fmt.Errorf("modern fixture resources = fonts:%d images:%d font-errors:%v image-errors:%v", len(page.Fonts), len(page.Images), page.FontErrors, page.ImageErrors)
	}
	counter, ok := page.Document.GetElementByID("next-counter")
	if !ok || !engine.DispatchClick(counter.ID, 0, 0) {
		return errors.New("Next.js counter Event was not handled")
	}
	if err := waitForText(engine, mutations, "next-count", "1"); err != nil {
		return err
	}
	navigation, ok := page.Document.GetElementByID("next-navigation")
	if !ok || !engine.DispatchClick(navigation.ID, 0, 0) {
		return errors.New("Next.js navigation Event was not handled")
	}
	if engine.Page().URL.Path != "/next/about" || text(engine.Page().Document, "next-route") != "/next/about" {
		return fmt.Errorf("Next.js navigation state = %s / %q", engine.Page().URL.Path, text(engine.Page().Document, "next-route"))
	}
	if engine.Page().RuntimeError != "" || len(engine.Page().ScriptErrors) != 0 {
		return fmt.Errorf("Next.js runtime errors = %q / %v", engine.Page().RuntimeError, engine.Page().ScriptErrors)
	}
	return nil
}

func waitForText(engine *browser.Browser, mutations <-chan struct{}, id, want string) error {
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	for {
		if text(engine.Page().Document, id) == want {
			return nil
		}
		select {
		case <-mutations:
		case <-deadline.C:
			page := engine.Page()
			return fmt.Errorf("fixture %s = %q, want %q; runtime=%q scripts=%v", id, text(page.Document, id), want, page.RuntimeError, page.ScriptErrors)
		}
	}
}

func modernHandler(root string) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		var target string
		switch request.URL.Path {
		case "/next/", "/next/about":
			target = filepath.Join(root, "fixtures", "nextjs", "index.html")
		case "/_next/static/css/app.css":
			target = filepath.Join(root, "fixtures", "nextjs", "app.css")
			response.Header().Set("Content-Type", "text/css; charset=utf-8")
		case "/_next/static/chunks/app.mjs":
			target = filepath.Join(root, "fixtures", "nextjs", "app.mjs")
			response.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		case "/_next/static/chunks/counter.chunk.mjs":
			target = filepath.Join(root, "fixtures", "nextjs", "counter.chunk.mjs")
			response.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		case "/assets/growse-regular.woff2":
			response.Header().Set("Content-Type", "font/woff2")
			_, _ = response.Write(modernSmokeFont)
			return
		case "/assets/pixel.png":
			response.Header().Set("Content-Type", "image/png")
			_, _ = response.Write(modernSmokePNG)
			return
		default:
			http.NotFound(response, request)
			return
		}
		http.ServeFile(response, request, target)
	})
}

var (
	modernSmokeFont = mustModernSmokeFont()
	modernSmokePNG  = mustModernSmokePNG()
)

func mustModernSmokeFont() []byte {
	encoded, err := woff2.Encode(goregular.TTF, nil)
	if err != nil {
		panic(err)
	}
	return encoded
}

func mustModernSmokePNG() []byte {
	pixel := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	for y := range 2 {
		for x := range 2 {
			pixel.Set(x, y, color.NRGBA{R: 37, G: 99, B: 235, A: 255})
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, pixel); err != nil {
		panic(err)
	}
	return encoded.Bytes()
}

func waitUntilReady(engine *browser.Browser, mutations <-chan struct{}) error {
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	for {
		ready := false
		engine.InspectPage(func(page *browser.Page) bool {
			ready = text(page.Document, "classic-state") == "external classic loaded" &&
				text(page.Document, "module-state") == "external module loaded" &&
				text(page.Document, "dynamic-state") == "dynamic import loaded" &&
				text(page.Document, "wasm-state") == "WASM answer=42" &&
				text(page.Document, "service-worker-state") == "registered and active" &&
				text(page.Document, "offline-state") == "offline response from Service Worker" &&
				text(page.Document, "mutation-state") == "mutated by external JavaScript"
			return true
		})
		if ready {
			return nil
		}
		select {
		case <-mutations:
		case <-deadline.C:
			page := engine.Page()
			return fmt.Errorf("fixture timed out: classic=%q module=%q dynamic=%q wasm=%q worker=%q offline=%q errors=%v runtime=%q",
				text(page.Document, "classic-state"), text(page.Document, "module-state"), text(page.Document, "dynamic-state"),
				text(page.Document, "wasm-state"), text(page.Document, "service-worker-state"), text(page.Document, "offline-state"),
				page.ScriptErrors, page.RuntimeError)
		}
	}
}

func topHandler(root, cdnOrigin, frameOrigin string) http.Handler {
	files := http.FileServer(http.Dir(root))
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/", "/app/":
			content, err := os.ReadFile(filepath.Join(root, "index.html"))
			if err != nil {
				http.Error(response, "fixture unavailable", http.StatusInternalServerError)
				return
			}
			page := strings.ReplaceAll(string(content), "{{CDN_ORIGIN}}", cdnOrigin)
			page = strings.ReplaceAll(page, "{{FRAME_ORIGIN}}", frameOrigin)
			response.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = response.Write([]byte(page))
		case "/answer.wasm":
			response.Header().Set("Content-Type", "application/wasm")
			_, _ = response.Write(answerWASM)
		case "/app/sw.js":
			http.ServeFile(response, request, filepath.Join(root, "sw.js"))
		default:
			files.ServeHTTP(response, request)
		}
	})
}

func cdnHandler(root string) http.Handler {
	files := http.FileServer(http.Dir(root))
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Access-Control-Allow-Origin", "*")
		switch request.URL.Path {
		case "/external.css":
			response.Header().Set("Content-Type", "text/css; charset=utf-8")
			_, _ = response.Write([]byte(`@import "/theme.css"; .cdn-style { color: #0b7285; }`))
		case "/theme.css":
			response.Header().Set("Content-Type", "text/css; charset=utf-8")
			_, _ = response.Write([]byte(`.cdn-style { border-color: #15aabf; }`))
		default:
			if strings.HasSuffix(request.URL.Path, ".mjs") {
				response.Header().Set("Content-Type", "text/javascript; charset=utf-8")
			}
			files.ServeHTTP(response, request)
		}
	})
}

func text(document *dom.Document, id string) string {
	if document == nil {
		return ""
	}
	node, ok := document.GetElementByID(id)
	if !ok {
		return ""
	}
	return node.TextContent()
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

var answerWASM = []byte{
	0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00,
	0x01, 0x05, 0x01, 0x60, 0x00, 0x01, 0x7f,
	0x03, 0x02, 0x01, 0x00,
	0x07, 0x0a, 0x01, 0x06, 0x61, 0x6e, 0x73, 0x77, 0x65, 0x72, 0x00, 0x00,
	0x0a, 0x06, 0x01, 0x04, 0x00, 0x41, 0x2a, 0x0b,
}
