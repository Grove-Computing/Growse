package javascript

import (
	"fmt"

	"github.com/dop251/goja"
)

func (runtime *Runtime) installGlobals(vm *goja.Runtime) error {
	global := vm.GlobalObject()
	for _, name := range []string{"window", "self", "globalThis"} {
		if err := global.DefineDataProperty(name, global, goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_FALSE); err != nil {
			return err
		}
	}

	navigator := vm.NewObject()
	properties := map[string]goja.Value{
		"userAgent":           vm.ToValue("Growse/0.14"),
		"platform":            vm.ToValue(""),
		"language":            vm.ToValue("en-US"),
		"hardwareConcurrency": vm.ToValue(1),
		"onLine":              vm.ToValue(true),
		"webdriver":           vm.ToValue(false),
	}
	for name, value := range properties {
		if err := navigator.DefineDataProperty(name, value, goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE); err != nil {
			return err
		}
	}
	languagesGetter := vm.ToValue(func(goja.FunctionCall) goja.Value { return vm.NewArray("en-US") })
	if err := navigator.DefineAccessorProperty("languages", languagesGetter, nil, goja.FLAG_FALSE, goja.FLAG_TRUE); err != nil {
		return err
	}
	if err := global.DefineDataProperty("navigator", navigator, goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE); err != nil {
		return err
	}
	origin := "null"
	if !runtime.environment.FramePolicy.HasOpaqueOrigin() && runtime.navigationAPI != nil {
		if current := runtime.navigationAPI.Current().Origin; current != "" {
			origin = current
		}
	}
	if err := global.DefineDataProperty("origin", vm.ToValue(origin), goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE); err != nil {
		return err
	}
	if err := global.DefineDataProperty("open", vm.ToValue(func(goja.FunctionCall) goja.Value {
		if !runtime.environment.FramePolicy.AllowsPopups() {
			panic(frameSecurityError(vm, "iframe sandbox blocks popup creation"))
		}
		return goja.Null()
	}), goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE); err != nil {
		return err
	}
	return global.DefineDataProperty("queueMicrotask", vm.ToValue(func(call goja.FunctionCall) goja.Value {
		if _, ok := goja.AssertFunction(call.Argument(0)); !ok {
			panic(vm.NewTypeError("queueMicrotask callback must be a function"))
		}
		runtime.mu.Lock()
		if len(runtime.microtasks) >= runtime.maxMicrotasks {
			runtime.mu.Unlock()
			panic(vm.NewTypeError("microtask queue limit exceeded"))
		}
		runtime.microtasks = append(runtime.microtasks, call.Argument(0))
		runtime.mu.Unlock()
		return goja.Undefined()
	}), goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE)
}

func (runtime *Runtime) drainMicrotasks(vm *goja.Runtime) {
	for count := 0; count < runtime.maxMicrotasks; count++ {
		runtime.mu.Lock()
		if len(runtime.microtasks) == 0 || runtime.stopped || runtime.runtimeCtx == nil || runtime.runtimeCtx.Err() != nil {
			runtime.mu.Unlock()
			return
		}
		value := runtime.microtasks[0]
		runtime.microtasks[0] = nil
		runtime.microtasks = runtime.microtasks[1:]
		runtime.mu.Unlock()
		callback, ok := goja.AssertFunction(value)
		if !ok {
			continue
		}
		if _, err := callback(vm.GlobalObject()); err != nil {
			runtime.recordError(fmt.Sprintf("JavaScript microtask callback: %v", err))
		}
	}
	runtime.mu.Lock()
	overflow := len(runtime.microtasks) != 0
	if overflow {
		runtime.microtasks = nil
	}
	runtime.mu.Unlock()
	if overflow {
		runtime.recordError("JavaScript microtask checkpoint limit exceeded")
	}
}
