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
	"sync"
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
	defaultFrameWidth   = 300
	defaultFrameHeight  = 150
	maxFrameDepth       = 8
	maxFramesPerPage    = 32
	maxFrameDocuments   = 32 << 20
	maxFrameNavigations = 64
	maxFrameCallbacks   = 256
)

type frameLoadState struct {
	browser         *Browser
	ctx             context.Context
	rootURL         *url.URL
	resourceClient  ResourceLoader
	runtimeClient   ResourceLoader
	engineFactory   runtimemodel.EngineFactory
	engine          runtimemodel.Engine
	storage         *storagecore.Manager
	fetchLimiter    *fetchapi.Limiter
	onMutation      func()
	reducedMotion   bool
	count           int
	documentBytes   int
	nextID          uint64
	generation      uint64
	registry        *windowRegistry
	rootPage        *Page
	callbackMu      sync.Mutex
	callbackWG      sync.WaitGroup
	callbacks       int
	callbacksClosed bool
	limitMu         sync.Mutex
}

func (b *Browser) loadFrames(ctx context.Context, page *Page, resourceClient, runtimeClient ResourceLoader, engineFactory runtimemodel.EngineFactory, engine runtimemodel.Engine, storageManager *storagecore.Manager, fetchLimiter *fetchapi.Limiter, onMutation func(), reducedMotion bool) {
	if b == nil || page == nil || page.Document == nil {
		return
	}
	state := &frameLoadState{
		browser: b, ctx: ctx, rootURL: cloneURL(page.URL), resourceClient: resourceClient, runtimeClient: runtimeClient,
		engineFactory: engineFactory, engine: engine, storage: storageManager, fetchLimiter: fetchLimiter,
		onMutation: onMutation, reducedMotion: reducedMotion, generation: page.HistoryID + 1,
		registry: newWindowRegistry(), rootPage: page,
	}
	top := windowReference(0, state.generation, page)
	page.windows = state.registry
	page.window = runtimemodel.WindowContext{Self: top, Parent: top, Top: top}
	state.registry.define(top)
	page.Frames = state.loadChildren(page, page.Document.Root, 0, 0)
	page.window.Children = childWindowReferences(page, page)
}

