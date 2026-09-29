package javascript

import (
	"fmt"
	"strings"
	"time"

	fetchapi "github.com/Grove-Computing/Growse/internal/webapi/fetch"
	"github.com/dop251/goja"
)

const (
	xhrUnsent = iota
	xhrOpened
	xhrHeadersReceived
	xhrLoading
	xhrDone
)

type xhrState struct {
	method, url, responseType string
	requestHeaders            *fetchapi.Headers
	responseHeaders           *fetchapi.ResponseHeaders
	controller                *fetchapi.AbortController
	readyState, status        int
	statusText, responseURL   string
	responseText              string
	response                  goja.Value
	timeout                   time.Duration
	withCredentials           bool
	sent, aborted             bool
	listeners                 map[string][]goja.Value
}

func (runtime *Runtime) installXMLHttpRequest(vm *goja.Runtime) error {
	constructor := func(call goja.ConstructorCall) *goja.Object {
		object := call.This
		state := &xhrState{readyState: xhrUnsent, requestHeaders: fetchapi.NewHeaders(), listeners: make(map[string][]goja.Value), response: goja.Null()}
		for _, name := range []string{"onabort", "onerror", "onload", "onloadend", "onloadstart", "onprogress", "onreadystatechange", "ontimeout"} {
			_ = object.Set(name, goja.Null())
		}
		_ = object.Set("upload", vm.NewObject())
		_ = object.DefineAccessorProperty("readyState", vm.ToValue(func(goja.FunctionCall) goja.Value { return vm.ToValue(state.readyState) }), nil, goja.FLAG_FALSE, goja.FLAG_TRUE)
		_ = object.DefineAccessorProperty("status", vm.ToValue(func(goja.FunctionCall) goja.Value { return vm.ToValue(state.status) }), nil, goja.FLAG_FALSE, goja.FLAG_TRUE)
		_ = object.DefineAccessorProperty("statusText", vm.ToValue(func(goja.FunctionCall) goja.Value { return vm.ToValue(state.statusText) }), nil, goja.FLAG_FALSE, goja.FLAG_TRUE)
		_ = object.DefineAccessorProperty("responseURL", vm.ToValue(func(goja.FunctionCall) goja.Value { return vm.ToValue(state.responseURL) }), nil, goja.FLAG_FALSE, goja.FLAG_TRUE)
		_ = object.DefineAccessorProperty("responseText", vm.ToValue(func(goja.FunctionCall) goja.Value { return vm.ToValue(state.responseText) }), nil, goja.FLAG_FALSE, goja.FLAG_TRUE)
		_ = object.DefineAccessorProperty("response", vm.ToValue(func(goja.FunctionCall) goja.Value { return state.response }), nil, goja.FLAG_FALSE, goja.FLAG_TRUE)
		_ = object.DefineAccessorProperty("responseType", vm.ToValue(func(goja.FunctionCall) goja.Value { return vm.ToValue(state.responseType) }), vm.ToValue(func(call goja.FunctionCall) goja.Value {
			state.responseType = strings.ToLower(call.Argument(0).String())
			return goja.Undefined()
		}), goja.FLAG_FALSE, goja.FLAG_TRUE)
		_ = object.DefineAccessorProperty("timeout", vm.ToValue(func(goja.FunctionCall) goja.Value {
			return vm.ToValue(float64(state.timeout) / float64(time.Millisecond))
		}), vm.ToValue(func(call goja.FunctionCall) goja.Value {
			state.timeout = time.Duration(call.Argument(0).ToFloat() * float64(time.Millisecond))
			return goja.Undefined()
		}), goja.FLAG_FALSE, goja.FLAG_TRUE)
		_ = object.DefineAccessorProperty("withCredentials", vm.ToValue(func(goja.FunctionCall) goja.Value { return vm.ToValue(state.withCredentials) }), vm.ToValue(func(call goja.FunctionCall) goja.Value {
			state.withCredentials = call.Argument(0).ToBoolean()
			return goja.Undefined()
		}), goja.FLAG_FALSE, goja.FLAG_TRUE)

		dispatch := func(eventType string) {
			event := vm.NewObject()
			_ = event.Set("type", eventType)
			_ = event.Set("target", object)
			_ = event.Set("currentTarget", object)
			if callback, ok := goja.AssertFunction(object.Get("on" + eventType)); ok {
				if _, err := callback(object, event); err != nil {
					runtime.recordError(fmt.Sprintf("XMLHttpRequest %s handler: %v", eventType, err))
				}
			}
			for _, value := range append([]goja.Value(nil), state.listeners[eventType]...) {
				if callback, ok := goja.AssertFunction(value); ok {
					if _, err := callback(object, event); err != nil {
						runtime.recordError(fmt.Sprintf("XMLHttpRequest %s listener: %v", eventType, err))
					}
				}
			}
		}
		setReadyState := func(value int) { state.readyState = value; dispatch("readystatechange") }
		_ = object.Set("addEventListener", func(call goja.FunctionCall) goja.Value {
			eventType := strings.ToLower(call.Argument(0).String())
			if _, ok := goja.AssertFunction(call.Argument(1)); ok {
				state.listeners[eventType] = append(state.listeners[eventType], call.Argument(1))
			}
			return goja.Undefined()
		})
		_ = object.Set("removeEventListener", func(call goja.FunctionCall) goja.Value {
			eventType, target := strings.ToLower(call.Argument(0).String()), call.Argument(1)
			filtered := state.listeners[eventType][:0]
			for _, value := range state.listeners[eventType] {
				if !value.SameAs(target) {
					filtered = append(filtered, value)
				}
			}
			state.listeners[eventType] = filtered
			return goja.Undefined()
		})
		_ = object.Set("open", func(call goja.FunctionCall) goja.Value {
			if !goja.IsUndefined(call.Argument(2)) && !call.Argument(2).ToBoolean() {
				panic(vm.NewTypeError("synchronous XMLHttpRequest is unsupported"))
			}
			state.method, state.url = strings.ToUpper(call.Argument(0).String()), call.Argument(1).String()
			state.requestHeaders, state.controller = fetchapi.NewHeaders(), fetchapi.NewAbortController()
			state.status, state.statusText, state.responseURL, state.responseText = 0, "", "", ""
			state.response, state.sent, state.aborted = goja.Null(), false, false
			setReadyState(xhrOpened)
			return goja.Undefined()
		})
		_ = object.Set("setRequestHeader", func(call goja.FunctionCall) goja.Value {
			if state.readyState != xhrOpened || state.sent {
				panic(vm.NewTypeError("XMLHttpRequest is not opened"))
			}
			if err := state.requestHeaders.Append(call.Argument(0).String(), call.Argument(1).String()); err != nil {
				panic(vm.NewTypeError(err.Error()))
			}
			return goja.Undefined()
		})
		_ = object.Set("getResponseHeader", func(call goja.FunctionCall) goja.Value {
			if value, ok := state.responseHeaders.Get(call.Argument(0).String()); ok {
				return vm.ToValue(value)
			}
			return goja.Null()
		})
		_ = object.Set("getAllResponseHeaders", func(goja.FunctionCall) goja.Value {
			if state.responseHeaders == nil {
				return vm.ToValue("")
			}
			var result strings.Builder
			for _, entry := range state.responseHeaders.Entries() {
				result.WriteString(entry.Name)
				result.WriteString(": ")
				result.WriteString(entry.Value)
				result.WriteString("\r\n")
			}
			return vm.ToValue(result.String())
		})
		_ = object.Set("overrideMimeType", func(goja.FunctionCall) goja.Value { return goja.Undefined() })
		_ = object.Set("abort", func(goja.FunctionCall) goja.Value {
			if state.controller != nil {
				state.aborted = true
				state.controller.Abort()
			}
			state.sent = false
			if state.readyState != xhrUnsent && state.readyState != xhrDone {
				setReadyState(xhrDone)
				dispatch("abort")
				dispatch("loadend")
			}
			return goja.Undefined()
		})
		_ = object.Set("send", func(call goja.FunctionCall) goja.Value {
			if state.readyState != xhrOpened || state.sent {
				panic(vm.NewTypeError("XMLHttpRequest is not opened"))
			}
			state.sent = true
			dispatch("loadstart")
			request := fetchapi.Request{Method: state.method, URL: state.url, Headers: state.requestHeaders, Timeout: state.timeout, Signal: state.controller.Signal()}
			if state.withCredentials {
				request.Credentials = fetchapi.CredentialsInclude
			}
			if body := call.Argument(0); body != nil && !goja.IsUndefined(body) && !goja.IsNull(body) {
				request.Text = body.String()
			}
			runtime.fetchAPI.Fetch(request, func(result fetchapi.Response) {
				if state.aborted {
					return
				}
				state.status, state.statusText, state.responseURL, state.responseHeaders = result.Status, result.StatusText, result.URL, result.Headers
				setReadyState(xhrHeadersReceived)
				setReadyState(xhrLoading)
				body, err := result.Bytes()
				if err != nil {
					state.status = 0
					setReadyState(xhrDone)
					dispatch("error")
					dispatch("loadend")
					return
				}
				state.responseText = string(body)
				switch state.responseType {
				case "arraybuffer":
					state.response = vm.ToValue(vm.NewArrayBuffer(body))
				case "json":
					value, parseErr := parseJSON(vm, state.responseText)
					if parseErr == nil {
						state.response = value
					} else {
						state.response = goja.Null()
					}
				default:
					state.response = vm.ToValue(state.responseText)
				}
				setReadyState(xhrDone)
				dispatch("load")
				dispatch("loadend")
			}, func(message string) {
				if state.aborted {
					return
				}
				state.status, state.statusText = 0, message
				setReadyState(xhrDone)
				dispatch("error")
				dispatch("loadend")
			})
			return goja.Undefined()
		})
		return object
	}
	if err := vm.Set("XMLHttpRequest", constructor); err != nil {
		return err
	}
	value, ok := vm.Get("XMLHttpRequest").(*goja.Object)
	if !ok {
		return fmt.Errorf("XMLHttpRequest constructor is unavailable")
	}
	for name, number := range map[string]int{"UNSENT": xhrUnsent, "OPENED": xhrOpened, "HEADERS_RECEIVED": xhrHeadersReceived, "LOADING": xhrLoading, "DONE": xhrDone} {
		_ = value.Set(name, number)
		if prototype, ok := value.Get("prototype").(*goja.Object); ok {
			_ = prototype.Set(name, number)
		}
	}
	return nil
}
