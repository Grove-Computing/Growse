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
	if err := vm.Set("fetch", func(call goja.FunctionCall) goja.Value {
		request, err := runtime.fetchRequest(vm, call)
		if err != nil {
			panic(vm.NewTypeError(err.Error()))
		}
		promise, resolve, reject := vm.NewPromise()
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
			return fetchapi.Request{}, errors.New("Fetch timeout exceeds the safety limit")
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
			return fetchapi.Request{}, errors.New("Fetch signal must be a Growse AbortSignal")
		}
		request.Signal = runtime.abortSignals[object]
	}
	return request, nil
}

func (runtime *Runtime) responseValue(vm *goja.Runtime, response fetchapi.Response) goja.Value {
	object := vm.NewObject()
	defineReadOnly := func(name string, value goja.Value) {
		_ = object.DefineDataProperty(name, value, goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE)
	}
	defineReadOnly("status", vm.ToValue(response.Status))
	defineReadOnly("statusText", vm.ToValue(response.StatusText))
	defineReadOnly("url", vm.ToValue(response.URL))
	defineReadOnly("redirected", vm.ToValue(response.Redirected))
	defineReadOnly("ok", vm.ToValue(response.Status >= 200 && response.Status <= 299))
	defineReadOnly("headers", runtime.responseHeadersValue(vm, response.Headers))
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
	return object
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
