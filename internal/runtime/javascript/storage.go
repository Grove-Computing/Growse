package javascript

import (
	"fmt"
	"strings"

	navigationapi "github.com/Grove-Computing/Growse/internal/webapi/navigation"
	storageapi "github.com/Grove-Computing/Growse/internal/webapi/storage"
	"github.com/dop251/goja"
)

func (runtime *Runtime) installStorage(vm *goja.Runtime) error {
	if err := vm.Set("localStorage", runtime.storageValue(vm, runtime.storageAPI.Local())); err != nil {
		return err
	}
	if err := vm.Set("sessionStorage", runtime.storageValue(vm, runtime.storageAPI.Session())); err != nil {
		return err
	}
	return vm.Set("addEventListener", func(call goja.FunctionCall) goja.Value {
		runtime.addWindowEventListener(vm, call.Argument(0).String(), call.Argument(1))
		return goja.Undefined()
	})
}

func (runtime *Runtime) storageValue(vm *goja.Runtime, storage *storageapi.Storage) goja.Value {
	object := vm.NewObject()
	_ = object.Set("getItem", func(call goja.FunctionCall) goja.Value {
		value, exists, err := storage.Get(call.Argument(0).String())
		if err != nil {
			panic(vm.NewTypeError(err.Error()))
		}
		if !exists {
			return goja.Null()
		}
		return vm.ToValue(value)
	})
	_ = object.Set("setItem", func(call goja.FunctionCall) goja.Value {
		if err := storage.Set(call.Argument(0).String(), call.Argument(1).String()); err != nil {
			panic(vm.NewTypeError(err.Error()))
		}
		return goja.Undefined()
	})
	_ = object.Set("removeItem", func(call goja.FunctionCall) goja.Value {
		if err := storage.Remove(call.Argument(0).String()); err != nil {
			panic(vm.NewTypeError(err.Error()))
		}
		return goja.Undefined()
	})
	_ = object.Set("clear", func(goja.FunctionCall) goja.Value {
		if err := storage.Clear(); err != nil {
			panic(vm.NewTypeError(err.Error()))
		}
		return goja.Undefined()
	})
	_ = object.Set("key", func(call goja.FunctionCall) goja.Value {
		value, exists := storage.Key(int(call.Argument(0).ToInteger()))
		if !exists {
			return goja.Null()
		}
		return vm.ToValue(value)
	})
	lengthGetter := vm.ToValue(func(goja.FunctionCall) goja.Value { return vm.ToValue(storage.Length()) })
	_ = object.DefineAccessorProperty("length", lengthGetter, nil, goja.FLAG_FALSE, goja.FLAG_TRUE)
	return object
}

func (runtime *Runtime) addWindowEventListener(vm *goja.Runtime, eventType string, value goja.Value) {
	callable, ok := goja.AssertFunction(value)
	if !ok {
		panic(vm.NewTypeError("window event listener must be a function"))
	}
	eventType = strings.ToLower(strings.TrimSpace(eventType))
	if eventType != "storage" && eventType != "popstate" && eventType != "hashchange" {
		panic(vm.NewTypeError("window event type is unsupported"))
	}
	runtime.mu.Lock()
	for _, listener := range runtime.windowListeners {
		if listener.eventType == eventType && listener.function.SameAs(value) {
			runtime.mu.Unlock()
			return
		}
	}
	if runtime.listenerCount >= runtime.maxListeners {
		runtime.mu.Unlock()
		panic(vm.NewTypeError("event listener limit exceeded"))
	}
	runtime.windowListeners = append(runtime.windowListeners, listenerRecord{eventType: eventType, function: value})
	runtime.listenerCount++
	runtime.mu.Unlock()

	switch eventType {
	case "storage":
		runtime.storageAPI.OnChange(func(event storageapi.Event) {
			object := runtime.storageEventValue(vm, event)
			if _, err := callable(goja.Undefined(), object); err != nil {
				runtime.recordError(fmt.Sprintf("JavaScript storage event handler: %v", err))
			}
		})
	case "popstate":
		runtime.navigationAPI.OnPopState(func(event navigationapi.PopStateEvent) {
			object := runtime.popStateEventValue(vm, event)
			if _, err := callable(goja.Undefined(), object); err != nil {
				runtime.recordError(fmt.Sprintf("JavaScript popstate event handler: %v", err))
			}
		})
	case "hashchange":
		runtime.navigationAPI.OnHashChange(func(event navigationapi.HashChangeEvent) {
			object := runtime.hashChangeEventValue(vm, event)
			if _, err := callable(goja.Undefined(), object); err != nil {
				runtime.recordError(fmt.Sprintf("JavaScript hashchange event handler: %v", err))
			}
		})
	}
}

func (runtime *Runtime) storageEventValue(vm *goja.Runtime, event storageapi.Event) goja.Value {
	object := vm.NewObject()
	_ = object.Set("type", "storage")
	if event.Cleared {
		_ = object.Set("key", goja.Null())
	} else {
		_ = object.Set("key", event.Key)
	}
	if event.HasOldValue {
		_ = object.Set("oldValue", event.OldValue)
	} else {
		_ = object.Set("oldValue", goja.Null())
	}
	if event.HasNewValue {
		_ = object.Set("newValue", event.NewValue)
	} else {
		_ = object.Set("newValue", goja.Null())
	}
	_ = object.Set("url", event.SourceURL)
	_ = object.Set("storageArea", vm.Get("localStorage"))
	return object
}
