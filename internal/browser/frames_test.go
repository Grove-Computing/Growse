package browser

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/layout"
	"github.com/Grove-Computing/Growse/internal/network"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
	"github.com/Grove-Computing/Growse/internal/runtime/isolated"
)

type frameLifecycleLoader struct {
	responses map[string]*network.Response
	started   chan struct{}
}

func (loader *frameLifecycleLoader) Get(ctx context.Context, target *url.URL) (*network.Response, error) {
	if target.Path == "/slow.html" {
		select {
		case <-loader.started:
		default:
			close(loader.started)
		}
		<-ctx.Done()
		return nil, ctx.Err()
	}
	response, ok := loader.responses[target.String()]
	if !ok {
		return nil, errors.New("unexpected URL")
	}
	return response, nil
}

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

func TestIframeSandboxTokensFailClosedBeforeCapabilitiesRun(t *testing.T) {
	parentURL := mustParseURL(t, "https://page.example/index.html")
	deniedURL := mustParseURL(t, "https://page.example/denied.html")
	opaqueURL := mustParseURL(t, "https://page.example/opaque.html")
	popupDeniedURL := mustParseURL(t, "https://page.example/popup-denied.html")
	sameURL := mustParseURL(t, "https://page.example/same.html")
	blockedScriptURL := mustParseURL(t, "https://page.example/blocked.js")
	loader := &routeLoader{responses: map[string]*network.Response{
		parentURL.String(): {
			URL: parentURL, StatusCode: 200, ContentType: "text/html",
			Body: []byte(`<iframe id="denied" sandbox src="/denied.html"></iframe>
				<iframe id="opaque" sandbox="ALLOW-SCRIPTS allow-forms allow-popups allow-top-navigation-by-user-activation unknown-token" src="/opaque.html"></iframe>
				<iframe id="popup-denied" sandbox="allow-scripts" src="/popup-denied.html"></iframe>
				<iframe id="same" sandbox="allow-same-origin" src="/same.html"></iframe>`),
		},
		deniedURL.String(): {
			URL: deniedURL, StatusCode: 200, ContentType: "text/html",
			Body: []byte(`<script src="/blocked.js"></script>`),
		},
		opaqueURL.String(): {
			URL: opaqueURL, StatusCode: 200, ContentType: "text/html",
			Body: []byte(`<script>
				let storageError = "allowed";
				try { localStorage; } catch (error) { storageError = error.name; }
				console.log([origin, location.origin, storageError, open("/popup") === null].join("|"));
			</script>`),
		},
		popupDeniedURL.String(): {
			URL: popupDeniedURL, StatusCode: 200, ContentType: "text/html",
			Body: []byte(`<script>
				try { open("/popup"); console.log("allowed"); } catch (error) { console.log(error.name); }
			</script>`),
		},
		sameURL.String(): {
			URL: sameURL, StatusCode: 200, ContentType: "text/html", Body: []byte(`<p>same origin</p>`),
		},
	}}
	browserState := NewWithEngineFactory(loader, func(engine runtimemodel.Engine) runtimemodel.Runtime { return isolated.New(engine) })
	t.Cleanup(func() { _ = browserState.Close() })
	if _, err := browserState.SetEngine(context.Background(), runtimemodel.EngineJavaScript); err != nil {
		t.Fatal(err)
	}
	page, err := browserState.Navigate(context.Background(), parentURL.String())
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Frames) != 4 {
		t.Fatalf("Frames = %d, want 4", len(page.Frames))
	}
	denied, opaque, popupDenied, same := page.Frames[0], page.Frames[1], page.Frames[2], page.Frames[3]
	if len(denied.Page.Scripts) != 0 || denied.Page.RuntimeStarted {
		t.Fatalf("denied scripts = %d, started=%t", len(denied.Page.Scripts), denied.Page.RuntimeStarted)
	}
	for _, requested := range loader.requested {
		if requested == blockedScriptURL.String() {
			t.Fatal("sandboxed external script was fetched before rejection")
		}
	}
	records := opaque.Page.DevTools.Console()
	if len(records) != 1 || records[0].Message != "null|null|SecurityError|true" || !opaque.Page.RuntimeStarted {
		t.Fatalf("opaque runtime = records:%#v started:%t", records, opaque.Page.RuntimeStarted)
	}
	popupRecords := popupDenied.Page.DevTools.Console()
	if len(popupRecords) != 1 || popupRecords[0].Message != "SecurityError" {
		t.Fatalf("sandboxed popup Console() = %#v", popupRecords)
	}
	if err := denied.AuthorizeFormSubmission(); err == nil {
		t.Fatal("empty sandbox allowed form submission")
	}
	if err := denied.AuthorizePopup(); err == nil {
		t.Fatal("empty sandbox allowed popup")
	}
	if err := denied.AuthorizeTopNavigation(true); err == nil {
		t.Fatal("empty sandbox allowed top navigation")
	}
	if err := opaque.AuthorizeFormSubmission(); err != nil {
		t.Fatalf("allow-forms rejected: %v", err)
	}
	if err := opaque.AuthorizePopup(); err != nil {
		t.Fatalf("allow-popups rejected: %v", err)
	}
	if err := opaque.AuthorizeTopNavigation(false); err == nil {
		t.Fatal("top navigation without user activation was allowed")
	}
	if err := opaque.AuthorizeTopNavigation(true); err != nil {
		t.Fatalf("activated top navigation rejected: %v", err)
	}
	access := runtimeFrameAccess(page)
	if access[0].SameOrigin || access[1].SameOrigin || access[2].SameOrigin || !access[3].SameOrigin || same.Page.FramePolicy.HasOpaqueOrigin() {
		t.Fatalf("sandbox Origin access = %#v", access)
	}
}

