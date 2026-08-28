package javascript

import (
	"crypto/sha256"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	dommodel "github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/events"
	"github.com/Grove-Computing/Growse/internal/network"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
	domapi "github.com/Grove-Computing/Growse/internal/webapi/dom"
	"github.com/dop251/goja"
)

const maxDynamicScriptBytes = 2 << 20

type dynamicScriptSnapshot struct {
	element     *domapi.Element
	id          dommodel.NodeID
	source      string
	sourceURL   *url.URL
	integrity   string
	crossOrigin string
}

func (runtime *Runtime) installScriptElement(vm *goja.Runtime, object *goja.Object, element *domapi.Element) {
	if element == nil || !strings.EqualFold(element.TagName(), "script") {
		return
	}
	runtime.reflectStringAttribute(vm, object, element, "src", "src")
	runtime.reflectStringAttribute(vm, object, element, "type", "type")
	runtime.reflectStringAttribute(vm, object, element, "integrity", "integrity")
	runtime.reflectStringAttribute(vm, object, element, "crossOrigin", "crossorigin")

	asyncGetter := vm.ToValue(func(goja.FunctionCall) goja.Value {
		_, present := element.GetAttribute("async")
		return vm.ToValue(present)
	})
	asyncSetter := vm.ToValue(func(call goja.FunctionCall) goja.Value {
		if call.Argument(0).ToBoolean() {
			element.SetAttribute("async", "")
		} else {
			element.RemoveAttribute("async")
		}
		return goja.Undefined()
	})
	_ = object.DefineAccessorProperty("async", asyncGetter, asyncSetter, goja.FLAG_FALSE, goja.FLAG_TRUE)

	textGetter := vm.ToValue(func(goja.FunctionCall) goja.Value { return vm.ToValue(element.Text()) })
	textSetter := vm.ToValue(func(call goja.FunctionCall) goja.Value {
		element.SetText(call.Argument(0).String())
		return goja.Undefined()
	})
	_ = object.DefineAccessorProperty("text", textGetter, textSetter, goja.FLAG_FALSE, goja.FLAG_TRUE)
}

func (runtime *Runtime) installLinkElement(vm *goja.Runtime, object *goja.Object, element *domapi.Element) {
	if element == nil || !strings.EqualFold(element.TagName(), "link") {
		return
	}
	runtime.reflectStringAttribute(vm, object, element, "href", "href")
	runtime.reflectStringAttribute(vm, object, element, "rel", "rel")
	runtime.reflectStringAttribute(vm, object, element, "as", "as")
	runtime.reflectStringAttribute(vm, object, element, "integrity", "integrity")
	runtime.reflectStringAttribute(vm, object, element, "crossOrigin", "crossorigin")
}

func (runtime *Runtime) reflectStringAttribute(vm *goja.Runtime, object *goja.Object, element *domapi.Element, property, attribute string) {
	getter := vm.ToValue(func(goja.FunctionCall) goja.Value {
		value, _ := element.GetAttribute(attribute)
		return vm.ToValue(value)
	})
	setter := vm.ToValue(func(call goja.FunctionCall) goja.Value {
		element.SetAttribute(attribute, call.Argument(0).String())
		runtime.resourceElementChanged(vm, element)
		return goja.Undefined()
	})
	_ = object.DefineAccessorProperty(property, getter, setter, goja.FLAG_FALSE, goja.FLAG_TRUE)
}

func (runtime *Runtime) prepareConnectedScripts(vm *goja.Runtime, roots ...*domapi.Element) {
	for _, root := range roots {
		if root == nil || !root.IsConnected() {
			continue
		}
		runtime.prepareConnectedScriptTree(vm, root)
	}
}

