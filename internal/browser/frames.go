package browser

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Grove-Computing/Growse/internal/devtools"
	"github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/events"
	htmlparser "github.com/Grove-Computing/Growse/internal/html"
	"github.com/Grove-Computing/Growse/internal/network"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
	storagecore "github.com/Grove-Computing/Growse/internal/storage"
	"github.com/Grove-Computing/Growse/internal/style"
	fetchapi "github.com/Grove-Computing/Growse/internal/webapi/fetch"
)

const (
	defaultFrameWidth  = 300
	defaultFrameHeight = 150
	maxFrameDepth      = 8
	maxFramesPerPage   = 32
	maxFrameDocuments  = 32 << 20
)

type frameLoadState struct {
	browser        *Browser
	ctx            context.Context
	rootURL        *url.URL
	resourceClient ResourceLoader
	runtimeClient  ResourceLoader
	engineFactory  runtimemodel.EngineFactory
	engine         runtimemodel.Engine
	storage        *storagecore.Manager
	fetchLimiter   *fetchapi.Limiter
	onMutation     func()
	reducedMotion  bool
	count          int
	documentBytes  int
	nextID         uint64
	generation     uint64
}

func (b *Browser) loadFrames(ctx context.Context, page *Page, resourceClient, runtimeClient ResourceLoader, engineFactory runtimemodel.EngineFactory, engine runtimemodel.Engine, storageManager *storagecore.Manager, fetchLimiter *fetchapi.Limiter, onMutation func(), reducedMotion bool) {
	if b == nil || page == nil || page.Document == nil {
		return
	}
	state := &frameLoadState{
		browser: b, ctx: ctx, rootURL: cloneURL(page.URL), resourceClient: resourceClient, runtimeClient: runtimeClient,
		engineFactory: engineFactory, engine: engine, storage: storageManager, fetchLimiter: fetchLimiter,
		onMutation: onMutation, reducedMotion: reducedMotion, generation: page.HistoryID + 1,
	}
	page.Frames = state.loadChildren(page, page.Document.Root, 0, 0)
}

func (state *frameLoadState) loadChildren(parentPage *Page, root *dom.Node, parentID uint64, depth int) []*Frame {
	if root == nil {
		return nil
	}
	var frames []*Frame
	var walk func(*dom.Node)
	walk = func(node *dom.Node) {
		if node == nil || state.count >= maxFramesPerPage {
			return
		}
		if node.Type == dom.NodeElement && node.TagName == "iframe" {
			frame := state.loadOne(parentPage, node, parentID, depth+1)
			frames = append(frames, frame)
			return
		}
		for _, child := range node.Children {
			walk(child)
		}
	}
	walk(root)
	return frames
}

func (state *frameLoadState) loadOne(parentPage *Page, element *dom.Node, parentID uint64, depth int) *Frame {
	state.count++
	state.nextID++
	frameContext, cancel := context.WithCancel(state.ctx)
	width, height := frameDimensions(element)
	policy := parseFramePolicy(element)
	frame := &Frame{
		ID: state.nextID, ElementID: element.ID, ParentID: parentID, Depth: depth, Generation: state.generation,
		Viewport: FrameViewport{Width: width, Height: height, ClipWidth: width, ClipHeight: height}, cancel: cancel,
		state: state, parentPage: parentPage, Sandbox: policy,
	}
	if depth > maxFrameDepth {
		frame.LoadError = fmt.Sprintf("iframe depth exceeds %d", maxFrameDepth)
		return frame
	}

	response, err := state.frameResponse(frameContext, parentPage.URL, element)
	if err != nil {
		frame.LoadError = err.Error()
		return frame
	}
	state.documentBytes += len(response.Body)
	if state.documentBytes > maxFrameDocuments {
		frame.LoadError = fmt.Sprintf("iframe documents exceed %d bytes", maxFrameDocuments)
		return frame
	}
	page, err := state.buildPage(frameContext, response, policy)
	if err != nil {
		frame.LoadError = err.Error()
		return frame
	}
	frame.URL = cloneURL(page.URL)
	frame.Page = page
	page.Frames = state.loadChildren(page, page.Document.Root, frame.ID, depth)
	frame.runtime = state.startFrameRuntime(frameContext, frame)
	page.RuntimeStarted = frame.runtime != nil
	page.Document.SetReadyState("complete")
	frame.Loaded = true
	return frame
}

