package javascript

import (
	"sort"
	"strings"
	"unicode"

	domapi "github.com/Grove-Computing/Growse/internal/webapi/dom"
	"github.com/dop251/goja"
)

type datasetObject struct {
	vm      *goja.Runtime
	element *domapi.Element
}

func (dataset *datasetObject) Get(key string) goja.Value {
	value, ok := dataset.element.GetAttribute(datasetAttributeName(key))
	if !ok {
		return nil
	}
	return dataset.vm.ToValue(value)
}

func (dataset *datasetObject) Set(key string, value goja.Value) bool {
	name := datasetAttributeName(key)
	return name != "" && dataset.element.SetAttribute(name, value.String())
}

func (dataset *datasetObject) Has(key string) bool {
	return dataset.element.HasAttribute(datasetAttributeName(key))
}

func (dataset *datasetObject) Delete(key string) bool {
	name := datasetAttributeName(key)
	return name != "" && (!dataset.element.HasAttribute(name) || dataset.element.RemoveAttribute(name))
}

func (dataset *datasetObject) Keys() []string {
	var result []string
	for _, name := range dataset.element.AttributeNames() {
		if strings.HasPrefix(name, "data-") {
			result = append(result, datasetPropertyName(name))
		}
	}
	sort.Strings(result)
	return result
}

func datasetAttributeName(key string) string {
	if key == "" || strings.Contains(key, "-") {
		return ""
	}
	var result strings.Builder
	result.WriteString("data-")
	for _, character := range key {
		if unicode.IsUpper(character) {
			result.WriteByte('-')
			result.WriteRune(unicode.ToLower(character))
			continue
		}
		result.WriteRune(character)
	}
	return result.String()
}

func datasetPropertyName(name string) string {
	name = strings.TrimPrefix(name, "data-")
	var result strings.Builder
	uppercase := false
	for _, character := range name {
		if character == '-' {
			uppercase = true
			continue
		}
		if uppercase {
			character = unicode.ToUpper(character)
			uppercase = false
		}
		result.WriteRune(character)
	}
	return result.String()
}

type inlineStyleObject struct {
	vm      *goja.Runtime
	element *domapi.Element
	methods map[string]goja.Value
}

func newInlineStyleObject(vm *goja.Runtime, element *domapi.Element) *goja.Object {
	style := &inlineStyleObject{vm: vm, element: element, methods: make(map[string]goja.Value)}
	style.methods["getPropertyValue"] = vm.ToValue(func(call goja.FunctionCall) goja.Value {
		value, _ := element.InlineStyleProperty(call.Argument(0).String())
		return vm.ToValue(value)
	})
	style.methods["getPropertyPriority"] = vm.ToValue(func(call goja.FunctionCall) goja.Value {
		_, priority := element.InlineStyleProperty(call.Argument(0).String())
		return vm.ToValue(priority)
	})
	style.methods["setProperty"] = vm.ToValue(func(call goja.FunctionCall) goja.Value {
		value := call.Argument(1).String()
		priority := ""
		if !goja.IsUndefined(call.Argument(2)) {
			priority = call.Argument(2).String()
		}
		if value == "" {
			element.RemoveInlineStyleProperty(call.Argument(0).String())
			return goja.Undefined()
		}
		if !element.SetInlineStyleProperty(call.Argument(0).String(), value, priority) {
			panic(vm.NewTypeError("invalid inline style declaration"))
		}
		return goja.Undefined()
	})
	style.methods["removeProperty"] = vm.ToValue(func(call goja.FunctionCall) goja.Value {
		return vm.ToValue(element.RemoveInlineStyleProperty(call.Argument(0).String()))
	})
	style.methods["item"] = vm.ToValue(func(call goja.FunctionCall) goja.Value {
		index := int(call.Argument(0).ToInteger())
		names := element.InlineStylePropertyNames()
		if index < 0 || index >= len(names) {
			return vm.ToValue("")
		}
		return vm.ToValue(names[index])
	})
	return vm.NewDynamicObject(style)
}

func (style *inlineStyleObject) Get(key string) goja.Value {
	if method := style.methods[key]; method != nil {
		return method
	}
	switch key {
	case "cssText":
		return style.vm.ToValue(style.element.InlineStyleText())
	case "length":
		return style.vm.ToValue(len(style.element.InlineStylePropertyNames()))
	}
	value, _ := style.element.InlineStyleProperty(cssPropertyName(key))
	return style.vm.ToValue(value)
}

func (style *inlineStyleObject) Set(key string, value goja.Value) bool {
	if key == "cssText" {
		return style.element.SetInlineStyleText(value.String())
	}
	if key == "length" || style.methods[key] != nil {
		return false
	}
	property := cssPropertyName(key)
	if value.String() == "" {
		style.element.RemoveInlineStyleProperty(property)
		return true
	}
	return style.element.SetInlineStyleProperty(property, value.String(), "")
}

func (style *inlineStyleObject) Has(key string) bool {
	if key == "cssText" || key == "length" || style.methods[key] != nil {
		return true
	}
	value, _ := style.element.InlineStyleProperty(cssPropertyName(key))
	return value != ""
}

func (style *inlineStyleObject) Delete(key string) bool {
	if key == "cssText" || key == "length" || style.methods[key] != nil {
		return false
	}
	style.element.RemoveInlineStyleProperty(cssPropertyName(key))
	return true
}

func (style *inlineStyleObject) Keys() []string {
	result := append([]string{"cssText", "length", "getPropertyValue", "getPropertyPriority", "setProperty", "removeProperty", "item"}, style.element.InlineStylePropertyNames()...)
	return result
}

func cssPropertyName(name string) string {
	if name == "cssFloat" {
		return "float"
	}
	if strings.HasPrefix(name, "--") || strings.Contains(name, "-") {
		return name
	}
	var result strings.Builder
	for _, character := range name {
		if unicode.IsUpper(character) {
			result.WriteByte('-')
			result.WriteRune(unicode.ToLower(character))
			continue
		}
		result.WriteRune(character)
	}
	return result.String()
}
