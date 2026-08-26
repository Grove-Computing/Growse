package javascript

import (
	"errors"
	"fmt"
	"math"

	"github.com/dop251/goja"
	wabinbinary "github.com/tetratelabs/wabin/binary"
	wabinwasm "github.com/tetratelabs/wabin/wasm"
	"github.com/tetratelabs/wazero"
	wazeroapi "github.com/tetratelabs/wazero/api"
)

func (api *wasmAPI) newMemory(vm *goja.Runtime, descriptor goja.Value) (*wasmMemoryValue, error) {
	object, err := descriptorObject(descriptor)
	if err != nil {
		return nil, err
	}
	initial, _, err := descriptorLimit(object, "initial", true)
	if err != nil {
		return nil, err
	}
	maximum, hasMaximum, err := descriptorLimit(object, "maximum", false)
	if err != nil {
		return nil, err
	}
	if !hasMaximum {
		maximum = wasmMemoryLimitPages
	}
	if initial > maximum || maximum > wasmMemoryLimitPages {
		return nil, fmt.Errorf("memory limits must satisfy initial <= maximum <= %d", wasmMemoryLimitPages)
	}
	memory := &wasmMemoryValue{initial: initial, maximum: maximum}
	module := &wabinwasm.Module{
		MemorySection: &wabinwasm.Memory{Min: initial, Max: maximum, IsMaxEncoded: true},
		ExportSection: []*wabinwasm.Export{{Type: wabinwasm.ExternTypeMemory, Name: "memory", Index: 0}},
	}
	runtime := api.newRuntime()
	compiled, err := runtime.CompileModule(api.ctx, wabinbinary.EncodeModule(module))
	if err != nil {
		_ = runtime.Close(api.ctx)
		return nil, err
	}
	instance, err := runtime.InstantiateModule(api.ctx, compiled, wazero.NewModuleConfig().WithName("").WithStartFunctions())
	if err != nil {
		_ = runtime.Close(api.ctx)
		return nil, err
	}
	memory.memory = instance.ExportedMemory("memory")
	api.runtimes = append(api.runtimes, runtime)
	return memory, nil
}

func (api *wasmAPI) defineMemory(vm *goja.Runtime, object *goja.Object, memory *wasmMemoryValue) {
	bufferGetter := vm.ToValue(func(goja.FunctionCall) goja.Value {
		if memory.memory == nil {
			return goja.Undefined()
		}
		if !memory.buffered {
			data, ok := memory.memory.Read(0, memory.memory.Size())
			if !ok {
				panic(vm.NewGoError(errors.New("WebAssembly.Memory buffer is unavailable")))
			}
			memory.buffer = vm.NewArrayBuffer(data)
			memory.buffered = true
		}
		return vm.ToValue(memory.buffer)
	})
	_ = object.DefineAccessorProperty("buffer", bufferGetter, nil, goja.FLAG_FALSE, goja.FLAG_TRUE)
	_ = object.Set("grow", func(call goja.FunctionCall) goja.Value {
		delta := call.Argument(0).ToInteger()
		if delta < 0 || delta > math.MaxUint32 {
			panic(vm.NewTypeError("WebAssembly.Memory.grow delta is out of range"))
		}
		previous, ok := memory.memory.Grow(uint32(delta))
		if !ok {
			panic(vm.NewGoError(errors.New("WebAssembly.Memory.grow exceeds maximum")))
		}
		if memory.buffered {
			memory.buffer.Detach()
			memory.buffered = false
		}
		return vm.ToValue(previous)
	})
}

func (api *wasmAPI) memoryObject(vm *goja.Runtime, memory *wasmMemoryValue) *goja.Object {
	object := vm.NewObject()
	_ = object.SetPrototype(api.memoryConstructor.Get("prototype").ToObject(vm))
	api.memories[object] = memory
	api.defineMemory(vm, object, memory)
	return object
}

