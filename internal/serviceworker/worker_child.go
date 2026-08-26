package serviceworker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"runtime/debug"
	"sync"
	"time"

	"github.com/Grove-Computing/Growse/internal/network"
	"github.com/Grove-Computing/Growse/internal/runtime/isolated"
	"github.com/Grove-Computing/Growse/internal/runtime/workerproto"
	"github.com/dop251/goja"
)

const (
	serviceWorkerEnvironmentKey = "GROWSE_SERVICE_WORKER"
	maxServiceWorkerHeapBytes   = 256 << 20
)

type serviceWorkerChild struct {
	peer        *workerPeer
	blobs       *workerBlobStore
	constraints []string
	mu          sync.Mutex
	runtime     *serviceWorkerScriptRuntime
	stop        chan struct{}
	stopOnce    sync.Once
}

type serviceWorkerScriptRuntime struct {
	evaluator *fetchEvaluator
	lifecycle map[string][]goja.Callable
	result    lifecycleResult
}

func init() {
	if os.Getenv(serviceWorkerEnvironmentKey) != "1" {
		return
	}
	if err := runServiceWorkerChild(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "service worker failed: %v\n", err)
		os.Exit(70)
	}
	os.Exit(0)
}

func runServiceWorkerChild() error {
	debug.SetMemoryLimit(maxServiceWorkerHeapBytes)
	constraints, err := isolated.ApplyAuxiliaryWorkerSandbox()
	if err != nil {
		return err
	}
	peer := newPausedWorkerPeer(os.Stdin, os.Stdout)
	child := &serviceWorkerChild{peer: peer, blobs: newWorkerBlobStore(peer), constraints: constraints, stop: make(chan struct{})}
	peer.handleRequest("worker.load", child.load)
	peer.handleRequest("worker.lifecycle", child.lifecycle)
	peer.handleRequest("worker.fetch", child.fetch)
	peer.handleRequest("worker.stop", child.stopWorker)
	peer.start()
	select {
	case <-peer.done:
	case <-child.stop:
	}
	return nil
}

func (child *serviceWorkerChild) stopWorker(context.Context, json.RawMessage) (any, error) {
	child.stopOnce.Do(func() {
		time.AfterFunc(time.Millisecond, func() { close(child.stop) })
	})
	return nil, nil
}

func (child *serviceWorkerChild) load(_ context.Context, payload json.RawMessage) (any, error) {
	var request workerLoadRequest
	if err := workerproto.DecodePayload(payload, &request); err != nil {
		return nil, err
	}
	workerURL, err := url.Parse(request.ScriptURL)
	if err != nil || !IsSecureContext(workerURL) || workerURL.User != nil {
		return nil, errors.New("service worker script URL is invalid")
	}
	source, err := child.blobs.take(request.Source)
	if err != nil || len(source) > MaxWorkerScriptBytes {
		return nil, errors.New("service worker script transfer is invalid")
	}
	child.mu.Lock()
	defer child.mu.Unlock()
	if child.runtime != nil {
		return nil, errors.New("service worker is already loaded")
	}
	runtime, err := newServiceWorkerScriptRuntime(workerURL, child.remoteHost)
	if err != nil {
		return nil, err
	}
	if _, err := runtime.evaluator.vm.RunScript(workerURL.String(), string(source)); err != nil {
		return nil, fmt.Errorf("evaluate service worker script: %w", err)
	}
	child.runtime = runtime
	return workerLoadResponse{Constraints: append([]string(nil), child.constraints...)}, nil
}

func (child *serviceWorkerChild) lifecycle(ctx context.Context, payload json.RawMessage) (any, error) {
	var request workerLifecycleRequest
	if err := workerproto.DecodePayload(payload, &request); err != nil {
		return nil, err
	}
	child.mu.Lock()
	defer child.mu.Unlock()
	if child.runtime == nil {
		return nil, errors.New("service worker is not loaded")
	}
	result, err := child.runtime.dispatchLifecycle(ctx, request.Activate)
	return workerLifecycleResponse{SkipWaiting: result.skipWaiting, Claim: result.claim}, err
}

