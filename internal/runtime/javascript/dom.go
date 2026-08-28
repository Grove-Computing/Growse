package javascript

import (
	"context"
	"errors"
	"fmt"
	"strings"

	domapi "github.com/Grove-Computing/Growse/internal/webapi/dom"
	"github.com/dop251/goja"
)

func (runtime *Runtime) installDOM(vm *goja.Runtime) error {
	document := vm.NewObject()
	readyStateGetter := vm.ToValue(func(goja.FunctionCall) goja.Value {
		if runtime.environment.Document == nil {
			return vm.ToValue("loading")
		}
		return vm.ToValue(runtime.environment.Document.ReadyState())
	})
	if err := document.DefineAccessorProperty("readyState", readyStateGetter, nil, goja.FLAG_FALSE, goja.FLAG_TRUE); err != nil {
		return err
	}
	if err := document.Set("addEventListener", func(call goja.FunctionCall) goja.Value {
		runtime.addDocumentEventListener(vm, call.Argument(0).String(), call.Argument(1))
		return goja.Undefined()
	}); err != nil {
		return err
	}
	if err := document.Set("getElementById", func(call goja.FunctionCall) goja.Value {
		element := runtime.domAPI.GetElementByID(call.Argument(0).String())
		return runtime.elementValue(vm, element)
	}); err != nil {
		return err
	}
	if err := document.Set("querySelector", func(call goja.FunctionCall) goja.Value {
		element := runtime.domAPI.QuerySelector(call.Argument(0).String())
		return runtime.elementValue(vm, element)
	}); err != nil {
		return err
	}
	if err := document.Set("querySelectorAll", func(call goja.FunctionCall) goja.Value {
		return runtime.elementArrayValue(vm, runtime.domAPI.QuerySelectorAll(call.Argument(0).String()))
	}); err != nil {
		return err
	}
	if err := document.Set("getElementsByClassName", func(call goja.FunctionCall) goja.Value {
		return runtime.elementArrayValue(vm, runtime.domAPI.GetElementsByClassName(call.Argument(0).String()))
	}); err != nil {
		return err
	}
	if err := document.Set("getElementsByTagName", func(call goja.FunctionCall) goja.Value {
		return runtime.elementArrayValue(vm, runtime.domAPI.GetElementsByTagName(call.Argument(0).String()))
	}); err != nil {
		return err
	}
	if err := document.Set("createElement", func(call goja.FunctionCall) goja.Value {
		element := runtime.domAPI.CreateElement(call.Argument(0).String())
		return runtime.elementValue(vm, element)
	}); err != nil {
		return err
	}
	if err := document.Set("createTextNode", func(call goja.FunctionCall) goja.Value {
		return runtime.elementValue(vm, runtime.domAPI.CreateTextNode(call.Argument(0).String()))
	}); err != nil {
		return err
	}
	return vm.Set("document", document)
}

