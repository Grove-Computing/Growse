package main

import (
	"context"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Grove-Computing/Growse/internal/browser"
	"github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/network"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
	"github.com/Grove-Computing/Growse/internal/runtime/isolated"
	"github.com/Grove-Computing/Growse/internal/serviceworker"
	storagecore "github.com/Grove-Computing/Growse/internal/storage"
)

func TestExternalWebPlatformShowcaseRunsOfflineAcrossOrigins(t *testing.T) {
	cdn := httptest.NewServer(externalCDNHandler())
	defer cdn.Close()
	frameOrigin := httptest.NewServer(externalFrameHandler())
	defer frameOrigin.Close()
	top := httptest.NewServer(externalWebPlatformHandler(cdn.URL, frameOrigin.URL))
	defer top.Close()

	workers := serviceworker.NewManager()
	t.Cleanup(func() { _ = workers.Close() })
	engine := browser.NewWithEngineFactoryAndStorageAndServiceWorkers(
		network.NewClientWithLimits(top.Client(), 4<<20),
		func(selected runtimemodel.Engine) runtimemodel.Runtime { return isolated.New(selected) },
		storagecore.NewManager(), workers,
	)
	t.Cleanup(func() { _ = engine.Close() })
	if _, err := engine.SetEngine(context.Background(), runtimemodel.EngineJavaScript); err != nil {
		t.Fatal(err)
	}
	mutations := make(chan struct{}, 128)
	engine.SetOnMutation(func() {
		select {
		case mutations <- struct{}{}:
		default:
		}
	})
	page, err := engine.Navigate(context.Background(), top.URL+"/app/?token=showcase-secret")
	if err != nil {
		t.Fatal(err)
	}
	waitForExternalPlatform(t, engine, workers, mutations)

	if !page.Sandbox.Ready || !page.Sandbox.ProcessBoundary || !page.Sandbox.BrokeredHostIO {
		t.Fatalf("top-level sandbox = %+v", page.Sandbox)
	}
	styled := externalNode(t, page.Document, "classic-state")
	computed, ok := page.ComputedStyles.For(styled.Parent)
	if !ok || computed.Color != 0x0b7285ff {
		t.Fatalf("cross-origin CSS style = (%+v, %v)", computed, ok)
	}
	mutation := externalNode(t, page.Document, "mutation-target")
	if class, _ := mutation.Attribute("class"); class != "mutated" {
		t.Fatalf("DOM mutation class = %q", class)
	}

	if len(page.Frames) != 3 {
		t.Fatalf("frame count = %d, want 3", len(page.Frames))
	}
	assertExternalFrameText(t, page, "same-frame", "frame-state", "same-origin frame loaded")
	assertExternalFrameText(t, page, "cross-frame", "frame-state", "cross-origin frame loaded")
	sandboxElement := externalNode(t, page.Document, "sandbox-frame")
	sandboxFrame, ok := page.FrameByElement(sandboxElement.ID)
	if !ok || sandboxFrame.Page == nil || !sandboxFrame.Page.FramePolicy.Sandboxed || sandboxFrame.Page.FramePolicy.AllowsScripts() || sandboxFrame.Page.RuntimeStarted {
		t.Fatalf("sandbox frame = %+v", sandboxFrame)
	}
	if value := externalText(sandboxFrame.Page.Document, "sandbox-state"); value != "script blocked by sandbox" {
		t.Fatalf("sandbox script result = %q", value)
	}
	assertRuntimeDiagnostics(t, page)
}

func assertRuntimeDiagnostics(t *testing.T, page *browser.Page) {
	t.Helper()
	contexts := page.RuntimeDiagnostics()
	kinds := make(map[string]int)
	workerGenerations := make(map[uint64]bool)
	foundModule := false
	for _, context := range contexts {
		kinds[context.Kind]++
		if context.Sandbox.Generation != 0 {
			workerGenerations[context.Sandbox.Generation] = true
		}
		if context.Kind == "service-worker" && (context.BrowsingGeneration == 0 || context.State != "activated") {
			t.Fatalf("Service Worker diagnostic = %+v", context)
		}
		for _, script := range context.Scripts {
			foundModule = foundModule || script.Kind == "module"
		}
	}
	if kinds["page"] != 1 || kinds["frame"] != 3 || kinds["service-worker"] != 1 || !foundModule || len(workerGenerations) < 3 {
		t.Fatalf("runtime diagnostics kinds=%v generations=%v module=%v contexts=%+v", kinds, workerGenerations, foundModule, contexts)
	}
	if output := fmt.Sprintf("%+v", contexts); strings.Contains(output, "showcase-secret") || strings.Contains(output, "token=") {
		t.Fatalf("runtime diagnostics leaked query secret: %s", output)
	}
}

func waitForExternalPlatform(t *testing.T, engine *browser.Browser, workers *serviceworker.Manager, mutations <-chan struct{}) {
	t.Helper()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for {
		ready := false
		engine.InspectPage(func(page *browser.Page) bool {
			ready = externalText(page.Document, "classic-state") == "external classic loaded" &&
				externalText(page.Document, "module-state") == "external module loaded" &&
				externalText(page.Document, "dynamic-state") == "dynamic import loaded" &&
				externalText(page.Document, "wasm-state") == "WASM answer=42" &&
				externalText(page.Document, "service-worker-state") == "registered and active" &&
				externalText(page.Document, "offline-state") == "offline response from Service Worker" &&
				externalText(page.Document, "mutation-state") == "mutated by external JavaScript"
			return true
		})
		if ready {
			return
		}
		select {
		case <-mutations:
		case <-deadline.C:
			page := engine.Page()
			t.Fatalf("Showcase did not become ready: classic=%q module=%q dynamic=%q wasm=%q worker=%q offline=%q mutation=%q controller=%+v errors=%v runtime=%q",
				externalText(page.Document, "classic-state"), externalText(page.Document, "module-state"),
				externalText(page.Document, "dynamic-state"), externalText(page.Document, "wasm-state"),
				externalText(page.Document, "service-worker-state"), externalText(page.Document, "offline-state"),
				externalText(page.Document, "mutation-state"), workers.Controller(page.URL), page.ScriptErrors, page.RuntimeError)
		}
	}
}

func assertExternalFrameText(t *testing.T, page *browser.Page, elementID, contentID, want string) {
	t.Helper()
	element := externalNode(t, page.Document, elementID)
	frame, ok := page.FrameByElement(element.ID)
	if !ok || frame.Page == nil || externalText(frame.Page.Document, contentID) != want {
		t.Fatalf("frame %s = %+v text=%q", elementID, frame, externalText(frame.Page.Document, contentID))
	}
}

func externalNode(t *testing.T, document *dom.Document, id string) *dom.Node {
	t.Helper()
	node, ok := document.GetElementByID(id)
	if !ok {
		t.Fatalf("node %q was not found", id)
	}
	return node
}

func externalText(document *dom.Document, id string) string {
	if document == nil {
		return ""
	}
	node, ok := document.GetElementByID(id)
	if !ok {
		return ""
	}
	return node.TextContent()
}