func (runtime *Runtime) prepareConnectedScriptTree(vm *goja.Runtime, element *domapi.Element) {
	if strings.EqualFold(element.TagName(), "script") {
		typeValue, _ := element.GetAttribute("type")
		if strings.EqualFold(strings.TrimSpace(typeValue), "module") {
			runtime.prepareDynamicModuleScript(vm, element)
		} else {
			runtime.prepareDynamicClassicScript(vm, element)
		}
	} else if strings.EqualFold(element.TagName(), "link") && linkRelIncludes(element, "modulepreload") {
		runtime.prepareModulePreload(vm, element)
	} else if strings.EqualFold(element.TagName(), "style") {
		runtime.prepareDynamicStyle(vm, element)
	} else if strings.EqualFold(element.TagName(), "link") {
		runtime.prepareDynamicLink(vm, element)
	} else if strings.EqualFold(element.TagName(), "img") {
		runtime.prepareDynamicImage(element)
	}
	for _, child := range element.Children() {
		runtime.prepareConnectedScriptTree(vm, child)
	}
}

func (runtime *Runtime) prepareInitialModulePreloads(vm *goja.Runtime) {
	runtime.mu.Lock()
	domAPI := runtime.domAPI
	runtime.mu.Unlock()
	if domAPI == nil {
		return
	}
	for _, link := range domAPI.GetElementsByTagName("link") {
		if linkRelIncludes(link, "modulepreload") {
			runtime.prepareModulePreload(vm, link)
		}
	}
}

func linkRelIncludes(element *domapi.Element, wanted string) bool {
	value, _ := element.GetAttribute("rel")
	for _, token := range strings.Fields(strings.ToLower(value)) {
		if token == wanted {
			return true
		}
	}
	return false
}

func (runtime *Runtime) prepareDynamicModuleScript(vm *goja.Runtime, element *domapi.Element) {
	id := uint64(element.ID())
	if err := runtime.claimDynamicScript(id); err != nil {
		if !errors.Is(err, errResourceAlreadyPrepared) {
			runtime.recordError(err.Error())
			runtime.dispatchDynamicScriptEvent(vm, dynamicScriptSnapshot{element: element, id: element.ID()}, events.Error)
		}
		return
	}
	runtime.mu.Lock()
	environment := runtime.environment
	runtime.mu.Unlock()

	snapshot := dynamicScriptSnapshot{element: element, id: element.ID(), source: element.Text()}
	source, hasSource := element.GetAttribute("src")
	snapshot.integrity, _ = element.GetAttribute("integrity")
	snapshot.crossOrigin, _ = element.GetAttribute("crossorigin")
	baseURL := environment.ResourceBaseURL
	if baseURL == nil {
		baseURL = environment.BaseURL
	}
	if !hasSource || strings.TrimSpace(source) == "" {
		if baseURL == nil {
			runtime.recordError("load dynamic module: document base URL is unavailable")
			runtime.dispatchDynamicScriptEvent(vm, snapshot, events.Error)
			return
		}
		inlineURL := *baseURL
		query := inlineURL.Query()
		query.Set("__growse_inline_module", fmt.Sprint(id))
		inlineURL.RawQuery = query.Encode()
		snapshot.sourceURL = &inlineURL
		if !runtime.reserveScriptBytes(len(snapshot.source)) {
			runtime.recordError(fmt.Sprintf("dynamic module exceeds Page source limit %d", maxPageScriptBytes))
			runtime.dispatchDynamicScriptEvent(vm, snapshot, events.Error)
			return
		}
		script := runtimemodel.Script{
			Engine: runtimemodel.EngineJavaScript, Kind: runtimemodel.ScriptModule, Inline: true,
			SourceURL: snapshot.sourceURL, Source: snapshot.source, DocumentOrder: int(id),
		}
		go runtime.evaluateDynamicModule(snapshot, script)
		return
	}
	target, err := resolveDynamicScriptURL(baseURL, source)
	if err != nil {
		runtime.recordError(fmt.Sprintf("load dynamic module: %v", err))
		runtime.dispatchDynamicScriptEvent(vm, snapshot, events.Error)
		return
	}
	snapshot.sourceURL = target
	go runtime.fetchAndEvaluateDynamicModule(snapshot)
}

