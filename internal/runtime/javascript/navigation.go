package javascript

import (
	"errors"
	"fmt"
	"strings"

	navigationapi "github.com/Grove-Computing/Growse/internal/webapi/navigation"
	"github.com/dop251/goja"
)

func (runtime *Runtime) installNavigation(vm *goja.Runtime) error {
	location := vm.NewObject()
	properties := map[string]func(navigationapi.Location) string{
		"href": func(value navigationapi.Location) string { return value.Href },
		"origin": func(value navigationapi.Location) string {
			if runtime.environment.FramePolicy.HasOpaqueOrigin() {
				return "null"
			}
			return value.Origin
		},
		"protocol": func(value navigationapi.Location) string { return value.Scheme + ":" },
		"host":     func(value navigationapi.Location) string { return value.Host },
		"hostname": func(value navigationapi.Location) string { return value.Hostname },
		"port":     func(value navigationapi.Location) string { return value.Port },
		"pathname": func(value navigationapi.Location) string { return value.Path },
		"search": func(value navigationapi.Location) string {
			if value.Query == "" {
				return ""
			}
			return "?" + value.Query
		},
		"hash": func(value navigationapi.Location) string {
			if value.Fragment == "" {
				return ""
			}
			return "#" + value.Fragment
		},
	}
	for name, selectValue := range properties {
		selector := selectValue
		getter := vm.ToValue(func(goja.FunctionCall) goja.Value {
			return vm.ToValue(selector(runtime.navigationAPI.Current()))
		})
		if err := location.DefineAccessorProperty(name, getter, nil, goja.FLAG_FALSE, goja.FLAG_TRUE); err != nil {
			return err
		}
	}
	if err := location.Set("assign", func(call goja.FunctionCall) goja.Value {
		if err := runtime.navigationAPI.Navigate(call.Argument(0).String()); err != nil {
			panic(vm.NewTypeError(err.Error()))
		}
		return goja.Undefined()
	}); err != nil {
		return err
	}
	if err := vm.Set("location", location); err != nil {
		return err
	}

	history := vm.NewObject()
	lengthGetter := vm.ToValue(func(goja.FunctionCall) goja.Value { return vm.ToValue(runtime.navigationAPI.HistoryLength()) })
	if err := history.DefineAccessorProperty("length", lengthGetter, nil, goja.FLAG_FALSE, goja.FLAG_TRUE); err != nil {
		return err
	}
	stateGetter := vm.ToValue(func(goja.FunctionCall) goja.Value {
		state := runtime.navigationAPI.HistoryState()
		if state == "" {
			return goja.Null()
		}
		value, err := parseJSON(vm, state)
		if err != nil {
			return goja.Null()
		}
		return value
	})
	if err := history.DefineAccessorProperty("state", stateGetter, nil, goja.FLAG_FALSE, goja.FLAG_TRUE); err != nil {
		return err
	}
	_ = history.Set("back", func(goja.FunctionCall) goja.Value {
		runtime.mustNavigate(vm, runtime.navigationAPI.Back())
		return goja.Undefined()
	})
	_ = history.Set("forward", func(goja.FunctionCall) goja.Value {
		runtime.mustNavigate(vm, runtime.navigationAPI.Forward())
		return goja.Undefined()
	})
	_ = history.Set("go", func(call goja.FunctionCall) goja.Value {
		runtime.mustNavigate(vm, runtime.navigationAPI.Go(int(call.Argument(0).ToInteger())))
		return goja.Undefined()
	})
	_ = history.Set("pushState", func(call goja.FunctionCall) goja.Value {
		state, err := stringifyJSON(vm, call.Argument(0))
		if err == nil {
			err = runtime.navigationAPI.PushState(state, optionalURL(call.Argument(2)))
		}
		runtime.mustNavigate(vm, err)
		return goja.Undefined()
	})
	_ = history.Set("replaceState", func(call goja.FunctionCall) goja.Value {
		state, err := stringifyJSON(vm, call.Argument(0))
		if err == nil {
			err = runtime.navigationAPI.ReplaceState(state, optionalURL(call.Argument(2)))
		}
		runtime.mustNavigate(vm, err)
		return goja.Undefined()
	})
	if err := vm.Set("history", history); err != nil {
		return err
	}
	return nil
}

func (runtime *Runtime) mustNavigate(vm *goja.Runtime, err error) {
	if err != nil {
		panic(vm.NewTypeError(err.Error()))
	}
}

func stringifyJSON(vm *goja.Runtime, value goja.Value) (string, error) {
	jsonObject := vm.Get("JSON").ToObject(vm)
	stringify, ok := goja.AssertFunction(jsonObject.Get("stringify"))
	if !ok {
		return "", errors.New("JSON.stringify is unavailable")
	}
	encoded, err := stringify(jsonObject, value)
	if err != nil {
		return "", fmt.Errorf("history state is not JSON serializable: %w", err)
	}
	if encoded == nil || goja.IsUndefined(encoded) {
		return "", errors.New("history state is not JSON serializable")
	}
	return encoded.String(), nil
}

func optionalURL(value goja.Value) string {
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		return ""
	}
	return value.String()
}

func (runtime *Runtime) popStateEventValue(vm *goja.Runtime, event navigationapi.PopStateEvent) goja.Value {
	object := vm.NewObject()
	_ = object.Set("type", "popstate")
	state := goja.Value(goja.Null())
	if strings.TrimSpace(event.State) != "" {
		if parsed, err := parseJSON(vm, event.State); err == nil {
			state = parsed
		}
	}
	_ = object.Set("state", state)
	return object
}

func (runtime *Runtime) hashChangeEventValue(vm *goja.Runtime, event navigationapi.HashChangeEvent) goja.Value {
	object := vm.NewObject()
	_ = object.Set("type", "hashchange")
	_ = object.Set("oldURL", event.OldURL)
	_ = object.Set("newURL", event.NewURL)
	return object
}