func (child *serviceWorkerChild) fetch(ctx context.Context, payload json.RawMessage) (any, error) {
	var request workerFetchRequest
	if err := workerproto.DecodePayload(payload, &request); err != nil {
		return nil, err
	}
	networkRequest, err := requestFromWire(child.blobs, request.Request)
	if err != nil {
		return nil, err
	}
	child.mu.Lock()
	defer child.mu.Unlock()
	if child.runtime == nil {
		return nil, errors.New("service worker is not loaded")
	}
	response, err := child.runtime.evaluator.dispatch(ctx, networkRequest)
	if err != nil {
		return nil, err
	}
	wireResponse, err := responseToWire(ctx, child.blobs, response)
	return workerFetchResponse{Response: wireResponse}, err
}

func newServiceWorkerScriptRuntime(workerURL *url.URL, host func(*fetchEvaluator) (NetworkFallback, scriptCacheStorage)) (*serviceWorkerScriptRuntime, error) {
	vm := goja.New()
	evaluator := &fetchEvaluator{
		vm: vm, workerURL: cloneURL(workerURL), responses: make(map[*goja.Object]*network.Response), requests: make(map[*goja.Object]*network.Request),
	}
	evaluator.fallback, evaluator.caches = host(evaluator)
	runtime := &serviceWorkerScriptRuntime{evaluator: evaluator, lifecycle: make(map[string][]goja.Callable)}
	global := vm.GlobalObject()
	_ = global.Set("self", global)
	_ = global.Set("addEventListener", func(call goja.FunctionCall) goja.Value {
		listener, ok := goja.AssertFunction(call.Argument(1))
		if !ok {
			panic(vm.NewTypeError("Service Worker listener must be a function"))
		}
		eventType := call.Argument(0).String()
		if eventType == "fetch" {
			evaluator.listeners = append(evaluator.listeners, listener)
		} else if eventType == "install" || eventType == "activate" {
			runtime.lifecycle[eventType] = append(runtime.lifecycle[eventType], listener)
		}
		return goja.Undefined()
	})
	_ = global.Set("skipWaiting", func(goja.FunctionCall) goja.Value {
		runtime.result.skipWaiting = true
		return evaluator.resolvedPromise(goja.Undefined())
	})
	clients := vm.NewObject()
	_ = clients.Set("claim", func(goja.FunctionCall) goja.Value {
		runtime.result.claim = true
		return evaluator.resolvedPromise(goja.Undefined())
	})
	_ = global.Set("clients", clients)
	if err := evaluator.installFetchPrimitives(global); err != nil {
		return nil, err
	}
	return runtime, nil
}

func (runtime *serviceWorkerScriptRuntime) dispatchLifecycle(_ context.Context, activateWithoutWaiting bool) (lifecycleResult, error) {
	runtime.result = lifecycleResult{}
	dispatch := func(eventType string) error {
		event := runtime.evaluator.vm.NewObject()
		var promises []goja.Value
		dispatching := true
		_ = event.Set("type", eventType)
		_ = event.Set("waitUntil", func(call goja.FunctionCall) goja.Value {
			if !dispatching {
				panic(runtime.evaluator.vm.NewTypeError("waitUntil must be called while dispatching a lifecycle event"))
			}
			promises = append(promises, call.Argument(0))
			return goja.Undefined()
		})
		for _, listener := range runtime.lifecycle[eventType] {
			if _, err := listener(runtime.evaluator.vm.GlobalObject(), event); err != nil {
				dispatching = false
				return fmt.Errorf("dispatch service worker %s event: %w", eventType, err)
			}
		}
		dispatching = false
		for _, promise := range promises {
			if _, err := settledValue(promise); err != nil {
				return fmt.Errorf("service worker %s waitUntil rejected: %w", eventType, err)
			}
		}
		return nil
	}
	if err := dispatch("install"); err != nil {
		return lifecycleResult{}, err
	}
	if activateWithoutWaiting || runtime.result.skipWaiting {
		if err := dispatch("activate"); err != nil {
			return lifecycleResult{}, err
		}
	}
	return runtime.result, nil
}

