package javascript

import (
	"strings"

	dommodel "github.com/Grove-Computing/Growse/internal/dom"
	domapi "github.com/Grove-Computing/Growse/internal/webapi/dom"
	"github.com/dop251/goja"
)

func (runtime *Runtime) focusElement(vm *goja.Runtime, element *domapi.Element) {
	if element == nil || !element.IsConnected() || !focusableElement(element) || runtime.activeElementID == uint64(element.ID()) {
		return
	}
	previous := runtime.domAPI.NodeByID(dommodel.NodeID(runtime.activeElementID))
	runtime.activeElementID = uint64(element.ID())
	if previous != nil {
		runtime.dispatchFocusEvent(vm, previous, "blur", element)
		runtime.dispatchFocusEvent(vm, previous, "focusout", element)
	}
	runtime.dispatchFocusEvent(vm, element, "focus", previous)
	runtime.dispatchFocusEvent(vm, element, "focusin", previous)
}

func (runtime *Runtime) blurElement(vm *goja.Runtime, element *domapi.Element) {
	if element == nil || runtime.activeElementID != uint64(element.ID()) {
		return
	}
	runtime.activeElementID = 0
	runtime.dispatchFocusEvent(vm, element, "blur", runtime.domAPI.Body())
	runtime.dispatchFocusEvent(vm, element, "focusout", runtime.domAPI.Body())
}

func focusableElement(element *domapi.Element) bool {
	if element.HasAttribute("disabled") {
		return false
	}
	if element.HasAttribute("tabindex") {
		return true
	}
	switch strings.ToLower(element.TagName()) {
	case "button", "input", "select", "textarea", "summary":
		return true
	case "a", "area":
		return element.HasAttribute("href")
	default:
		return false
	}
}

func (runtime *Runtime) dispatchFocusEvent(vm *goja.Runtime, target *domapi.Element, eventType string, related *domapi.Element) {
	if target == nil {
		return
	}
	init := vm.NewObject()
	_ = init.Set("bubbles", eventType == "focusin" || eventType == "focusout")
	_ = init.Set("relatedTarget", runtime.elementValue(vm, related))
	constructor, _ := vm.Get("FocusEvent").(*goja.Object)
	if constructor == nil {
		return
	}
	event, err := vm.New(constructor, vm.ToValue(eventType), init)
	if err == nil {
		runtime.dispatchJSEvent(vm, target, event)
	}
}

type rangeState struct {
	start       *domapi.Element
	startOffset int
	end         *domapi.Element
	endOffset   int
}

func (runtime *Runtime) newRangeValue(vm *goja.Runtime, root *domapi.Element) goja.Value {
	return runtime.newRangeValueWithState(vm, rangeState{start: root, end: root})
}

func (runtime *Runtime) newRangeValueWithState(vm *goja.Runtime, state rangeState) goja.Value {
	object := vm.NewObject()
	if prototype := domInterfacePrototype(vm, "Range"); prototype != nil {
		_ = object.SetPrototype(prototype)
	}
	defineNode := func(name string, read func() *domapi.Element) {
		_ = object.DefineAccessorProperty(name, vm.ToValue(func(goja.FunctionCall) goja.Value {
			return runtime.elementValue(vm, read())
		}), nil, goja.FLAG_FALSE, goja.FLAG_TRUE)
	}
	defineInt := func(name string, read func() int) {
		_ = object.DefineAccessorProperty(name, vm.ToValue(func(goja.FunctionCall) goja.Value {
			return vm.ToValue(read())
		}), nil, goja.FLAG_FALSE, goja.FLAG_TRUE)
	}
	defineNode("startContainer", func() *domapi.Element { return state.start })
	defineNode("endContainer", func() *domapi.Element { return state.end })
	defineNode("commonAncestorContainer", func() *domapi.Element { return commonRangeAncestor(state.start, state.end) })
	defineInt("startOffset", func() int { return state.startOffset })
	defineInt("endOffset", func() int { return state.endOffset })
	_ = object.DefineAccessorProperty("collapsed", vm.ToValue(func(goja.FunctionCall) goja.Value {
		return vm.ToValue(state.start == state.end && state.startOffset == state.endOffset)
	}), nil, goja.FLAG_FALSE, goja.FLAG_TRUE)
	setBoundary := func(start bool, call goja.FunctionCall) goja.Value {
		nodeObject, ok := call.Argument(0).(*goja.Object)
		node := runtime.elements[nodeObject]
		offset := int(call.Argument(1).ToInteger())
		if !ok || node == nil || offset < 0 || offset > rangeNodeLength(node) {
			panic(vm.NewTypeError("Range boundary is invalid"))
		}
		if start {
			state.start, state.startOffset = node, offset
		} else {
			state.end, state.endOffset = node, offset
		}
		return goja.Undefined()
	}
	_ = object.Set("setStart", func(call goja.FunctionCall) goja.Value { return setBoundary(true, call) })
	_ = object.Set("setEnd", func(call goja.FunctionCall) goja.Value { return setBoundary(false, call) })
	_ = object.Set("selectNodeContents", func(call goja.FunctionCall) goja.Value {
		nodeObject, ok := call.Argument(0).(*goja.Object)
		node := runtime.elements[nodeObject]
		if !ok || node == nil {
			panic(vm.NewTypeError("Range node is invalid"))
		}
		state.start, state.end = node, node
		state.startOffset, state.endOffset = 0, rangeNodeLength(node)
		return goja.Undefined()
	})
	_ = object.Set("collapse", func(call goja.FunctionCall) goja.Value {
		if call.Argument(0).ToBoolean() {
			state.end, state.endOffset = state.start, state.startOffset
		} else {
			state.start, state.startOffset = state.end, state.endOffset
		}
		return goja.Undefined()
	})
	_ = object.Set("cloneRange", func(goja.FunctionCall) goja.Value {
		return runtime.newRangeValueWithState(vm, state)
	})
	_ = object.Set("toString", func(goja.FunctionCall) goja.Value {
		if state.start == nil || state.start != state.end {
			return vm.ToValue("")
		}
		text := []rune(state.start.Text())
		if state.startOffset <= state.endOffset && state.endOffset <= len(text) {
			return vm.ToValue(string(text[state.startOffset:state.endOffset]))
		}
		return vm.ToValue(state.start.Text())
	})
	return object
}

func rangeNodeLength(node *domapi.Element) int {
	if node == nil {
		return 0
	}
	if node.NodeType() == dommodel.NodeText {
		return len([]rune(node.Text()))
	}
	return len(node.ChildNodes())
}

func commonRangeAncestor(left, right *domapi.Element) *domapi.Element {
	ancestors := make(map[dommodel.NodeID]struct{})
	for node := left; node != nil; node = node.ParentNode() {
		ancestors[node.ID()] = struct{}{}
	}
	for node := right; node != nil; node = node.ParentNode() {
		if _, ok := ancestors[node.ID()]; ok {
			return node
		}
	}
	return nil
}