func TestIframeDepthCountDocumentNavigationAndCallbackLimits(t *testing.T) {
	countURL := mustParseURL(t, "https://limits.example/count.html")
	countHTML := strings.Repeat(`<iframe srcdoc="<p>frame</p>"></iframe>`, maxFramesPerPage+8)
	countBrowser := New(&routeLoader{responses: map[string]*network.Response{
		countURL.String(): {URL: countURL, StatusCode: 200, ContentType: "text/html", Body: []byte(countHTML)},
	}})
	countPage, err := countBrowser.Navigate(context.Background(), countURL.String())
	if err != nil {
		t.Fatal(err)
	}
	if len(countPage.Frames) != maxFramesPerPage {
		t.Fatalf("Frame count = %d, want %d", len(countPage.Frames), maxFramesPerPage)
	}
	if err := countBrowser.Close(); err != nil {
		t.Fatal(err)
	}

	depthURL := mustParseURL(t, "https://limits.example/root.html")
	responses := map[string]*network.Response{
		depthURL.String(): {URL: depthURL, StatusCode: 200, ContentType: "text/html", Body: []byte(`<iframe src="/depth-1.html"></iframe>`)},
	}
	for depth := 1; depth <= maxFrameDepth; depth++ {
		current := mustParseURL(t, fmt.Sprintf("https://limits.example/depth-%d.html", depth))
		body := `<p>last</p>`
		if depth <= maxFrameDepth {
			body = fmt.Sprintf(`<iframe src="/depth-%d.html"></iframe>`, depth+1)
		}
		responses[current.String()] = &network.Response{URL: current, StatusCode: 200, ContentType: "text/html", Body: []byte(body)}
	}
	depthBrowser := New(&routeLoader{responses: responses})
	depthPage, err := depthBrowser.Navigate(context.Background(), depthURL.String())
	if err != nil {
		t.Fatal(err)
	}
	deepest := depthPage.Frames[0]
	for deepest.Depth <= maxFrameDepth && deepest.Page != nil && len(deepest.Page.Frames) != 0 {
		deepest = deepest.Page.Frames[0]
	}
	if deepest.Depth != maxFrameDepth+1 || deepest.Loaded || !strings.Contains(deepest.LoadError, "depth") {
		t.Fatalf("deepest Frame = depth:%d loaded:%t error:%q", deepest.Depth, deepest.Loaded, deepest.LoadError)
	}
	if err := depthBrowser.Close(); err != nil {
		t.Fatal(err)
	}

	state := &frameLoadState{documentBytes: maxFrameDocuments - 1}
	if !state.reserveDocumentBytes(1) || state.reserveDocumentBytes(1) {
		t.Fatal("Frame document byte limit was not enforced at 32 MiB")
	}
	frame := &Frame{lifecycle: context.Background()}
	for index := 0; index < maxFrameNavigations; index++ {
		if err := frame.reserveNavigation(); err != nil {
			t.Fatalf("Navigation reservation %d: %v", index, err)
		}
	}
	if err := frame.reserveNavigation(); err == nil || !strings.Contains(err.Error(), "navigation limit") {
		t.Fatalf("Navigation overflow error = %v", err)
	}

	callbackContext, cancelCallbacks := context.WithCancel(context.Background())
	callbackState := &frameLoadState{ctx: callbackContext}
	release := make(chan struct{})
	for index := 0; index < maxFrameCallbacks; index++ {
		if err := callbackState.queueCallback(func() { <-release }); err != nil {
			t.Fatalf("callback reservation %d: %v", index, err)
		}
	}
	if err := callbackState.queueCallback(func() {}); err == nil || !strings.Contains(err.Error(), "callback limit") {
		t.Fatalf("callback overflow error = %v", err)
	}
	close(release)
	callbackState.waitCallbacks()
	cancelCallbacks()
	if err := callbackState.queueCallback(func() {}); err == nil {
		t.Fatal("callback was accepted after parent lifecycle cancellation")
	}
}

func TestIframeCloseCancelsNavigationAndWaitsForIt(t *testing.T) {
	parentURL := mustParseURL(t, "https://lifecycle.example/index.html")
	frameURL := mustParseURL(t, "https://lifecycle.example/frame.html")
	loader := &frameLifecycleLoader{
		started: make(chan struct{}),
		responses: map[string]*network.Response{
			parentURL.String(): {URL: parentURL, StatusCode: 200, ContentType: "text/html", Body: []byte(`<iframe src="/frame.html"></iframe>`)},
			frameURL.String():  {URL: frameURL, StatusCode: 200, ContentType: "text/html", Body: []byte(`<p>frame</p>`)},
		},
	}
	browserState := New(loader)
	page, err := browserState.Navigate(context.Background(), parentURL.String())
	if err != nil {
		t.Fatal(err)
	}
	frame := page.Frames[0]
	navigationDone := make(chan error, 1)
	go func() { navigationDone <- frame.Navigate(context.Background(), "/slow.html") }()
	select {
	case <-loader.started:
	case <-time.After(time.Second):
		t.Fatal("Frame navigation did not start")
	}
	if err := frame.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-navigationDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled Frame navigation error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Frame Close did not wait for navigation callback")
	}
	if err := browserState.Close(); err != nil {
		t.Fatal(err)
	}
}