func (child *serviceWorkerChild) remoteHost(evaluator *fetchEvaluator) (NetworkFallback, scriptCacheStorage) {
	host := &remoteScriptHost{peer: child.peer, blobs: child.blobs, context: func() context.Context {
		if evaluator.ctx != nil {
			return evaluator.ctx
		}
		return context.Background()
	}}
	return host.fetch, host
}

type remoteScriptHost struct {
	peer    *workerPeer
	blobs   *workerBlobStore
	context func() context.Context
}

func (host *remoteScriptHost) fetch(ctx context.Context, request *network.Request) (*network.Response, error) {
	wireRequest, err := requestToWire(ctx, host.blobs, request)
	if err != nil {
		return nil, err
	}
	var result workerFetchResponse
	if err := host.peer.call(ctx, "host.network", workerFetchRequest{Request: wireRequest}, &result); err != nil {
		return nil, err
	}
	return responseFromWire(host.blobs, result.Response)
}

func (host *remoteScriptHost) Open(name string) (scriptCache, error) {
	if err := host.peer.call(host.context(), "host.cache.open", workerCacheNameRequest{Name: name}, nil); err != nil {
		return nil, err
	}
	return &remoteScriptCache{host: host, name: name}, nil
}

func (host *remoteScriptHost) Match(request *network.Request) (*network.Response, bool, error) {
	return host.match("", request)
}

func (host *remoteScriptHost) Has(name string) (bool, error) {
	var result workerBoolResponse
	err := host.peer.call(host.context(), "host.cache.has", workerCacheNameRequest{Name: name}, &result)
	return result.Value, err
}

func (host *remoteScriptHost) Delete(name string) (bool, error) {
	var result workerBoolResponse
	err := host.peer.call(host.context(), "host.cache.delete", workerCacheNameRequest{Name: name}, &result)
	return result.Value, err
}

func (host *remoteScriptHost) Keys() ([]string, error) {
	var result workerCacheNamesResponse
	err := host.peer.call(host.context(), "host.cache.keys", nil, &result)
	return result.Names, err
}

func (host *remoteScriptHost) match(name string, request *network.Request) (*network.Response, bool, error) {
	wireRequest, err := requestToWire(host.context(), host.blobs, request)
	if err != nil {
		return nil, false, err
	}
	var result workerCacheMatchResponse
	if err := host.peer.call(host.context(), "host.cache.match", workerCacheRequest{Name: name, Request: wireRequest}, &result); err != nil {
		return nil, false, err
	}
	response, err := responseFromWire(host.blobs, result.Response)
	return response, result.Response.Found, err
}

type remoteScriptCache struct {
	host *remoteScriptHost
	name string
}

func (cache *remoteScriptCache) Match(request *network.Request) (*network.Response, bool, error) {
	return cache.host.match(cache.name, request)
}

func (cache *remoteScriptCache) Put(request *network.Request, response *network.Response) error {
	ctx := cache.host.context()
	wireRequest, err := requestToWire(ctx, cache.host.blobs, request)
	if err != nil {
		return err
	}
	wireResponse, err := responseToWire(ctx, cache.host.blobs, response)
	if err != nil {
		return err
	}
	return cache.host.peer.call(ctx, "host.cache.put", workerCachePutRequest{Name: cache.name, Request: wireRequest, Response: wireResponse}, nil)
}

func (cache *remoteScriptCache) Delete(request *network.Request) (bool, error) {
	ctx := cache.host.context()
	wireRequest, err := requestToWire(ctx, cache.host.blobs, request)
	if err != nil {
		return false, err
	}
	var result workerBoolResponse
	err = cache.host.peer.call(ctx, "host.cache.entry-delete", workerCacheRequest{Name: cache.name, Request: wireRequest}, &result)
	return result.Value, err
}

func (cache *remoteScriptCache) Keys() ([]*network.Request, error) {
	var result workerCacheKeysResponse
	if err := cache.host.peer.call(cache.host.context(), "host.cache.entry-keys", workerCacheNameRequest{Name: cache.name}, &result); err != nil {
		return nil, err
	}
	requests := make([]*network.Request, 0, len(result.Requests))
	for _, item := range result.Requests {
		request, err := requestFromWire(cache.host.blobs, item)
		if err != nil {
			return nil, err
		}
		requests = append(requests, request)
	}
	return requests, nil
}
