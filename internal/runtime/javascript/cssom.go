package javascript

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/Grove-Computing/Growse/internal/css"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
	stylemodel "github.com/Grove-Computing/Growse/internal/style"
	domapi "github.com/Grove-Computing/Growse/internal/webapi/dom"
	"github.com/dop251/goja"
)

type mediaQueryRecord struct {
	query     string
	object    *goja.Object
	matches   bool
	listeners []goja.Value
	onchange  goja.Value
}

func (runtime *Runtime) installCSSOM(vm *goja.Runtime) error {
	if err := vm.Set("getComputedStyle", func(call goja.FunctionCall) goja.Value {
		element := runtime.domElementForThis(vm, call.Argument(0))
		snapshot := runtime.mustReadRender(vm, element)
		return newComputedStyleObject(vm, snapshot.Style)
	}); err != nil {
		return err
	}
	if err := vm.Set("matchMedia", func(call goja.FunctionCall) goja.Value {
		return runtime.newMediaQueryList(vm, call.Argument(0).String())
	}); err != nil {
		return err
	}
	global := vm.GlobalObject()
	for name, value := range map[string]func() float32{
		"innerWidth":  func() float32 { return runtime.media.ViewportWidth },
		"innerHeight": func() float32 { return runtime.media.ViewportHeight },
		"scrollX":     func() float32 { return 0 },
		"scrollY":     func() float32 { return 0 },
		"pageXOffset": func() float32 { return 0 },
		"pageYOffset": func() float32 { return 0 },
	} {
		read := value
		getter := vm.ToValue(func(goja.FunctionCall) goja.Value { return vm.ToValue(read()) })
		if err := global.DefineAccessorProperty(name, getter, nil, goja.FLAG_FALSE, goja.FLAG_TRUE); err != nil {
			return err
		}
	}
	return nil
}

func (runtime *Runtime) installElementGeometry(vm *goja.Runtime, object *goja.Object, element *domapi.Element) {
	_ = object.Set("getBoundingClientRect", func(goja.FunctionCall) goja.Value {
		return domRectValue(vm, runtime.mustReadRender(vm, element).Rect)
	})
	properties := map[string]func(runtimemodel.RenderSnapshot) float32{
		"clientWidth":  func(value runtimemodel.RenderSnapshot) float32 { return value.ClientWidth },
		"clientHeight": func(value runtimemodel.RenderSnapshot) float32 { return value.ClientHeight },
		"scrollWidth":  func(value runtimemodel.RenderSnapshot) float32 { return value.ScrollWidth },
		"scrollHeight": func(value runtimemodel.RenderSnapshot) float32 { return value.ScrollHeight },
		"scrollTop":    func(runtimemodel.RenderSnapshot) float32 { return 0 },
		"scrollLeft":   func(runtimemodel.RenderSnapshot) float32 { return 0 },
	}
	for name, selectValue := range properties {
		selector := selectValue
		getter := vm.ToValue(func(goja.FunctionCall) goja.Value {
			return vm.ToValue(selector(runtime.mustReadRender(vm, element)))
		})
		_ = object.DefineAccessorProperty(name, getter, nil, goja.FLAG_FALSE, goja.FLAG_TRUE)
	}
}

func (runtime *Runtime) mustReadRender(vm *goja.Runtime, element *domapi.Element) runtimemodel.RenderSnapshot {
	runtime.forcedReadCount++
	if runtime.forcedReadCount > runtime.maxForcedReadsPerTask {
		panic(vm.NewTypeError("forced layout read limit exceeded"))
	}
	if element == nil || runtime.environment.ReadRender == nil {
		panic(vm.NewTypeError("render snapshot is unavailable"))
	}
	ctx := runtime.runtimeCtx
	if ctx == nil {
		ctx = context.Background()
	}
	snapshot, err := runtime.environment.ReadRender(ctx, element.ID())
	if err != nil {
		panic(vm.NewTypeError(err.Error()))
	}
	return snapshot
}

