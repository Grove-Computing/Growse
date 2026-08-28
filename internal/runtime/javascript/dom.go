package javascript

import (
	"context"
	"errors"
	"fmt"
	"strings"

	dommodel "github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/events"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
	domapi "github.com/Grove-Computing/Growse/internal/webapi/dom"
	"github.com/dop251/goja"
)

func (runtime *Runtime) installDOM(vm *goja.Runtime) error {
	if err := runtime.installDOMInterfaces(vm); err != nil {
		return err
	}
	if err := runtime.installEventConstructors(vm); err != nil {
		return err
	}
	document := vm.NewObject()
	if prototype := domInterfacePrototype(vm, "Document"); prototype != nil {
		_ = document.SetPrototype(prototype)
	}
	for name, getter := range map[string]func() *domapi.Element{
		"documentElement": runtime.domAPI.DocumentElement,
		"head":            runtime.domAPI.Head,
		"body":            runtime.domAPI.Body,
	} {
		accessor := vm.ToValue(func(goja.FunctionCall) goja.Value { return runtime.elementValue(vm, getter()) })
		if err := document.DefineAccessorProperty(name, accessor, nil, goja.FLAG_FALSE, goja.FLAG_TRUE); err != nil {
			return err
		}
	}
	var documentRoot *domapi.Element
	if runtime.environment.Document != nil && runtime.environment.Document.Root != nil {
		documentRoot = runtime.domAPI.NodeByID(runtime.environment.Document.Root.ID)
		if documentRoot != nil {
			runtime.elements[document] = documentRoot
			runtime.elementByID[uint64(documentRoot.ID())] = document
		}
	}
	_ = document.DefineDataProperty("nodeType", vm.ToValue(9), goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE)
	_ = document.DefineDataProperty("nodeName", vm.ToValue("#document"), goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE)
	_ = document.DefineDataProperty("parentNode", goja.Null(), goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE)
	documentChildrenGetter := vm.ToValue(func(goja.FunctionCall) goja.Value {
		return runtime.nodeCollectionValue(vm, documentRoot.ChildNodes())
	})
	_ = document.DefineAccessorProperty("childNodes", documentChildrenGetter, nil, goja.FLAG_FALSE, goja.FLAG_TRUE)
	documentFirstChildGetter := vm.ToValue(func(goja.FunctionCall) goja.Value {
		return runtime.nodeValue(vm, documentRoot.FirstChild())
	})
	_ = document.DefineAccessorProperty("firstChild", documentFirstChildGetter, nil, goja.FLAG_FALSE, goja.FLAG_TRUE)
	documentLastChildGetter := vm.ToValue(func(goja.FunctionCall) goja.Value {
		return runtime.nodeValue(vm, documentRoot.LastChild())
	})
	_ = document.DefineAccessorProperty("lastChild", documentLastChildGetter, nil, goja.FLAG_FALSE, goja.FLAG_TRUE)
	currentScriptGetter := vm.ToValue(func(goja.FunctionCall) goja.Value {
		if runtime.currentScript == nil {
			return goja.Null()
		}
		return runtime.currentScript
	})
	if err := document.DefineAccessorProperty("currentScript", currentScriptGetter, nil, goja.FLAG_FALSE, goja.FLAG_TRUE); err != nil {
		return err
	}
	scriptsGetter := vm.ToValue(func(goja.FunctionCall) goja.Value {
		return runtime.elementCollectionValue(vm, runtime.domAPI.GetElementsByTagName("script"))
	})
	if err := document.DefineAccessorProperty("scripts", scriptsGetter, nil, goja.FLAG_FALSE, goja.FLAG_TRUE); err != nil {
		return err
	}
	styleSheetsGetter := vm.ToValue(func(goja.FunctionCall) goja.Value {
		return runtime.styleSheetCollectionValue(vm)
	})
	if err := document.DefineAccessorProperty("styleSheets", styleSheetsGetter, nil, goja.FLAG_FALSE, goja.FLAG_TRUE); err != nil {
		return err
	}
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
		eventType := strings.ToLower(strings.TrimSpace(call.Argument(0).String()))
		if eventType == "readystatechange" || eventType == "domcontentloaded" {
			runtime.addDocumentEventListener(vm, eventType, call.Argument(1))
		} else if documentRoot != nil {
			runtime.addEventListener(vm, document, documentRoot, eventType, call.Argument(1), call.Argument(2))
		}
		return goja.Undefined()
	}); err != nil {
		return err
	}
	if err := document.Set("removeEventListener", func(call goja.FunctionCall) goja.Value {
		if documentRoot != nil {
			runtime.removeEventListener(documentRoot, call.Argument(0).String(), call.Argument(1), call.Argument(2))
		}
		return goja.Undefined()
	}); err != nil {
		return err
	}
	if err := document.Set("dispatchEvent", func(call goja.FunctionCall) goja.Value {
		if documentRoot == nil {
			return vm.ToValue(true)
		}
		return vm.ToValue(runtime.dispatchJSEvent(vm, documentRoot, call.Argument(0)))
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
		return runtime.mustCreateNodeValue(vm, element, "createElement")
	}); err != nil {
		return err
	}
	if err := document.Set("createElementNS", func(call goja.FunctionCall) goja.Value {
		namespace := ""
		if !goja.IsNull(call.Argument(0)) && !goja.IsUndefined(call.Argument(0)) {
			namespace = call.Argument(0).String()
		}
		return runtime.mustCreateNodeValue(vm, runtime.domAPI.CreateElementNS(namespace, call.Argument(1).String()), "createElementNS")
	}); err != nil {
		return err
	}
	if err := document.Set("createDocumentFragment", func(goja.FunctionCall) goja.Value {
		return runtime.mustCreateNodeValue(vm, runtime.domAPI.CreateDocumentFragment(), "createDocumentFragment")
	}); err != nil {
		return err
	}
	if err := document.Set("importNode", func(call goja.FunctionCall) goja.Value {
		object, ok := call.Argument(0).(*goja.Object)
		if !ok || runtime.elements[object] == nil {
			panic(vm.NewTypeError("importNode source is not a Node"))
		}
		return runtime.mustCreateNodeValue(vm, runtime.domAPI.ImportNode(runtime.elements[object], call.Argument(1).ToBoolean()), "importNode")
	}); err != nil {
		return err
	}
	if err := document.Set("createTextNode", func(call goja.FunctionCall) goja.Value {
		return runtime.mustCreateNodeValue(vm, runtime.domAPI.CreateTextNode(call.Argument(0).String()), "createTextNode")
	}); err != nil {
		return err
	}
	return vm.Set("document", document)
}

