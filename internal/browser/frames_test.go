package browser

import (
	"context"
	"testing"

	"github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/layout"
	"github.com/Grove-Computing/Growse/internal/network"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
)

func TestIframeLoadsSrcSrcdocNestedDocumentsAndIndependentRuntimes(t *testing.T) {
	parentURL := mustParseURL(t, "https://page.example/app/index.html")
	childURL := mustParseURL(t, "https://child.example/frame.html")
	nestedURL := mustParseURL(t, "https://child.example/nested.html")
	navigatedURL := mustParseURL(t, "https://child.example/next.html")
	loader := &routeLoader{responses: map[string]*network.Response{
		parentURL.String(): {
			URL: parentURL, StatusCode: 200, ContentType: "text/html",
			Body: []byte(`<script>parentLoaded = true</script>
				<iframe id="external" src="https://child.example/frame.html" width="420" height="220"></iframe>
				<iframe id="inline" srcdoc="<p id='srcdoc-result'>srcdoc</p><script>inlineLoaded = true</script>"></iframe>`),
		},
		childURL.String(): {
			URL: childURL, StatusCode: 200, ContentType: "text/html",
			Body: []byte(`<p id="child-result">child</p><script>childLoaded = true</script><iframe src="nested.html"></iframe>`),
		},
		nestedURL.String(): {
			URL: nestedURL, StatusCode: 200, ContentType: "text/html",
			Body: []byte(`<p id="nested-result">nested</p><script>nestedLoaded = true</script>`),
		},
		navigatedURL.String(): {
			URL: navigatedURL, StatusCode: 200, ContentType: "text/html",
			Body: []byte(`<p id="next-result">navigated</p><script>navigated = true</script>`),
		},
	}}
	var runtimes []*runtimeStub
	browserState := NewWithEngineFactory(loader, func(runtimemodel.Engine) runtimemodel.Runtime {
		runtime := &runtimeStub{}
		runtimes = append(runtimes, runtime)
		return runtime
	})
	if _, err := browserState.SetEngine(context.Background(), runtimemodel.EngineJavaScript); err != nil {
		t.Fatal(err)
	}
	page, err := browserState.Navigate(context.Background(), parentURL.String())
	if err != nil {
		t.Fatalf("Navigate() error = %v", err)
	}
	if len(page.Frames) != 2 || len(runtimes) != 4 {
		t.Fatalf("top-level Frames / runtimes = %d / %d, want 2 / 4", len(page.Frames), len(runtimes))
	}
	externalElement, _ := page.Document.GetElementByID("external")
	external, ok := page.FrameByElement(externalElement.ID)
	if !ok || !external.Loaded || external.Page == nil || external.URL.String() != childURL.String() || len(external.Page.Frames) != 1 {
		t.Fatalf("external Frame = %#v", external)
	}
	childResult, _ := external.Page.Document.GetElementByID("child-result")
	if childResult.TextContent() != "child" || external.Page.Frames[0].URL.String() != nestedURL.String() {
		t.Fatalf("child document = %q nested=%v", childResult.TextContent(), external.Page.Frames[0].URL)
	}
	nested := external.Page.Frames[0]
	inlineElement, _ := page.Document.GetElementByID("inline")
	inline, ok := page.FrameByElement(inlineElement.ID)
	if !ok || !inline.Loaded || inline.URL.String() != parentURL.String() {
		t.Fatalf("srcdoc Frame = %#v", inline)
	}
	srcdocResult, _ := inline.Page.Document.GetElementByID("srcdoc-result")
	if srcdocResult.TextContent() != "srcdoc" {
		t.Fatalf("srcdoc text = %q", srcdocResult.TextContent())
	}
	if external.Viewport.Width != 420 || external.Viewport.Height != 220 || external.Viewport.ClipWidth != 420 || external.Viewport.ClipHeight != 220 {
		t.Fatalf("external viewport = %#v", external.Viewport)
	}
	parentLayout := layout.Build(page.Document, page.ComputedStyles, 800)
	page.SyncFrameViewports(parentLayout)
	if external.Viewport.Width != 420 || external.Viewport.Height != 220 || external.Viewport.ClipWidth != 420 || external.Viewport.ClipHeight != 220 {
		t.Fatalf("laid out external viewport = %#v", external.Viewport)
	}

	page.SyncFrameViewports(&layout.Tree{Bounds: map[dom.NodeID]layout.Rect{
		external.ElementID: {X: 12, Y: 24, Width: 360, Height: 180},
	}})
	if external.Viewport.X != 12 || external.Viewport.Y != 24 || external.Viewport.ClipWidth != 360 || external.Viewport.ClipHeight != 180 {
		t.Fatalf("synced external viewport = %#v", external.Viewport)
	}
	external.SetScroll(14, 28)
	if external.ScrollX != 14 || external.ScrollY != 28 {
		t.Fatalf("Frame scroll = %v,%v", external.ScrollX, external.ScrollY)
	}
	oldRuntime := external.runtime.(*runtimeStub)
	if err := external.Navigate(context.Background(), "next.html"); err != nil {
		t.Fatalf("Frame.Navigate() error = %v", err)
	}
	nextResult, _ := external.Page.Document.GetElementByID("next-result")
	if external.URL.String() != navigatedURL.String() || nextResult.TextContent() != "navigated" || external.Generation != 2 || oldRuntime.stopCalls.Load() != 1 || !nested.Closed {
		t.Fatalf("Frame navigation = URL:%v text:%q generation:%d oldStop:%d nestedClosed:%t", external.URL, nextResult.TextContent(), external.Generation, oldRuntime.stopCalls.Load(), nested.Closed)
	}
	navigatedRuntime := external.runtime.(*runtimeStub)
	if err := external.Reload(context.Background()); err != nil {
		t.Fatalf("Frame.Reload() error = %v", err)
	}
	if external.Generation != 3 || navigatedRuntime.stopCalls.Load() != 1 {
		t.Fatalf("Frame reload generation=%d previous Stop=%d", external.Generation, navigatedRuntime.stopCalls.Load())
	}

	if err := browserState.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	for index, runtime := range runtimes {
		if runtime.stopCalls.Load() != 1 {
			t.Fatalf("Runtime %d Stop() calls = %d, want 1", index, runtime.stopCalls.Load())
		}
	}
	if !external.Closed || !inline.Closed || !nested.Closed {
		t.Fatal("Browser Close did not recursively close Frames")
	}
}

func TestIframeFailureDoesNotFailParentPage(t *testing.T) {
	parentURL := mustParseURL(t, "https://page.example/index.html")
	loader := &routeLoader{responses: map[string]*network.Response{
		parentURL.String(): {
			URL: parentURL, StatusCode: 200, ContentType: "text/html",
			Body: []byte(`<p id="parent">visible</p><iframe src="https://missing.example/frame.html"></iframe>`),
		},
	}}
	browserState := New(loader)
	t.Cleanup(func() { _ = browserState.Close() })
	page, err := browserState.Navigate(context.Background(), parentURL.String())
	if err != nil {
		t.Fatalf("parent Navigate() error = %v", err)
	}
	parent, _ := page.Document.GetElementByID("parent")
	if parent.TextContent() != "visible" || len(page.Frames) != 1 || page.Frames[0].LoadError == "" {
		t.Fatalf("parent / failed Frame = %q / %#v", parent.TextContent(), page.Frames)
	}
}
