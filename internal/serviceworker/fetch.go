package serviceworker

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Grove-Computing/Growse/internal/network"
	"github.com/dop251/goja"
)

const (
	maxFetchEventTime     = 5 * time.Second
	maxSyntheticBodyBytes = 4 << 20
)

type fetchEvaluator struct {
	vm            *goja.Runtime
	workerURL     *url.URL
	origin        string
	fallback      NetworkFallback
	cacheStorage  *CacheStorage
	responses     map[*goja.Object]*network.Response
	requests      map[*goja.Object]*network.Request
	listeners     []goja.Callable
	ctx           context.Context
	dispatching   bool
	responded     bool
	responseValue goja.Value
}

func evaluateFetch(ctx context.Context, source []byte, workerURL *url.URL, request *network.Request, fallback NetworkFallback, cacheStorage *CacheStorage) (*network.Response, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	eventContext, cancel := context.WithTimeout(ctx, maxFetchEventTime)
	defer cancel()
	vm := goja.New()
	evaluator := &fetchEvaluator{
		vm: vm, workerURL: cloneURL(workerURL), origin: originString(workerURL), fallback: fallback, cacheStorage: cacheStorage, ctx: eventContext,
		responses: make(map[*goja.Object]*network.Response), requests: make(map[*goja.Object]*network.Request),
	}
	timer := time.AfterFunc(maxFetchEventTime, func() { vm.Interrupt("service worker fetch event timeout") })
	defer timer.Stop()
	stopContext := context.AfterFunc(eventContext, func() { vm.Interrupt(eventContext.Err()) })
	defer stopContext()
	if err := evaluator.installGlobals(); err != nil {
		return nil, err
	}
	if _, err := vm.RunScript(workerURL.String(), string(source)); err != nil {
		return nil, fmt.Errorf("evaluate service worker fetch script: %w", err)
	}
	return evaluator.dispatch(eventContext, request)
}

func (evaluator *fetchEvaluator) installGlobals() error {
	global := evaluator.vm.GlobalObject()
	if err := global.Set("self", global); err != nil {
		return err
	}
	if err := global.Set("addEventListener", func(call goja.FunctionCall) goja.Value {
		if call.Argument(0).String() != "fetch" {
			return goja.Undefined()
		}
		listener, ok := goja.AssertFunction(call.Argument(1))
		if !ok {
			panic(evaluator.vm.NewTypeError("Service Worker listener must be a function"))
		}
		evaluator.listeners = append(evaluator.listeners, listener)
		return goja.Undefined()
	}); err != nil {
		return err
	}
	if err := global.Set("fetch", evaluator.fetch); err != nil {
		return err
	}
	_ = global.Set("skipWaiting", func(goja.FunctionCall) goja.Value { return goja.Undefined() })
	clients := evaluator.vm.NewObject()
	_ = clients.Set("claim", func(goja.FunctionCall) goja.Value { return goja.Undefined() })
	_ = global.Set("clients", clients)
	if err := global.Set("Request", evaluator.requestConstructor); err != nil {
		return err
	}
	if err := global.Set("Response", evaluator.responseConstructor); err != nil {
		return err
	}
	return evaluator.installCacheStorage(global)
}

func (evaluator *fetchEvaluator) installFetchPrimitives(global *goja.Object) error {
	if err := global.Set("fetch", evaluator.fetch); err != nil {
		return err
	}
	if err := global.Set("Request", evaluator.requestConstructor); err != nil {
		return err
	}
	if err := global.Set("Response", evaluator.responseConstructor); err != nil {
		return err
	}
	return evaluator.installCacheStorage(global)
}

func (evaluator *fetchEvaluator) installCacheStorage(global *goja.Object) error {
	if evaluator.cacheStorage == nil || evaluator.origin == "" {
		return nil
	}
	storage := evaluator.vm.NewObject()
	_ = storage.Set("open", func(call goja.FunctionCall) goja.Value {
		return evaluator.promiseAction(func() (any, error) {
			cache, err := evaluator.cacheStorage.Open(evaluator.origin, call.Argument(0).String())
			if err != nil {
				return nil, err
			}
			return evaluator.cacheValue(cache), nil
		})
	})
	_ = storage.Set("match", func(call goja.FunctionCall) goja.Value {
		return evaluator.promiseAction(func() (any, error) {
			request, err := evaluator.requestFromValue(call.Argument(0), goja.Undefined())
			if err != nil {
				return nil, err
			}
			response, found := evaluator.cacheStorage.Match(evaluator.origin, request)
			if !found {
				return goja.Undefined(), nil
			}
			return evaluator.responseValueFor(response), nil
		})
	})
	_ = storage.Set("has", func(call goja.FunctionCall) goja.Value {
		return evaluator.resolvedPromise(evaluator.cacheStorage.Has(evaluator.origin, call.Argument(0).String()))
	})
	_ = storage.Set("delete", func(call goja.FunctionCall) goja.Value {
		return evaluator.resolvedPromise(evaluator.cacheStorage.Delete(evaluator.origin, call.Argument(0).String()))
	})
	_ = storage.Set("keys", func(goja.FunctionCall) goja.Value {
		names := evaluator.cacheStorage.Keys(evaluator.origin)
		values := make([]any, len(names))
		for index, name := range names {
			values[index] = name
		}
		return evaluator.resolvedPromise(evaluator.vm.NewArray(values...))
	})
	return global.Set("caches", storage)
}