func (runtime *Runtime) fetchAndEvaluateDynamicModule(snapshot dynamicScriptSnapshot) {
	runtime.mu.Lock()
	registry, runtimeContext := runtime.moduleRegistry, runtime.runtimeCtx
	runtime.mu.Unlock()
	if registry == nil || runtimeContext == nil {
		runtime.finishDynamicModule(snapshot, errors.New("module registry is unavailable"))
		return
	}
	credentials := network.CredentialsSameOrigin
	if strings.EqualFold(strings.TrimSpace(snapshot.crossOrigin), "use-credentials") {
		credentials = network.CredentialsInclude
	}
	response, err := registry.fetch(runtimeContext, snapshot.sourceURL, credentials)
	if err == nil {
		err = verifyDynamicScriptIntegrity(response.Body, snapshot.integrity)
	}
	if err != nil {
		runtime.finishDynamicModule(snapshot, err)
		return
	}
	finalURL := snapshot.sourceURL
	if response.URL != nil {
		finalURL = response.URL
	}
	script := runtimemodel.Script{
		Engine: runtimemodel.EngineJavaScript, Kind: runtimemodel.ScriptModule,
		SourceURL: finalURL, Source: string(response.Body), CrossOrigin: strings.ToLower(strings.TrimSpace(snapshot.crossOrigin)),
	}
	runtime.evaluateDynamicModule(snapshot, script)
}

func (runtime *Runtime) evaluateDynamicModule(snapshot dynamicScriptSnapshot, script runtimemodel.Script) {
	runtime.mu.Lock()
	ctx := runtime.runtimeCtx
	runtime.mu.Unlock()
	if ctx == nil {
		return
	}
	name := network.RedactedURL(script.SourceURL)
	err := runtime.evaluateModuleScript(ctx, name, script)
	runtime.finishDynamicModule(snapshot, err)
}

func (runtime *Runtime) finishDynamicModule(snapshot dynamicScriptSnapshot, moduleErr error) {
	runtime.mu.Lock()
	ctx := runtime.runtimeCtx
	runtime.mu.Unlock()
	if ctx == nil {
		return
	}
	_ = runtime.runSync(ctx, func(vm *goja.Runtime) error {
		if moduleErr != nil {
			runtime.recordError(fmt.Sprintf("load dynamic module %s: %v", network.RedactedURL(snapshot.sourceURL), moduleErr))
			runtime.dispatchDynamicScriptEvent(vm, snapshot, events.Error)
		} else {
			runtime.dispatchDynamicScriptEvent(vm, snapshot, events.Load)
		}
		return nil
	})
}

func (runtime *Runtime) prepareModulePreload(vm *goja.Runtime, element *domapi.Element) {
	id := uint64(element.ID())
	runtime.mu.Lock()
	if _, prepared := runtime.modulePreloads[id]; prepared {
		runtime.mu.Unlock()
		return
	}
	if len(runtime.modulePreloads)+len(runtime.preloadStates) >= maxPagePreloads {
		runtime.mu.Unlock()
		runtime.recordError(fmt.Sprintf("Page exceeds %d preloads", maxPagePreloads))
		runtime.dispatchDynamicScriptEvent(vm, dynamicScriptSnapshot{element: element, id: element.ID()}, events.Error)
		return
	}
	runtime.modulePreloads[id] = struct{}{}
	environment, registry, runtimeContext := runtime.environment, runtime.moduleRegistry, runtime.runtimeCtx
	runtime.mu.Unlock()
	href, _ := element.GetAttribute("href")
	integrity, _ := element.GetAttribute("integrity")
	crossOrigin, _ := element.GetAttribute("crossorigin")
	baseURL := environment.ResourceBaseURL
	if baseURL == nil {
		baseURL = environment.BaseURL
	}
	target, err := resolveDynamicScriptURL(baseURL, href)
	snapshot := dynamicScriptSnapshot{element: element, id: element.ID(), sourceURL: target, integrity: integrity, crossOrigin: crossOrigin}
	if err != nil || registry == nil || runtimeContext == nil {
		if err == nil {
			err = errors.New("module registry is unavailable")
		}
		runtime.recordError(fmt.Sprintf("modulepreload: %v", err))
		runtime.dispatchDynamicScriptEvent(vm, snapshot, events.Error)
		return
	}
	go func() {
		credentials := network.CredentialsSameOrigin
		if strings.EqualFold(strings.TrimSpace(crossOrigin), "use-credentials") {
			credentials = network.CredentialsInclude
		}
		response, preloadErr := registry.fetch(runtimeContext, target, credentials)
		if preloadErr == nil {
			preloadErr = verifyDynamicScriptIntegrity(response.Body, integrity)
		}
		runtime.finishModulePreload(snapshot, preloadErr)
	}()
}