func (state *frameLoadState) frameResponse(ctx context.Context, parentURL *url.URL, element *dom.Node) (*network.Response, error) {
	if source, ok := element.Attribute("srcdoc"); ok {
		return &network.Response{
			URL: cloneURL(parentURL), StatusCode: http.StatusOK, Status: "OK", ContentType: "text/html; charset=utf-8", Body: []byte(source),
		}, nil
	}
	source, _ := element.Attribute("src")
	source = strings.TrimSpace(source)
	if source == "" {
		return &network.Response{
			URL: cloneURL(parentURL), StatusCode: http.StatusOK, Status: "OK", ContentType: "text/html; charset=utf-8", Body: []byte("<!doctype html><html><body></body></html>"),
		}, nil
	}
	reference, err := url.Parse(source)
	if err != nil || parentURL == nil {
		return nil, errors.New("iframe src is invalid")
	}
	target := parentURL.ResolveReference(reference)
	if !isHTTPURL(target) || target.User != nil {
		return nil, errors.New("iframe src must use HTTP(S) without userinfo")
	}
	if loader, ok := state.runtimeClient.(requestLoader); ok {
		return loader.Do(ctx, &network.Request{
			Method: http.MethodGet, URL: target, SiteURL: cloneURL(state.rootURL), Kind: network.RequestNavigation,
		})
	}
	if state.runtimeClient == nil {
		return nil, errors.New("iframe network loader is unavailable")
	}
	return state.runtimeClient.Get(ctx, target)
}

func (state *frameLoadState) buildPage(ctx context.Context, response *network.Response, policy runtimemodel.FramePolicy) (*Page, error) {
	if response == nil || response.URL == nil {
		return nil, errors.New("iframe response is invalid")
	}
	mediaType, _, err := mime.ParseMediaType(response.ContentType)
	if err != nil || mediaType != "text/html" && mediaType != "application/xhtml+xml" {
		return nil, fmt.Errorf("iframe Content-Type %q is unsupported", response.ContentType)
	}
	document, err := htmlparser.Parse(bytes.NewReader(response.Body))
	if err != nil {
		return nil, fmt.Errorf("build iframe DOM: %w", err)
	}
	pageStore := state.browser.newDevToolsPageStore()
	styleResources, imageResources, scriptResources := state.resourceClient, state.resourceClient, state.resourceClient
	if loader, ok := state.resourceClient.(requestLoader); ok {
		styleResources = pageResourceLoader{loader: loader, siteURL: state.rootURL, kind: network.RequestStylesheet, observer: pageStore.ObserveNetwork}
		imageResources = pageResourceLoader{loader: loader, siteURL: state.rootURL, kind: network.RequestImage, observer: pageStore.ObserveNetwork}
		scriptResources = pageResourceLoader{loader: loader, siteURL: state.rootURL, kind: network.RequestScript, engine: string(state.engine), observer: pageStore.ObserveNetwork}
	}
	stylesheet, err := state.browser.loadStyles(ctx, styleResources, response.URL, document)
	if err != nil {
		pageStore.Close()
		return nil, fmt.Errorf("load iframe styles: %w", err)
	}
	computed := style.ComputeWithEnvironment(document, stylesheet, style.InteractionState{}, style.Environment{
		ViewportWidth: defaultFrameWidth, ViewportHeight: defaultFrameHeight, RootFontSize: 16, ResolutionDPI: 96,
		ColorScheme: "light", Hover: true, Pointer: "fine", ReducedMotion: state.reducedMotion,
	})
	backgroundImages, backgroundErrors := loadBackgroundImages(ctx, imageResources, computed)
	var scripts []Script
	var scriptErrors []string
	var importMap map[string]string
	if policy.AllowsScripts() {
		scripts, scriptErrors = loadScriptsForEngine(ctx, scriptResources, response.URL, document, state.engine)
	}
	if policy.AllowsScripts() && state.engine == runtimemodel.EngineJavaScript {
		var importErrors []string
		importMap, importErrors = loadImportMap(document, response.URL)
		scriptErrors = append(scriptErrors, importErrors...)
	}
	page := &Page{
		URL: cloneURL(response.URL), StatusCode: response.StatusCode, ContentType: response.ContentType, Source: append([]byte(nil), response.Body...),
		Document: document, Events: events.NewDispatcher(), Stylesheet: stylesheet, ComputedStyles: computed,
		Animations: style.NewAnimationRegistry(), Transitions: style.NewTransitionRegistry(), BackgroundImages: backgroundImages,
		BackgroundErrors: backgroundErrors, Engine: state.engine, Scripts: scripts, ImportMap: importMap, ScriptErrors: scriptErrors,
		StyleRevision: 1, ReducedMotion: state.reducedMotion, ViewportWidth: defaultFrameWidth, ViewportHeight: defaultFrameHeight, DevTools: pageStore,
		FramePolicy: policy,
	}
	for _, scriptError := range scriptErrors {
		pageStore.AddConsole(devtools.ConsoleError, "script", scriptError)
	}
	page.Animations.Reconcile(computed, state.browser.currentTime())
	return page, nil
}

