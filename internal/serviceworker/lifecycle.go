package serviceworker

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/Grove-Computing/Growse/internal/network"
	"github.com/dop251/goja"
)

const maxLifecycleEventTime = 5 * time.Second

type lifecycleResult struct {
	skipWaiting bool
	claim       bool
}

func evaluateLifecycle(ctx context.Context, source []byte, activateWithoutWaiting bool, workerURL *url.URL, caches *CacheStorage, fallback NetworkFallback) (lifecycleResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	vm := goja.New()
	timer := time.AfterFunc(maxLifecycleEventTime, func() { vm.Interrupt("service worker lifecycle timeout") })
	defer timer.Stop()
	stopContext := context.AfterFunc(ctx, func() { vm.Interrupt(ctx.Err()) })
	defer stopContext()
	global := vm.GlobalObject()
	listeners := map[string][]goja.Callable{}
	result := lifecycleResult{}
	helpers := &fetchEvaluator{
		vm: vm, workerURL: cloneURL(workerURL), origin: originString(workerURL), fallback: fallback, cacheStorage: caches, ctx: ctx,
		responses: make(map[*goja.Object]*network.Response), requests: make(map[*goja.Object]*network.Request),
	}
	_ = global.Set("self", global)
	_ = global.Set("skipWaiting", func(goja.FunctionCall) goja.Value {
		result.skipWaiting = true
		return helpers.resolvedPromise(goja.Undefined())
	})
	_ = global.Set("addEventListener", func(call goja.FunctionCall) goja.Value {
		listener, ok := goja.AssertFunction(call.Argument(1))
		if !ok {
			panic(vm.NewTypeError("Service Worker listener must be a function"))
		}
		listeners[call.Argument(0).String()] = append(listeners[call.Argument(0).String()], listener)
		return goja.Undefined()
	})
	clients := vm.NewObject()
	_ = clients.Set("claim", func(goja.FunctionCall) goja.Value {
		result.claim = true
		return helpers.resolvedPromise(goja.Undefined())
	})
	_ = global.Set("clients", clients)
	if err := helpers.installFetchPrimitives(global); err != nil {
		return lifecycleResult{}, err
	}
	if _, err := vm.RunString(string(source)); err != nil {
		return lifecycleResult{}, fmt.Errorf("evaluate service worker script: %w", err)
	}
	dispatch := func(eventType string) error {
		event := vm.NewObject()
		var promises []goja.Value
		dispatching := true
		_ = event.Set("type", eventType)
		_ = event.Set("waitUntil", func(call goja.FunctionCall) goja.Value {
			if !dispatching {
				panic(vm.NewTypeError("waitUntil must be called while dispatching a lifecycle event"))
			}
			promises = append(promises, call.Argument(0))
			return goja.Undefined()
		})
		for _, listener := range listeners[eventType] {
			if _, err := listener(global, event); err != nil {
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
	if activateWithoutWaiting || result.skipWaiting {
		if err := dispatch("activate"); err != nil {
			return lifecycleResult{}, err
		}
	}
	return result, nil
}
