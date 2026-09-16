package main

import (
	"context"
	"math"
	"net/http/httptest"
	"testing"

	"github.com/Grove-Computing/Growse/internal/browser"
	"github.com/Grove-Computing/Growse/internal/dom"
	layoutmodel "github.com/Grove-Computing/Growse/internal/layout"
	"github.com/Grove-Computing/Growse/internal/network"
	paintmodel "github.com/Grove-Computing/Growse/internal/paint"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
	"github.com/Grove-Computing/Growse/internal/runtime/javascript"
)

const frameworkScrollViewportHeight = float32(32)

func TestV019FrameworkCorpusDesktopNarrowHydrationInteractionAndScroll(t *testing.T) {
	server := httptest.NewServer(modernWebCompatibilityHandler())
	defer server.Close()

	for _, fixture := range []struct {
		name, path, rootID, markerID, targetID string
		interact                               func(*testing.T, *browser.Browser, *browser.Page)
	}{
		{
			name: "nextjs", path: "/next/", rootID: "__next", markerID: "next-hydration-marker", targetID: "next-counter",
			interact: func(t *testing.T, engine *browser.Browser, page *browser.Page) {
				if !engine.DispatchClick(fixtureNode(t, page, "next-counter").ID, 0, 0) || fixtureNode(t, page, "next-count").TextContent() != "1" {
					t.Fatal("Next.js hydrated interaction failed")
				}
			},
		},
		{
			name: "sveltekit", path: "/svelte/", rootID: "svelte", markerID: "svelte-hydration-marker", targetID: "svelte-reactive",
			interact: func(t *testing.T, engine *browser.Browser, page *browser.Page) {
				if !engine.DispatchClick(fixtureNode(t, page, "svelte-reactive").ID, 0, 0) || fixtureNode(t, page, "svelte-state").TextContent() != "reactive:1" {
					t.Fatal("SvelteKit hydrated interaction failed")
				}
			},
		},
	} {
		t.Run(fixture.name, func(t *testing.T) {
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
			page, err := engine.Navigate(context.Background(), server.URL+fixture.path)
			if err != nil {
				t.Fatal(err)
			}
			rootID := fixtureNode(t, page, fixture.rootID).ID
			waitForFixtureText(t, engine, mutations, fixture.markerID, "hydrated")
			page = engine.Page()
			if fixtureNode(t, page, fixture.rootID).ID != rootID {
				t.Fatal("hydration replaced the SSR root")
			}
			fixture.interact(t, engine, page)
			for _, viewport := range []struct {
				name  string
				width float32
			}{{name: "desktop", width: 1024}, {name: "narrow", width: 640}} {
				t.Run(viewport.name, func(t *testing.T) {
					if !engine.UpdateViewport(viewport.width, frameworkScrollViewportHeight) {
						t.Fatal("viewport update was rejected")
					}
					assertFrameworkLayoutState(t, engine.Page(), fixture.rootID, fixture.targetID, viewport.width)
				})
			}
			if page.RuntimeError != "" || len(page.ScriptErrors) != 0 {
				t.Fatalf("framework runtime errors = %q / %v", page.RuntimeError, page.ScriptErrors)
			}
		})
	}

	t.Run("tailwind-css", func(t *testing.T) {
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
		if _, err := engine.Navigate(context.Background(), server.URL+"/tailwind/"); err != nil {
			t.Fatal(err)
		}
		for _, viewport := range []struct {
			name  string
			width float32
		}{{name: "desktop", width: 1024}, {name: "narrow", width: 640}} {
			t.Run(viewport.name, func(t *testing.T) {
				if !engine.UpdateViewport(viewport.width, frameworkScrollViewportHeight) {
					t.Fatal("viewport update was rejected")
				}
				assertFrameworkLayoutState(t, engine.Page(), "tailwind-root", "tailwind-button", viewport.width)
			})
		}
	})
}

func assertFrameworkLayoutState(t *testing.T, page *browser.Page, rootElementID, targetElementID string, viewportWidth float32) {
	t.Helper()
	tree := layoutmodel.BuildWithScrollAndResources(
		page.Document, page.ComputedStyles, page.ImageResources, page.WebFonts,
		viewportWidth, frameworkScrollViewportHeight, 0, 0,
	)
	root := fixtureNode(t, page, rootElementID)
	target := fixtureNode(t, page, targetElementID)
	rootBounds, rootExists := tree.Bounds[root.ID]
	targetBounds, targetExists := tree.Bounds[target.ID]
	if !rootExists || !targetExists || !finiteRect(rootBounds) || !finiteRect(targetBounds) || rootBounds.Width <= 0 || targetBounds.Width <= 0 || targetBounds.Height <= 0 {
		t.Fatalf("invalid framework geometry root=%#v/%t target=%#v/%t", rootBounds, rootExists, targetBounds, targetExists)
	}
	if tree.ScrollWidth > viewportWidth+2 || tree.ScrollHeight <= frameworkScrollViewportHeight {
		t.Fatalf("framework scroll extent = %.1fx%.1f viewport=%.1fx%.1f", tree.ScrollWidth, tree.ScrollHeight, viewportWidth, frameworkScrollViewportHeight)
	}
	if len(tree.Fallbacks) != 0 {
		t.Fatalf("framework layout used safety fallback: %+v", tree.Fallbacks)
	}
	nativeTarget := false
	for _, box := range tree.Boxes {
		if box.NodeID == target.ID && box.Button {
			nativeTarget = true
			break
		}
	}
	if target.TagName == "button" && !nativeTarget {
		t.Fatalf("button %d did not produce native control geometry", target.ID)
	}
	hit, ok := layoutmodel.HitTest(tree, targetBounds.X+targetBounds.Width/2, targetBounds.Y+targetBounds.Height/2)
	if !ok || !nodeIsOrDescendsFrom(page.Document, hit, target.ID) {
		t.Fatalf("framework pointer geometry hit=%d/%t target=%d bounds=%#v", hit, ok, target.ID, targetBounds)
	}
	beforeRoot := rootBounds
	beforeHeight := tree.ScrollHeight
	layoutmodel.ApplyScrollOffset(tree, page.ComputedStyles, 0, 24)
	if tree.ScrollY != 24 || tree.ScrollHeight != beforeHeight || tree.Bounds[root.ID] != beforeRoot {
		t.Fatalf("framework scroll changed normal-flow geometry: scroll=%v extent=%v root=%#v", tree.ScrollY, tree.ScrollHeight, tree.Bounds[root.ID])
	}
	if display := paintmodel.Build(tree); len(display.Commands) == 0 {
		t.Fatal("framework scroll produced an empty display list")
	}
}

func finiteRect(rect layoutmodel.Rect) bool {
	for _, value := range []float32{rect.X, rect.Y, rect.Width, rect.Height} {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return false
		}
	}
	return true
}

func nodeIsOrDescendsFrom(document *dom.Document, nodeID, ancestorID dom.NodeID) bool {
	node, exists := document.NodeByID(nodeID)
	if !exists {
		return false
	}
	for current := node; current != nil; current = current.Parent {
		if current.ID == ancestorID {
			return true
		}
	}
	return false
}
