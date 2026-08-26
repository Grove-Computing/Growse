package javascript

import (
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
	"github.com/dop251/goja"
)

func (runtime *Runtime) installServiceWorker(vm *goja.Runtime) error {
	host := runtime.environment.ServiceWorker
	if host == nil {
		return nil
	}
	navigator := vm.Get("navigator").ToObject(vm)
	container := vm.NewObject()
	_ = container.Set("register", func(call goja.FunctionCall) goja.Value {
		scriptURL := call.Argument(0).String()
		scope := ""
		if options := call.Argument(1); !goja.IsUndefined(options) && !goja.IsNull(options) {
			value := options.ToObject(vm).Get("scope")
			if value != nil && !goja.IsUndefined(value) {
				scope = value.String()
			}
		}
		return runtime.resolvedPromise(vm, func() (goja.Value, error) {
			registration, err := host.Register(scriptURL, scope)
			if err != nil {
				return nil, err
			}
			return runtime.serviceWorkerRegistrationValue(vm, registration), nil
		})
	})
	_ = container.Set("getRegistration", func(call goja.FunctionCall) goja.Value {
		clientURL := ""
		if !goja.IsUndefined(call.Argument(0)) {
			clientURL = call.Argument(0).String()
		}
		return runtime.resolvedPromise(vm, func() (goja.Value, error) {
			registration, err := host.GetRegistration(clientURL)
			if err != nil {
				return nil, err
			}
			if registration == nil {
				return goja.Undefined(), nil
			}
			return runtime.serviceWorkerRegistrationValue(vm, *registration), nil
		})
	})
	_ = container.Set("getRegistrations", func(goja.FunctionCall) goja.Value {
		return runtime.resolvedPromise(vm, func() (goja.Value, error) {
			registrations, err := host.GetRegistrations()
			if err != nil {
				return nil, err
			}
			values := make([]any, 0, len(registrations))
			for _, registration := range registrations {
				values = append(values, runtime.serviceWorkerRegistrationValue(vm, registration))
			}
			return vm.NewArray(values...), nil
		})
	})
	controller := vm.ToValue(func(goja.FunctionCall) goja.Value {
		registration := host.Controller()
		if registration == nil {
			return goja.Null()
		}
		return runtime.serviceWorkerValue(vm, *registration, registration.Active)
	})
	_ = container.DefineAccessorProperty("controller", controller, nil, goja.FLAG_FALSE, goja.FLAG_TRUE)
	ready := vm.ToValue(func(goja.FunctionCall) goja.Value {
		promise, resolve, reject := vm.NewPromise()
		registration, err := host.GetRegistration("")
		if err != nil {
			_ = reject(vm.NewTypeError(err.Error()))
		} else if registration != nil && registration.Active == runtimemodel.ServiceWorkerActivated {
			_ = resolve(runtime.serviceWorkerRegistrationValue(vm, *registration))
		}
		return vm.ToValue(promise)
	})
	_ = container.DefineAccessorProperty("ready", ready, nil, goja.FLAG_FALSE, goja.FLAG_TRUE)
	return navigator.DefineDataProperty("serviceWorker", container, goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE)
}

func (runtime *Runtime) serviceWorkerRegistrationValue(vm *goja.Runtime, registration runtimemodel.ServiceWorkerRegistration) goja.Value {
	host := runtime.environment.ServiceWorker
	if host == nil {
		panic(vm.NewTypeError("service worker is unavailable"))
	}
	object := vm.NewObject()
	_ = object.DefineDataProperty("scope", vm.ToValue(registration.Scope), goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE)
	defineWorker := func(name string, state runtimemodel.ServiceWorkerState) {
		value := goja.Value(goja.Null())
		if state != "" {
			value = runtime.serviceWorkerValue(vm, registration, state)
		}
		_ = object.DefineDataProperty(name, value, goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE)
	}
	defineWorker("installing", registration.Installing)
	defineWorker("waiting", registration.Waiting)
	defineWorker("active", registration.Active)
	_ = object.Set("update", func(goja.FunctionCall) goja.Value {
		return runtime.resolvedPromise(vm, func() (goja.Value, error) {
			updated, err := host.Update(registration.Scope)
			if err != nil {
				return nil, err
			}
			return runtime.serviceWorkerRegistrationValue(vm, updated), nil
		})
	})
	_ = object.Set("unregister", func(goja.FunctionCall) goja.Value {
		return runtime.resolvedPromise(vm, func() (goja.Value, error) {
			removed, err := host.Unregister(registration.Scope)
			return vm.ToValue(removed), err
		})
	})
	return object
}

func (runtime *Runtime) serviceWorkerValue(vm *goja.Runtime, registration runtimemodel.ServiceWorkerRegistration, state runtimemodel.ServiceWorkerState) goja.Value {
	if state == "" {
		return goja.Null()
	}
	scriptURL := registration.ScriptURL
	switch state {
	case runtimemodel.ServiceWorkerInstalling:
		scriptURL = registration.InstallingScriptURL
	case runtimemodel.ServiceWorkerInstalled:
		scriptURL = registration.WaitingScriptURL
	case runtimemodel.ServiceWorkerActivated:
		scriptURL = registration.ActiveScriptURL
	}
	if scriptURL == "" {
		panic(vm.NewTypeError("service worker has no script URL"))
	}
	object := vm.NewObject()
	_ = object.DefineDataProperty("scriptURL", vm.ToValue(scriptURL), goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE)
	_ = object.DefineDataProperty("state", vm.ToValue(string(state)), goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE)
	return object
}
