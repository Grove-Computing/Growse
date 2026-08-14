package dashboard

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/browser"
	"github.com/Grove-Computing/Growse/internal/dom"
	layoutmodel "github.com/Grove-Computing/Growse/internal/layout"
	"github.com/Grove-Computing/Growse/internal/network"
	paintmodel "github.com/Grove-Computing/Growse/internal/paint"
	"github.com/Grove-Computing/Growse/internal/style"
)

func TestDashboardDemoExercisesGridAndAdvancedPaint(t *testing.T) {
	server := httptest.NewServer(http.FileServer(http.Dir(".")))
	defer server.Close()
	browserState := browser.New(network.NewClient())
	defer browserState.Close()
	page, err := browserState.Navigate(context.Background(), server.URL+"/index.html")
	if err != nil {
		t.Fatal(err)
	}
	dashboard := elementByClass(t, page, "dashboard")
	sidebar := elementByClass(t, page, "sidebar")
	metric := elementByClass(t, page, "metric")
	release := elementByClass(t, page, "release")
	statusPanel := elementByClass(t, page, "status-panel")

	dashboardStyle, _ := page.ComputedStyles.For(dashboard)
	if dashboardStyle.Display != style.DisplayGrid || len(dashboardStyle.GridTemplateColumns) != 2 || len(dashboardStyle.GridTemplateAreas) != 4 {
		t.Fatalf("dashboard grid style = %#v", dashboardStyle)
	}
	sidebarStyle, _ := page.ComputedStyles.For(sidebar)
	if len(sidebarStyle.BackgroundLayers) != 2 || len(sidebarStyle.BoxShadows) != 1 || sidebarStyle.BorderRadius.TopLeft.X.Pixels != 18 {
		t.Fatalf("sidebar paint style = %#v", sidebarStyle)
	}
	releaseStyle, _ := page.ComputedStyles.For(release)
	statusStyle, _ := page.ComputedStyles.For(statusPanel)
	if len(releaseStyle.Transform) == 0 || statusStyle.Opacity != .96 || statusStyle.Position != style.PositionRelative {
		t.Fatalf("advanced effects = release:%#v status:%#v", releaseStyle, statusStyle)
	}

	tree := layoutmodel.BuildWithViewport(page.Document, page.ComputedStyles, 1040, 720)
	list := paintmodel.Build(tree)
	dashboardRect := decorationForNode(t, tree, dashboard.ID)
	sidebarRect := decorationForNode(t, tree, sidebar.ID)
	metricRect := decorationForNode(t, tree, metric.ID)
	if sidebarRect.Height < 250 || metricRect.X <= sidebarRect.X+sidebarRect.Width || dashboardRect.Width <= 800 {
		t.Fatalf("dashboard geometry = dashboard:%#v sidebar:%#v metric:%#v", dashboardRect.Rect, sidebarRect.Rect, metricRect.Rect)
	}
	var multipleBackgrounds, shadow, transformed, opacityLayer bool
	for _, command := range list.Commands {
		box, ok := command.(paintmodel.DrawBox)
		if !ok {
			continue
		}
		multipleBackgrounds = multipleBackgrounds || len(box.Layers) == 2
		shadow = shadow || len(box.BoxShadows) != 0
		transformed = transformed || box.Transform != (style.Matrix{}) && box.Transform != style.IdentityMatrix()
	}
	for _, layer := range list.CompositingLayers {
		opacityLayer = opacityLayer || layer.NodeID == statusPanel.ID && layer.Offscreen
	}
	if !multipleBackgrounds || !shadow || !transformed || !opacityLayer {
		t.Fatalf("advanced display list = backgrounds:%v shadow:%v transform:%v opacity:%v", multipleBackgrounds, shadow, transformed, opacityLayer)
	}

	for _, path := range []string{"index.html", "style.css"} {
		if info, err := os.Stat(path); err != nil || info.Size() == 0 {
			t.Fatalf("demo file %s is unavailable or empty: %v", path, err)
		}
	}
}

func elementByClass(t *testing.T, page *browser.Page, className string) *dom.Node {
	t.Helper()
	var found *dom.Node
	var walk func(*dom.Node)
	walk = func(node *dom.Node) {
		if node == nil || found != nil {
			return
		}
		if value, ok := node.Attribute("class"); ok {
			for _, candidate := range strings.Fields(value) {
				if candidate == className {
					found = node
					return
				}
			}
		}
		for _, child := range node.Children {
			walk(child)
		}
	}
	walk(page.Document.Root)
	if found == nil {
		t.Fatalf(".%s was not found", className)
	}
	return found
}

func decorationForNode(t *testing.T, tree *layoutmodel.Tree, nodeID dom.NodeID) layoutmodel.Decoration {
	t.Helper()
	for _, decoration := range tree.Decorations {
		if decoration.NodeID == nodeID {
			return decoration
		}
	}
	t.Fatalf("decoration for node %d was not found", nodeID)
	return layoutmodel.Decoration{}
}