func domRectValue(vm *goja.Runtime, rectangle runtimemodel.DOMRect) *goja.Object {
	object := vm.NewObject()
	values := map[string]float32{
		"x": rectangle.X, "y": rectangle.Y, "width": rectangle.Width, "height": rectangle.Height,
		"top": rectangle.Y, "right": rectangle.X + rectangle.Width,
		"bottom": rectangle.Y + rectangle.Height, "left": rectangle.X,
	}
	for name, value := range values {
		_ = object.DefineDataProperty(name, vm.ToValue(value), goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE)
	}
	_ = object.Set("toJSON", func(goja.FunctionCall) goja.Value {
		result := vm.NewObject()
		for name, value := range values {
			_ = result.Set(name, value)
		}
		return result
	})
	return object
}

type computedStyleObject struct {
	vm      *goja.Runtime
	values  map[string]string
	names   []string
	methods map[string]goja.Value
}

func newComputedStyleObject(vm *goja.Runtime, values map[string]string) *goja.Object {
	copyValues := make(map[string]string, len(values))
	names := make([]string, 0, len(values))
	for name, value := range values {
		copyValues[name] = value
		names = append(names, name)
	}
	sort.Strings(names)
	style := &computedStyleObject{vm: vm, values: copyValues, names: names, methods: make(map[string]goja.Value)}
	style.methods["getPropertyValue"] = vm.ToValue(func(call goja.FunctionCall) goja.Value {
		return vm.ToValue(style.values[strings.ToLower(call.Argument(0).String())])
	})
	style.methods["getPropertyPriority"] = vm.ToValue(func(goja.FunctionCall) goja.Value { return vm.ToValue("") })
	style.methods["item"] = vm.ToValue(func(call goja.FunctionCall) goja.Value {
		index := int(call.Argument(0).ToInteger())
		if index < 0 || index >= len(style.names) {
			return vm.ToValue("")
		}
		return vm.ToValue(style.names[index])
	})
	style.methods["setProperty"] = vm.ToValue(func(goja.FunctionCall) goja.Value { panic(vm.NewTypeError("computed style is read-only")) })
	style.methods["removeProperty"] = vm.ToValue(func(goja.FunctionCall) goja.Value { panic(vm.NewTypeError("computed style is read-only")) })
	object := vm.NewDynamicObject(style)
	if prototype := domInterfacePrototype(vm, "CSSStyleDeclaration"); prototype != nil {
		_ = object.SetPrototype(prototype)
	}
	return object
}

func (style *computedStyleObject) Get(key string) goja.Value {
	if method := style.methods[key]; method != nil {
		return method
	}
	if key == "length" {
		return style.vm.ToValue(len(style.names))
	}
	if key == "cssText" {
		return style.vm.ToValue("")
	}
	return style.vm.ToValue(style.values[cssPropertyName(key)])
}

func (*computedStyleObject) Set(string, goja.Value) bool { return false }
func (style *computedStyleObject) Has(key string) bool {
	if key == "length" || key == "cssText" || style.methods[key] != nil {
		return true
	}
	_, ok := style.values[cssPropertyName(key)]
	return ok
}
func (*computedStyleObject) Delete(string) bool { return false }
func (style *computedStyleObject) Keys() []string {
	return append([]string{"cssText", "length", "getPropertyValue", "getPropertyPriority", "setProperty", "removeProperty", "item"}, style.names...)
}