func (runtime *Runtime) addDocumentEventListener(vm *goja.Runtime, eventType string, value goja.Value) {
	_, ok := goja.AssertFunction(value)
	if !ok {
		panic(vm.NewTypeError("document event listener must be a function"))
	}
	eventType = strings.ToLower(strings.TrimSpace(eventType))
	if eventType != "readystatechange" && eventType != "domcontentloaded" {
		panic(vm.NewTypeError("document event type is unsupported"))
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	for _, listener := range runtime.documentListeners {
		if listener.eventType == eventType && listener.function.SameAs(value) {
			return
		}
	}
	if runtime.listenerCount >= runtime.maxListeners {
		panic(vm.NewTypeError("event listener limit exceeded"))
	}
	runtime.documentListeners = append(runtime.documentListeners, listenerRecord{eventType: eventType, function: value})
	runtime.listenerCount++
}

func (runtime *Runtime) setDocumentReadyState(vm *goja.Runtime, state string) {
	document := runtime.environment.Document
	if document == nil || !document.SetReadyState(state) {
		return
	}
	if runtime.environment.OnMutation != nil {
		runtime.environment.OnMutation()
	}
	runtime.dispatchLifecycleEvent(vm, "readystatechange", true)
}

func (runtime *Runtime) dispatchLifecycleEvent(vm *goja.Runtime, eventType string, documentTarget bool) {
	lower := strings.ToLower(eventType)
	runtime.mu.Lock()
	listeners := runtime.windowListeners
	target := vm.GlobalObject()
	if documentTarget {
		listeners = runtime.documentListeners
		if document, ok := vm.Get("document").(*goja.Object); ok {
			target = document
		}
	}
	listeners = append([]listenerRecord(nil), listeners...)
	runtime.mu.Unlock()
	event := vm.NewObject()
	_ = event.Set("type", eventType)
	_ = event.Set("target", target)
	for _, listener := range listeners {
		if listener.eventType != lower {
			continue
		}
		callable, ok := goja.AssertFunction(listener.function)
		if !ok {
			continue
		}
		if _, err := callable(target, event); err != nil {
			runtime.recordError(fmt.Sprintf("JavaScript %s event handler: %v", eventType, err))
		}
	}
	runtime.drainMicrotasks(vm)
}

func (runtime *Runtime) elementValue(vm *goja.Runtime, element *domapi.Element) goja.Value {
	if element == nil || element.ID() == 0 {
		return goja.Null()
	}
	id := uint64(element.ID())
	if existing := runtime.elementByID[id]; existing != nil {
		return existing
	}
	object := vm.NewObject()
	runtime.elements[object] = element
	runtime.elementByID[id] = object

	_ = object.Set("getAttribute", func(call goja.FunctionCall) goja.Value {
		value, ok := element.GetAttribute(call.Argument(0).String())
		if !ok {
			return goja.Null()
		}
		return vm.ToValue(value)
	})
	_ = object.Set("setAttribute", func(call goja.FunctionCall) goja.Value {
		changed := element.SetAttribute(call.Argument(0).String(), call.Argument(1).String())
		runtime.resourceElementChanged(vm, element)
		return vm.ToValue(changed)
	})
	_ = object.Set("removeAttribute", func(call goja.FunctionCall) goja.Value {
		changed := element.RemoveAttribute(call.Argument(0).String())
		runtime.resourceElementChanged(vm, element)
		return vm.ToValue(changed)
	})
	_ = object.Set("appendChild", func(call goja.FunctionCall) goja.Value {
		childObject, ok := call.Argument(0).(*goja.Object)
		if !ok {
			return goja.Null()
		}
		child := runtime.elements[childObject]
		if child == nil || !element.AppendChild(child) {
			return goja.Null()
		}
		runtime.prepareConnectedScripts(vm, child)
		return childObject
	})
	_ = object.Set("append", func(call goja.FunctionCall) goja.Value {
		children, ok := runtime.domArguments(vm, call.Arguments)
		if !ok || len(children) != 0 && !element.Append(children...) {
			panic(vm.NewTypeError("append arguments are invalid"))
		}
		runtime.prepareConnectedScripts(vm, children...)
		return goja.Undefined()
	})
	_ = object.Set("prepend", func(call goja.FunctionCall) goja.Value {
		children, ok := runtime.domArguments(vm, call.Arguments)
		if !ok || len(children) != 0 && !element.Prepend(children...) {
			panic(vm.NewTypeError("prepend arguments are invalid"))
		}
		runtime.prepareConnectedScripts(vm, children...)
		return goja.Undefined()
	})
	_ = object.Set("removeChild", func(call goja.FunctionCall) goja.Value {
		childObject, ok := call.Argument(0).(*goja.Object)
		if !ok {
			return goja.Null()
		}
		child := runtime.elements[childObject]
		if child == nil || !element.RemoveChild(child) {
			return goja.Null()
		}
		runtime.refreshRemovedStyleTree(child)
		return childObject
	})
	_ = object.Set("replaceChildren", func(call goja.FunctionCall) goja.Value {
		oldChildren := element.Children()
		children, ok := runtime.domArguments(vm, call.Arguments)
		if !ok || !element.ReplaceChildren(children...) {
			panic(vm.NewTypeError("replaceChildren arguments are invalid"))
		}
		runtime.prepareConnectedScripts(vm, children...)
		for _, oldChild := range oldChildren {
			runtime.refreshRemovedStyleTree(oldChild)
		}
		return goja.Undefined()
	})
	_ = object.Set("remove", func(goja.FunctionCall) goja.Value {
		hadStyles := containsStyleResource(element)
		element.Remove()
		if hadStyles {
			runtime.refreshInlineStyles(dynamicStyleSnapshot{})
		}
		return goja.Undefined()
	})
	_ = object.Set("addEventListener", func(call goja.FunctionCall) goja.Value {
		runtime.addEventListener(vm, object, element, call.Argument(0).String(), call.Argument(1), call.Argument(2))
		return goja.Undefined()
	})
	_ = object.Set("removeEventListener", func(call goja.FunctionCall) goja.Value {
		runtime.removeEventListener(element, call.Argument(0).String(), call.Argument(1), call.Argument(2))
		return goja.Undefined()
	})

	classList := vm.NewObject()
	_ = classList.Set("add", func(call goja.FunctionCall) goja.Value {
		return vm.ToValue(element.AddClass(call.Argument(0).String()))
	})
	_ = classList.Set("remove", func(call goja.FunctionCall) goja.Value {
		return vm.ToValue(element.RemoveClass(call.Argument(0).String()))
	})
	_ = classList.Set("contains", func(call goja.FunctionCall) goja.Value {
		return vm.ToValue(element.ContainsClass(call.Argument(0).String()))
	})
	_ = classList.Set("toggle", func(call goja.FunctionCall) goja.Value {
		var force *bool
		if !goja.IsUndefined(call.Argument(1)) {
			value := call.Argument(1).ToBoolean()
			force = &value
		}
		return vm.ToValue(element.ToggleClass(call.Argument(0).String(), force))
	})
	_ = object.DefineDataProperty("classList", classList, goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE)

	textGetter := vm.ToValue(func(goja.FunctionCall) goja.Value { return vm.ToValue(element.Text()) })
	textSetter := vm.ToValue(func(call goja.FunctionCall) goja.Value {
		element.SetText(call.Argument(0).String())
		runtime.resourceElementChanged(vm, element)
		return goja.Undefined()
	})
	_ = object.DefineAccessorProperty("textContent", textGetter, textSetter, goja.FLAG_FALSE, goja.FLAG_TRUE)
	idGetter := vm.ToValue(func(goja.FunctionCall) goja.Value { return vm.ToValue(element.IDValue()) })
	idSetter := vm.ToValue(func(call goja.FunctionCall) goja.Value {
		element.SetIDValue(call.Argument(0).String())
		return goja.Undefined()
	})
	_ = object.DefineAccessorProperty("id", idGetter, idSetter, goja.FLAG_FALSE, goja.FLAG_TRUE)
	classNameGetter := vm.ToValue(func(goja.FunctionCall) goja.Value { return vm.ToValue(element.ClassName()) })
	classNameSetter := vm.ToValue(func(call goja.FunctionCall) goja.Value {
		element.SetClassName(call.Argument(0).String())
		return goja.Undefined()
	})
	_ = object.DefineAccessorProperty("className", classNameGetter, classNameSetter, goja.FLAG_FALSE, goja.FLAG_TRUE)
	_ = object.DefineDataProperty("tagName", vm.ToValue(element.TagName()), goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE)
	connectedGetter := vm.ToValue(func(goja.FunctionCall) goja.Value { return vm.ToValue(element.IsConnected()) })
	_ = object.DefineAccessorProperty("isConnected", connectedGetter, nil, goja.FLAG_FALSE, goja.FLAG_TRUE)
	childrenGetter := vm.ToValue(func(goja.FunctionCall) goja.Value { return runtime.elementArrayValue(vm, element.Children()) })
	_ = object.DefineAccessorProperty("children", childrenGetter, nil, goja.FLAG_FALSE, goja.FLAG_TRUE)
	parentGetter := vm.ToValue(func(goja.FunctionCall) goja.Value { return runtime.elementValue(vm, element.ParentElement()) })
	_ = object.DefineAccessorProperty("parentElement", parentGetter, nil, goja.FLAG_FALSE, goja.FLAG_TRUE)
	innerHTMLGetter := vm.ToValue(func(goja.FunctionCall) goja.Value { return vm.ToValue(element.InnerHTML()) })
	innerHTMLSetter := vm.ToValue(func(call goja.FunctionCall) goja.Value {
		if !element.SetInnerHTML(call.Argument(0).String()) {
			panic(vm.NewTypeError("innerHTML fragment was rejected"))
		}
		runtime.prepareConnectedScripts(vm, element.Children()...)
		runtime.refreshInlineStyles(dynamicStyleSnapshot{})
		return goja.Undefined()
	})
	_ = object.DefineAccessorProperty("innerHTML", innerHTMLGetter, innerHTMLSetter, goja.FLAG_FALSE, goja.FLAG_TRUE)
	valueGetter := vm.ToValue(func(goja.FunctionCall) goja.Value { return vm.ToValue(element.Value()) })
	valueSetter := vm.ToValue(func(call goja.FunctionCall) goja.Value {
		element.SetValue(call.Argument(0).String())
		return goja.Undefined()
	})
	_ = object.DefineAccessorProperty("value", valueGetter, valueSetter, goja.FLAG_FALSE, goja.FLAG_TRUE)
	runtime.installScriptElement(vm, object, element)
	runtime.installLinkElement(vm, object, element)
	runtime.installStyleElement(vm, object, element)
	runtime.installFrameElement(vm, object, id)
	return object
}

func (runtime *Runtime) elementArrayValue(vm *goja.Runtime, elements []*domapi.Element) goja.Value {
	values := make([]any, len(elements))
	for index, element := range elements {
		values[index] = runtime.elementValue(vm, element)
	}
	return vm.NewArray(values...)
}

func (runtime *Runtime) domArguments(vm *goja.Runtime, values []goja.Value) ([]*domapi.Element, bool) {
	result := make([]*domapi.Element, 0, len(values))
	for _, value := range values {
		if object, ok := value.(*goja.Object); ok {
			if element := runtime.elements[object]; element != nil {
				result = append(result, element)
				continue
			}
		}
		text := runtime.domAPI.CreateTextNode(value.String())
		if text == nil {
			return nil, false
		}
		result = append(result, text)
	}
	return result, true
}

func (runtime *Runtime) addEventListener(vm *goja.Runtime, object *goja.Object, element *domapi.Element, eventType string, value, options goja.Value) {
	callable, ok := goja.AssertFunction(value)
	if !ok {
		panic(vm.NewTypeError("event listener must be a function"))
	}
	eventType = strings.ToLower(strings.TrimSpace(eventType))
	capture := eventCaptureOption(options)
	id := uint64(element.ID())
	runtime.mu.Lock()
	for _, listener := range runtime.listeners {
		if listener.elementID == id && listener.eventType == eventType && listener.capture == capture && listener.function.SameAs(value) {
			runtime.mu.Unlock()
			return
		}
	}
	if runtime.listenerCount >= runtime.maxListeners {
		runtime.mu.Unlock()
		panic(vm.NewTypeError("event listener limit exceeded"))
	}
	runtime.mu.Unlock()

	token := element.AddEventListenerForJavaScript(eventType, capture, func(event domapi.Event) {
		invoke := func(vm *goja.Runtime) error {
			eventObject := runtime.eventValue(vm, object, event)
			_, err := callable(object, eventObject)
			return err
		}
		var err error
		if runtime.executing.Load() {
			runtime.mu.Lock()
			activeVM := runtime.vm
			stopped := runtime.stopped
			runtime.mu.Unlock()
			if !stopped && activeVM != nil {
				err = invoke(activeVM)
			}
		} else {
			err = runtime.runSync(context.Background(), invoke)
		}
		if err != nil && !errorsIsRuntimeStop(err) {
			runtime.recordError(fmt.Sprintf("JavaScript %s event handler: %v", eventType, err))
		}
	})
	if token == 0 {
		panic(vm.NewTypeError("event target is disconnected or event type is unsupported"))
	}
	runtime.mu.Lock()
	runtime.listeners = append(runtime.listeners, listenerRecord{elementID: id, eventType: eventType, function: value, capture: capture, token: token})
	runtime.listenerCount++
	runtime.mu.Unlock()
}

func (runtime *Runtime) removeEventListener(element *domapi.Element, eventType string, value, options goja.Value) {
	if element == nil {
		return
	}
	eventType = strings.ToLower(strings.TrimSpace(eventType))
	capture := eventCaptureOption(options)
	id := uint64(element.ID())
	runtime.mu.Lock()
	index := -1
	var record listenerRecord
	for candidate, listener := range runtime.listeners {
		if listener.elementID == id && listener.eventType == eventType && listener.capture == capture && listener.function.SameAs(value) {
			index, record = candidate, listener
			break
		}
	}
	if index >= 0 {
		runtime.listeners = append(runtime.listeners[:index], runtime.listeners[index+1:]...)
		runtime.listenerCount--
	}
	runtime.mu.Unlock()
	if index >= 0 {
		element.RemoveEventListener(eventType, record.token)
	}
}

func eventCaptureOption(value goja.Value) bool {
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		return false
	}
	if object, ok := value.(*goja.Object); ok {
		return object.Get("capture").ToBoolean()
	}
	return value.ToBoolean()
}

