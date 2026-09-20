package realsitecompat

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/browser"
	"github.com/Grove-Computing/Growse/internal/dom"
	layoutmodel "github.com/Grove-Computing/Growse/internal/layout"
	"github.com/Grove-Computing/Growse/internal/network"
)

func TestSaku0512LiveCSSKeepsProfileAndLaterSectionsVisible(t *testing.T) {
	for _, width := range []float32{1280, 720} {
		t.Run(viewportName(width), func(t *testing.T) {
			page, tree := loadCrossSiteFixture(t, "fixtures/saku0512.html", width, 800)
			navigation := regionBounds(t, page.Document, tree, "navigation")
			profile := regionBounds(t, page.Document, tree, "profile")
			achievements := regionBounds(t, page.Document, tree, "achievements")
			later := regionBounds(t, page.Document, tree, "later")
			if navigation.Height <= 0 || navigation.Height > 160 || profile.Width <= 0 || achievements.Width <= 0 {
				t.Fatalf("missing or oversized header/profile regions: nav=%#v profile=%#v achievements=%#v", navigation, profile, achievements)
			}
			if width > 720 {
				if achievements.X <= profile.X+profile.Width || abs32(achievements.Y-profile.Y) > 1 {
					t.Fatalf("desktop profile grid is not two columns: profile=%#v achievements=%#v", profile, achievements)
				}
			} else if achievements.Y < profile.Y+profile.Height-1 {
				t.Fatalf("narrow profile cards overlap: profile=%#v achievements=%#v", profile, achievements)
			}
			if later.Y < max(profile.Y+profile.Height, achievements.Y+achievements.Height)-1 {
				t.Fatalf("later section overlaps profile grid: later=%#v", later)
			}
			assertRegionTextWithinViewport(t, page.Document, tree, width, "navigation", "profile", "achievements", "later")
		})
	}
}

func TestWikipediaLiveCSSKeepsHeaderWelcomeAndColumnsVisible(t *testing.T) {
	for _, width := range []float32{1280, 720} {
		t.Run(viewportName(width), func(t *testing.T) {
			page, tree := loadCrossSiteFixture(t, "fixtures/wikipedia-ja.html", width, 800)
			header := regionBounds(t, page.Document, tree, "header")
			heading := regionBounds(t, page.Document, tree, "heading")
			article := regionBounds(t, page.Document, tree, "article")
			picture := regionBounds(t, page.Document, tree, "image")
			if header.Height <= 0 || header.Height > 140 || heading.Y > 260 {
				t.Fatalf("Wikipedia header/welcome escaped initial viewport: header=%#v heading=%#v", header, heading)
			}
			if width > 720 {
				if picture.X <= article.X+article.Width || abs32(picture.Y-article.Y) > 1 {
					t.Fatalf("desktop Wikipedia columns are not adjacent: article=%#v image=%#v", article, picture)
				}
			} else if picture.Y < article.Y+article.Height-1 {
				t.Fatalf("narrow Wikipedia columns overlap: article=%#v image=%#v", article, picture)
			}
			assertRegionTextWithinViewport(t, page.Document, tree, width, "header", "navigation", "heading", "article", "image")
		})
	}
}

func loadCrossSiteFixture(t *testing.T, fixture string, width, height float32) (*browser.Page, *layoutmodel.Tree) {
	t.Helper()
	server := httptest.NewServer(http.FileServer(http.Dir(".")))
	t.Cleanup(server.Close)
	engine := browser.New(network.NewClientWithLimits(server.Client(), 8<<20))
	t.Cleanup(func() { _ = engine.Close() })
	if _, err := engine.Navigate(context.Background(), server.URL+"/"+fixture); err != nil {
		t.Fatal(err)
	}
	engine.UpdateViewport(width, height)
	page := engine.Page()
	return page, layoutmodel.BuildWithScrollAndResources(page.Document, page.ComputedStyles, page.ImageResources, page.WebFonts, width, height, 0, 0)
}

func regionBounds(t *testing.T, document *dom.Document, tree *layoutmodel.Tree, name string) layoutmodel.Rect {
	t.Helper()
	node := regionNode(document, name)
	if node == nil {
		t.Fatalf("region %q was not found", name)
	}
	bounds, ok := tree.Bounds[node.ID]
	if !ok || bounds.Width <= 0 || bounds.Height <= 0 {
		t.Fatalf("region %q has no visible bounds: %#v", name, bounds)
	}
	return bounds
}

func regionNode(document *dom.Document, name string) *dom.Node {
	var found *dom.Node
	var walk func(*dom.Node)
	walk = func(node *dom.Node) {
		if node == nil || found != nil {
			return
		}
		if value, ok := node.Attribute("data-region"); ok && value == name {
			found = node
			return
		}
		for _, child := range node.Children {
			walk(child)
		}
	}
	walk(document.Root)
	return found
}

func assertRegionTextWithinViewport(t *testing.T, document *dom.Document, tree *layoutmodel.Tree, width float32, names ...string) {
	t.Helper()
	for _, name := range names {
		node := regionNode(document, name)
		for _, box := range tree.Boxes {
			if nodeContains(document, node, box.NodeID) && strings.TrimSpace(box.Text) != "" && (box.X < -1 || box.X+box.Width > width+1) {
				t.Fatalf("%s text %q escapes %.0fpx viewport: %#v", name, box.Text, width, box)
			}
		}
	}
}

func viewportName(width float32) string {
	if width > 720 {
		return "desktop"
	}
	return "narrow"
}

func abs32(value float32) float32 {
	if value < 0 {
		return -value
	}
	return value
}
