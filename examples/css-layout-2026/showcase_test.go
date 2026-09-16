package main

import (
	"context"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Grove-Computing/Growse/internal/browser"
	"github.com/Grove-Computing/Growse/internal/css"
	"github.com/Grove-Computing/Growse/internal/dom"
	htmlparser "github.com/Grove-Computing/Growse/internal/html"
	"github.com/Grove-Computing/Growse/internal/layout"
	"github.com/Grove-Computing/Growse/internal/network"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
	"github.com/Grove-Computing/Growse/internal/runtime/javascript"
	"github.com/Grove-Computing/Growse/internal/style"
)

func TestCSSLayoutShowcaseServesLayoutStagesAndLateImage(t *testing.T) {
	server := httptest.NewServer(cssLayoutHandler())
	defer server.Close()
	for _, route := range []struct {
		path, contentType, marker string
	}{
		{"/", "text/html", "Multi-column &amp; fragmentation"},
		{"/differential.html", "text/html", "css-layout-2026-desktop"},
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

func TestCSSLayoutShowcaseOperatesEveryMajorStageThroughJavaScriptReflow(t *testing.T) {
	server := httptest.NewServer(cssLayoutHandler())
	defer server.Close()
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
	if _, err := engine.Navigate(context.Background(), server.URL+"/"); err != nil {
		t.Fatal(err)
	}
	if !engine.UpdateViewport(1050, 700) {
		t.Fatal("showcase desktop viewport was rejected")
	}
	waitForShowcaseText(t, engine, mutations, "flow-status", "LEFT FLOAT · CLEAR BOTH")
	waitForShowcaseImage(t, engine, mutations)

	for _, operation := range []struct {
		toggle, status, want, stage string
	}{
		{toggle: "flow-toggle", status: "flow-status", want: "RIGHT FLOAT · FLOW ROOT", stage: "flow-stage"},
		{toggle: "table-toggle", status: "table-status", want: "COLLAPSED · AUTO", stage: "layout-table"},
		{toggle: "layout-toggle", status: "layout-status", want: "VERTICAL-LR · RTL · UNSAFE", stage: "layout-playground"},
		{toggle: "writing-toggle", status: "writing-status", want: "VERTICAL-LR · RTL", stage: "writing-playground"},
		{toggle: "position-toggle", status: "position-status", want: "SHIFTED · ROTATED · WIDER", stage: "position-playground"},
		{toggle: "column-toggle", status: "column-status", want: "2 COLUMNS · AUTO FILL", stage: "column-stage"},
	} {
		page := engine.Page()
		beforeRevision := page.StyleRevision
		if !engine.DispatchClick(showcaseNode(t, page, operation.toggle).ID, 0, 0) {
			t.Fatalf("%s did not handle click", operation.toggle)
		}
		waitForShowcaseText(t, engine, mutations, operation.status, operation.want)
		page = engine.Page()
		if page.StyleRevision <= beforeRevision {
			t.Fatalf("%s style revision = %d, want > %d", operation.toggle, page.StyleRevision, beforeRevision)
		}
		tree := layout.BuildWithScrollAndResources(page.Document, page.ComputedStyles, page.ImageResources, page.WebFonts, 1050, 700, 0, 0)
		stage := showcaseNode(t, page, operation.stage)
		bounds, exists := tree.Bounds[stage.ID]
		if !exists || bounds.Width <= 0 || bounds.Height <= 0 || len(tree.Fallbacks) != 0 {
			t.Fatalf("%s reflow geometry = %#v/%t fallbacks=%+v", operation.stage, bounds, exists, tree.Fallbacks)
		}
	}

	page := engine.Page()
	flowFloatStyle, _ := page.ComputedStyles.For(showcaseSelector(t, page, ".flow-float"))
	flowClearStyle, _ := page.ComputedStyles.For(showcaseSelector(t, page, ".flow-clear"))
	tableStyle, _ := page.ComputedStyles.For(showcaseNode(t, page, "layout-table"))
	logicalStyle, _ := page.ComputedStyles.For(showcaseSelector(t, page, ".logical-stage"))
	verticalStyle, _ := page.ComputedStyles.For(showcaseSelector(t, page, ".vertical-flow"))
	nestedScrollStyle, _ := page.ComputedStyles.For(showcaseSelector(t, page, ".nested-scroll"))
	columnStyle, _ := page.ComputedStyles.For(showcaseNode(t, page, "column-stage"))
	if flowFloatStyle.Float != style.FloatRight || flowClearStyle.Clear != style.ClearRight ||
		tableStyle.TableLayout != style.TableLayoutAuto || tableStyle.BorderCollapse != style.BorderCollapseCollapse ||
		logicalStyle.WritingMode != style.WritingModeVerticalLR || verticalStyle.WritingMode != style.WritingModeVerticalLR ||
		nestedScrollStyle.Width.Kind != style.SizeLength || nestedScrollStyle.Width.Value.Pixels != 340 ||
		columnStyle.ColumnCount != 2 || columnStyle.ColumnFill != style.ColumnFillAuto {
		t.Fatalf("showcase alternate computed styles = float:%v clear:%v table:%v/%v logical:%v vertical:%v nested:%#v columns:%d/%v",
			flowFloatStyle.Float, flowClearStyle.Clear, tableStyle.TableLayout, tableStyle.BorderCollapse,
			logicalStyle.WritingMode, verticalStyle.WritingMode, nestedScrollStyle.Width, columnStyle.ColumnCount, columnStyle.ColumnFill)
	}
}

func waitForShowcaseImage(t *testing.T, engine *browser.Browser, mutations <-chan struct{}) {
	t.Helper()
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	for {
		for _, resource := range engine.Page().ImageResources {
			if resource.Loaded {
				return
			}
		}
		select {
		case <-mutations:
		case <-deadline.C:
			t.Fatalf("late sizing image did not complete before the showcase gate: %+v", engine.Page().ImageResources)
		}
	}
}

func waitForShowcaseText(t *testing.T, engine *browser.Browser, mutations <-chan struct{}, id, want string) {
	t.Helper()
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	for {
		if page := engine.Page(); page != nil && showcaseNode(t, page, id).TextContent() == want {
			return
		}
		select {
		case <-mutations:
		case <-deadline.C:
			t.Fatalf("%s text did not become %q", id, want)
		}
	}
}

func showcaseNode(t *testing.T, page *browser.Page, id string) *dom.Node {
	t.Helper()
	node, exists := page.Document.GetElementByID(id)
	if !exists {
		t.Fatalf("showcase node #%s is missing", id)
	}
	return node
}

func showcaseSelector(t *testing.T, page *browser.Page, selector string) *dom.Node {
	t.Helper()
	node, exists := page.Document.QuerySelector(selector)
	if !exists {
		t.Fatalf("showcase selector %s is missing", selector)
	}
	return node
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
	cards := make([]*dom.Node, 0, 5)
	for _, child := range stage.Children {
		if child.Type != dom.NodeElement || child == spanner {
			continue
		}
		cards = append(cards, child)
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
	if len(cards) != 5 || tree.Bounds[cards[0].ID].X == tree.Bounds[cards[1].ID].X {
		t.Fatalf("balanced avoid cards did not use separate columns: %#v", cards)
	}
	for _, group := range [][]*dom.Node{cards[:2], cards[2:]} {
		for _, card := range group[1:] {
			if delta := tree.Bounds[card.ID].Y - tree.Bounds[group[0].ID].Y; delta < -0.01 || delta > 0.01 {
				t.Fatalf("balanced cards do not share a fragmentainer start: first=%#v card=%#v", tree.Bounds[group[0].ID], tree.Bounds[card.ID])
			}
		}
	}
	for _, card := range cards {
		bounds := tree.Bounds[card.ID]
		if bounds.Y+bounds.Height > stageBounds.Y+stageBounds.Height+0.01 {
			t.Fatalf("column card escaped stage block-size: stage=%#v card=%#v", stageBounds, bounds)
		}
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
	for _, box := range tree.Boxes {
		if box.NodeID != target.ID || !box.Button {
			continue
		}
		x, y := box.Transform.TransformPoint(targetBounds.X+targetBounds.Width/2, targetBounds.Y+targetBounds.Height/2)
		hit, hitOK := layout.HitTest(tree, x, y)
		hitNode, exists := document.NodeByID(hit)
		for hitNode != nil && hitNode != target {
			hitNode = hitNode.Parent
		}
		if !hitOK || !exists || hitNode != target {
			t.Fatalf("nested showcase transformed hit = %d/%v target=%d raw=%#v bounds=%#v box=%#v container=%#v", hit, hitOK, target.ID, func() *dom.Node { node, _ := document.NodeByID(hit); return node }(), targetBounds, box, tree.ScrollContainers[scroller.ID])
		}
		return
	}
	t.Fatal("nested showcase native button box is missing")
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