func (runtime *Runtime) finishModulePreload(snapshot dynamicScriptSnapshot, preloadErr error) {
	runtime.mu.Lock()
	ctx := runtime.runtimeCtx
	runtime.mu.Unlock()
	if ctx == nil {
		return
	}
	_ = runtime.runSync(ctx, func(vm *goja.Runtime) error {
		if preloadErr != nil {
			runtime.recordError(fmt.Sprintf("modulepreload %s: %v", network.RedactedURL(snapshot.sourceURL), preloadErr))
			runtime.dispatchDynamicScriptEvent(vm, snapshot, events.Error)
		} else {
			runtime.dispatchDynamicScriptEvent(vm, snapshot, events.Load)
		}
		return nil
	})
}

func (runtime *Runtime) prepareDynamicClassicScript(vm *goja.Runtime, element *domapi.Element) {
	typeValue, _ := element.GetAttribute("type")
	switch strings.ToLower(strings.TrimSpace(typeValue)) {
	case "", "text/javascript", "application/javascript", "application/ecmascript", "text/ecmascript":
	default:
		return
	}

	id := uint64(element.ID())
	if err := runtime.claimDynamicScript(id); err != nil {
		if !errors.Is(err, errResourceAlreadyPrepared) {
			runtime.recordError(err.Error())
			runtime.dispatchDynamicScriptEvent(vm, dynamicScriptSnapshot{element: element, id: element.ID()}, events.Error)
		}
		return
	}
	runtime.mu.Lock()
	environment := runtime.environment
	runtime.mu.Unlock()

	snapshot := dynamicScriptSnapshot{element: element, id: element.ID(), source: element.Text()}
	source, hasSource := element.GetAttribute("src")
	snapshot.integrity, _ = element.GetAttribute("integrity")
	snapshot.crossOrigin, _ = element.GetAttribute("crossorigin")
	if !hasSource || strings.TrimSpace(source) == "" {
		if snapshot.source == "" {
			return
		}
		if !runtime.reserveScriptBytes(len(snapshot.source)) {
			runtime.recordError(fmt.Sprintf("dynamic script exceeds Page source limit %d", maxPageScriptBytes))
			runtime.dispatchDynamicScriptEvent(vm, snapshot, events.Error)
			return
		}
		if !runtime.beginDynamicInsertion() {
			runtime.recordError(fmt.Sprintf("dynamic script insertion exceeds depth %d", maxDynamicInsertDepth))
			runtime.dispatchDynamicScriptEvent(vm, snapshot, events.Error)
			return
		}
		defer runtime.endDynamicInsertion()
		previousScript := runtime.currentScript
		runtime.setCurrentScript(vm, element)
		_, scriptErr := vm.RunScript(fmt.Sprintf("dynamic-inline-script-%d.js", id), snapshot.source)
		runtime.currentScript = previousScript
		if scriptErr != nil {
			runtime.recordScriptError(fmt.Sprintf("dynamic-inline-script-%d.js", id), scriptErr)
		}
		return
	}

	baseURL := environment.ResourceBaseURL
	if baseURL == nil {
		baseURL = environment.BaseURL
	}
	target, err := resolveDynamicScriptURL(baseURL, source)
	if err != nil {
		runtime.recordError(fmt.Sprintf("load dynamic script: %v", err))
		runtime.dispatchDynamicScriptEvent(vm, snapshot, events.Error)
		return
	}
	snapshot.sourceURL = target
	go runtime.fetchDynamicClassicScript(snapshot)
}