func (runtime *Runtime) newMediaQueryList(vm *goja.Runtime, query string) *goja.Object {
	query = strings.TrimSpace(query)
	record := &mediaQueryRecord{query: query}
	record.matches = matchesMedia(query, runtime.media)
	object := vm.NewObject()
	record.object = object
	mediaGetter := vm.ToValue(func(goja.FunctionCall) goja.Value { return vm.ToValue(record.query) })
	matchesGetter := vm.ToValue(func(goja.FunctionCall) goja.Value { return vm.ToValue(record.matches) })
	onchangeGetter := vm.ToValue(func(goja.FunctionCall) goja.Value {
		if record.onchange == nil {
			return goja.Null()
		}
		return record.onchange
	})
	onchangeSetter := vm.ToValue(func(call goja.FunctionCall) goja.Value {
		value := call.Argument(0)
		if value == nil || goja.IsNull(value) || goja.IsUndefined(value) {
			record.onchange = nil
		} else if _, ok := goja.AssertFunction(value); ok {
			record.onchange = value
		}
		return goja.Undefined()
	})
	_ = object.DefineAccessorProperty("media", mediaGetter, nil, goja.FLAG_FALSE, goja.FLAG_TRUE)
	_ = object.DefineAccessorProperty("matches", matchesGetter, nil, goja.FLAG_FALSE, goja.FLAG_TRUE)
	_ = object.DefineAccessorProperty("onchange", onchangeGetter, onchangeSetter, goja.FLAG_FALSE, goja.FLAG_TRUE)
	add := func(call goja.FunctionCall) goja.Value {
		value := call.Argument(1)
		if _, ok := goja.AssertFunction(value); !ok {
			return goja.Undefined()
		}
		for _, listener := range record.listeners {
			if listener.SameAs(value) {
				return goja.Undefined()
			}
		}
		record.listeners = append(record.listeners, value)
		return goja.Undefined()
	}
	remove := func(call goja.FunctionCall) goja.Value {
		value := call.Argument(1)
		for index, listener := range record.listeners {
			if listener.SameAs(value) {
				record.listeners = append(record.listeners[:index], record.listeners[index+1:]...)
				break
			}
		}
		return goja.Undefined()
	}
	_ = object.Set("addEventListener", add)
	_ = object.Set("removeEventListener", remove)
	_ = object.Set("addListener", func(call goja.FunctionCall) goja.Value {
		call.Arguments = append([]goja.Value{vm.ToValue("change")}, call.Arguments...)
		return add(call)
	})
	_ = object.Set("removeListener", func(call goja.FunctionCall) goja.Value {
		call.Arguments = append([]goja.Value{vm.ToValue("change")}, call.Arguments...)
		return remove(call)
	})
	runtime.mediaQueries = append(runtime.mediaQueries, record)
	return object
}

func matchesMedia(query string, media runtimemodel.MediaEnvironment) bool {
	queries := css.ParseMediaQueryList(query)
	return len(queries) != 0 && stylemodel.MatchesMediaQueryList(queries, stylemodel.Environment{
		ViewportWidth: media.ViewportWidth, ViewportHeight: media.ViewportHeight, RootFontSize: 16, ResolutionDPI: 96,
		ColorScheme: media.ColorScheme, Hover: media.Hover, Pointer: media.Pointer, ReducedMotion: media.ReducedMotion,
	})
}

// UpdateMediaEnvironment re-evaluates live MediaQueryLists on the Page queue.
func (runtime *Runtime) UpdateMediaEnvironment(media runtimemodel.MediaEnvironment) {
	_ = runtime.runSync(context.Background(), func(vm *goja.Runtime) error {
		runtime.media = media
		if len(runtime.intersectionObservers) != 0 {
			runtime.requestObserverFrame()
		}
		for _, record := range runtime.mediaQueries {
			matched := matchesMedia(record.query, media)
			if matched == record.matches {
				continue
			}
			record.matches = matched
			event := vm.NewObject()
			_ = event.Set("type", "change")
			_ = event.Set("matches", matched)
			_ = event.Set("media", record.query)
			_ = event.Set("target", record.object)
			_ = event.Set("currentTarget", record.object)
			listeners := append([]goja.Value(nil), record.listeners...)
			if record.onchange != nil {
				listeners = append(listeners, record.onchange)
			}
			for _, value := range listeners {
				callback, ok := goja.AssertFunction(value)
				if !ok {
					continue
				}
				if _, err := callback(record.object, event); err != nil {
					runtime.recordError(fmt.Sprintf("JavaScript matchMedia change handler: %v", err))
				}
			}
		}
		return nil
	})
}
