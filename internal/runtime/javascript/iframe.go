package javascript

import (
	"context"

	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
	domapi "github.com/Grove-Computing/Growse/internal/webapi/dom"
	"github.com/dop251/goja"
)

type frameObjectKey struct {
	id         uint64
	generation uint64
}

func (runtime *Runtime) setFrameAccess(frames []runtimemodel.FrameAccess) {
	runtime.frameAccess = make(map[uint64]runtimemodel.FrameAccess, len(frames))
	runtime.frameByElement = make(map[uint64]uint64, len(frames))
	for _, frame := range frames {
		runtime.frameAccess[frame.ID] = frame
		runtime.frameByElement[uint64(frame.ElementID)] = frame.ID
	}
	if runtime.frameWindows == nil {
		runtime.frameWindows = make(map[frameObjectKey]*goja.Object)
	}
	if runtime.frameDocuments == nil {
		runtime.frameDocuments = make(map[frameObjectKey]*goja.Object)
	}
}

// UpdateFrames replaces direct child access on the serialized Page queue.
func (runtime *Runtime) UpdateFrames(frames []runtimemodel.FrameAccess) {
	if runtime == nil {
		return
	}
	copy := append([]runtimemodel.FrameAccess(nil), frames...)
	_ = runtime.runSync(context.Background(), func(*goja.Runtime) error {
		runtime.setFrameAccess(copy)
		return nil
	})
}

func (runtime *Runtime) installFrameElement(vm *goja.Runtime, object *goja.Object, elementID uint64) {
	frameID, ok := runtime.frameByElement[elementID]
	if !ok {
		return
	}
	documentGetter := vm.ToValue(func(goja.FunctionCall) goja.Value {
		frame, ok := runtime.frameAccess[frameID]
		if !ok || !frame.SameOrigin || frame.Document == nil {
			return goja.Null()
		}
		return runtime.frameDocumentValue(vm, frame)
	})
	windowGetter := vm.ToValue(func(goja.FunctionCall) goja.Value {
		frame, ok := runtime.frameAccess[frameID]
		if !ok {
			return goja.Null()
		}
		return runtime.frameWindowValue(vm, frame)
	})
	_ = object.DefineAccessorProperty("contentDocument", documentGetter, nil, goja.FLAG_FALSE, goja.FLAG_TRUE)
	_ = object.DefineAccessorProperty("contentWindow", windowGetter, nil, goja.FLAG_FALSE, goja.FLAG_TRUE)
}

func (runtime *Runtime) frameWindowValue(vm *goja.Runtime, frame runtimemodel.FrameAccess) *goja.Object {
	key := frameObjectKey{id: frame.ID, generation: frame.Generation}
	if existing := runtime.frameWindows[key]; existing != nil {
		return existing
	}
	window := vm.NewObject()
	closed := vm.ToValue(func(goja.FunctionCall) goja.Value {
		current, ok := runtime.frameAccess[frame.ID]
		return vm.ToValue(!ok || current.Generation != frame.Generation)
	})
	_ = window.DefineAccessorProperty("closed", closed, nil, goja.FLAG_FALSE, goja.FLAG_TRUE)
	if frame.SameOrigin {
		document := vm.ToValue(func(goja.FunctionCall) goja.Value {
			current := runtime.requireCurrentFrame(vm, frame.ID, frame.Generation, true)
			return runtime.frameDocumentValue(vm, current)
		})
		_ = window.DefineAccessorProperty("document", document, nil, goja.FLAG_FALSE, goja.FLAG_TRUE)
		location := vm.NewObject()
		href := vm.ToValue(func(goja.FunctionCall) goja.Value {
			current := runtime.requireCurrentFrame(vm, frame.ID, frame.Generation, true)
			return vm.ToValue(current.URL)
		})
		origin := vm.ToValue(func(goja.FunctionCall) goja.Value {
			current := runtime.requireCurrentFrame(vm, frame.ID, frame.Generation, true)
			return vm.ToValue(current.Origin)
		})
		_ = location.DefineAccessorProperty("href", href, nil, goja.FLAG_FALSE, goja.FLAG_TRUE)
		_ = location.DefineAccessorProperty("origin", origin, nil, goja.FLAG_FALSE, goja.FLAG_TRUE)
		_ = window.DefineDataProperty("location", location, goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE)
	}
	runtime.frameWindows[key] = window
	return window
}

