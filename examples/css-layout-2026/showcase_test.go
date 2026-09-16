package main

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/css"
	"github.com/Grove-Computing/Growse/internal/dom"
	htmlparser "github.com/Grove-Computing/Growse/internal/html"
	"github.com/Grove-Computing/Growse/internal/layout"
	"github.com/Grove-Computing/Growse/internal/style"
)

func TestCSSLayoutShowcaseServesLayoutStagesAndLateImage(t *testing.T) {
	server := httptest.NewServer(cssLayoutHandler())
	defer server.Close()
	for _, route := range []struct {
		path, contentType, marker string
	}{
		{"/", "text/html", "Multi-column &amp; fragmentation"},
		{"/style.css", "text/css", ".column-stage"},
		{"/app.mjs", "text/javascript", "2 COLUMNS · AUTO FILL"},
		{"/assets/late-layout.png", "image/png", ""},
	} {
		response, err := server.Client().Get(server.URL + route.path)
		if err != nil {
			t.Fatal(err)
		}
		body, readErr := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if readErr != nil || response.StatusCode != 200 || !strings.Contains(response.Header.Get("Content-Type"), route.contentType) ||
			route.marker != "" && !strings.Contains(string(body), route.marker) {
			t.Fatalf("GET %s = status:%d type:%q marker:%t err:%v", route.path, response.StatusCode, response.Header.Get("Content-Type"), strings.Contains(string(body), route.marker), readErr)
		}
	}
}

func TestCSSLayoutShowcaseBalancesColumnsAndSpans(t *testing.T) {
	htmlSource, err := cssLayoutAssets.ReadFile("index.html")
	if err != nil {
		t.Fatal(err)
	}
	cssSource, err := cssLayoutAssets.ReadFile("style.css")
	if err != nil {
		t.Fatal(err)
	}
	document, err := htmlparser.Parse(strings.NewReader(string(htmlSource)))
	if err != nil {
		t.Fatal(err)
	}
	stylesheet, err := css.Parse(strings.NewReader(string(cssSource)))
	if err != nil {
		t.Fatal(err)
	}
	stage, ok := document.QuerySelector("#column-stage")
	if !ok {
		t.Fatal("multi-column stage is missing")
	}
	spanner, ok := document.QuerySelector(".column-spanner")
	if !ok {
		t.Fatal("column spanner is missing")
	}
	computed := style.ComputeWithEnvironment(document, stylesheet, style.InteractionState{}, style.Environment{ViewportWidth: 1057, ViewportHeight: 700, RootFontSize: 16, ResolutionDPI: 96})
	tree := layout.BuildWithScrollAndResources(document, computed, nil, layout.NewFontSetWithSystemFallback(nil), 1057, 700, 0, 0)
	stageBounds, spanBounds := tree.Bounds[stage.ID], tree.Bounds[spanner.ID]
	columns := make(map[int]bool)
	for _, child := range stage.Children {
		if child.Type != dom.NodeElement || child == spanner {
			continue
		}
		if bounds, exists := tree.Bounds[child.ID]; exists {
			columns[int(bounds.X+0.5)] = true
		}
		fragments := 0
		for _, decoration := range tree.Decorations {
			if decoration.NodeID == child.ID {
				fragments++
			}
		}
		if fragments != 1 {
			t.Fatalf("break-inside card %d produced %d decoration fragments, bounds=%#v", child.ID, fragments, tree.Bounds[child.ID])
		}
		descendants := make(map[dom.NodeID]bool)
		var collectDescendants func(*dom.Node)
		collectDescendants = func(node *dom.Node) {
			descendants[node.ID] = true
			for _, descendant := range node.Children {
				collectDescendants(descendant)
			}
		}
		collectDescendants(child)
		cardBounds := tree.Bounds[child.ID]
		for _, box := range tree.Boxes {
			if !descendants[box.NodeID] {
				continue
			}
			if box.X < cardBounds.X-0.01 || box.X+box.Width > cardBounds.X+cardBounds.Width+0.01 {
				t.Fatalf("break-inside card %d content escaped its fragment: card=%#v box=%#v", child.ID, cardBounds, box)
			}
		}
	}
	if len(columns) < 3 || spanBounds.Width != stageBounds.Width-28 {
		t.Fatalf("showcase columns = positions:%v stage:%#v span:%#v", columns, stageBounds, spanBounds)
	}
}

