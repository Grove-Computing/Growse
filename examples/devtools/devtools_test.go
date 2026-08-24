package main

import (
	"context"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Grove-Computing/Growse/internal/browser"
	"github.com/Grove-Computing/Growse/internal/devtools"
	"github.com/Grove-Computing/Growse/internal/layout"
	"github.com/Grove-Computing/Growse/internal/network"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
	"github.com/Grove-Computing/Growse/internal/runtime/yaegi"
)

func TestDevToolsShowcaseExercisesAllPanelsWithoutCredentialLeaks(t *testing.T) {
	server := httptest.NewServer(devToolsHandler())
	defer server.Close()
	engine := browser.NewWithRuntimeFactory(network.NewClientWithLimits(server.Client(), 1<<20), func() runtimemodel.Runtime { return yaegi.New() })
	defer engine.Close()
	mutations := make(chan struct{}, 32)
	engine.SetOnMutation(func() {
		select {
		case mutations <- struct{}{}:
		default:
		}
	})
	page, err := engine.Navigate(context.Background(), server.URL+"/?credential=showcase-navigation-secret")
	if err != nil {
		t.Fatal(err)
	}
	waitForShowcase(t, engine, mutations)

	consoleRecords := page.DevTools.Console()
	if len(consoleRecords) != 4 {
		t.Fatalf("console records = %+v", consoleRecords)
	}
	levels := map[devtools.ConsoleLevel]bool{}
	for _, record := range consoleRecords {
		levels[record.Level] = true
	}
	for _, level := range []devtools.ConsoleLevel{devtools.ConsoleLog, devtools.ConsoleInfo, devtools.ConsoleWarn, devtools.ConsoleError} {
		if !levels[level] {
			t.Fatalf("missing console level %s: %+v", level, consoleRecords)
		}
	}

	networkRecords := page.DevTools.Network()
	var redirected, cacheHit, httpError, timeout bool
	for _, record := range networkRecords {
		redirected = redirected || record.Redirected
		cacheHit = cacheHit || record.CacheStatus == "hit"
		httpError = httpError || record.ErrorCategory == "http" && record.StatusCode == 503
		timeout = timeout || record.ErrorCategory == "timeout"
	}
	if !redirected || !cacheHit || !httpError || !timeout {
		t.Fatalf("network states redirect=%v cache=%v http=%v timeout=%v records=%+v", redirected, cacheHit, httpError, timeout, networkRecords)
	}

	password, ok := page.Document.GetElementByID("password")
	if !ok {
		t.Fatal("password fixture missing")
	}
	tree := layout.BuildWithViewport(page.Document, page.ComputedStyles, 960, 720)
	showcase, ok := page.Document.GetElementByID("showcase")
	if !ok {
		t.Fatal("showcase fixture missing")
	}
	inspector := devtools.SnapshotInspector(page.Document, page.ComputedStyles, tree, showcase.ID)
	if inspector.SelectedNode == nil || inspector.Layout == nil || len(inspector.Styles) == 0 {
		t.Fatalf("inspector state = %+v", inspector)
	}
	passwordInspector := devtools.SnapshotInspector(page.Document, page.ComputedStyles, tree, password.ID)
	allSnapshots := fmt.Sprintf("console=%+v network=%+v inspector=%+v password=%+v", consoleRecords, networkRecords, inspector, passwordInspector)
	for _, secret := range []string{"showcase-navigation-secret", "showcase-query-secret", "showcase-api-secret", "showcase-password"} {
		if strings.Contains(allSnapshots, secret) {
			t.Fatalf("credential %q remained in DevTools snapshot", secret)
		}
	}
}

func waitForShowcase(t *testing.T, engine *browser.Browser, mutations <-chan struct{}) {
	t.Helper()
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	for {
		ready := false
		engine.InspectPage(func(page *browser.Page) bool {
			ready = showcaseText(page, "success-state") == "success" &&
				showcaseText(page, "redirect-state") == "redirect followed" &&
				showcaseText(page, "cache-state") == "miss then hit" &&
				showcaseText(page, "error-state") == "HTTP 503" &&
				showcaseText(page, "timeout-state") == "timeout" &&
				showcaseText(page, "mutation-state") == "mutated by WebGo"
			return true
		})
		if ready {
			return
		}
		select {
		case <-mutations:
		case <-deadline.C:
			t.Fatal("DevTools Showcase did not reach all deterministic states")
		}
	}
}

func showcaseText(page *browser.Page, id string) string {
	if page == nil || page.Document == nil {
		return ""
	}
	element, ok := page.Document.GetElementByID(id)
	if !ok {
		return ""
	}
	return element.TextContent()
}
