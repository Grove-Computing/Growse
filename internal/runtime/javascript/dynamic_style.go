package javascript

import (
	"context"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"strings"

	"github.com/Grove-Computing/Growse/internal/events"
	"github.com/Grove-Computing/Growse/internal/network"
	domapi "github.com/Grove-Computing/Growse/internal/webapi/dom"
	"github.com/dop251/goja"
)

const (
	maxDynamicStylesheetBytes = 4 << 20
	maxDynamicStylesheets     = 128
	maxPagePreloads           = 256
)

type dynamicStyleSnapshot struct {
	element     *domapi.Element
	id          uint64
	signature   string
	target      *url.URL
	integrity   string
	crossOrigin string
	kind        network.RequestKind
}

func (runtime *Runtime) installStyleElement(vm *goja.Runtime, object *goja.Object, element *domapi.Element) {
	if element == nil || !strings.EqualFold(element.TagName(), "style") {
		return
	}
	runtime.reflectStringAttribute(vm, object, element, "media", "media")
}

func (runtime *Runtime) resourceElementChanged(vm *goja.Runtime, element *domapi.Element) {
	if element == nil || !element.IsConnected() {
		return
	}
	switch strings.ToLower(element.TagName()) {
	case "style":
		runtime.prepareDynamicStyle(vm, element)
	case "link":
		runtime.prepareDynamicLink(vm, element)
	}
}

func (runtime *Runtime) prepareDynamicStyle(vm *goja.Runtime, element *domapi.Element) {
	signature := "style\x00" + element.Text()
	changed, err := runtime.updateResourceSignature(runtime.stylesheetStates, uint64(element.ID()), signature, maxDynamicStylesheets)
	if err != nil {
		runtime.recordError(err.Error())
		return
	}
	if !changed {
		return
	}
	snapshot := dynamicStyleSnapshot{element: element, id: uint64(element.ID()), signature: signature}
	runtime.refreshInlineStyles(snapshot)
}

func (runtime *Runtime) prepareDynamicLink(vm *goja.Runtime, element *domapi.Element) {
	if linkRelIncludes(element, "stylesheet") {
		runtime.prepareExternalStylesheet(vm, element)
		return
	}
	if linkRelIncludes(element, "preload") {
		runtime.prepareResourcePreload(vm, element)
		return
	}
	if runtime.clearResourceSignature(runtime.stylesheetStates, uint64(element.ID())) {
		runtime.refreshInlineStyles(dynamicStyleSnapshot{element: element, id: uint64(element.ID())})
	}
}

func (runtime *Runtime) prepareExternalStylesheet(vm *goja.Runtime, element *domapi.Element) {
	href, _ := element.GetAttribute("href")
	integrity, _ := element.GetAttribute("integrity")
	crossOrigin, _ := element.GetAttribute("crossorigin")
	mediaValue, _ := element.GetAttribute("media")
	signature := strings.Join([]string{"stylesheet", href, integrity, crossOrigin, mediaValue}, "\x00")
	changed, prepareErr := runtime.updateResourceSignature(runtime.stylesheetStates, uint64(element.ID()), signature, maxDynamicStylesheets)
	if prepareErr != nil {
		runtime.recordError(prepareErr.Error())
		runtime.dispatchDynamicStyleEvent(vm, dynamicStyleSnapshot{element: element, id: uint64(element.ID())}, events.Error)
		return
	}
	if !changed {
		return
	}
	target, err := runtime.resolvePageResourceURL(href)
	snapshot := dynamicStyleSnapshot{
		element: element, id: uint64(element.ID()), signature: signature, target: target,
		integrity: integrity, crossOrigin: crossOrigin, kind: network.RequestStylesheet,
	}
	if err != nil {
		runtime.recordError(fmt.Sprintf("load dynamic stylesheet: %v", err))
		runtime.dispatchDynamicStyleEvent(vm, snapshot, events.Error)
		return
	}
	go runtime.fetchDynamicStylesheet(snapshot)
}

func (runtime *Runtime) fetchDynamicStylesheet(snapshot dynamicStyleSnapshot) {
	response, err := runtime.fetchDynamicResource(snapshot)
	if err == nil && !isCSSMIME(response.ContentType) {
		err = fmt.Errorf("unsupported Content-Type %q", response.ContentType)
	}
	if err == nil {
		err = runtime.refreshStylesSerialized()
	}
	runtime.finishDynamicStyle(snapshot, err)
}

