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
	reflectStringAttribute(vm, object, element, "src", "src")
	reflectStringAttribute(vm, object, element, "type", "type")
	reflectStringAttribute(vm, object, element, "integrity", "integrity")
	reflectStringAttribute(vm, object, element, "crossOrigin", "crossorigin")

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

func reflectStringAttribute(vm *goja.Runtime, object *goja.Object, element *domapi.Element, property, attribute string) {
	getter := vm.ToValue(func(goja.FunctionCall) goja.Value {
		value, _ := element.GetAttribute(attribute)
		return vm.ToValue(value)
	})
	setter := vm.ToValue(func(call goja.FunctionCall) goja.Value {
		element.SetAttribute(attribute, call.Argument(0).String())
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
		runtime.prepareDynamicClassicScript(vm, element)
	}
	for _, child := range element.Children() {
		runtime.prepareConnectedScriptTree(vm, child)
	}
}

func (runtime *Runtime) prepareDynamicClassicScript(vm *goja.Runtime, element *domapi.Element) {
	typeValue, _ := element.GetAttribute("type")
	switch strings.ToLower(strings.TrimSpace(typeValue)) {
	case "", "text/javascript", "application/javascript", "application/ecmascript", "text/ecmascript":
	default:
		return
	}

	id := uint64(element.ID())
	runtime.mu.Lock()
	if _, prepared := runtime.dynamicScripts[id]; prepared {
		runtime.mu.Unlock()
		return
	}
	runtime.dynamicScripts[id] = struct{}{}
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
		if _, err := vm.RunScript(fmt.Sprintf("dynamic-inline-script-%d.js", id), snapshot.source); err != nil {
			runtime.recordScriptError(fmt.Sprintf("dynamic-inline-script-%d.js", id), err)
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
	}
	response, err := environment.Fetch(runtimeContext, request)
	if err == nil {
		err = validateDynamicClassicScriptResponse(environment.BaseURL, snapshot, response)
	}
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
		if _, err := vm.RunScript(name, string(response.Body)); err != nil {
			runtime.recordScriptError(name, err)
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