func (state *frameLoadState) loadChildren(parentPage *Page, root *dom.Node, parentID uint64, depth int) []*Frame {
	if root == nil {
		return nil
	}
	var frames []*Frame
	var walk func(*dom.Node)
	walk = func(node *dom.Node) {
		if node == nil || state.frameLimitReached() {
			return
		}
		if node.Type == dom.NodeElement && node.TagName == "iframe" {
			frame := state.loadOne(parentPage, node, parentID, depth+1)
			if frame != nil {
				frames = append(frames, frame)
			}
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
	state.limitMu.Lock()
	if state.count >= maxFramesPerPage {
		state.limitMu.Unlock()
		return nil
	}
	state.count++
	state.nextID++
	frameID := state.nextID
	state.limitMu.Unlock()
	frameContext, cancel := context.WithCancel(state.ctx)
	width, height := frameDimensions(element)
	policy := parseFramePolicy(element)
	frame := &Frame{
		ID: frameID, ElementID: element.ID, ParentID: parentID, Depth: depth, Generation: state.generation,
		Viewport: FrameViewport{Width: width, Height: height, ClipWidth: width, ClipHeight: height}, cancel: cancel,
		state: state, parentPage: parentPage, Sandbox: policy, lifecycle: frameContext,
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
	if !state.reserveDocumentBytes(len(response.Body)) {
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
	self := windowReference(frame.ID, frame.Generation, page)
	page.windows = state.registry
	page.window = runtimemodel.WindowContext{
		Self: self, Parent: relativeWindowReference(parentPage.window.Self, page),
		Top: relativeWindowReference(state.rootPage.window.Self, page),
	}
	state.registry.define(self)
	page.Frames = state.loadChildren(page, page.Document.Root, frame.ID, depth)
	page.window.Children = childWindowReferences(page, page)
	frame.runtime = state.startFrameRuntime(frameContext, frame)
	page.RuntimeStarted = frame.runtime != nil
	page.Document.SetReadyState("complete")
	frame.Loaded = true
	return frame
}

func (state *frameLoadState) frameLimitReached() bool {
	state.limitMu.Lock()
	reached := state.count >= maxFramesPerPage
	state.limitMu.Unlock()
	return reached
}

func (state *frameLoadState) reserveDocumentBytes(size int) bool {
	state.limitMu.Lock()
	state.documentBytes += size
	allowed := state.documentBytes <= maxFrameDocuments
	state.limitMu.Unlock()
	return allowed
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
		FramePolicy:    policy,
		serviceWorkers: state.rootPage.serviceWorkers,
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
			if err := frame.reserveNavigation(); err != nil {
				return err
			}
			return state.queueCallback(func() {
				if err := frame.navigateReserved(context.Background(), resolved); err != nil && !errors.Is(err, context.Canceled) {
					frame.setLoadError(err.Error())
				}
			})
		},
		func(value string, target *url.URL) error { return frame.updateHistory(value, target, false) },
		func(value string, target *url.URL) error { return frame.updateHistory(value, target, true) },
		func(int) error { return errors.New("iframe history traversal is unavailable") },
		func() (int, string) { return 1, "" },
	)
	return runtime
}

func (state *frameLoadState) queueCallback(callback func()) error {
	if state == nil || callback == nil || state.ctx == nil || state.ctx.Err() != nil {
		return errors.New("iframe parent lifecycle is closed")
	}
	state.callbackMu.Lock()
	if state.callbacksClosed || state.ctx.Err() != nil {
		state.callbackMu.Unlock()
		return errors.New("iframe parent lifecycle is closed")
	}
	if state.callbacks >= maxFrameCallbacks {
		state.callbackMu.Unlock()
		return fmt.Errorf("iframe callback limit exceeds %d", maxFrameCallbacks)
	}
	state.callbacks++
	state.callbackWG.Add(1)
	state.callbackMu.Unlock()
	go func() {
		defer func() {
			state.callbackMu.Lock()
			state.callbacks--
			state.callbackMu.Unlock()
			state.callbackWG.Done()
		}()
		if state.ctx.Err() == nil {
			callback()
		}
	}()
	return nil
}

func (state *frameLoadState) waitCallbacks() {
	if state != nil {
		state.callbackMu.Lock()
		state.callbacksClosed = true
		state.callbackMu.Unlock()
		state.callbackWG.Wait()
	}
}

// Navigate loads a new independent Document into this Frame.
func (frame *Frame) Navigate(ctx context.Context, rawURL string) error {
	if frame == nil {
		return errors.New("iframe is closed")
	}
	frame.lifecycleMu.Lock()
	closed := frame.Closed || frame.Page == nil || frame.Page.URL == nil
	var currentURL *url.URL
	if !closed {
		currentURL = cloneURL(frame.Page.URL)
	}
	frame.lifecycleMu.Unlock()
	if closed {
		return errors.New("iframe is closed")
	}
	reference, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return errors.New("iframe navigation URL is invalid")
	}
	return frame.navigate(ctx, currentURL.ResolveReference(reference))
}

// Reload reloads this Frame without replacing its parent element identity.
func (frame *Frame) Reload(ctx context.Context) error {
	if frame == nil {
		return errors.New("iframe is unavailable")
	}
	frame.lifecycleMu.Lock()
	target := cloneURL(frame.URL)
	closed := frame.Closed || target == nil
	frame.lifecycleMu.Unlock()
	if closed {
		return errors.New("iframe is unavailable")
	}
	return frame.navigate(ctx, target)
}

func (frame *Frame) navigate(ctx context.Context, target *url.URL) error {
	if err := frame.reserveNavigation(); err != nil {
		return err
	}
	return frame.navigateReserved(ctx, target)
}

func (frame *Frame) reserveNavigation() error {
	if frame == nil {
		return errors.New("iframe navigation is unavailable")
	}
	frame.lifecycleMu.Lock()
	closed := frame.Closed || frame.lifecycle == nil || frame.lifecycle.Err() != nil
	frame.lifecycleMu.Unlock()
	if closed {
		return errors.New("iframe is closed")
	}
	frame.quotaMu.Lock()
	defer frame.quotaMu.Unlock()
	if frame.navigations >= maxFrameNavigations {
		return fmt.Errorf("iframe navigation limit exceeds %d", maxFrameNavigations)
	}
	frame.navigations++
	return nil
}

func (frame *Frame) navigateReserved(ctx context.Context, target *url.URL) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if frame == nil || frame.state == nil || target == nil || !isHTTPURL(target) || target.User != nil {
		return errors.New("iframe navigation is unavailable")
	}
	frame.navigationMu.Lock()
	defer frame.navigationMu.Unlock()
	frame.lifecycleMu.Lock()
	lifecycle := frame.lifecycle
	closed := frame.Closed || lifecycle == nil
	frame.lifecycleMu.Unlock()
	if closed {
		return errors.New("iframe is closed")
	}
	navigationContext, cancelNavigation := context.WithCancel(ctx)
	stopLifecycle := context.AfterFunc(lifecycle, cancelNavigation)
	defer func() {
		stopLifecycle()
		cancelNavigation()
	}()
	state := frame.state
	response, err := state.fetchFrameURL(navigationContext, target)
	if err != nil {
		frame.setLoadError(err.Error())
		return err
	}
	page, err := state.buildPage(navigationContext, response, frame.Sandbox)
	if err != nil {
		frame.setLoadError(err.Error())
		return err
	}
	nextGeneration := frame.Generation + 1
	self := windowReference(frame.ID, nextGeneration, page)
	page.windows = state.registry
	page.window = runtimemodel.WindowContext{
		Self: self, Parent: relativeWindowReference(frame.parentPage.window.Self, page),
		Top: relativeWindowReference(state.rootPage.window.Self, page),
	}
	state.registry.define(self)
	page.Frames = state.loadChildren(page, page.Document.Root, frame.ID, frame.Depth)
	page.window.Children = childWindowReferences(page, page)
	frame.lifecycleMu.Lock()
	if frame.Closed || frame.lifecycle != lifecycle || navigationContext.Err() != nil {
		frame.lifecycleMu.Unlock()
		_ = closePageFrames(page)
		page.closeDevTools()
		return context.Canceled
	}
	oldPage, oldRuntime, oldCancel := frame.Page, frame.runtime, frame.cancel
	frameContext, frameCancel := context.WithCancel(state.ctx)
	frame.Page, frame.URL, frame.cancel, frame.lifecycle = page, cloneURL(page.URL), frameCancel, frameContext
	frame.Generation = nextGeneration
	frame.LoadError = ""
	page.Document.SetReadyState("complete")
	frame.Loaded = true
	frame.lifecycleMu.Unlock()
	newRuntime := state.startFrameRuntime(frameContext, frame)
	frame.lifecycleMu.Lock()
	if frame.Closed || frame.Page != page || frame.Generation != nextGeneration {
		frame.lifecycleMu.Unlock()
		if newRuntime != nil {
			_ = newRuntime.Stop()
		}
		_ = closePageFrames(page)
		page.closeDevTools()
		return context.Canceled
	}
	frame.runtime = newRuntime
	page.RuntimeStarted = newRuntime != nil
	frame.lifecycleMu.Unlock()
	if oldCancel != nil {
		oldCancel()
	}
	if oldRuntime != nil {
		state.registry.unregister(oldPage.window.Self)
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

func (frame *Frame) setLoadError(message string) {
	if frame == nil {
		return
	}
	frame.lifecycleMu.Lock()
	if !frame.Closed {
		frame.LoadError = message
	}
	frame.lifecycleMu.Unlock()
}

func windowReference(id, generation uint64, page *Page) runtimemodel.WindowReference {
	reference := runtimemodel.WindowReference{ID: id, Generation: generation, SameOrigin: true}
	if page == nil || page.URL == nil {
		return reference
	}
	reference.URL = page.URL.String()
	if page.FramePolicy.HasOpaqueOrigin() {
		reference.Origin = "null"
		return reference
	}
	if origin, err := network.OriginFromURL(page.URL); err == nil {
		reference.Origin = origin.String()
	}
	return reference
}

func relativeWindowReference(reference runtimemodel.WindowReference, viewer *Page) runtimemodel.WindowReference {
	reference.SameOrigin = viewer != nil && !viewer.FramePolicy.HasOpaqueOrigin() && originsMatch(reference.Origin, windowReference(0, 0, viewer).Origin)
	return reference
}

func childWindowReferences(page, viewer *Page) []runtimemodel.WindowReference {
	if page == nil {
		return nil
	}
	result := make([]runtimemodel.WindowReference, 0, len(page.Frames))
	for _, frame := range page.Frames {
		if frame == nil || frame.Page == nil || frame.Closed {
			continue
		}
		result = append(result, relativeWindowReference(frame.Page.window.Self, viewer))
	}
	return result
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
	parent.window.Children = childWindowReferences(parent, parent)
	if updater, ok := runtime.(runtimemodel.WindowUpdater); ok {
		updater.UpdateWindow(parent.window)
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
	if frame == nil {
		return errors.New("iframe history update is unavailable")
	}
	frame.lifecycleMu.Lock()
	defer frame.lifecycleMu.Unlock()
	if frame.Closed || frame.Page == nil || target == nil || frame.Page.URL == nil || !network.SameOrigin(frame.Page.URL, target) {
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
	if frame == nil {
		return nil
	}
	frame.lifecycleMu.Lock()
	if frame.Closed {
		frame.lifecycleMu.Unlock()
		return nil
	}
	frame.Closed = true
	cancel, page, pageRuntime := frame.cancel, frame.Page, frame.runtime
	frame.cancel = nil
	frame.lifecycle = nil
	frame.runtime = nil
	frame.lifecycleMu.Unlock()
	if cancel != nil {
		cancel()
	}
	frame.navigationMu.Lock()
	defer frame.navigationMu.Unlock()
	var result error
	if page != nil {
		if err := closePageFrames(page); err != nil {
			result = err
		}
		if page.Animations != nil {
			page.Animations.Clear()
		}
		if page.Transitions != nil {
			page.Transitions.Clear()
		}
		page.closeDevTools()
		if page.windows != nil {
			page.windows.unregister(page.window.Self)
		}
	}
	if pageRuntime != nil {
		if err := pageRuntime.Stop(); err != nil && result == nil {
			result = err
		}
	}
	return result
}

func closePageFrames(page *Page) error {
	if page == nil {
		return nil
	}
	var result error
	var state *frameLoadState
	if page.window.Self.ID == 0 && len(page.Frames) != 0 && page.Frames[0] != nil {
		state = page.Frames[0].state
	}
	for _, frame := range page.Frames {
		if err := frame.Close(); err != nil && result == nil {
			result = err
		}
	}
	page.Frames = nil
	if page.window.Self.ID == 0 && page.windows != nil {
		state.waitCallbacks()
		page.windows.close()
	}
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