func (runtime *Runtime) refreshInlineStyles(snapshot dynamicStyleSnapshot) {
	_ = snapshot
	if err := runtime.refreshStylesSerialized(); err != nil {
		runtime.recordError(fmt.Sprintf("refresh dynamic style: %v", err))
	}
}

func (runtime *Runtime) refreshStylesSerialized() error {
	runtime.mu.Lock()
	refresh, ctx := runtime.environment.RefreshStyles, runtime.runtimeCtx
	runtime.mu.Unlock()
	if refresh == nil {
		return errors.New("browser stylesheet refresh is unavailable")
	}
	if ctx == nil {
		return context.Canceled
	}
	if runtime.executing.Load() {
		return refresh(ctx)
	}
	return runtime.runSync(ctx, func(*goja.Runtime) error { return refresh(ctx) })
}

func (runtime *Runtime) prepareResourcePreload(vm *goja.Runtime, element *domapi.Element) {
	href, _ := element.GetAttribute("href")
	asValue, _ := element.GetAttribute("as")
	integrity, _ := element.GetAttribute("integrity")
	crossOrigin, _ := element.GetAttribute("crossorigin")
	signature := strings.Join([]string{"preload", href, asValue, integrity, crossOrigin}, "\x00")
	runtime.mu.Lock()
	_, existingPreload := runtime.preloadStates[uint64(element.ID())]
	preloadCount := len(runtime.preloadStates) + len(runtime.modulePreloads)
	runtime.mu.Unlock()
	if !existingPreload && preloadCount >= maxPagePreloads {
		runtime.recordError(fmt.Sprintf("Page exceeds %d preloads", maxPagePreloads))
		runtime.dispatchDynamicStyleEvent(vm, dynamicStyleSnapshot{element: element, id: uint64(element.ID())}, events.Error)
		return
	}
	changed, prepareErr := runtime.updateResourceSignature(runtime.preloadStates, uint64(element.ID()), signature, maxPagePreloads)
	if prepareErr != nil {
		runtime.recordError(prepareErr.Error())
		runtime.dispatchDynamicStyleEvent(vm, dynamicStyleSnapshot{element: element, id: uint64(element.ID())}, events.Error)
		return
	}
	if !changed {
		return
	}
	kind, ok := preloadRequestKind(asValue)
	target, err := runtime.resolvePageResourceURL(href)
	snapshot := dynamicStyleSnapshot{
		element: element, id: uint64(element.ID()), signature: signature, target: target,
		integrity: integrity, crossOrigin: crossOrigin, kind: kind,
	}
	if !ok {
		err = fmt.Errorf("unsupported preload destination %q", asValue)
	}
	if err != nil {
		runtime.recordError(fmt.Sprintf("preload: %v", err))
		runtime.dispatchDynamicStyleEvent(vm, snapshot, events.Error)
		return
	}
	go func() {
		_, preloadErr := runtime.fetchDynamicResource(snapshot)
		runtime.finishDynamicStyle(snapshot, preloadErr)
	}()
}

func preloadRequestKind(value string) (network.RequestKind, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "style":
		return network.RequestStylesheet, true
	case "script":
		return network.RequestScript, true
	case "image":
		return network.RequestImage, true
	case "font":
		return network.RequestSubresource, true
	default:
		return network.RequestSubresource, false
	}
}

