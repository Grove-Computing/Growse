package flexbox

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/saku0512/growse/internal/browser"
	"github.com/saku0512/growse/internal/dom"
	layoutmodel "github.com/saku0512/growse/internal/layout"
	"github.com/saku0512/growse/internal/network"
	paintmodel "github.com/saku0512/growse/internal/paint"
	"github.com/saku0512/growse/internal/style"
)

func TestFlexboxDemoReachesLayoutPaintAndHitTesting(t *testing.T) {
	server := httptest.NewServer(http.FileServer(http.Dir(".")))
	defer server.Close()

	browserState := browser.New(network.NewClient())
	defer browserState.Close()
	page, err := browserState.Navigate(context.Background(), server.URL+"/index.html")
	if err != nil {
		t.Fatal(err)
	}
	toolbar := elementByClass(t, page, "toolbar")
	cards := elementByClass(t, page, "cards")
	featured := elementByClass(t, page, "featured")
	nested := elementByClass(t, page, "nested")

	toolbarStyle, _ := page.ComputedStyles.For(toolbar)
	if toolbarStyle.Display != style.DisplayFlex || toolbarStyle.AlignItems != style.AlignCenter || toolbarStyle.ColumnGap.Pixels != 12 {
		t.Fatalf("toolbar style = %#v", toolbarStyle)
	}
	cardsStyle, _ := page.ComputedStyles.For(cards)
	if cardsStyle.FlexWrap != style.FlexWrapLines || cardsStyle.RowGap.Pixels != 16 || cardsStyle.ColumnGap.Pixels != 16 {
		t.Fatalf("cards style = %#v", cardsStyle)
	}
	featuredStyle, _ := page.ComputedStyles.For(featured)
	if featuredStyle.FlexGrow != 2 || featuredStyle.FlexBasis.Kind != style.FlexBasisLength || featuredStyle.FlexBasis.Value.Pixels != 220 {
		t.Fatalf("featured style = %#v", featuredStyle)
	}

	tree := layoutmodel.BuildWithViewport(page.Document, page.ComputedStyles, 900, 640)
	list := paintmodel.Build(tree)
	featuredDecoration := decorationForNode(t, tree, featured.ID)
	nestedDecoration := decorationForNode(t, tree, nested.ID)
	if featuredDecoration.Width <= 220 || nestedDecoration.Width <= featuredDecoration.Width {
		t.Fatalf("demo flex geometry = featured %#v, nested %#v", featuredDecoration.Rect, nestedDecoration.Rect)
	}
	if hit, ok := layoutmodel.HitTest(tree, featuredDecoration.X+featuredDecoration.Width-5, featuredDecoration.Y+featuredDecoration.Height-5); !ok || hit != featured.ID {
		t.Fatalf("featured hit = (%d, %v), want %d", hit, ok, featured.ID)
	}
	if len(list.Commands) == 0 || list.ScrollWidth != tree.ScrollWidth {
		t.Fatalf("display list = commands:%d scroll:%v/%v", len(list.Commands), list.ScrollWidth, tree.ScrollWidth)
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
		if node.Type == dom.NodeElement {
			classes, _ := node.Attribute("class")
			for _, candidate := range strings.Fields(classes) {
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