func (evaluator *fetchEvaluator) cacheValue(cache *Cache) *goja.Object {
	object := evaluator.vm.NewObject()
	_ = object.Set("match", func(call goja.FunctionCall) goja.Value {
		return evaluator.promiseAction(func() (any, error) {
			request, err := evaluator.requestFromValue(call.Argument(0), goja.Undefined())
			if err != nil {
				return nil, err
			}
			response, found := cache.Match(request)
			if !found {
				return goja.Undefined(), nil
			}
			return evaluator.responseValueFor(response), nil
		})
	})
	_ = object.Set("put", func(call goja.FunctionCall) goja.Value {
		return evaluator.promiseAction(func() (any, error) {
			request, err := evaluator.requestFromValue(call.Argument(0), goja.Undefined())
			if err != nil {
				return nil, err
			}
			responseObject, ok := call.Argument(1).(*goja.Object)
			if !ok || evaluator.responses[responseObject] == nil {
				return nil, errors.New("service worker Cache.put requires a Response")
			}
			if err := cache.Put(request, evaluator.responses[responseObject]); err != nil {
				return nil, err
			}
			return goja.Undefined(), nil
		})
	})
	_ = object.Set("delete", func(call goja.FunctionCall) goja.Value {
		return evaluator.promiseAction(func() (any, error) {
			request, err := evaluator.requestFromValue(call.Argument(0), goja.Undefined())
			if err != nil {
				return nil, err
			}
			return cache.Delete(request), nil
		})
	})
	_ = object.Set("keys", func(goja.FunctionCall) goja.Value {
		requests := cache.Keys()
		values := make([]any, 0, len(requests))
		for _, request := range requests {
			values = append(values, evaluator.requestValue(request))
		}
		return evaluator.resolvedPromise(evaluator.vm.NewArray(values...))
	})
	return object
}

func (evaluator *fetchEvaluator) dispatch(ctx context.Context, request *network.Request) (*network.Response, error) {
	evaluator.ctx = ctx
	defer func() { evaluator.ctx = nil }()
	requestCopy := cloneRequest(request)
	event := evaluator.vm.NewObject()
	_ = event.Set("type", "fetch")
	_ = event.Set("request", evaluator.requestValue(requestCopy))
	_ = event.Set("respondWith", func(call goja.FunctionCall) goja.Value {
		if !evaluator.dispatching {
			panic(evaluator.vm.NewTypeError("respondWith must be called while dispatching the fetch event"))
		}
		if evaluator.responded {
			panic(evaluator.vm.NewTypeError("respondWith may only be called once"))
		}
		evaluator.responded = true
		evaluator.responseValue = call.Argument(0)
		return goja.Undefined()
	})
	evaluator.dispatching = true
	for _, listener := range evaluator.listeners {
		if _, err := listener(evaluator.vm.GlobalObject(), event); err != nil {
			evaluator.dispatching = false
			return nil, fmt.Errorf("dispatch service worker fetch event: %w", err)
		}
	}
	evaluator.dispatching = false
	if !evaluator.responded {
		return evaluator.fallback(ctx, requestCopy)
	}
	value, err := settledValue(evaluator.responseValue)
	if err != nil {
		return nil, fmt.Errorf("service worker respondWith rejected: %w", err)
	}
	object, ok := value.(*goja.Object)
	if !ok || evaluator.responses[object] == nil {
		return nil, errors.New("service worker respondWith requires a Response")
	}
	response := cloneResponse(evaluator.responses[object])
	if response.URL == nil {
		response.URL = cloneURL(request.URL)
	}
	return response, nil
}

func settledValue(value goja.Value) (goja.Value, error) {
	if value == nil {
		return nil, errors.New("response is missing")
	}
	promise, ok := value.Export().(*goja.Promise)
	if !ok {
		return value, nil
	}
	switch promise.State() {
	case goja.PromiseStateFulfilled:
		return promise.Result(), nil
	case goja.PromiseStateRejected:
		return nil, errors.New(promise.Result().String())
	default:
		return nil, errors.New("response Promise did not settle within the fetch event")
	}
}

