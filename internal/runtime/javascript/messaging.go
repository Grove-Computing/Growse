package javascript

import (
	contextpkg "context"
	"errors"
	"fmt"

	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
	"github.com/dop251/goja"
)

const maxStructuredMessageBytes = 1 << 20

func (runtime *Runtime) installMessaging(vm *goja.Runtime) error {
	serializer, err := vm.RunString(`(function(value) {
		function clone(input, stack, depth) {
			if (depth > 64) throw new TypeError("structured clone depth limit exceeded");
			if (input === null || typeof input === "string" || typeof input === "boolean") return input;
			if (typeof input === "number") {
				if (!Number.isFinite(input)) throw new TypeError("structured clone requires finite numbers");
				return input;
			}
			if (typeof input !== "object") throw new TypeError("unsupported structured clone value");
			if (stack.indexOf(input) !== -1) throw new TypeError("cyclic structured clone value");
			stack.push(input);
			var output;
			if (Array.isArray(input)) {
				output = input.map(function(item) { return clone(item, stack, depth + 1); });
			} else {
				var prototype = Object.getPrototypeOf(input);
				if (prototype !== Object.prototype && prototype !== null) throw new TypeError("structured clone requires a plain object");
				output = {};
				Object.keys(input).forEach(function(key) { output[key] = clone(input[key], stack, depth + 1); });
			}
			stack.pop();
			return output;
		}
		return JSON.stringify(clone(value, [], 0));
	})`)
	if err != nil {
		return err
	}
	runtime.structuredClone, _ = goja.AssertFunction(serializer)
	if runtime.structuredClone == nil {
		return errors.New("structured clone serializer is unavailable")
	}
	return runtime.applyWindowContext(vm, runtime.environment.Window)
}

func (runtime *Runtime) applyWindowContext(vm *goja.Runtime, context runtimemodel.WindowContext) error {
	runtime.environment.Window = context
	global := vm.GlobalObject()
	parent := runtime.windowReferenceValue(vm, context.Parent)
	top := runtime.windowReferenceValue(vm, context.Top)
	if err := global.DefineDataProperty("parent", parent, goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE); err != nil {
		return err
	}
	if err := global.DefineDataProperty("top", top, goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE); err != nil {
		return err
	}
	children := make([]any, 0, len(context.Children))
	for _, child := range context.Children {
		children = append(children, runtime.windowReferenceValue(vm, child))
	}
	frames := vm.NewArray(children...)
	if err := global.DefineDataProperty("frames", frames, goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE); err != nil {
		return err
	}
	if err := global.DefineDataProperty("length", vm.ToValue(len(children)), goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE); err != nil {
		return err
	}
	runtime.installPostMessage(vm, global, context.Self)
	return nil
}

// UpdateWindow refreshes parent, top, and child WindowProxy relationships.
func (runtime *Runtime) UpdateWindow(context runtimemodel.WindowContext) {
	if runtime == nil {
		return
	}
	_ = runtime.runSync(contextpkg.Background(), func(vm *goja.Runtime) error {
		return runtime.applyWindowContext(vm, context)
	})
}

func (runtime *Runtime) installPostMessage(vm *goja.Runtime, object *goja.Object, target runtimemodel.WindowReference) {
	_ = object.Set("postMessage", func(call goja.FunctionCall) goja.Value {
		if runtime.environment.PostMessage == nil {
			panic(frameSecurityError(vm, "postMessage broker is unavailable"))
		}
		serialized, err := runtime.structuredClone(goja.Undefined(), call.Argument(0))
		if err != nil {
			panic(vm.NewTypeError(err.Error()))
		}
		payload := []byte(serialized.String())
		if len(payload) > maxStructuredMessageBytes {
			panic(vm.NewTypeError("window message exceeds 1 MiB"))
		}
		targetOrigin := "/"
		if !goja.IsUndefined(call.Argument(1)) {
			targetOrigin = call.Argument(1).String()
		}
		if err := runtime.environment.PostMessage(target, targetOrigin, payload); err != nil {
			panic(frameSecurityError(vm, err.Error()))
		}
		return goja.Undefined()
	})
}

func (runtime *Runtime) windowReferenceValue(vm *goja.Runtime, reference runtimemodel.WindowReference) *goja.Object {
	self := runtime.environment.Window.Self
	if reference.ID == self.ID && reference.Generation == self.Generation {
		return vm.GlobalObject()
	}
	if frame, ok := runtime.frameAccess[reference.ID]; ok && frame.Generation == reference.Generation {
		return runtime.frameWindowValue(vm, frame)
	}
	key := frameObjectKey{id: reference.ID, generation: reference.Generation}
	if existing := runtime.windowProxies[key]; existing != nil {
		return existing
	}
	proxy := vm.NewObject()
	_ = proxy.DefineDataProperty("closed", vm.ToValue(false), goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE)
	if reference.SameOrigin {
		location := vm.NewObject()
		_ = location.DefineDataProperty("href", vm.ToValue(reference.URL), goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE)
		_ = location.DefineDataProperty("origin", vm.ToValue(reference.Origin), goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE)
		_ = proxy.DefineDataProperty("location", location, goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE)
	}
	runtime.installPostMessage(vm, proxy, reference)
	runtime.windowProxies[key] = proxy
	return proxy
}

// DispatchMessage adds one message event to the serialized Page queue.
func (runtime *Runtime) DispatchMessage(event runtimemodel.MessageEvent) error {
	if len(event.Data) > maxStructuredMessageBytes {
		return errors.New("window message exceeds 1 MiB")
	}
	return runtime.runSync(contextpkg.Background(), func(vm *goja.Runtime) error {
		data, err := parseJSON(vm, string(event.Data))
		if err != nil {
			return fmt.Errorf("decode structured clone: %w", err)
		}
		object := vm.NewObject()
		_ = object.Set("type", "message")
		_ = object.Set("data", data)
		_ = object.Set("origin", event.Origin)
		_ = object.Set("source", runtime.windowReferenceValue(vm, event.Source))
		runtime.mu.Lock()
		listeners := append([]listenerRecord(nil), runtime.windowListeners...)
		runtime.mu.Unlock()
		for _, listener := range listeners {
			if listener.eventType != "message" {
				continue
			}
			callable, ok := goja.AssertFunction(listener.function)
			if !ok {
				continue
			}
			if _, err := callable(vm.GlobalObject(), object); err != nil {
				runtime.recordError(fmt.Sprintf("JavaScript message event handler: %v", err))
			}
		}
		runtime.drainMicrotasks(vm)
		return nil
	})
}
