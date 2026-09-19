package realsitecompat

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Grove-Computing/Growse/internal/browser"
	"github.com/Grove-Computing/Growse/internal/dom"
	layoutmodel "github.com/Grove-Computing/Growse/internal/layout"
	"github.com/Grove-Computing/Growse/internal/network"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
	"github.com/Grove-Computing/Growse/internal/runtime/javascript"
)

func TestCorpusDesktopAndNarrowLifecycleKeepsEveryRequiredRegionUsable(t *testing.T) {
	manifest := readCorpusManifest(t)
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
	for _, fixture := range manifest.Cases {
		for _, viewport := range manifest.Viewports {
			t.Run(fixture.ID+"-"+viewport.Name, func(t *testing.T) {
				page, err := engine.Navigate(context.Background(), server.URL+"/"+fixture.Fixture)
				if err != nil {
					t.Fatal(err)
				}
				engine.UpdateViewport(float32(viewport.Width), float32(viewport.Height))
				assertCorpusLifecycleState(t, engine, page, fixture.Regions, viewport.Width, viewport.Height)

				reloaded, err := engine.Reload(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				engine.UpdateViewport(float32(viewport.Width), float32(viewport.Height))
				assertCorpusLifecycleState(t, engine, reloaded, fixture.Regions, viewport.Width, viewport.Height)
			})
		}
	}
}

func assertCorpusLifecycleState(t *testing.T, engine *browser.Browser, page *browser.Page, required []string, width, height int) {
	t.Helper()
	if page.WebFonts == nil || len(page.FontErrors) != 0 || len(page.ImageErrors) != 0 {
		t.Fatalf("resource completion state = webFonts:%p fontErrors:%#v imageErrors:%#v", page.WebFonts, page.FontErrors, page.ImageErrors)
	}
	anchor := firstElement(page.Document.Root, "a")
	if anchor == nil {
		t.Fatal("fixture has no focusable link")
	}
	if destination, _, ok := page.LinkDestination(anchor.ID); !ok || destination == nil {
		t.Fatalf("link destination was not resolved for node %d", anchor.ID)
	}
	if !engine.UpdateFocus(anchor.ID) || engine.Page().FocusTarget != anchor.ID {
		t.Fatalf("link focus was not applied: target=%d want=%d", engine.Page().FocusTarget, anchor.ID)
	}
	if !engine.UpdateFocus(0) || engine.Page().FocusTarget != 0 {
		t.Fatal("link focus was not cleared")
	}
	for _, scrollY := range []float32{0, 48} {
		tree := layoutmodel.BuildWithScrollAndResources(page.Document, page.ComputedStyles, page.ImageResources, page.WebFonts, float32(width), float32(height), 0, scrollY)
		if tree.ScrollY != scrollY {
			t.Fatalf("scroll layout offset = %v, want %v", tree.ScrollY, scrollY)
		}
		_, issues := analyzeRegions(page.Document, tree, required)
		if len(issues) != 0 {
			for index, box := range tree.Boxes {
				t.Logf("box[%d]=node:%d tag:%s text:%q rect:%v,%v %vx%v", index, box.NodeID, box.Tag, box.Text, box.X, box.Y, box.Width, box.Height)
			}
			t.Fatalf("lifecycle state has release blockers at scroll %v: %#v", scrollY, issues)
		}
	}
}

func firstElement(node *dom.Node, tag string) *dom.Node {
	if node == nil {
		return nil
	}
	if node.Type == dom.NodeElement && node.TagName == tag {
		return node
	}
	for _, child := range node.Children {
		if found := firstElement(child, tag); found != nil {
			return found
		}
	}
	return nil
}