func (evaluator *fetchEvaluator) fetch(call goja.FunctionCall) goja.Value {
	promise, resolve, reject := evaluator.vm.NewPromise()
	request, err := evaluator.requestFromValue(call.Argument(0), call.Argument(1))
	if err == nil {
		request.Kind = network.RequestServiceWorkerFetch
		request.SiteURL = cloneURL(evaluator.workerURL)
		request.CORS = true
		request.Observer = nil
		var response *network.Response
		response, err = evaluator.fallback(evaluator.ctx, request)
		if err == nil && response == nil {
			err = errors.New("service worker fetch returned no response")
		}
		if err == nil {
			err = resolve(evaluator.responseValueFor(response))
		}
	}
	if err != nil {
		_ = reject(evaluator.vm.NewTypeError(err.Error()))
	}
	return evaluator.vm.ToValue(promise)
}

func (evaluator *fetchEvaluator) requestConstructor(call goja.ConstructorCall) *goja.Object {
	request, err := evaluator.requestFromValue(call.Argument(0), call.Argument(1))
	if err != nil {
		panic(evaluator.vm.NewTypeError(err.Error()))
	}
	return evaluator.requestValue(request)
}

func (evaluator *fetchEvaluator) requestFromValue(input, init goja.Value) (*network.Request, error) {
	var request *network.Request
	if object, ok := input.(*goja.Object); ok && evaluator.requests[object] != nil {
		request = cloneRequest(evaluator.requests[object])
	} else {
		reference, err := url.Parse(strings.TrimSpace(input.String()))
		if err != nil {
			return nil, errors.New("invalid Service Worker Request URL")
		}
		resolved := reference
		if evaluator.workerURL != nil {
			resolved = evaluator.workerURL.ResolveReference(reference)
		}
		if resolved == nil || resolved.Host == "" || resolved.Scheme != "http" && resolved.Scheme != "https" {
			return nil, errors.New("service worker Request URL must use HTTP(S)")
		}
		request = &network.Request{Method: http.MethodGet, URL: resolved, Credentials: network.CredentialsSameOrigin}
	}
	if !goja.IsUndefined(init) && !goja.IsNull(init) {
		options := init.ToObject(evaluator.vm)
		if value := options.Get("method"); value != nil && !goja.IsUndefined(value) {
			request.Method = strings.ToUpper(strings.TrimSpace(value.String()))
		}
		if value := options.Get("body"); value != nil && !goja.IsUndefined(value) && !goja.IsNull(value) {
			body, _, err := extractBody(value)
			if err != nil {
				return nil, err
			}
			request.Body = body
		}
		if value := options.Get("headers"); value != nil && !goja.IsUndefined(value) && !goja.IsNull(value) {
			request.Header = headersFromValue(evaluator.vm, value)
		}
	}
	if request.Method == "" {
		request.Method = http.MethodGet
	}
	if (request.Method == http.MethodGet || request.Method == http.MethodHead) && len(request.Body) != 0 {
		return nil, errors.New("GET and HEAD service worker requests cannot have a body")
	}
	return request, nil
}

func (evaluator *fetchEvaluator) requestValue(request *network.Request) *goja.Object {
	object := evaluator.vm.NewObject()
	copy := cloneRequest(request)
	evaluator.requests[object] = copy
	_ = object.DefineDataProperty("url", evaluator.vm.ToValue(copy.URL.String()), goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE)
	_ = object.DefineDataProperty("method", evaluator.vm.ToValue(copy.Method), goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE)
	_ = object.DefineDataProperty("headers", evaluator.headersValue(copy.Header), goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE)
	_ = object.Set("clone", func(goja.FunctionCall) goja.Value { return evaluator.requestValue(copy) })
	_ = object.Set("text", func(goja.FunctionCall) goja.Value { return evaluator.resolvedPromise(string(copy.Body)) })
	_ = object.Set("arrayBuffer", func(goja.FunctionCall) goja.Value {
		return evaluator.resolvedPromise(evaluator.vm.NewArrayBuffer(append([]byte(nil), copy.Body...)))
	})
	return object
}

func (evaluator *fetchEvaluator) responseConstructor(call goja.ConstructorCall) *goja.Object {
	body, defaultType, err := extractBody(call.Argument(0))
	if err != nil {
		panic(evaluator.vm.NewTypeError(err.Error()))
	}
	response := &network.Response{StatusCode: http.StatusOK, Status: http.StatusText(http.StatusOK), Header: make(http.Header), Body: body}
	init := call.Argument(1)
	if !goja.IsUndefined(init) && !goja.IsNull(init) {
		options := init.ToObject(evaluator.vm)
		if value := options.Get("status"); value != nil && !goja.IsUndefined(value) {
			response.StatusCode = int(value.ToInteger())
			if response.StatusCode < 200 || response.StatusCode > 599 {
				panic(evaluator.vm.NewTypeError("Response status must be between 200 and 599"))
			}
			response.Status = http.StatusText(response.StatusCode)
		}
		if value := options.Get("statusText"); value != nil && !goja.IsUndefined(value) {
			response.Status = value.String()
		}
		if value := options.Get("headers"); value != nil && !goja.IsUndefined(value) && !goja.IsNull(value) {
			response.Header = headersFromValue(evaluator.vm, value)
		}
	}
	if response.Header.Get("Content-Type") == "" && defaultType != "" {
		response.Header.Set("Content-Type", defaultType)
	}
	response.ContentType = response.Header.Get("Content-Type")
	object := evaluator.vm.NewObject()
	evaluator.responses[object] = response
	return evaluator.decorateResponse(object, response)
}