func TestCSSLayoutShowcaseNestedScrollSharesTransformClipAndHitGeometry(t *testing.T) {
	htmlSource, err := cssLayoutAssets.ReadFile("index.html")
	if err != nil {
		t.Fatal(err)
	}
	cssSource, err := cssLayoutAssets.ReadFile("style.css")
	if err != nil {
		t.Fatal(err)
	}
	document, err := htmlparser.Parse(strings.NewReader(string(htmlSource)))
	if err != nil {
		t.Fatal(err)
	}
	stylesheet, err := css.Parse(strings.NewReader(string(cssSource)))
	if err != nil {
		t.Fatal(err)
	}
	scroller, ok := document.QuerySelector(".nested-scroll")
	if !ok {
		t.Fatal("nested scroll fixture is missing")
	}
	target, ok := document.QuerySelector(".scroll-target")
	if !ok {
		t.Fatal("nested scroll target is missing")
	}
	computed := style.Compute(document, stylesheet)
	tree := layout.BuildWithViewport(document, computed, 1050, 700)
	container, ok := tree.ScrollContainers[scroller.ID]
	if !ok || container.ScrollWidth <= container.Viewport.Width || container.ScrollHeight <= container.Viewport.Height {
		t.Fatalf("nested showcase scroll geometry = %#v exists:%v", container, ok)
	}
	if dirty := layout.ApplyScrollContainerOffset(tree, computed, scroller.ID, 120, 80); len(dirty) == 0 {
		t.Fatal("nested showcase did not scroll")
	}
	targetBounds := tree.Bounds[target.ID]
	for _, decoration := range tree.Decorations {
		if decoration.NodeID != target.ID {
			continue
		}
		x, y := decoration.Transform.TransformPoint(targetBounds.X+targetBounds.Width/2, targetBounds.Y+targetBounds.Height/2)
		hit, hitOK := layout.HitTest(tree, x, y)
		hitNode, exists := document.NodeByID(hit)
		for hitNode != nil && hitNode != target {
			hitNode = hitNode.Parent
		}
		if !hitOK || !exists || hitNode != target {
			t.Fatalf("nested showcase transformed hit = %d/%v target=%d raw=%#v bounds=%#v decoration=%#v container=%#v", hit, hitOK, target.ID, func() *dom.Node { node, _ := document.NodeByID(hit); return node }(), targetBounds, decoration, tree.ScrollContainers[scroller.ID])
		}
		return
	}
	t.Fatal("nested showcase target decoration is missing")
}

func TestCSSLayoutShowcaseUsesVerticalGlyphAndLogicalAxes(t *testing.T) {
	htmlSource, err := cssLayoutAssets.ReadFile("index.html")
	if err != nil {
		t.Fatal(err)
	}
	cssSource, err := cssLayoutAssets.ReadFile("style.css")
	if err != nil {
		t.Fatal(err)
	}
	document, err := htmlparser.Parse(strings.NewReader(string(htmlSource)))
	if err != nil {
		t.Fatal(err)
	}
	stylesheet, err := css.Parse(strings.NewReader(string(cssSource)))
	if err != nil {
		t.Fatal(err)
	}
	flex, ok := document.QuerySelector(".vertical-flex")
	if !ok || len(flex.Children) == 0 {
		t.Fatal("vertical flex fixture is missing")
	}
	var firstItem = flex.Children[0]
	for _, child := range flex.Children {
		if child.TagName == "span" {
			firstItem = child
			break
		}
	}
	imageNode, ok := document.QuerySelector(".vertical-image")
	if !ok {
		t.Fatal("vertical image fixture is missing")
	}
	images := map[dom.NodeID]layout.ImageResource{
		imageNode.ID: {URL: "/assets/late-layout.png", IntrinsicWidth: 240, IntrinsicHeight: 120, Loaded: true},
	}
	tree := layout.BuildWithScrollAndImages(document, style.Compute(document, stylesheet), images, 1050, 700, 0, 0)
	for _, box := range tree.Boxes {
		if box.NodeID != firstItem.ID || len(box.Runs) < 2 {
			continue
		}
		if box.WritingMode != style.WritingModeVerticalRL || box.Runs[0].OffsetX != box.Runs[1].OffsetX || box.Runs[1].OffsetY <= box.Runs[0].OffsetY {
			t.Fatalf("vertical flex showcase glyph geometry = %#v", box.Runs)
		}
		return
	}
	t.Fatal("vertical flex showcase text box is missing")
}
