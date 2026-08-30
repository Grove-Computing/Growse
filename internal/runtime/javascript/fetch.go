package javascript

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	fetchapi "github.com/Grove-Computing/Growse/internal/webapi/fetch"
	"github.com/dop251/goja"
)

func (runtime *Runtime) installFetch(vm *goja.Runtime) error {
	if err := runtime.installFetchInterfaces(vm); err != nil {
		return err
	}
	if err := vm.Set("fetch", func(call goja.FunctionCall) goja.Value {
		promise, resolve, reject := vm.NewPromise()
		request, err := runtime.fetchRequest(vm, call)
		if err != nil {
			_ = reject(fetchRejection(vm, err.Error()))
			return vm.ToValue(promise)
		}
		runtime.fetchAPI.Fetch(request, func(response fetchapi.Response) {
			if err := resolve(runtime.responseValue(vm, response)); err != nil {
				runtime.recordError(fmt.Sprintf("resolve JavaScript Fetch: %v", err))
			}
		}, func(message string) {
			if err := reject(fetchRejection(vm, message)); err != nil {
				runtime.recordError(fmt.Sprintf("reject JavaScript Fetch: %v", err))
			}
		})
		return vm.ToValue(promise)
	}); err != nil {
		return err
	}
	return runtime.installAbortController(vm)
}

func (runtime *Runtime) fetchRequest(vm *goja.Runtime, call goja.FunctionCall) (fetchapi.Request, error) {
	request := fetchapi.Request{URL: call.Argument(0).String()}
	initValue := call.Argument(1)
	if goja.IsUndefined(initValue) || goja.IsNull(initValue) {
		return request, nil
	}
	init := initValue.ToObject(vm)
	if value := init.Get("method"); value != nil && !goja.IsUndefined(value) {
		request.Method = value.String()
	}
	if value := init.Get("body"); value != nil && !goja.IsUndefined(value) && !goja.IsNull(value) {
		request.Text = value.String()
	}
	if value := init.Get("credentials"); value != nil && !goja.IsUndefined(value) {
		request.Credentials = fetchapi.CredentialsMode(value.String())
	}
	if value := init.Get("timeout"); value != nil && !goja.IsUndefined(value) {
		milliseconds := value.ToFloat()
		if math.IsNaN(milliseconds) || milliseconds < 0 {
			milliseconds = 0
		}
		if math.IsInf(milliseconds, 0) || milliseconds > float64((365*24*time.Hour)/time.Millisecond) {
			return fetchapi.Request{}, errors.New("fetch timeout exceeds the safety limit")
		}
		request.Timeout = time.Duration(milliseconds * float64(time.Millisecond))
	}
	if value := init.Get("headers"); value != nil && !goja.IsUndefined(value) && !goja.IsNull(value) {
		headers := fetchapi.NewHeaders()
		object := value.ToObject(vm)
		for _, name := range object.Keys() {
			if err := headers.Append(name, object.Get(name).String()); err != nil {
				return fetchapi.Request{}, err
			}
		}
		request.Headers = headers
	}
	if value := init.Get("signal"); value != nil && !goja.IsUndefined(value) && !goja.IsNull(value) {
		object, ok := value.(*goja.Object)
		if !ok || runtime.abortSignals[object] == nil {
			return fetchapi.Request{}, errors.New("fetch signal must be a Growse AbortSignal")
		}
		request.Signal = runtime.abortSignals[object]
	}
	return request, nil
}