func (runtime *Runtime) mustCreateNodeValue(vm *goja.Runtime, element *domapi.Element, operation string) goja.Value {
	if element == nil {
		panic(vm.NewTypeError(operation + " exceeds a DOM safety limit or has invalid input"))
	}
	return runtime.elementValue(vm, element)
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
	if prototype := runtime.elementPrototype(vm, element); prototype != nil {
		_ = object.SetPrototype(prototype)
	}

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
	_ = object.Set("hasAttribute", func(call goja.FunctionCall) goja.Value {
		return vm.ToValue(element.HasAttribute(call.Argument(0).String()))
	})
	_ = object.Set("toggleAttribute", func(call goja.FunctionCall) goja.Value {
		var force *bool
		if !goja.IsUndefined(call.Argument(1)) {
			value := call.Argument(1).ToBoolean()
			force = &value
		}
		present, ok := element.ToggleAttribute(call.Argument(0).String(), force)
		if !ok {
			panic(vm.NewTypeError("invalid attribute name"))
		}
		runtime.resourceElementChanged(vm, element)
		return vm.ToValue(present)
	})
	_ = object.Set("getAttributeNames", func(goja.FunctionCall) goja.Value {
		names := element.AttributeNames()
		values := make([]any, len(names))
		for index, name := range names {
			values[index] = name
		}
		return vm.NewArray(values...)
	})
	_ = object.Set("matches", func(call goja.FunctionCall) goja.Value {
		return vm.ToValue(element.Matches(call.Argument(0).String()))
	})
	_ = object.Set("closest", func(call goja.FunctionCall) goja.Value {
		return runtime.nodeValue(vm, element.Closest(call.Argument(0).String()))
	})
	_ = object.Set("querySelector", func(call goja.FunctionCall) goja.Value {
		return runtime.nodeValue(vm, element.QuerySelector(call.Argument(0).String()))
	})
	_ = object.Set("querySelectorAll", func(call goja.FunctionCall) goja.Value {
		return runtime.nodeCollectionValue(vm, element.QuerySelectorAll(call.Argument(0).String()))
	})
	_ = object.Set("appendChild", func(call goja.FunctionCall) goja.Value {
		childObject, ok := call.Argument(0).(*goja.Object)
		if !ok {
			return goja.Null()
		}
		child := runtime.elements[childObject]
		preparationRoots := runtime.connectionPreparationRoots(child)
		if child == nil || !element.AppendChild(child) {
			return goja.Null()
		}
		runtime.prepareConnectedScripts(vm, preparationRoots...)
		return childObject
	})
	_ = object.Set("append", func(call goja.FunctionCall) goja.Value {
		children, ok := runtime.domArguments(vm, call.Arguments)
		preparationRoots := runtime.connectionPreparationRoots(children...)
		if !ok || len(children) != 0 && !element.Append(children...) {
			panic(vm.NewTypeError("append arguments are invalid"))
		}
		runtime.prepareConnectedScripts(vm, preparationRoots...)
		return goja.Undefined()
	})
	_ = object.Set("prepend", func(call goja.FunctionCall) goja.Value {
		children, ok := runtime.domArguments(vm, call.Arguments)
		preparationRoots := runtime.connectionPreparationRoots(children...)
		if !ok || len(children) != 0 && !element.Prepend(children...) {
			panic(vm.NewTypeError("prepend arguments are invalid"))
		}
		runtime.prepareConnectedScripts(vm, preparationRoots...)
		return goja.Undefined()
	})
	_ = object.Set("insertBefore", func(call goja.FunctionCall) goja.Value {
		childObject, ok := call.Argument(0).(*goja.Object)
		if !ok || runtime.elements[childObject] == nil {
			return goja.Null()
		}
		var reference *domapi.Element
		if !goja.IsNull(call.Argument(1)) && !goja.IsUndefined(call.Argument(1)) {
			referenceObject, valid := call.Argument(1).(*goja.Object)
			if !valid || runtime.elements[referenceObject] == nil {
				return goja.Null()
			}
			reference = runtime.elements[referenceObject]
		}
		child := runtime.elements[childObject]
		preparationRoots := runtime.connectionPreparationRoots(child)
		if !element.InsertBefore(child, reference) {
			return goja.Null()
		}
		runtime.prepareConnectedScripts(vm, preparationRoots...)
		return childObject
	})
	_ = object.Set("replaceChild", func(call goja.FunctionCall) goja.Value {
		childObject, childOK := call.Argument(0).(*goja.Object)
		replacedObject, replacedOK := call.Argument(1).(*goja.Object)
		child, replaced := runtime.elements[childObject], runtime.elements[replacedObject]
		if !childOK || !replacedOK || child == nil || replaced == nil {
			return goja.Null()
		}
		preparationRoots := runtime.connectionPreparationRoots(child)
		if !element.ReplaceChild(child, replaced) {
			return goja.Null()
		}
		runtime.prepareConnectedScripts(vm, preparationRoots...)
		return replacedObject
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
		preparationRoots := runtime.connectionPreparationRoots(children...)
		if !ok || !element.ReplaceChildren(children...) {
			panic(vm.NewTypeError("replaceChildren arguments are invalid"))
		}
		runtime.prepareConnectedScripts(vm, preparationRoots...)
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
	_ = object.Set("before", func(call goja.FunctionCall) goja.Value {
		parent := element.ParentNode()
		children, ok := runtime.domArguments(vm, call.Arguments)
		if parent == nil || !ok {
			return goja.Undefined()
		}
		preparationRoots := runtime.connectionPreparationRoots(children...)
		for _, child := range children {
			if !parent.InsertBefore(child, element) {
				panic(vm.NewTypeError("before arguments are invalid"))
			}
		}
		runtime.prepareConnectedScripts(vm, preparationRoots...)
		return goja.Undefined()
	})
	_ = object.Set("after", func(call goja.FunctionCall) goja.Value {
		parent, reference := element.ParentNode(), element.NextSibling()
		children, ok := runtime.domArguments(vm, call.Arguments)
		if parent == nil || !ok {
			return goja.Undefined()
		}
		preparationRoots := runtime.connectionPreparationRoots(children...)
		for _, child := range children {
			if !parent.InsertBefore(child, reference) {
				panic(vm.NewTypeError("after arguments are invalid"))
			}
		}
		runtime.prepareConnectedScripts(vm, preparationRoots...)
		return goja.Undefined()
	})
	_ = object.Set("replaceWith", func(call goja.FunctionCall) goja.Value {
		parent := element.ParentNode()
		children, ok := runtime.domArguments(vm, call.Arguments)
		if parent == nil || !ok {
			return goja.Undefined()
		}
		if len(children) == 0 {
			element.Remove()
			return goja.Undefined()
		}
		fragment := runtime.domAPI.CreateDocumentFragment()
		if fragment == nil || !fragment.Append(children...) {
			panic(vm.NewTypeError("replaceWith arguments are invalid"))
		}
		preparationRoots := runtime.connectionPreparationRoots(fragment)
		if !parent.ReplaceChild(fragment, element) {
			panic(vm.NewTypeError("replaceWith target is invalid"))
		}
		runtime.prepareConnectedScripts(vm, preparationRoots...)
		return goja.Undefined()
	})
	_ = object.Set("cloneNode", func(call goja.FunctionCall) goja.Value {
		return runtime.mustCreateNodeValue(vm, element.CloneNode(call.Argument(0).ToBoolean()), "cloneNode")
	})
	_ = object.Set("contains", func(call goja.FunctionCall) goja.Value {
		candidateObject, ok := call.Argument(0).(*goja.Object)
		if !ok {
			return vm.ToValue(false)
		}
		return vm.ToValue(element.Contains(runtime.elements[candidateObject]))
	})
	_ = object.Set("addEventListener", func(call goja.FunctionCall) goja.Value {
		runtime.addEventListener(vm, object, element, call.Argument(0).String(), call.Argument(1), call.Argument(2))
		return goja.Undefined()
	})
	_ = object.Set("removeEventListener", func(call goja.FunctionCall) goja.Value {
		runtime.removeEventListener(element, call.Argument(0).String(), call.Argument(1), call.Argument(2))
		return goja.Undefined()
	})
	_ = object.Set("dispatchEvent", func(call goja.FunctionCall) goja.Value {
		return vm.ToValue(runtime.dispatchJSEvent(vm, element, call.Argument(0)))
	})

	classList := vm.NewObject()
	_ = classList.Set("add", func(call goja.FunctionCall) goja.Value {
		changed := false
		for _, value := range call.Arguments {
			if element.AddClass(value.String()) {
				changed = true
			}
		}
		return vm.ToValue(changed)
	})
	_ = classList.Set("remove", func(call goja.FunctionCall) goja.Value {
		changed := false
		for _, value := range call.Arguments {
			if element.RemoveClass(value.String()) {
				changed = true
			}
		}
		return vm.ToValue(changed)
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
	_ = classList.Set("item", func(call goja.FunctionCall) goja.Value {
		classes := strings.Fields(element.ClassName())
		index := int(call.Argument(0).ToInteger())
		if index < 0 || index >= len(classes) {
			return goja.Null()
		}
		return vm.ToValue(classes[index])
	})
	_ = classList.Set("replace", func(call goja.FunctionCall) goja.Value {
		oldClass, newClass := call.Argument(0).String(), call.Argument(1).String()
		if !element.ContainsClass(oldClass) || !element.RemoveClass(oldClass) {
			return vm.ToValue(false)
		}
		element.AddClass(newClass)
		return vm.ToValue(true)
	})
	classLengthGetter := vm.ToValue(func(goja.FunctionCall) goja.Value { return vm.ToValue(len(strings.Fields(element.ClassName()))) })
	_ = classList.DefineAccessorProperty("length", classLengthGetter, nil, goja.FLAG_FALSE, goja.FLAG_TRUE)
	classValueGetter := vm.ToValue(func(goja.FunctionCall) goja.Value { return vm.ToValue(element.ClassName()) })
	classValueSetter := vm.ToValue(func(call goja.FunctionCall) goja.Value {
		element.SetClassName(call.Argument(0).String())
		return goja.Undefined()
	})
	_ = classList.DefineAccessorProperty("value", classValueGetter, classValueSetter, goja.FLAG_FALSE, goja.FLAG_TRUE)
	_ = object.DefineDataProperty("classList", classList, goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE)
	_ = object.DefineDataProperty("dataset", vm.NewDynamicObject(&datasetObject{vm: vm, element: element}), goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE)
	inlineStyle := newInlineStyleObject(vm, element)
	if prototype := domInterfacePrototype(vm, "CSSStyleDeclaration"); prototype != nil {
		_ = inlineStyle.SetPrototype(prototype)
	}
	_ = object.DefineDataProperty("style", inlineStyle, goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE)

	textGetter := vm.ToValue(func(goja.FunctionCall) goja.Value { return vm.ToValue(element.Text()) })
	textSetter := vm.ToValue(func(call goja.FunctionCall) goja.Value {
		if !element.SetText(call.Argument(0).String()) {
			panic(vm.NewTypeError("textContent exceeds a DOM safety limit"))
		}
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
	_ = object.DefineDataProperty("nodeType", vm.ToValue(domNodeTypeValue(element.NodeType())), goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE)
	_ = object.DefineDataProperty("nodeName", vm.ToValue(element.NodeName()), goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE)
	parentNodeGetter := vm.ToValue(func(goja.FunctionCall) goja.Value { return runtime.nodeValue(vm, element.ParentNode()) })
	_ = object.DefineAccessorProperty("parentNode", parentNodeGetter, nil, goja.FLAG_FALSE, goja.FLAG_TRUE)
	childrenGetter := vm.ToValue(func(goja.FunctionCall) goja.Value { return runtime.elementArrayValue(vm, element.Children()) })
	_ = object.DefineAccessorProperty("children", childrenGetter, nil, goja.FLAG_FALSE, goja.FLAG_TRUE)
	childNodesGetter := vm.ToValue(func(goja.FunctionCall) goja.Value { return runtime.nodeCollectionValue(vm, element.ChildNodes()) })
	_ = object.DefineAccessorProperty("childNodes", childNodesGetter, nil, goja.FLAG_FALSE, goja.FLAG_TRUE)
	firstChildGetter := vm.ToValue(func(goja.FunctionCall) goja.Value { return runtime.nodeValue(vm, element.FirstChild()) })
	_ = object.DefineAccessorProperty("firstChild", firstChildGetter, nil, goja.FLAG_FALSE, goja.FLAG_TRUE)
	lastChildGetter := vm.ToValue(func(goja.FunctionCall) goja.Value { return runtime.nodeValue(vm, element.LastChild()) })
	_ = object.DefineAccessorProperty("lastChild", lastChildGetter, nil, goja.FLAG_FALSE, goja.FLAG_TRUE)
	nextSiblingGetter := vm.ToValue(func(goja.FunctionCall) goja.Value { return runtime.nodeValue(vm, element.NextSibling()) })
	_ = object.DefineAccessorProperty("nextSibling", nextSiblingGetter, nil, goja.FLAG_FALSE, goja.FLAG_TRUE)
	previousSiblingGetter := vm.ToValue(func(goja.FunctionCall) goja.Value { return runtime.nodeValue(vm, element.PreviousSibling()) })
	_ = object.DefineAccessorProperty("previousSibling", previousSiblingGetter, nil, goja.FLAG_FALSE, goja.FLAG_TRUE)
	parentGetter := vm.ToValue(func(goja.FunctionCall) goja.Value { return runtime.elementValue(vm, element.ParentElement()) })
	_ = object.DefineAccessorProperty("parentElement", parentGetter, nil, goja.FLAG_FALSE, goja.FLAG_TRUE)
	if strings.EqualFold(element.TagName(), "template") {
		contentGetter := vm.ToValue(func(goja.FunctionCall) goja.Value {
			return runtime.elementValue(vm, element.TemplateContent())
		})
		_ = object.DefineAccessorProperty("content", contentGetter, nil, goja.FLAG_FALSE, goja.FLAG_TRUE)
	}
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
	outerHTMLGetter := vm.ToValue(func(goja.FunctionCall) goja.Value { return vm.ToValue(element.OuterHTML()) })
	outerHTMLSetter := vm.ToValue(func(call goja.FunctionCall) goja.Value {
		hadStyles := containsStyleResource(element)
		inserted, ok := element.SetOuterHTML(call.Argument(0).String())
		if !ok {
			panic(vm.NewTypeError("outerHTML fragment was rejected"))
		}
		runtime.prepareConnectedScripts(vm, inserted...)
		if hadStyles {
			runtime.refreshInlineStyles(dynamicStyleSnapshot{})
		}
		return goja.Undefined()
	})
	_ = object.DefineAccessorProperty("outerHTML", outerHTMLGetter, outerHTMLSetter, goja.FLAG_FALSE, goja.FLAG_TRUE)
	_ = object.Set("insertAdjacentHTML", func(call goja.FunctionCall) goja.Value {
		inserted, ok := element.InsertAdjacentHTML(call.Argument(0).String(), call.Argument(1).String())
		if !ok {
			panic(vm.NewTypeError("insertAdjacentHTML position or fragment was rejected"))
		}
		runtime.prepareConnectedScripts(vm, inserted...)
		return goja.Undefined()
	})
	runtime.installElementGeometry(vm, object, element)
	valueGetter := vm.ToValue(func(goja.FunctionCall) goja.Value { return vm.ToValue(element.Value()) })
	valueSetter := vm.ToValue(func(call goja.FunctionCall) goja.Value {
		element.SetValue(call.Argument(0).String())
		return goja.Undefined()
	})
	_ = object.DefineAccessorProperty("value", valueGetter, valueSetter, goja.FLAG_FALSE, goja.FLAG_TRUE)
	runtime.installScriptElement(vm, object, element)
	runtime.installLinkElement(vm, object, element)
	runtime.installStyleElement(vm, object, element)
	runtime.installHTMLElementReflection(vm, object, element)
	runtime.installImageElement(vm, object, element)
	runtime.installFrameElement(vm, object, id)
	return object
}

func (runtime *Runtime) installImageElement(vm *goja.Runtime, object *goja.Object, element *domapi.Element) {
	if element == nil || !strings.EqualFold(element.TagName(), "img") {
		return
	}
	read := func() runtimemodel.ImageState {
		runtime.mu.Lock()
		callback := runtime.environment.ReadImage
		runtime.mu.Unlock()
		if callback == nil {
			return runtimemodel.ImageState{}
		}
		return callback(element.ID())
	}
	_ = object.DefineAccessorProperty("complete", vm.ToValue(func(goja.FunctionCall) goja.Value { return vm.ToValue(read().Complete) }), nil, goja.FLAG_FALSE, goja.FLAG_TRUE)
	_ = object.DefineAccessorProperty("naturalWidth", vm.ToValue(func(goja.FunctionCall) goja.Value { return vm.ToValue(read().NaturalWidth) }), nil, goja.FLAG_FALSE, goja.FLAG_TRUE)
	_ = object.DefineAccessorProperty("naturalHeight", vm.ToValue(func(goja.FunctionCall) goja.Value { return vm.ToValue(read().NaturalHeight) }), nil, goja.FLAG_FALSE, goja.FLAG_TRUE)
	_ = object.DefineAccessorProperty("currentSrc", vm.ToValue(func(goja.FunctionCall) goja.Value { return vm.ToValue(read().URL) }), nil, goja.FLAG_FALSE, goja.FLAG_TRUE)
}

func (runtime *Runtime) installDOMInterfaces(vm *goja.Runtime) error {
	_, err := vm.RunString(`
		(function (global) {
			function illegal() { throw new TypeError("Illegal constructor"); }
			function EventTarget() { illegal(); }
			function Node() { illegal(); }
			function Document() { illegal(); }
			function DocumentFragment() { illegal(); }
			function Element() { illegal(); }
			function HTMLElement() { illegal(); }
			function HTMLScriptElement() { illegal(); }
			function HTMLLinkElement() { illegal(); }
			function HTMLImageElement() { illegal(); }
			function HTMLTemplateElement() { illegal(); }
			function CSSStyleDeclaration() { illegal(); }

			Object.setPrototypeOf(Node.prototype, EventTarget.prototype);
			Object.setPrototypeOf(Document.prototype, Node.prototype);
			Object.setPrototypeOf(DocumentFragment.prototype, Node.prototype);
			Object.setPrototypeOf(Element.prototype, Node.prototype);
			Object.setPrototypeOf(HTMLElement.prototype, Element.prototype);
			Object.setPrototypeOf(HTMLScriptElement.prototype, HTMLElement.prototype);
			Object.setPrototypeOf(HTMLLinkElement.prototype, HTMLElement.prototype);
			Object.setPrototypeOf(HTMLImageElement.prototype, HTMLElement.prototype);
			Object.setPrototypeOf(HTMLTemplateElement.prototype, HTMLElement.prototype);

			var interfaces = {
				EventTarget: EventTarget,
				Node: Node,
				Document: Document,
				DocumentFragment: DocumentFragment,
				Element: Element,
				HTMLElement: HTMLElement,
				HTMLScriptElement: HTMLScriptElement,
				HTMLLinkElement: HTMLLinkElement,
				HTMLImageElement: HTMLImageElement,
				HTMLTemplateElement: HTMLTemplateElement,
				CSSStyleDeclaration: CSSStyleDeclaration
			};
			Object.keys(interfaces).forEach(function (name) {
				Object.defineProperty(interfaces[name].prototype, Symbol.toStringTag, { value: name });
				Object.defineProperty(global, name, { value: interfaces[name], writable: true, configurable: true });
			});
		})(globalThis);
	`)
	if err != nil {
		return fmt.Errorf("install DOM interfaces: %w", err)
	}
	elementPrototype := domInterfacePrototype(vm, "Element")
	if elementPrototype == nil {
		return errors.New("element prototype is unavailable")
	}
	return elementPrototype.Set("getAttribute", func(call goja.FunctionCall) goja.Value {
		element := runtime.domElementForThis(vm, call.This)
		value, ok := element.GetAttribute(call.Argument(0).String())
		if !ok {
			return goja.Null()
		}
		return vm.ToValue(value)
	})
}

func domInterfacePrototype(vm *goja.Runtime, name string) *goja.Object {
	constructor, ok := vm.Get(name).(*goja.Object)
	if !ok {
		return nil
	}
	prototype, _ := constructor.Get("prototype").(*goja.Object)
	return prototype
}

func (runtime *Runtime) elementPrototype(vm *goja.Runtime, element *domapi.Element) *goja.Object {
	name := "Node"
	if element.NodeType() == dommodel.NodeDocumentFragment {
		name = "DocumentFragment"
	} else if tagName := element.TagName(); tagName != "" {
		name = "HTMLElement"
		switch strings.ToLower(tagName) {
		case "script":
			name = "HTMLScriptElement"
		case "link":
			name = "HTMLLinkElement"
		case "img":
			name = "HTMLImageElement"
		case "template":
			name = "HTMLTemplateElement"
		}
	}
	return domInterfacePrototype(vm, name)
}

func (runtime *Runtime) domElementForThis(vm *goja.Runtime, value goja.Value) *domapi.Element {
	object, ok := value.(*goja.Object)
	if !ok || runtime.elements[object] == nil {
		panic(vm.NewTypeError("Illegal invocation"))
	}
	return runtime.elements[object]
}

func (runtime *Runtime) installHTMLElementReflection(vm *goja.Runtime, object *goja.Object, element *domapi.Element) {
	for property, attribute := range map[string]string{
		"title": "title", "name": "name", "type": "type", "alt": "alt", "href": "href", "src": "src",
		"rel": "rel", "width": "width", "height": "height", "loading": "loading", "htmlFor": "for",
	} {
		attributeName := attribute
		getter := vm.ToValue(func(goja.FunctionCall) goja.Value {
			value, _ := element.GetAttribute(attributeName)
			return vm.ToValue(value)
		})
		setter := vm.ToValue(func(call goja.FunctionCall) goja.Value {
			element.SetAttribute(attributeName, call.Argument(0).String())
			runtime.resourceElementChanged(vm, element)
			return goja.Undefined()
		})
		_ = object.DefineAccessorProperty(property, getter, setter, goja.FLAG_FALSE, goja.FLAG_TRUE)
	}
	for property, attribute := range map[string]string{
		"disabled": "disabled", "checked": "checked", "selected": "selected", "multiple": "multiple",
		"required": "required", "readOnly": "readonly", "async": "async", "defer": "defer", "noModule": "nomodule",
	} {
		attributeName := attribute
		getter := vm.ToValue(func(goja.FunctionCall) goja.Value { return vm.ToValue(element.HasAttribute(attributeName)) })
		setter := vm.ToValue(func(call goja.FunctionCall) goja.Value {
			wanted := call.Argument(0).ToBoolean()
			_, _ = element.ToggleAttribute(attributeName, &wanted)
			runtime.resourceElementChanged(vm, element)
			return goja.Undefined()
		})
		_ = object.DefineAccessorProperty(property, getter, setter, goja.FLAG_FALSE, goja.FLAG_TRUE)
	}
}

func (runtime *Runtime) elementArrayValue(vm *goja.Runtime, elements []*domapi.Element) goja.Value {
	values := make([]any, len(elements))
	for index, element := range elements {
		values[index] = runtime.elementValue(vm, element)
	}
	return vm.NewArray(values...)
}

func (runtime *Runtime) nodeCollectionValue(vm *goja.Runtime, nodes []*domapi.Element) goja.Value {
	return runtime.elementCollectionValue(vm, nodes)
}

func (runtime *Runtime) nodeValue(vm *goja.Runtime, node *domapi.Element) goja.Value {
	if node == nil {
		return goja.Null()
	}
	if node.NodeType() == dommodel.NodeDocument {
		if document, ok := vm.Get("document").(*goja.Object); ok {
			return document
		}
	}
	return runtime.elementValue(vm, node)
}

func domNodeTypeValue(nodeType dommodel.NodeType) int {
	switch nodeType {
	case dommodel.NodeElement:
		return 1
	case dommodel.NodeText:
		return 3
	case dommodel.NodeDocument:
		return 9
	case dommodel.NodeDocumentFragment:
		return 11
	default:
		return 0
	}
}

func (runtime *Runtime) connectionPreparationRoots(nodes ...*domapi.Element) []*domapi.Element {
	var result []*domapi.Element
	for _, node := range nodes {
		if node == nil {
			continue
		}
		if node.NodeType() == dommodel.NodeDocumentFragment {
			result = append(result, node.ChildNodes()...)
			continue
		}
		result = append(result, node)
	}
	return result
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
	listenerOptions := parseEventListenerOptions(options)
	capture := listenerOptions.capture
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

	var token events.ListenerID
	token = element.AddEventListenerForJavaScriptWithOptions(eventType, capture, listenerOptions.once, listenerOptions.passive, func(event domapi.Event) {
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
		if listenerOptions.once {
			runtime.removeListenerRecord(uint64(element.ID()), eventType, token)
		}
	})
	if token == 0 {
		panic(vm.NewTypeError("event target is disconnected or event type is unsupported"))
	}
	runtime.mu.Lock()
	runtime.listeners = append(runtime.listeners, listenerRecord{
		elementID: id, eventType: eventType, function: value, capture: capture,
		once: listenerOptions.once, passive: listenerOptions.passive, token: token,
	})
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
	return parseEventListenerOptions(value).capture
}

type eventListenerOptions struct {
	capture bool
	once    bool
	passive bool
}

func parseEventListenerOptions(value goja.Value) eventListenerOptions {
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		return eventListenerOptions{}
	}
	if object, ok := value.(*goja.Object); ok {
		return eventListenerOptions{
			capture: jsBoolean(object.Get("capture")),
			once:    jsBoolean(object.Get("once")),
			passive: jsBoolean(object.Get("passive")),
		}
	}
	return eventListenerOptions{capture: value.ToBoolean()}
}

func jsBoolean(value goja.Value) bool { return value != nil && value.ToBoolean() }

func (runtime *Runtime) removeListenerRecord(elementID uint64, eventType string, token events.ListenerID) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	for index, listener := range runtime.listeners {
		if listener.elementID == elementID && listener.eventType == eventType && listener.token == token {
			runtime.listeners = append(runtime.listeners[:index], runtime.listeners[index+1:]...)
			runtime.listenerCount--
			return
		}
	}
}

func (runtime *Runtime) eventValue(vm *goja.Runtime, currentTarget *goja.Object, event domapi.Event) goja.Value {
	runtime.mu.Lock()
	object := runtime.jsEventObjects[event.RuntimeID]
	runtime.mu.Unlock()
	if object == nil {
		constructor, _ := vm.Get("Event").(*goja.Object)
		if constructor != nil {
			object, _ = vm.New(constructor, vm.ToValue(event.Type), eventInitValue(vm, event))
		}
	}
	if object == nil {
		object = vm.NewObject()
	}
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
		_ = object.Set("defaultPrevented", event.DefaultPrevented != nil && event.DefaultPrevented())
		return goja.Undefined()
	})
	_ = object.Set("stopPropagation", func(goja.FunctionCall) goja.Value {
		event.StopPropagation()
		return goja.Undefined()
	})
	_ = object.Set("stopImmediatePropagation", func(goja.FunctionCall) goja.Value {
		event.StopImmediatePropagation()
		return goja.Undefined()
	})
	_ = object.Set("defaultPrevented", event.DefaultPrevented != nil && event.DefaultPrevented())
	return object
}

func eventInitValue(vm *goja.Runtime, event domapi.Event) goja.Value {
	init := vm.NewObject()
	_ = init.Set("bubbles", event.Bubbles)
	_ = init.Set("cancelable", event.Cancelable)
	return init
}

func errorsIsRuntimeStop(err error) bool {
	return err == nil || errors.Is(err, errRuntimeStopped) || errors.Is(err, context.Canceled)
}