func (runtime *Runtime) eventValue(vm *goja.Runtime, currentTarget *goja.Object, event domapi.Event) goja.Value {
	object := vm.NewObject()
	target := currentTarget
	if targetElement := runtime.domAPI.ElementByNodeID(event.TargetNodeID); targetElement != nil {
		if targetObject, ok := runtime.elementValue(vm, targetElement).(*goja.Object); ok {
			target = targetObject
		}
	}
	_ = object.Set("type", event.Type)
	_ = object.Set("target", target)
	_ = object.Set("currentTarget", currentTarget)
	_ = object.Set("bubbles", event.Bubbles)
	_ = object.Set("cancelable", event.Cancelable)
	_ = object.Set("eventPhase", int(event.Phase))
	_ = object.Set("value", event.Value)
	_ = object.Set("clientX", event.X)
	_ = object.Set("clientY", event.Y)
	_ = object.Set("preventDefault", func(goja.FunctionCall) goja.Value {
		event.PreventDefault()
		return goja.Undefined()
	})
	_ = object.Set("stopPropagation", func(goja.FunctionCall) goja.Value {
		event.StopPropagation()
		return goja.Undefined()
	})
	defaultPreventedGetter := vm.ToValue(func(goja.FunctionCall) goja.Value {
		return vm.ToValue(event.DefaultPrevented != nil && event.DefaultPrevented())
	})
	_ = object.DefineAccessorProperty("defaultPrevented", defaultPreventedGetter, nil, goja.FLAG_FALSE, goja.FLAG_TRUE)
	return object
}

func errorsIsRuntimeStop(err error) bool {
	return err == nil || errors.Is(err, errRuntimeStopped) || errors.Is(err, context.Canceled)
}
