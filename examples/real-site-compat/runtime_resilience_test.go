package realsitecompat

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/browser"
	layoutmodel "github.com/Grove-Computing/Growse/internal/layout"
	"github.com/Grove-Computing/Growse/internal/network"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
	"github.com/Grove-Computing/Growse/internal/runtime/javascript"
)

func TestUnsupportedCustomElementKeepsGitHubSSRVisibleAndDiagnosable(t *testing.T) {
	server := httptest.NewServer(http.FileServer(http.Dir(".")))
	defer server.Close()
	engine := browser.NewWithEngineFactory(network.NewClientWithLimits(server.Client(), 8<<20), func(selected runtimemodel.Engine) runtimemodel.Runtime {
		if selected == runtimemodel.EngineJavaScript {
			return javascript.New()
		}
		return nil
	})
	defer engine.Close()
	if _, err := engine.SetEngine(context.Background(), runtimemodel.EngineJavaScript); err != nil {
		t.Fatal(err)
	}
	page, err := engine.Navigate(context.Background(), server.URL+"/fixtures/github-profile.html")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(page.Document.Root.TextContent(), "Saku0512") || !strings.Contains(page.Document.Root.TextContent(), "Popular repositories") {
		t.Fatalf("unsupported script erased SSR content: %q", page.Document.Root.TextContent())
	}
	tree := layoutmodel.BuildWithScrollAndResources(page.Document, page.ComputedStyles, page.ImageResources, page.WebFonts, 1280, 840, 0, 0)
	regions, issues := analyzeRegions(page.Document, tree, []string{"navigation", "profile", "heading", "main"})
	if len(regions) != 4 || len(issues) != 0 {
		t.Fatalf("SSR regions after script failure = regions:%#v issues:%#v", regions, issues)
	}
	found := false
	for _, runtime := range page.RuntimeDiagnostics() {
		for _, diagnostic := range runtime.Diagnostics {
			if diagnostic.Category == "runtime" && diagnostic.State == "error" && diagnostic.Reason == "unsupported-global" {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("custom element failure lacks unsupported-global diagnostic: %#v", page.RuntimeDiagnostics())
	}
}