func (api *wasmAPI) newGlobal(vm *goja.Runtime, descriptor, initialValue goja.Value) (*wasmGlobalValue, error) {
	object, err := descriptorObject(descriptor)
	if err != nil {
		return nil, err
	}
	valueType, err := parseWasmValueType(object.Get("value").String())
	if err != nil {
		return nil, err
	}
	mutable := object.Get("mutable").ToBoolean()
	if goja.IsUndefined(initialValue) {
		initialValue = vm.ToValue(0)
	}
	bits, err := encodeWasmValue(initialValue, valueType)
	if err != nil {
		return nil, err
	}
	global := &wasmGlobalValue{valueType: valueType, mutable: mutable, initial: bits}
	constant, err := constantExpression(valueType, bits)
	if err != nil {
		return nil, err
	}
	module := &wabinwasm.Module{
		GlobalSection: []*wabinwasm.Global{{Type: &wabinwasm.GlobalType{ValType: valueType, Mutable: mutable}, Init: constant}},
		ExportSection: []*wabinwasm.Export{{Type: wabinwasm.ExternTypeGlobal, Name: "global", Index: 0}},
	}
	runtime := api.newRuntime()
	compiled, err := runtime.CompileModule(api.ctx, wabinbinary.EncodeModule(module))
	if err != nil {
		_ = runtime.Close(api.ctx)
		return nil, err
	}
	instance, err := runtime.InstantiateModule(api.ctx, compiled, wazero.NewModuleConfig().WithName("").WithStartFunctions())
	if err != nil {
		_ = runtime.Close(api.ctx)
		return nil, err
	}
	global.global = instance.ExportedGlobal("global")
	api.runtimes = append(api.runtimes, runtime)
	return global, nil
}

func (api *wasmAPI) defineGlobal(vm *goja.Runtime, object *goja.Object, global *wasmGlobalValue) {
	currentValue := func() goja.Value {
		bits := global.initial
		if global.global != nil {
			bits = global.global.Get()
		}
		return decodeWasmValue(vm, bits, global.valueType)
	}
	getter := vm.ToValue(func(goja.FunctionCall) goja.Value { return currentValue() })
	setter := vm.ToValue(func(call goja.FunctionCall) goja.Value {
		if !global.mutable {
			panic(vm.NewTypeError("WebAssembly.Global is immutable"))
		}
		bits, err := encodeWasmValue(call.Argument(0), global.valueType)
		if err != nil {
			panic(vm.NewTypeError("WebAssembly.Global value: %v", err))
		}
		global.initial = bits
		mutable, ok := global.global.(wazeroapi.MutableGlobal)
		if !ok {
			panic(vm.NewTypeError("WebAssembly.Global is immutable"))
		}
		mutable.Set(bits)
		return goja.Undefined()
	})
	_ = object.DefineAccessorProperty("value", getter, setter, goja.FLAG_FALSE, goja.FLAG_TRUE)
	_ = object.Set("valueOf", func(goja.FunctionCall) goja.Value { return currentValue() })
}

func (api *wasmAPI) globalObject(vm *goja.Runtime, global *wasmGlobalValue) *goja.Object {
	object := vm.NewObject()
	_ = object.SetPrototype(api.globalConstructor.Get("prototype").ToObject(vm))
	api.globals[object] = global
	api.defineGlobal(vm, object, global)
	return object
}

func newWasmTable(vm *goja.Runtime, descriptor, initialValue goja.Value) (*wasmTableValue, error) {
	object, err := descriptorObject(descriptor)
	if err != nil {
		return nil, err
	}
	element := object.Get("element").String()
	if element == "anyfunc" {
		element = "funcref"
	}
	if element != "funcref" && element != "externref" {
		return nil, errors.New("descriptor.element must be funcref or externref")
	}
	initial, _, err := descriptorLimit(object, "initial", true)
	if err != nil {
		return nil, err
	}
	maximum, hasMaximum, err := descriptorLimit(object, "maximum", false)
	if err != nil {
		return nil, err
	}
	if !hasMaximum {
		maximum = math.MaxUint32
	}
	if initial > maximum {
		return nil, errors.New("table initial exceeds maximum")
	}
	if goja.IsUndefined(initialValue) {
		initialValue = goja.Null()
	}
	values := make([]goja.Value, initial)
	for index := range values {
		values[index] = initialValue
	}
	return &wasmTableValue{element: element, maximum: maximum, values: values}, nil
}