func (state *frameLoadState) startFrameRuntime(ctx context.Context, frame *Frame) runtimemodel.Runtime {
	if frame == nil || frame.Page == nil {
		return nil
	}
	page := frame.Page
	runtime := startRuntime(ctx, state.engineFactory, state.engine, page, state.runtimeClient, state.storage,
		state.browser.storageSourceID+frame.ID, state.fetchLimiter, state.onMutation, state.browser.currentTime,
		func(target *url.URL) error {
			if target == nil || !isHTTPURL(target) {
				return errors.New("iframe navigation URL is invalid")
			}
			resolved := cloneURL(target)
			go func() {
				if err := frame.navigate(context.Background(), resolved); err != nil {
					frame.LoadError = err.Error()
				}
			}()
			return nil
		},
		func(value string, target *url.URL) error { return frame.updateHistory(value, target, false) },
		func(value string, target *url.URL) error { return frame.updateHistory(value, target, true) },
		func(int) error { return errors.New("iframe history traversal is unavailable") },
		func() (int, string) { return 1, "" },
	)
	return runtime
}

// Navigate loads a new independent Document into this Frame.
func (frame *Frame) Navigate(ctx context.Context, rawURL string) error {
	if frame == nil || frame.Closed || frame.Page == nil || frame.Page.URL == nil {
		return errors.New("iframe is closed")
	}
	reference, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return errors.New("iframe navigation URL is invalid")
	}
	return frame.navigate(ctx, frame.Page.URL.ResolveReference(reference))
}

// Reload reloads this Frame without replacing its parent element identity.
func (frame *Frame) Reload(ctx context.Context) error {
	if frame == nil || frame.URL == nil {
		return errors.New("iframe is unavailable")
	}
	return frame.navigate(ctx, cloneURL(frame.URL))
}

func (frame *Frame) navigate(ctx context.Context, target *url.URL) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if frame == nil || frame.Closed || frame.state == nil || target == nil || !isHTTPURL(target) || target.User != nil {
		return errors.New("iframe navigation is unavailable")
	}
	state := frame.state
	response, err := state.fetchFrameURL(ctx, target)
	if err != nil {
		frame.LoadError = err.Error()
		return err
	}
	page, err := state.buildPage(ctx, response, frame.Sandbox)
	if err != nil {
		frame.LoadError = err.Error()
		return err
	}
	page.Frames = state.loadChildren(page, page.Document.Root, frame.ID, frame.Depth)
	oldPage, oldRuntime, oldCancel := frame.Page, frame.runtime, frame.cancel
	frameContext, frameCancel := context.WithCancel(state.ctx)
	frame.Page, frame.URL, frame.cancel = page, cloneURL(page.URL), frameCancel
	frame.Generation++
	frame.LoadError = ""
	frame.runtime = state.startFrameRuntime(frameContext, frame)
	page.RuntimeStarted = frame.runtime != nil
	page.Document.SetReadyState("complete")
	frame.Loaded = true
	if oldCancel != nil {
		oldCancel()
	}
	if oldRuntime != nil {
		_ = oldRuntime.Stop()
	}
	_ = closePageFrames(oldPage)
	if oldPage != nil {
		oldPage.closeDevTools()
	}
	if state.onMutation != nil {
		state.onMutation()
	}
	if frame.parentPage != nil {
		state.browser.refreshFrameAccess(frame.parentPage)
		state.browser.dispatchPageEvent(frame.parentPage, events.Event{Type: events.Type("load"), Target: frame.ElementID})
	}
	return nil
}

