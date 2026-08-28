package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/browser"
	layoutmodel "github.com/Grove-Computing/Growse/internal/layout"
	"github.com/Grove-Computing/Growse/internal/network"
	paintmodel "github.com/Grove-Computing/Growse/internal/paint"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
	"github.com/Grove-Computing/Growse/internal/runtime/javascript"
)

type frameworkVisualGolden struct {
	Environment string   `json:"environment"`
	States      []string `json:"states"`
}

func TestFrameworkFixtureVisualRegression(t *testing.T) {
	server := httptest.NewServer(modernWebCompatibilityHandler())
	defer server.Close()
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

	page, err := engine.Navigate(context.Background(), server.URL+"/next/")
	if err != nil {
		t.Fatal(err)
	}
	engine.UpdateViewport(1024, 720)
	actual := frameworkVisualGolden{Environment: "viewport=1024x720 scale=1 font=goregular clock=fixed"}
	actual.States = append(actual.States, visualFixtureState("next-ssr", page, "__next", "next-hydration-marker", "next-count"))
	if _, err := engine.SetEngine(context.Background(), runtimemodel.EngineJavaScript); err != nil {
		t.Fatal(err)
	}
	engine.UpdateViewport(1024, 720)
	waitForFixtureText(t, engine, mutations, "next-hydration-marker", "hydrated")
	page = engine.Page()
	actual.States = append(actual.States, visualFixtureState("next-hydrated", page, "__next", "next-hydration-marker", "next-count"))
	if !engine.DispatchClick(fixtureNode(t, page, "next-counter").ID, 0, 0) {
		t.Fatal("counter visual state was not reachable")
	}
	actual.States = append(actual.States, visualFixtureState("next-interaction", page, "__next", "next-hydration-marker", "next-count"))

	if _, err := engine.Navigate(context.Background(), server.URL+"/tailwind/"); err != nil {
		t.Fatal(err)
	}
	engine.UpdateViewport(1024, 720)
	actual.States = append(actual.States, visualFixtureState("tailwind-responsive", engine.Page(), "tailwind-root", "tailwind-card", "tailwind-grid"))
	engine.UpdateViewport(640, 720)
	actual.States = append(actual.States, visualFixtureState("tailwind-narrow", engine.Page(), "tailwind-root", "tailwind-card", "tailwind-grid"))

	if _, err := engine.Navigate(context.Background(), server.URL+"/diagnostics/"); err != nil {
		t.Fatal(err)
	}
	engine.UpdateViewport(1024, 720)
	waitForFixtureText(t, engine, mutations, "chunk-state", "chunk failure isolated")
	actual.States = append(actual.States, visualFixtureState("fallback-devtools", engine.Page(), "diagnostic-root", "chunk-state", "broken-image"))

	wantBytes, err := os.ReadFile("testdata/framework-visual.golden.json")
	if err != nil {
		encoded, _ := json.MarshalIndent(actual, "", "  ")
		t.Fatalf("read framework visual golden: %v\n--- actual ---\n%s", err, encoded)
	}
	var want frameworkVisualGolden
	if err := json.Unmarshal(wantBytes, &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(actual, want) {
		encoded, _ := json.MarshalIndent(actual, "", "  ")
		t.Fatalf("framework visual snapshot changed; inspect before updating golden\n--- actual ---\n%s", encoded)
	}
}

func visualFixtureState(name string, page *browser.Page, rootID string, textIDs ...string) string {
	tree := layoutmodel.BuildWithScrollAndResources(page.Document, page.ComputedStyles, page.ImageResources, page.WebFonts, page.ViewportWidth, page.ViewportHeight, 0, 0)
	display := paintmodel.Build(tree)
	root, _ := page.Document.GetElementByID(rootID)
	bounds := tree.Bounds[root.ID]
	texts := make([]string, 0, len(textIDs))
	for _, id := range textIDs {
		node, _ := page.Document.GetElementByID(id)
		computed, _ := page.ComputedStyles.For(node)
		texts = append(texts, fmt.Sprintf("%s=%q/display:%d/color:%08x/bg:%08x", id, node.TextContent(), computed.Display, computed.Color, computed.BackgroundColor))
	}
	reasons := make([]string, 0)
	for _, context := range page.RuntimeDiagnostics() {
		for _, diagnostic := range context.Diagnostics {
			if diagnostic.State == "fallback" || diagnostic.State == "error" {
				reasons = append(reasons, diagnostic.Category+":"+diagnostic.Reason)
			}
		}
	}
	sort.Strings(reasons)
	return fmt.Sprintf("%s engine=%s root=%.0f,%.0f %.0fx%.0f boxes=%d paint=%d fonts=%d images=%d revision=%d text=[%s] diagnostics=[%s]",
		name, page.Engine, bounds.X, bounds.Y, bounds.Width, bounds.Height, len(tree.Boxes), len(display.Commands), len(page.Fonts), len(page.ImageResources), page.StyleRevision,
		strings.Join(texts, ";"), strings.Join(reasons, ","))
}