func (api *wasmAPI) defineTable(vm *goja.Runtime, object *goja.Object, table *wasmTableValue) {
	length := vm.ToValue(func(goja.FunctionCall) goja.Value { return vm.ToValue(len(table.values)) })
	_ = object.DefineAccessorProperty("length", length, nil, goja.FLAG_FALSE, goja.FLAG_TRUE)
	_ = object.Set("get", func(call goja.FunctionCall) goja.Value {
		index := call.Argument(0).ToInteger()
		if index < 0 || index >= int64(len(table.values)) {
			panic(vm.NewTypeError("WebAssembly.Table index is out of range"))
		}
		return table.values[index]
	})
	_ = object.Set("set", func(call goja.FunctionCall) goja.Value {
		index := call.Argument(0).ToInteger()
		if index < 0 || index >= int64(len(table.values)) {
			panic(vm.NewTypeError("WebAssembly.Table index is out of range"))
		}
		table.values[index] = call.Argument(1)
		return goja.Undefined()
	})
	_ = object.Set("grow", func(call goja.FunctionCall) goja.Value {
		delta := call.Argument(0).ToInteger()
		if delta < 0 || uint64(len(table.values))+uint64(delta) > uint64(table.maximum) {
			panic(vm.NewGoError(errors.New("WebAssembly.Table.grow exceeds maximum")))
		}
		value := call.Argument(1)
		if goja.IsUndefined(value) {
			value = goja.Null()
		}
		previous := len(table.values)
		for range delta {
			table.values = append(table.values, value)
		}
		return vm.ToValue(previous)
	})
}

func (api *wasmAPI) tableObject(vm *goja.Runtime, table *wasmTableValue) *goja.Object {
	object := vm.NewObject()
	_ = object.SetPrototype(api.tableConstructor.Get("prototype").ToObject(vm))
	api.tables[object] = table
	api.defineTable(vm, object, table)
	return object
}

func exportedTableDescriptor(module *wabinwasm.Module, index uint32) *wasmTableValue {
	imported := uint32(0)
	for _, entry := range module.ImportSection {
		if entry.Type != wabinwasm.ExternTypeTable {
			continue
		}
		if imported == index {
			return tableFromDescriptor(entry.DescTable)
		}
		imported++
	}
	defined := index - imported
	if int(defined) >= len(module.TableSection) {
		return nil
	}
	return tableFromDescriptor(module.TableSection[defined])
}

func tableFromDescriptor(descriptor *wabinwasm.Table) *wasmTableValue {
	if descriptor == nil {
		return nil
	}
	maximum := uint32(math.MaxUint32)
	if descriptor.Max != nil {
		maximum = *descriptor.Max
	}
	element := "funcref"
	if descriptor.Type == wabinwasm.RefTypeExternref {
		element = "externref"
	}
	values := make([]goja.Value, descriptor.Min)
	for index := range values {
		values[index] = goja.Null()
	}
	return &wasmTableValue{element: element, maximum: maximum, values: values}
}

func parseWasmValueType(value string) (wabinwasm.ValueType, error) {
	switch value {
	case "i32":
		return wabinwasm.ValueTypeI32, nil
	case "i64":
		return wabinwasm.ValueTypeI64, nil
	case "f32":
		return wabinwasm.ValueTypeF32, nil
	case "f64":
		return wabinwasm.ValueTypeF64, nil
	default:
		return 0, fmt.Errorf("unsupported value type %q", value)
	}
}