func resolveDynamicScriptURL(baseURL *url.URL, source string) (*url.URL, error) {
	if baseURL == nil {
		return nil, errors.New("document base URL is unavailable")
	}
	reference, err := url.Parse(strings.TrimSpace(source))
	if err != nil {
		return nil, fmt.Errorf("parse script URL: %w", err)
	}
	target := baseURL.ResolveReference(reference)
	if target.User != nil || target.Host == "" || !strings.EqualFold(target.Scheme, "http") && !strings.EqualFold(target.Scheme, "https") {
		return nil, fmt.Errorf("script URL must be HTTP(S) without userinfo: %s", network.RedactedURL(target))
	}
	target.Fragment = ""
	return target, nil
}

func (runtime *Runtime) fetchDynamicClassicScript(snapshot dynamicScriptSnapshot) {
	runtime.mu.Lock()
	environment, runtimeContext := runtime.environment, runtime.runtimeCtx
	runtime.mu.Unlock()
	if runtimeContext == nil || environment.Fetch == nil {
		runtime.finishDynamicScript(snapshot, nil, errors.New("script fetch is unavailable"))
		return
	}

	crossOrigin := strings.ToLower(strings.TrimSpace(snapshot.crossOrigin))
	hasCORS := crossOrigin != ""
	credentials := network.CredentialsInclude
	if hasCORS && crossOrigin != "use-credentials" {
		credentials = network.CredentialsSameOrigin
	}
	request := &network.Request{
		Method: http.MethodGet, URL: snapshot.sourceURL, SiteURL: environment.BaseURL,
		Kind: network.RequestScript, Engine: "javascript", CORS: hasCORS, Credentials: credentials,
		Initiator: "dom-insertion", Schedule: "dynamic",
	}
	key := snapshot.sourceURL.String()
	if !runtime.allowResourceAttempt(key) {
		runtime.finishDynamicScript(snapshot, nil, fmt.Errorf("resource failure retry limit %d exceeded", maxResourceFailureRetries))
		return
	}
	response, err := environment.Fetch(runtimeContext, request)
	if err == nil {
		err = validateDynamicClassicScriptResponse(environment.BaseURL, snapshot, response)
	}
	if err == nil && !runtime.reserveScriptBytes(len(response.Body)) {
		err = fmt.Errorf("JavaScript Page source exceeds %d bytes", maxPageScriptBytes)
	}
	runtime.recordResourceFailure(key, err)
	runtime.finishDynamicScript(snapshot, response, err)
}

func validateDynamicClassicScriptResponse(pageURL *url.URL, snapshot dynamicScriptSnapshot, response *network.Response) error {
	if response == nil {
		return errors.New("empty script response")
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("HTTP status %d", response.StatusCode)
	}
	if len(response.Body) > maxDynamicScriptBytes {
		return fmt.Errorf("script exceeds %d bytes", maxDynamicScriptBytes)
	}
	if !isModuleContentType(response.ContentType) {
		return fmt.Errorf("unsupported Content-Type %q", response.ContentType)
	}
	finalURL := response.URL
	if finalURL == nil {
		finalURL = snapshot.sourceURL
	}
	if _, err := resolveDynamicScriptURL(finalURL, finalURL.String()); err != nil {
		return fmt.Errorf("redirected script URL: %w", err)
	}
	if mixedModuleContent(pageURL, finalURL) {
		return fmt.Errorf("blocked mixed-content script %s", network.RedactedURL(finalURL))
	}
	if snapshot.integrity != "" && !sameOriginURL(pageURL, finalURL) && strings.TrimSpace(snapshot.crossOrigin) == "" {
		return errors.New("cross-origin integrity requires crossorigin")
	}
	if err := verifyDynamicScriptIntegrity(response.Body, snapshot.integrity); err != nil {
		return fmt.Errorf("integrity: %w", err)
	}
	return nil
}

