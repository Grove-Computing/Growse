package javascript

import (
	"strings"

	domapi "github.com/Grove-Computing/Growse/internal/webapi/dom"
	"github.com/dop251/goja"
)

func (runtime *Runtime) setCurrentScript(vm *goja.Runtime, element *domapi.Element) {
	if element == nil {
		runtime.currentScript = nil
		return
	}
	object, _ := runtime.elementValue(vm, element).(*goja.Object)
	runtime.currentScript = object
}

func (runtime *Runtime) initialScriptElement(documentOrder int) *domapi.Element {
	if runtime.domAPI == nil || documentOrder < 0 {
		return nil
	}
	order := 0
	for _, element := range runtime.domAPI.GetElementsByTagName("script") {
		typeValue, present := element.GetAttribute("type")
		if !javaScriptElementType(typeValue, present) {
			continue
		}
		if order == documentOrder {
			return element
		}
		order++
	}
	return nil
}

func javaScriptElementType(value string, present bool) bool {
	if !present || strings.TrimSpace(value) == "" {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "module", "text/javascript", "application/javascript", "application/ecmascript", "text/ecmascript":
		return true
	default:
		return false
	}
}

func (runtime *Runtime) elementCollectionValue(vm *goja.Runtime, elements []*domapi.Element) goja.Value {
	values := make([]any, len(elements))
	for index, element := range elements {
		values[index] = runtime.elementValue(vm, element)
	}
	collection := vm.NewArray(values...)
	_ = collection.Set("item", func(call goja.FunctionCall) goja.Value {
		index := int(call.Argument(0).ToInteger())
		if index < 0 || index >= len(elements) {
			return goja.Null()
		}
		return runtime.elementValue(vm, elements[index])
	})
	return collection
}

func (runtime *Runtime) styleSheetCollectionValue(vm *goja.Runtime) goja.Value {
	var elements []*domapi.Element
	for _, element := range runtime.domAPI.GetElementsByTagName("*") {
		switch strings.ToLower(element.TagName()) {
		case "style":
			elements = append(elements, element)
		case "link":
			if linkRelIncludes(element, "stylesheet") {
				elements = append(elements, element)
			}
		}
	}
	values := make([]any, len(elements))
	for index, element := range elements {
		values[index] = runtime.styleSheetValue(vm, element)
	}
	collection := vm.NewArray(values...)
	_ = collection.Set("item", func(call goja.FunctionCall) goja.Value {
		index := int(call.Argument(0).ToInteger())
		if index < 0 || index >= len(elements) {
			return goja.Null()
		}
		return runtime.styleSheetValue(vm, elements[index])
	})
	return collection
}

func (runtime *Runtime) styleSheetValue(vm *goja.Runtime, element *domapi.Element) goja.Value {
	object := vm.NewObject()
	_ = object.DefineDataProperty("ownerNode", runtime.elementValue(vm, element), goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE)
	href := ""
	if strings.EqualFold(element.TagName(), "link") {
		href, _ = element.GetAttribute("href")
		if resolved, err := runtime.resolvePageResourceURL(href); err == nil {
			href = resolved.String()
		}
	}
	_ = object.DefineDataProperty("href", vm.ToValue(href), goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE)
	_ = object.DefineDataProperty("disabled", vm.ToValue(false), goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE)
	_ = object.DefineDataProperty("cssRules", vm.NewArray(), goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE)
	return object
}