func (runtime *Runtime) fetchDynamicResource(snapshot dynamicStyleSnapshot) (response *network.Response, resourceErr error) {
	runtime.mu.Lock()
	environment, ctx := runtime.environment, runtime.runtimeCtx
	runtime.mu.Unlock()
	if environment.Fetch == nil || ctx == nil {
		return nil, errors.New("resource fetch is unavailable")
	}
	key := fmt.Sprintf("%d:%s", snapshot.kind, snapshot.target)
	if !runtime.allowResourceAttempt(key) {
		return nil, fmt.Errorf("resource failure retry limit %d exceeded", maxResourceFailureRetries)
	}
	defer func() { runtime.recordResourceFailure(key, resourceErr) }()
	hasCORS := strings.TrimSpace(snapshot.crossOrigin) != ""
	credentials := network.CredentialsInclude
	if hasCORS && !strings.EqualFold(strings.TrimSpace(snapshot.crossOrigin), "use-credentials") {
		credentials = network.CredentialsSameOrigin
	}
	response, resourceErr = environment.Fetch(ctx, &network.Request{
		Method: http.MethodGet, URL: snapshot.target, SiteURL: environment.BaseURL,
		Kind: snapshot.kind, Engine: "javascript", CORS: hasCORS, Credentials: credentials,
	})
	if resourceErr != nil {
		return nil, resourceErr
	}
	if response == nil {
		return nil, errors.New("empty resource response")
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("HTTP status %d", response.StatusCode)
	}
	if len(response.Body) > maxDynamicStylesheetBytes {
		return nil, fmt.Errorf("resource exceeds %d bytes", maxDynamicStylesheetBytes)
	}
	finalURL := response.URL
	if finalURL == nil {
		finalURL = snapshot.target
	}
	if _, err := resolveDynamicScriptURL(finalURL, finalURL.String()); err != nil {
		return nil, err
	}
	if mixedModuleContent(environment.BaseURL, finalURL) {
		return nil, fmt.Errorf("blocked mixed-content resource %s", network.RedactedURL(finalURL))
	}
	if snapshot.integrity != "" && !sameOriginURL(environment.BaseURL, finalURL) && !hasCORS {
		return nil, errors.New("cross-origin integrity requires crossorigin")
	}
	if err := verifyDynamicScriptIntegrity(response.Body, snapshot.integrity); err != nil {
		return nil, fmt.Errorf("integrity: %w", err)
	}
	return response, nil
}

func (runtime *Runtime) finishDynamicStyle(snapshot dynamicStyleSnapshot, resourceErr error) {
	runtime.mu.Lock()
	ctx := runtime.runtimeCtx
	current := runtime.resourceSignature(snapshot)
	runtime.mu.Unlock()
	if ctx == nil || snapshot.signature != "" && current != snapshot.signature {
		return
	}
	_ = runtime.runSync(ctx, func(vm *goja.Runtime) error {
		if resourceErr != nil {
			runtime.recordError(fmt.Sprintf("load resource %s: %v", network.RedactedURL(snapshot.target), resourceErr))
			runtime.dispatchDynamicStyleEvent(vm, snapshot, events.Error)
		} else {
			runtime.dispatchDynamicStyleEvent(vm, snapshot, events.Load)
		}
		return nil
	})
}

func (runtime *Runtime) dispatchDynamicStyleEvent(_ *goja.Runtime, snapshot dynamicStyleSnapshot, eventType events.Type) {
	runtime.mu.Lock()
	environment := runtime.environment
	runtime.mu.Unlock()
	if environment.Document == nil || environment.Events == nil || snapshot.element == nil || !snapshot.element.IsConnected() {
		return
	}
	environment.Events.DispatchTree(environment.Document, events.Event{Type: eventType, Target: snapshot.element.ID()})
}

func (runtime *Runtime) resolvePageResourceURL(reference string) (*url.URL, error) {
	runtime.mu.Lock()
	baseURL := runtime.environment.ResourceBaseURL
	if baseURL == nil {
		baseURL = runtime.environment.BaseURL
	}
	runtime.mu.Unlock()
	return resolveDynamicScriptURL(baseURL, reference)
}

func (runtime *Runtime) clearResourceSignature(states map[uint64]string, id uint64) bool {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if _, exists := states[id]; !exists {
		return false
	}
	delete(states, id)
	return true
}

func (runtime *Runtime) resourceSignature(snapshot dynamicStyleSnapshot) string {
	if snapshot.kind == network.RequestStylesheet && strings.HasPrefix(snapshot.signature, "stylesheet\x00") {
		return runtime.stylesheetStates[snapshot.id]
	}
	return runtime.preloadStates[snapshot.id]
}

func (runtime *Runtime) refreshRemovedStyleTree(element *domapi.Element) {
	if !containsStyleResource(element) {
		return
	}
	runtime.refreshInlineStyles(dynamicStyleSnapshot{})
}

func containsStyleResource(element *domapi.Element) bool {
	if element == nil {
		return false
	}
	if strings.EqualFold(element.TagName(), "style") || strings.EqualFold(element.TagName(), "link") && linkRelIncludes(element, "stylesheet") {
		return true
	}
	for _, child := range element.Children() {
		if containsStyleResource(child) {
			return true
		}
	}
	return false
}

func isCSSMIME(value string) bool {
	mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(value))
	return err == nil && strings.EqualFold(mediaType, "text/css")
}
