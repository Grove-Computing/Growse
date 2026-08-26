package serviceworker

import (
	"context"
	"fmt"
	"time"

	"github.com/dop251/goja"
)

const maxLifecycleEventTime = 5 * time.Second

type lifecycleResult struct {
	skipWaiting bool
	claim       bool
}

func evaluateLifecycle(ctx context.Context, source []byte, activateWithoutWaiting bool) (lifecycleResult, error) {
	vm := goja.New()
	timer := time.AfterFunc(maxLifecycleEventTime, func() { vm.Interrupt("service worker lifecycle timeout") })
	defer timer.Stop()
	stopContext := context.AfterFunc(ctx, func() { vm.Interrupt(ctx.Err()) })
	defer stopContext()
	global := vm.GlobalObject()
	listeners := map[string][]goja.Callable{}
	result := lifecycleResult{}
	_ = global.Set("self", global)
	_ = global.Set("skipWaiting", func(goja.FunctionCall) goja.Value {
		result.skipWaiting = true
		return goja.Undefined()
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
		return goja.Undefined()
	})
	_ = global.Set("clients", clients)
	if _, err := vm.RunString(string(source)); err != nil {
		return lifecycleResult{}, fmt.Errorf("evaluate service worker script: %w", err)
	}
	dispatch := func(eventType string) error {
		event := vm.NewObject()
		_ = event.Set("type", eventType)
		_ = event.Set("waitUntil", func(call goja.FunctionCall) goja.Value { return call.Argument(0) })
		for _, listener := range listeners[eventType] {
			if _, err := listener(global, event); err != nil {
				return fmt.Errorf("dispatch service worker %s event: %w", eventType, err)
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