func (runtime *Runtime) frameDocumentValue(vm *goja.Runtime, frame runtimemodel.FrameAccess) *goja.Object {
	key := frameObjectKey{id: frame.ID, generation: frame.Generation}
	if existing := runtime.frameDocuments[key]; existing != nil {
		return existing
	}
	document := vm.NewObject()
	_ = document.Set("getElementById", func(call goja.FunctionCall) goja.Value {
		current, api := runtime.currentFrameDOM(vm, frame.ID, frame.Generation)
		return runtime.frameElementValue(vm, current, api.GetElementByID(call.Argument(0).String()))
	})
	_ = document.Set("querySelector", func(call goja.FunctionCall) goja.Value {
		current, api := runtime.currentFrameDOM(vm, frame.ID, frame.Generation)
		return runtime.frameElementValue(vm, current, api.QuerySelector(call.Argument(0).String()))
	})
	_ = document.Set("createElement", func(call goja.FunctionCall) goja.Value {
		current, api := runtime.currentFrameDOM(vm, frame.ID, frame.Generation)
		return runtime.frameElementValue(vm, current, api.CreateElement(call.Argument(0).String()))
	})
	runtime.frameDocuments[key] = document
	return document
}

func (runtime *Runtime) currentFrameDOM(vm *goja.Runtime, frameID, generation uint64) (runtimemodel.FrameAccess, *domapi.API) {
	frame := runtime.requireCurrentFrame(vm, frameID, generation, true)
	onMutation := func() {
		if runtime.environment.FrameMutation == nil || frame.Document == nil {
			return
		}
		if err := runtime.environment.FrameMutation(frame.ID, frame.Generation, frame.Document.Snapshot()); err != nil {
			panic(frameSecurityError(vm, err.Error()))
		}
	}
	return frame, domapi.New(frame.Document, nil, onMutation)
}

func (runtime *Runtime) frameElementValue(vm *goja.Runtime, frame runtimemodel.FrameAccess, element *domapi.Element) goja.Value {
	if element == nil || element.ID() == 0 {
		return goja.Null()
	}
	id := element.ID()
	object := vm.NewObject()
	resolve := func() *domapi.Element {
		_, api := runtime.currentFrameDOM(vm, frame.ID, frame.Generation)
		resolved := api.ElementByNodeID(id)
		if resolved == nil {
			panic(frameSecurityError(vm, "Frame Element is disconnected"))
		}
		return resolved
	}
	_ = object.Set("getAttribute", func(call goja.FunctionCall) goja.Value {
		value, ok := resolve().GetAttribute(call.Argument(0).String())
		if !ok {
			return goja.Null()
		}
		return vm.ToValue(value)
	})
	_ = object.Set("setAttribute", func(call goja.FunctionCall) goja.Value {
		return vm.ToValue(resolve().SetAttribute(call.Argument(0).String(), call.Argument(1).String()))
	})
	getter := vm.ToValue(func(goja.FunctionCall) goja.Value { return vm.ToValue(resolve().Text()) })
	setter := vm.ToValue(func(call goja.FunctionCall) goja.Value {
		resolve().SetText(call.Argument(0).String())
		return goja.Undefined()
	})
	_ = object.DefineAccessorProperty("textContent", getter, setter, goja.FLAG_FALSE, goja.FLAG_TRUE)
	return object
}

func (runtime *Runtime) requireCurrentFrame(vm *goja.Runtime, frameID, generation uint64, sameOrigin bool) runtimemodel.FrameAccess {
	frame, ok := runtime.frameAccess[frameID]
	if !ok || frame.Generation != generation {
		panic(frameSecurityError(vm, "stale Frame object"))
	}
	if sameOrigin && (!frame.SameOrigin || frame.Document == nil) {
		panic(frameSecurityError(vm, "cross-origin Frame access is denied"))
	}
	return frame
}

func frameSecurityError(vm *goja.Runtime, message string) *goja.Object {
	errorObject := vm.NewTypeError(message)
	_ = errorObject.Set("name", "SecurityError")
	return errorObject
}