func (runtime *Runtime) responseValue(vm *goja.Runtime, response fetchapi.Response) goja.Value {
	object := vm.NewObject()
	runtime.responseValues[object] = response
	defineReadOnly := func(name string, value goja.Value) {
		_ = object.DefineDataProperty(name, value, goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE)
	}
	defineReadOnly("status", vm.ToValue(response.Status))
	defineReadOnly("statusText", vm.ToValue(response.StatusText))
	defineReadOnly("url", vm.ToValue(response.URL))
	defineReadOnly("redirected", vm.ToValue(response.Redirected))
	defineReadOnly("ok", vm.ToValue(response.Status >= 200 && response.Status <= 299))
	defineReadOnly("headers", runtime.responseHeadersValue(vm, response.Headers))
	defineReadOnly("body", runtime.responseBodyValue(vm, response))
	bodyUsedGetter := vm.ToValue(func(goja.FunctionCall) goja.Value { return vm.ToValue(response.BodyUsed()) })
	_ = object.DefineAccessorProperty("bodyUsed", bodyUsedGetter, nil, goja.FLAG_FALSE, goja.FLAG_TRUE)
	_ = object.Set("text", func(goja.FunctionCall) goja.Value {
		return runtime.resolvedPromise(vm, func() (goja.Value, error) {
			value, err := response.Text()
			return vm.ToValue(value), err
		})
	})
	_ = object.Set("json", func(goja.FunctionCall) goja.Value {
		return runtime.resolvedPromise(vm, func() (goja.Value, error) {
			value, err := response.Text()
			if err != nil {
				return nil, err
			}
			return parseJSON(vm, value)
		})
	})
	_ = object.Set("arrayBuffer", func(goja.FunctionCall) goja.Value {
		return runtime.resolvedPromise(vm, func() (goja.Value, error) {
			value, err := response.Bytes()
			if err != nil {
				return nil, err
			}
			return vm.ToValue(vm.NewArrayBuffer(value)), nil
		})
	})
	return object
}

func (runtime *Runtime) installFetchInterfaces(vm *goja.Runtime) error {
	_, err := vm.RunString(`
		(function (global) {
			function illegal() { throw new TypeError("Illegal constructor"); }
			function ReadableStream() { illegal(); }
			function ReadableStreamDefaultReader() { illegal(); }
			Object.defineProperty(ReadableStream.prototype, Symbol.toStringTag, { value: "ReadableStream" });
			Object.defineProperty(ReadableStreamDefaultReader.prototype, Symbol.toStringTag, { value: "ReadableStreamDefaultReader" });
			global.ReadableStream = ReadableStream;
			global.ReadableStreamDefaultReader = ReadableStreamDefaultReader;
		})(globalThis);
	`)
	if err != nil {
		return fmt.Errorf("install Fetch stream interfaces: %w", err)
	}
	return nil
}

func (runtime *Runtime) responseBodyValue(vm *goja.Runtime, response fetchapi.Response) goja.Value {
	stream := vm.NewObject()
	if prototype := domInterfacePrototype(vm, "ReadableStream"); prototype != nil {
		_ = stream.SetPrototype(prototype)
	}
	locked := false
	_ = stream.DefineAccessorProperty("locked", vm.ToValue(func(goja.FunctionCall) goja.Value {
		return vm.ToValue(locked)
	}), nil, goja.FLAG_FALSE, goja.FLAG_TRUE)
	_ = stream.Set("getReader", func(goja.FunctionCall) goja.Value {
		if locked {
			panic(vm.NewTypeError("ReadableStream is locked"))
		}
		bodyReader, err := response.Stream()
		if err != nil {
			panic(vm.NewTypeError(err.Error()))
		}
		locked = true
		reader := vm.NewObject()
		if prototype := domInterfacePrototype(vm, "ReadableStreamDefaultReader"); prototype != nil {
			_ = reader.SetPrototype(prototype)
		}
		pending, released := false, false
		closedPromise, closeStream, failStream := vm.NewPromise()
		_ = reader.DefineDataProperty("closed", vm.ToValue(closedPromise), goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE)
		_ = reader.Set("read", func(goja.FunctionCall) goja.Value {
			promise, resolve, reject := vm.NewPromise()
			if released {
				_ = reject(vm.NewTypeError("ReadableStream reader was released"))
				return vm.ToValue(promise)
			}
			if pending {
				_ = reject(vm.NewTypeError("ReadableStream backpressure limit: one pending read"))
				return vm.ToValue(promise)
			}
			pending = true
			if !runtime.enqueueCallback(func() {
				pending = false
				chunk, done, readErr := bodyReader.Read()
				if readErr != nil {
					_ = reject(vm.NewTypeError(readErr.Error()))
					_ = failStream(vm.NewTypeError(readErr.Error()))
					return
				}
				result := vm.NewObject()
				_ = result.Set("done", done)
				if done {
					_ = result.Set("value", goja.Undefined())
					_ = closeStream(goja.Undefined())
				} else {
					arrayBuffer := vm.NewArrayBuffer(chunk)
					constructor, _ := vm.Get("Uint8Array").(*goja.Object)
					value, typedErr := vm.New(constructor, vm.ToValue(arrayBuffer))
					if typedErr != nil {
						_ = reject(vm.NewTypeError(typedErr.Error()))
						_ = failStream(vm.NewTypeError(typedErr.Error()))
						return
					}
					_ = result.Set("value", value)
				}
				_ = resolve(result)
			}) {
				pending = false
				_ = reject(fetchRejection(vm, "AbortError: Page task queue is closed"))
			}
			return vm.ToValue(promise)
		})
		_ = reader.Set("cancel", func(goja.FunctionCall) goja.Value {
			bodyReader.Cancel()
			_ = closeStream(goja.Undefined())
			return runtime.resolvedPromise(vm, func() (goja.Value, error) { return goja.Undefined(), nil })
		})
		_ = reader.Set("releaseLock", func(goja.FunctionCall) goja.Value {
			if pending {
				panic(vm.NewTypeError("cannot release a reader with a pending read"))
			}
			if !released {
				released = true
				locked = false
				bodyReader.Release()
				_ = failStream(vm.NewTypeError("ReadableStream reader was released"))
			}
			return goja.Undefined()
		})
		return reader
	})
	return stream
}

