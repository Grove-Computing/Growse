package javascript

import (
	"errors"
	"fmt"
	"mime"
	"strings"

	"github.com/dop251/goja"
)

func (api *wasmAPI) installStreaming(vm *goja.Runtime, namespace *goja.Object) error {
	if err := namespace.Set("compileStreaming", func(call goja.FunctionCall) goja.Value {
		return api.streamingPromise(vm, call.Argument(0), func(source []byte) (goja.Value, error) {
			module, err := api.compile(source)
			if err != nil {
				return nil, fmt.Errorf("WebAssembly.CompileError: %w", err)
			}
			return api.moduleObject(vm, module), nil
		})
	}); err != nil {
		return err
	}
	return namespace.Set("instantiateStreaming", func(call goja.FunctionCall) goja.Value {
		imports := call.Argument(1)
		return api.streamingPromise(vm, call.Argument(0), func(source []byte) (goja.Value, error) {
			module, err := api.compile(source)
			if err != nil {
				return nil, fmt.Errorf("WebAssembly.CompileError: %w", err)
			}
			instance, err := api.instantiate(vm, module, imports)
			if err != nil {
				return nil, fmt.Errorf("WebAssembly.LinkError: %w", err)
			}
			result := vm.NewObject()
			_ = result.Set("module", api.moduleObject(vm, module))
			_ = result.Set("instance", api.instanceObject(vm, instance, module))
			return result, nil
		})
	})
}

func (api *wasmAPI) streamingPromise(vm *goja.Runtime, source goja.Value, action func([]byte) (goja.Value, error)) goja.Value {
	promise, resolve, reject := vm.NewPromise()
	fulfill := vm.ToValue(func(call goja.FunctionCall) goja.Value {
		binary, err := api.streamingResponse(call.Argument(0))
		if err == nil {
			var value goja.Value
			value, err = action(binary)
			if err == nil {
				_ = resolve(value)
			}
		}
		if err != nil {
			_ = reject(vm.NewTypeError(err.Error()))
		}
		return goja.Undefined()
	})
	rejectSource := vm.ToValue(func(call goja.FunctionCall) goja.Value {
		_ = reject(call.Argument(0))
		return goja.Undefined()
	})

	if object, ok := source.(*goja.Object); ok {
		if _, response := api.responses[object]; response {
			callback, _ := goja.AssertFunction(fulfill)
			_, _ = callback(goja.Undefined(), source)
			return vm.ToValue(promise)
		}
		if then, ok := goja.AssertFunction(object.Get("then")); ok {
			if _, err := then(object, fulfill, rejectSource); err != nil {
				_ = reject(vm.NewTypeError(err.Error()))
			}
			return vm.ToValue(promise)
		}
	}
	_ = reject(vm.NewTypeError("WebAssembly streaming source must be a Fetch Response or Promise<Response>"))
	return vm.ToValue(promise)
}

func (api *wasmAPI) streamingResponse(value goja.Value) ([]byte, error) {
	object, ok := value.(*goja.Object)
	if !ok {
		return nil, errors.New("WebAssembly streaming source did not resolve to a Fetch Response")
	}
	response, ok := api.responses[object]
	if !ok {
		return nil, errors.New("WebAssembly streaming source did not resolve to a Growse Fetch Response")
	}
	contentType, ok := response.Headers.Get("Content-Type")
	if !ok {
		return nil, errors.New("WebAssembly streaming response has no Content-Type")
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil || !strings.EqualFold(mediaType, "application/wasm") {
		return nil, fmt.Errorf("WebAssembly streaming response MIME must be application/wasm, got %q", contentType)
	}
	binary, err := response.Bytes()
	if err != nil {
		return nil, fmt.Errorf("consume WebAssembly streaming response: %w", err)
	}
	return binary, nil
}
