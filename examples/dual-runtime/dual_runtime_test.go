package main

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Grove-Computing/Growse/internal/browser"
	"github.com/Grove-Computing/Growse/internal/devtools"
	"github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/network"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
	"github.com/Grove-Computing/Growse/internal/runtime/javascript"
	"github.com/Grove-Computing/Growse/internal/runtime/yaegi"
	storagecore "github.com/Grove-Computing/Growse/internal/storage"
)

func TestDualRuntimeShowcaseSwitchesSamePageFromGoToJavaScript(t *testing.T) {
	server := httptest.NewServer(dualRuntimeHandler())
	defer server.Close()
	client := network.NewClientWithLimits(server.Client(), 1<<20)
	engine := browser.NewWithEngineFactoryAndStorage(client, func(selected runtimemodel.Engine) runtimemodel.Runtime {
		switch selected {
		case runtimemodel.EngineGo:
			return yaegi.New()
		case runtimemodel.EngineJavaScript:
			return javascript.New()
		default:
			return nil
		}
	}, storagecore.NewManager())
	defer engine.Close()
	mutations := make(chan struct{}, 64)
	engine.SetOnMutation(func() {
		select {
		case mutations <- struct{}{}:
		default:
		}
	})

	goPage, err := engine.Navigate(context.Background(), server.URL+"/")
	if err != nil {
		t.Fatal(err)
	}
	waitForDualRuntime(t, engine, mutations, "go", "go timer fired")
	assertSelectedScriptRecord(t, goPage, "go", "/_app.go")
	if !hasEngineConsole(goPage.DevTools.Console(), "go") {
		t.Fatalf("Go Console records = %#v", goPage.DevTools.Console())
	}
	if !engine.DispatchClick(dualNodeID(t, engine, "increment"), 0, 0) || dualText(t, engine, "count") != "1" {
		t.Fatal("Go Counter did not increment")
	}
	if !engine.SetInputValue(dualNodeID(t, engine, "note"), "shared note") {
		t.Fatal("Go input did not update")
	}
	if !engine.DispatchClick(dualNodeID(t, engine, "route"), 0, 0) || !strings.Contains(engine.Page().URL.String(), "view=go#history") {
		t.Fatalf("Go History URL = %v", engine.Page().URL)
	}

	jsPage, err := engine.SetEngine(context.Background(), runtimemodel.EngineJavaScript)
	if err != nil {
		t.Fatal(err)
	}
	waitForDualRuntime(t, engine, mutations, "javascript", "javascript timer fired")
	if dualText(t, engine, "storage") != "shared note" {
		t.Fatalf("JavaScript did not restore Go localStorage value: %q", dualText(t, engine, "storage"))
	}
	assertSelectedScriptRecord(t, jsPage, "javascript", "/app.js")
	if !hasEngineConsole(jsPage.DevTools.Console(), "javascript") {
		t.Fatalf("JavaScript Console records = %#v", jsPage.DevTools.Console())
	}
	if !engine.DispatchClick(dualNodeID(t, engine, "increment"), 0, 0) || dualText(t, engine, "count") != "1" {
		t.Fatal("JavaScript Counter did not increment")
	}
	if !engine.DispatchClick(dualNodeID(t, engine, "route"), 0, 0) || !strings.Contains(engine.Page().URL.String(), "view=javascript#history") {
		t.Fatalf("JavaScript History URL = %v", engine.Page().URL)
	}
	if !engine.DispatchClick(dualNodeID(t, engine, "runtime-error"), 0, 0) {
		t.Fatal("JavaScript error fixture listener did not run")
	}
	if !consoleContains(jsPage.DevTools.Console(), "intentional JavaScript showcase error") {
		t.Fatalf("JavaScript error was not isolated in Console: %#v", jsPage.DevTools.Console())
	}
}

func waitForDualRuntime(t *testing.T, engine *browser.Browser, mutations <-chan struct{}, runtimeName, timer string) {
	t.Helper()
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	for {
		if dualText(t, engine, "engine") == runtimeName && dualText(t, engine, "timer") == timer &&
			dualText(t, engine, "fetch-success") == "offline fixture ready" && dualText(t, engine, "fetch-failure") == "HTTP 503" {
			return
		}
		select {
		case <-mutations:
		case <-deadline.C:
			t.Fatalf("%s Showcase did not reach deterministic ready state", runtimeName)
		}
	}
}

func dualText(t *testing.T, engine *browser.Browser, id string) string {
	t.Helper()
	var result string
	if !engine.InspectPage(func(page *browser.Page) bool {
		if node, ok := page.Document.GetElementByID(id); ok {
			result = node.TextContent()
		}
		return true
	}) {
		t.Fatal("InspectPage() failed")
	}
	return result
}

func dualNodeID(t *testing.T, engine *browser.Browser, id string) dom.NodeID {
	t.Helper()
	var result dom.NodeID
	engine.InspectPage(func(page *browser.Page) bool {
		if node, ok := page.Document.GetElementByID(id); ok {
			result = node.ID
		}
		return true
	})
	if result == 0 {
		t.Fatalf("node %q was not found", id)
	}
	return result
}

func assertSelectedScriptRecord(t *testing.T, page *browser.Page, wantEngine, wantPath string) {
	t.Helper()
	found := false
	for _, record := range page.DevTools.Network() {
		if record.Kind != "script" {
			continue
		}
		found = true
		if record.Engine != wantEngine || !strings.Contains(record.URL, wantPath) {
			t.Fatalf("selected Script record = %#v, want %s %s", record, wantEngine, wantPath)
		}
	}
	if !found {
		t.Fatal("selected external Script was not observed")
	}
}

func hasEngineConsole(records []devtools.ConsoleRecord, engine string) bool {
	for _, record := range records {
		if record.Engine == engine {
			return true
		}
	}
	return false
}

func consoleContains(records []devtools.ConsoleRecord, value string) bool {
	for _, record := range records {
		if strings.Contains(record.Message, value) {
			return true
		}
	}
	return false
}