func (runtime *Runtime) resolvedPromise(vm *goja.Runtime, action func() (goja.Value, error)) goja.Value {
	promise, resolve, reject := vm.NewPromise()
	value, err := action()
	if err != nil {
		_ = reject(vm.NewTypeError(err.Error()))
	} else {
		_ = resolve(value)
	}
	return vm.ToValue(promise)
}

func (runtime *Runtime) responseHeadersValue(vm *goja.Runtime, headers *fetchapi.ResponseHeaders) goja.Value {
	object := vm.NewObject()
	_ = object.Set("get", func(call goja.FunctionCall) goja.Value {
		value, ok := headers.Get(call.Argument(0).String())
		if !ok {
			return goja.Null()
		}
		return vm.ToValue(value)
	})
	_ = object.Set("has", func(call goja.FunctionCall) goja.Value {
		_, ok := headers.Get(call.Argument(0).String())
		return vm.ToValue(ok)
	})
	_ = object.Set("entries", func(goja.FunctionCall) goja.Value {
		entries := headers.Entries()
		values := make([]any, 0, len(entries))
		for _, entry := range entries {
			values = append(values, []string{entry.Name, entry.Value})
		}
		return vm.ToValue(values)
	})
	return object
}

func (runtime *Runtime) installAbortController(vm *goja.Runtime) error {
	return vm.Set("AbortController", func(call goja.ConstructorCall) *goja.Object {
		controller := fetchapi.NewAbortController()
		signal := vm.NewObject()
		runtime.abortSignals[signal] = controller.Signal()
		abortedGetter := vm.ToValue(func(goja.FunctionCall) goja.Value {
			return vm.ToValue(controller.Signal().Aborted())
		})
		_ = signal.DefineAccessorProperty("aborted", abortedGetter, nil, goja.FLAG_FALSE, goja.FLAG_TRUE)
		_ = call.This.DefineDataProperty("signal", signal, goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE)
		_ = call.This.Set("abort", func(goja.FunctionCall) goja.Value {
			controller.Abort()
			return goja.Undefined()
		})
		return call.This
	})
}

func parseJSON(vm *goja.Runtime, source string) (goja.Value, error) {
	jsonObject := vm.Get("JSON").ToObject(vm)
	parse, ok := goja.AssertFunction(jsonObject.Get("parse"))
	if !ok {
		return nil, errors.New("JSON.parse is unavailable")
	}
	value, err := parse(jsonObject, vm.ToValue(source))
	if err != nil {
		return nil, fmt.Errorf("parse Fetch JSON: %w", err)
	}
	return value, nil
}

func fetchErrorName(message string) string {
	if index := strings.IndexByte(message, ':'); index > 0 {
		return message[:index]
	}
	return "TypeError"
}

func fetchRejection(vm *goja.Runtime, message string) *goja.Object {
	name := fetchErrorName(message)
	detail := message
	if name != "TypeError" {
		detail = strings.TrimSpace(strings.TrimPrefix(message, name+":"))
	}
	result := vm.NewTypeError(detail)
	_ = result.Set("name", name)
	return result
}