func runtimeFrameAccess(page *Page) []runtimemodel.FrameAccess {
	if page == nil || page.URL == nil {
		return nil
	}
	result := make([]runtimemodel.FrameAccess, 0, len(page.Frames))
	for _, frame := range page.Frames {
		if frame == nil || frame.Closed {
			continue
		}
		access := runtimemodel.FrameAccess{ID: frame.ID, ElementID: frame.ElementID, Generation: frame.Generation}
		if frame.Page != nil && frame.URL != nil {
			if origin, err := network.OriginFromURL(frame.URL); err == nil {
				access.Origin = origin.String()
			}
			access.SameOrigin = !page.FramePolicy.HasOpaqueOrigin() && !frame.Page.FramePolicy.HasOpaqueOrigin() && network.SameOrigin(page.URL, frame.URL)
			if access.SameOrigin {
				access.URL = frame.URL.String()
				access.Document = frame.Page.Document
			}
		}
		result = append(result, access)
	}
	return result
}

func applyFrameMutation(parent *Page, frameID, generation uint64, snapshot dom.DocumentSnapshot, onMutation func(), current time.Time) error {
	if parent == nil {
		return errors.New("frame parent is unavailable")
	}
	for _, frame := range parent.Frames {
		if frame == nil || frame.ID != frameID {
			continue
		}
		if frame.Closed || frame.Generation != generation || frame.Page == nil || parent.FramePolicy.HasOpaqueOrigin() || frame.Page.FramePolicy.HasOpaqueOrigin() || !network.SameOrigin(parent.URL, frame.URL) {
			return errors.New("stale or cross-origin Frame mutation was rejected")
		}
		if err := frame.Page.Document.ApplySnapshot(snapshot); err != nil {
			return err
		}
		recomputePageStyles(frame.Page, current)
		if onMutation != nil {
			onMutation()
		}
		return nil
	}
	return errors.New("frame mutation target was not found")
}

func (b *Browser) refreshFrameAccess(parent *Page) {
	if b == nil || parent == nil {
		return
	}
	b.mu.RLock()
	runtime := b.activeRuntime
	if b.page != parent {
		runtime = runtimeForFramePage(b.page, parent)
	}
	b.mu.RUnlock()
	if updater, ok := runtime.(runtimemodel.FrameUpdater); ok {
		updater.UpdateFrames(runtimeFrameAccess(parent))
	}
}

func runtimeForFramePage(root, target *Page) runtimemodel.Runtime {
	if root == nil || target == nil {
		return nil
	}
	for _, frame := range root.Frames {
		if frame == nil || frame.Closed {
			continue
		}
		if frame.Page == target {
			return frame.runtime
		}
		if runtime := runtimeForFramePage(frame.Page, target); runtime != nil {
			return runtime
		}
	}
	return nil
}

func (state *frameLoadState) fetchFrameURL(ctx context.Context, target *url.URL) (*network.Response, error) {
	if loader, ok := state.runtimeClient.(requestLoader); ok {
		return loader.Do(ctx, &network.Request{Method: http.MethodGet, URL: target, SiteURL: cloneURL(state.rootURL), Kind: network.RequestNavigation})
	}
	if state.runtimeClient == nil {
		return nil, errors.New("iframe network loader is unavailable")
	}
	return state.runtimeClient.Get(ctx, target)
}

func (frame *Frame) updateHistory(_ string, target *url.URL, replace bool) error {
	if frame == nil || frame.Closed || frame.Page == nil || target == nil || frame.Page.URL == nil || !network.SameOrigin(frame.Page.URL, target) {
		return errors.New("iframe history update is unavailable")
	}
	frame.Page.URL = cloneURL(target)
	frame.URL = cloneURL(target)
	_ = replace
	return nil
}

func parseFramePolicy(element *dom.Node) runtimemodel.FramePolicy {
	if element == nil {
		return runtimemodel.FramePolicy{}
	}
	value, sandboxed := element.Attribute("sandbox")
	policy := runtimemodel.FramePolicy{Sandboxed: sandboxed}
	if !sandboxed {
		return policy
	}
	for _, token := range strings.Fields(strings.ToLower(value)) {
		switch token {
		case "allow-scripts":
			policy.AllowScripts = true
		case "allow-same-origin":
			policy.AllowSameOrigin = true
		case "allow-forms":
			policy.AllowForms = true
		case "allow-popups":
			policy.AllowPopups = true
		case "allow-top-navigation-by-user-activation":
			policy.AllowTopNavigationByActivation = true
		}
	}
	return policy
}