func (runtime *Runtime) finishDynamicScript(snapshot dynamicScriptSnapshot, response *network.Response, fetchErr error) {
	runtime.mu.Lock()
	ctx := runtime.runtimeCtx
	runtime.mu.Unlock()
	if ctx == nil {
		return
	}
	_ = runtime.runSync(ctx, func(vm *goja.Runtime) error {
		if fetchErr != nil {
			runtime.recordError(fmt.Sprintf("load dynamic script %s: %v", network.RedactedURL(snapshot.sourceURL), fetchErr))
			runtime.dispatchDynamicScriptEvent(vm, snapshot, events.Error)
			return nil
		}
		name := network.RedactedURL(snapshot.sourceURL)
		if response != nil && response.URL != nil {
			name = network.RedactedURL(response.URL)
		}
		if !runtime.beginDynamicInsertion() {
			runtime.recordError(fmt.Sprintf("dynamic script insertion exceeds depth %d", maxDynamicInsertDepth))
			runtime.dispatchDynamicScriptEvent(vm, snapshot, events.Error)
			return nil
		}
		defer runtime.endDynamicInsertion()
		previousScript := runtime.currentScript
		runtime.setCurrentScript(vm, snapshot.element)
		_, scriptErr := vm.RunScript(name, string(response.Body))
		runtime.currentScript = previousScript
		if scriptErr != nil {
			runtime.recordScriptError(name, scriptErr)
			runtime.dispatchDynamicScriptEvent(vm, snapshot, events.Error)
			return nil
		}
		runtime.dispatchDynamicScriptEvent(vm, snapshot, events.Load)
		return nil
	})
}

func (runtime *Runtime) dispatchDynamicScriptEvent(_ *goja.Runtime, snapshot dynamicScriptSnapshot, eventType events.Type) {
	runtime.mu.Lock()
	environment := runtime.environment
	runtime.mu.Unlock()
	if environment.Document == nil || environment.Events == nil || snapshot.element == nil || !snapshot.element.IsConnected() {
		return
	}
	environment.Events.DispatchTree(environment.Document, events.Event{Type: eventType, Target: snapshot.id})
}

func sameOriginURL(left, right *url.URL) bool {
	if left == nil || right == nil || !strings.EqualFold(left.Scheme, right.Scheme) || !strings.EqualFold(left.Hostname(), right.Hostname()) {
		return false
	}
	return effectiveURLPort(left) == effectiveURLPort(right)
}

func effectiveURLPort(value *url.URL) string {
	if port := value.Port(); port != "" {
		return port
	}
	if strings.EqualFold(value.Scheme, "https") {
		return "443"
	}
	return "80"
}

func verifyDynamicScriptIntegrity(body []byte, metadata string) error {
	metadata = strings.TrimSpace(metadata)
	if metadata == "" {
		return nil
	}
	type candidate struct {
		strength int
		digest   []byte
	}
	var candidates []candidate
	strongest := 0
	for _, token := range strings.Fields(metadata) {
		token, _, _ = strings.Cut(token, "?")
		algorithm, encoded, found := strings.Cut(token, "-")
		if !found {
			continue
		}
		decoded, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			continue
		}
		strength := 0
		switch strings.ToLower(algorithm) {
		case "sha256":
			strength = 1
		case "sha384":
			strength = 2
		case "sha512":
			strength = 3
		}
		if strength != 0 {
			candidates = append(candidates, candidate{strength: strength, digest: decoded})
			if strength > strongest {
				strongest = strength
			}
		}
	}
	if strongest == 0 {
		return errors.New("no supported digest")
	}
	var actual []byte
	switch strongest {
	case 1:
		digest := sha256.Sum256(body)
		actual = digest[:]
	case 2:
		digest := sha512.Sum384(body)
		actual = digest[:]
	case 3:
		digest := sha512.Sum512(body)
		actual = digest[:]
	}
	for _, expected := range candidates {
		if expected.strength == strongest && len(expected.digest) == len(actual) && subtle.ConstantTimeCompare(expected.digest, actual) == 1 {
			return nil
		}
	}
	return errors.New("digest mismatch")
}