func (evaluator *fetchEvaluator) responseValueFor(response *network.Response) *goja.Object {
	object := evaluator.vm.NewObject()
	copy := cloneResponse(response)
	evaluator.responses[object] = copy
	return evaluator.decorateResponse(object, copy)
}

func (evaluator *fetchEvaluator) decorateResponse(object *goja.Object, response *network.Response) *goja.Object {
	urlValue := ""
	if response.URL != nil {
		urlValue = response.URL.String()
	}
	_ = object.DefineDataProperty("status", evaluator.vm.ToValue(response.StatusCode), goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE)
	_ = object.DefineDataProperty("statusText", evaluator.vm.ToValue(response.Status), goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE)
	_ = object.DefineDataProperty("url", evaluator.vm.ToValue(urlValue), goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE)
	_ = object.DefineDataProperty("ok", evaluator.vm.ToValue(response.StatusCode >= 200 && response.StatusCode <= 299), goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE)
	_ = object.DefineDataProperty("headers", evaluator.headersValue(response.Header), goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE)
	_ = object.Set("clone", func(goja.FunctionCall) goja.Value { return evaluator.responseValueFor(response) })
	_ = object.Set("text", func(goja.FunctionCall) goja.Value { return evaluator.resolvedPromise(string(response.Body)) })
	_ = object.Set("arrayBuffer", func(goja.FunctionCall) goja.Value {
		return evaluator.resolvedPromise(evaluator.vm.NewArrayBuffer(append([]byte(nil), response.Body...)))
	})
	return object
}

func (evaluator *fetchEvaluator) headersValue(header http.Header) *goja.Object {
	object := evaluator.vm.NewObject()
	_ = object.Set("get", func(call goja.FunctionCall) goja.Value {
		value := header.Get(call.Argument(0).String())
		if value == "" {
			return goja.Null()
		}
		return evaluator.vm.ToValue(value)
	})
	_ = object.Set("has", func(call goja.FunctionCall) goja.Value {
		return evaluator.vm.ToValue(header.Get(call.Argument(0).String()) != "")
	})
	return object
}

func (evaluator *fetchEvaluator) resolvedPromise(value any) goja.Value {
	promise, resolve, _ := evaluator.vm.NewPromise()
	_ = resolve(value)
	return evaluator.vm.ToValue(promise)
}

func (evaluator *fetchEvaluator) promiseAction(action func() (any, error)) goja.Value {
	promise, resolve, reject := evaluator.vm.NewPromise()
	value, err := action()
	if err != nil {
		_ = reject(evaluator.vm.NewTypeError(err.Error()))
	} else {
		_ = resolve(value)
	}
	return evaluator.vm.ToValue(promise)
}

func extractBody(value goja.Value) ([]byte, string, error) {
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		return nil, "", nil
	}
	var body []byte
	switch exported := value.Export().(type) {
	case goja.ArrayBuffer:
		body = append([]byte(nil), exported.Bytes()...)
	case []byte:
		body = append([]byte(nil), exported...)
	default:
		body = []byte(value.String())
	}
	if len(body) > maxSyntheticBodyBytes {
		return nil, "", fmt.Errorf("service worker response exceeds %d bytes", maxSyntheticBodyBytes)
	}
	return body, "text/plain;charset=UTF-8", nil
}

func headersFromValue(vm *goja.Runtime, value goja.Value) http.Header {
	header := make(http.Header)
	object := value.ToObject(vm)
	for _, name := range object.Keys() {
		header.Set(name, object.Get(name).String())
	}
	return header
}

func cloneRequest(source *network.Request) *network.Request {
	if source == nil {
		return nil
	}
	copy := *source
	copy.URL = cloneURL(source.URL)
	copy.SiteURL = cloneURL(source.SiteURL)
	copy.Header = source.Header.Clone()
	copy.Body = append([]byte(nil), source.Body...)
	return &copy
}

func cloneResponse(source *network.Response) *network.Response {
	if source == nil {
		return nil
	}
	copy := *source
	copy.URL = cloneURL(source.URL)
	copy.Header = source.Header.Clone()
	copy.Body = append([]byte(nil), source.Body...)
	return &copy
}