// AuthorizeFormSubmission applies the sandbox form gate before any side effect.
func (frame *Frame) AuthorizeFormSubmission() error {
	if frame == nil || frame.Closed || !frame.Sandbox.AllowsForms() {
		return errors.New("iframe sandbox blocks form submission")
	}
	return nil
}

// AuthorizePopup applies the sandbox auxiliary-context gate before any side effect.
func (frame *Frame) AuthorizePopup() error {
	if frame == nil || frame.Closed || !frame.Sandbox.AllowsPopups() {
		return errors.New("iframe sandbox blocks popup creation")
	}
	return nil
}

// AuthorizeTopNavigation applies the user-activation sandbox gate.
func (frame *Frame) AuthorizeTopNavigation(userActivated bool) error {
	if frame == nil || frame.Closed || !frame.Sandbox.AllowsTopNavigation(userActivated) {
		return errors.New("iframe sandbox blocks top navigation")
	}
	return nil
}

func frameDimensions(element *dom.Node) (float32, float32) {
	read := func(name string, fallback float32) float32 {
		value, ok := element.Attribute(name)
		if !ok {
			return fallback
		}
		parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 32)
		if err != nil || parsed <= 0 || parsed > 16_384 {
			return fallback
		}
		return float32(parsed)
	}
	return read("width", defaultFrameWidth), read("height", defaultFrameHeight)
}

// SetScroll updates the child viewport scroll offset without escaping its document extent.
func (frame *Frame) SetScroll(x, y float32) {
	if frame == nil || frame.Closed {
		return
	}
	frame.ScrollX = max(float32(0), x)
	frame.ScrollY = max(float32(0), y)
}

// SetViewport records the iframe border box and its mandatory clipping region.
func (frame *Frame) SetViewport(x, y, width, height float32) {
	if frame == nil || frame.Closed || width <= 0 || height <= 0 {
		return
	}
	frame.Viewport = FrameViewport{X: x, Y: y, Width: width, Height: height, ClipX: x, ClipY: y, ClipWidth: width, ClipHeight: height}
	if frame.Page != nil {
		frame.Page.ViewportWidth, frame.Page.ViewportHeight = width, height
	}
}

// Close stops this Frame and every descendant exactly once.
func (frame *Frame) Close() error {
	if frame == nil || frame.Closed {
		return nil
	}
	frame.Closed = true
	if frame.cancel != nil {
		frame.cancel()
		frame.cancel = nil
	}
	var result error
	if frame.Page != nil {
		if err := closePageFrames(frame.Page); err != nil {
			result = err
		}
		if frame.Page.Animations != nil {
			frame.Page.Animations.Clear()
		}
		if frame.Page.Transitions != nil {
			frame.Page.Transitions.Clear()
		}
		frame.Page.closeDevTools()
	}
	if frame.runtime != nil {
		if err := frame.runtime.Stop(); err != nil && result == nil {
			result = err
		}
		frame.runtime = nil
	}
	return result
}

func closePageFrames(page *Page) error {
	if page == nil {
		return nil
	}
	var result error
	for _, frame := range page.Frames {
		if err := frame.Close(); err != nil && result == nil {
			result = err
		}
	}
	page.Frames = nil
	return result
}

func frameRuntimes(page *Page) []runtimemodel.Runtime {
	if page == nil {
		return nil
	}
	var result []runtimemodel.Runtime
	for _, frame := range page.Frames {
		if frame == nil || frame.Closed {
			continue
		}
		if frame.runtime != nil {
			result = append(result, frame.runtime)
		}
		result = append(result, frameRuntimes(frame.Page)...)
	}
	return result
}

func dispatchFrameLoadEvents(browserState *Browser, page *Page) {
	if browserState == nil || page == nil {
		return
	}
	for _, frame := range page.Frames {
		if frame != nil && frame.Loaded && !frame.Closed {
			browserState.dispatchPageEvent(page, events.Event{Type: events.Type("load"), Target: frame.ElementID})
		}
	}
}
