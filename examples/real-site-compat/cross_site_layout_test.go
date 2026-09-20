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
			header := regionBounds(t, page.Document, tree, "header")
			brandIcon := regionBounds(t, page.Document, tree, "brand-icon")
			navigation := regionBounds(t, page.Document, tree, "navigation")
			profile := regionBounds(t, page.Document, tree, "profile")
			achievements := regionBounds(t, page.Document, tree, "achievements")
			later := regionBounds(t, page.Document, tree, "later")
			if header.Height <= 0 || header.Height > 160 || navigation.Height <= 0 || navigation.Height > 160 || profile.Width <= 0 || achievements.Width <= 0 {
				t.Fatalf("missing or oversized header/profile regions: nav=%#v profile=%#v achievements=%#v", navigation, profile, achievements)
			}
			if brandIcon.Width != 32 || brandIcon.Height != 32 {
				t.Fatalf("loaded intrinsic brand icon ignored CSS size: %#v", brandIcon)
			}
			if width > 720 {
				if abs32(navigation.Y-brandIcon.Y) > 16 {
					t.Fatalf("desktop header navigation wrapped below brand: icon=%#v nav=%#v", brandIcon, navigation)
				}
				if achievements.X <= profile.X+profile.Width || abs32(achievements.Y-profile.Y) > 1 {
					t.Fatalf("desktop profile grid is not two columns: profile=%#v achievements=%#v", profile, achievements)
				}
			} else if achievements.Y < profile.Y+profile.Height-1 {
				t.Fatalf("narrow profile cards overlap: profile=%#v achievements=%#v", profile, achievements)
			}
			if later.Y < max(profile.Y+profile.Height, achievements.Y+achievements.Height)-1 {
				t.Fatalf("later section overlaps profile grid: later=%#v", later)
			}
			assertRegionBorderAndRadius(t, page.Document, tree, "profile")
			assertRegionBorderAndRadius(t, page.Document, tree, "achievements")
			assertRegionTextWithinViewport(t, page.Document, tree, width, "navigation", "profile", "achievements", "later")
		})
	}
}

func TestWikipediaLiveCSSKeepsHeaderWelcomeAndColumnsVisible(t *testing.T) {
	for _, width := range []float32{1280, 720} {
		t.Run(viewportName(width), func(t *testing.T) {
			page, tree := loadCrossSiteFixture(t, "fixtures/wikipedia-ja.html", width, 800)
			header := regionBounds(t, page.Document, tree, "header")
			logoIcon := regionBounds(t, page.Document, tree, "logo-icon")
			navigation := regionBounds(t, page.Document, tree, "navigation")
			heading := regionBounds(t, page.Document, tree, "heading")
			article := regionBounds(t, page.Document, tree, "article")
			picture := regionBounds(t, page.Document, tree, "image")
			if header.Height <= 0 || header.Height > 140 || heading.Y > 260 {
				t.Fatalf("Wikipedia header/welcome escaped initial viewport: header=%#v heading=%#v", header, heading)
			}
			if logoIcon.Width != 50 || logoIcon.Height != 50 {
				t.Fatalf("Wikipedia responsive logo icon is not visible at %.0fpx: %#v", width, logoIcon)
			}
			if gap := navigation.Y - (header.Y + header.Height); gap < 23 || gap > 25 {
				t.Fatalf("empty site notice margin was not collapsed once: gap=%v header=%#v nav=%#v", gap, header, navigation)
			}
			if width > 720 {
				if picture.X <= article.X+article.Width || abs32(picture.Y-article.Y) > 1 {
					t.Fatalf("desktop Wikipedia columns are not adjacent: article=%#v image=%#v", article, picture)
				}
			} else if picture.Y < article.Y+article.Height-1 {
				t.Fatalf("narrow Wikipedia columns overlap: article=%#v image=%#v", article, picture)
			}
			assertRegionBorderAndRadius(t, page.Document, tree, "article")
			assertRegionBorderAndRadius(t, page.Document, tree, "image")
			assertRegionTextWithinViewport(t, page.Document, tree, width, "header", "navigation", "heading", "article", "image")
		})
	}
}

func assertRegionBorderAndRadius(t *testing.T, document *dom.Document, tree *layoutmodel.Tree, name string) {
	t.Helper()
	node := regionNode(document, name)
	for _, decoration := range tree.Decorations {
		if decoration.NodeID != node.ID {
			continue
		}
		if decoration.Border.Top.Width <= 0 || decoration.Border.Right.Width <= 0 || decoration.Border.Bottom.Width <= 0 || decoration.Border.Left.Width <= 0 {
			t.Fatalf("region %q lost a card border: %#v", name, decoration.Border)
		}
		if decoration.Radius.TopLeft.X <= 0 || decoration.Radius.TopRight.X <= 0 || decoration.Radius.BottomRight.X <= 0 || decoration.Radius.BottomLeft.X <= 0 {
			t.Fatalf("region %q lost rounded corners: %#v", name, decoration.Radius)
		}
		return
	}
	t.Fatalf("region %q has no decoration", name)
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
