package javascript

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"math/big"
	"sort"

	"github.com/dop251/goja"
	wabinbinary "github.com/tetratelabs/wabin/binary"
	"github.com/tetratelabs/wabin/leb128"
	wabinwasm "github.com/tetratelabs/wabin/wasm"
	"github.com/tetratelabs/wazero"
	wazeroapi "github.com/tetratelabs/wazero/api"
)

const wasmMemoryLimitPages = 4_096

type wasmModuleValue struct {
	binary  []byte
	decoded *wabinwasm.Module
}

type wasmInstanceValue struct {
	runtime wazero.Runtime
	module  wazeroapi.Module
}

type wasmMemoryValue struct {
	initial  uint32
	maximum  uint32
	memory   wazeroapi.Memory
	buffer   goja.ArrayBuffer
	buffered bool
}

type wasmGlobalValue struct {
	valueType wabinwasm.ValueType
	mutable   bool
	initial   uint64
	global    wazeroapi.Global
}

type wasmTableValue struct {
	element string
	maximum uint32
	values  []goja.Value
}

type wasmAPI struct {
	ctx context.Context

	modules   map[*goja.Object]*wasmModuleValue
	instances map[*goja.Object]*wasmInstanceValue
	memories  map[*goja.Object]*wasmMemoryValue
	globals   map[*goja.Object]*wasmGlobalValue
	tables    map[*goja.Object]*wasmTableValue
	runtimes  []wazero.Runtime

	moduleConstructor   *goja.Object
	instanceConstructor *goja.Object
	memoryConstructor   *goja.Object
	globalConstructor   *goja.Object
	tableConstructor    *goja.Object
}

func newWasmAPI(ctx context.Context) *wasmAPI {
	return &wasmAPI{
		ctx: ctx, modules: make(map[*goja.Object]*wasmModuleValue), instances: make(map[*goja.Object]*wasmInstanceValue),
		memories: make(map[*goja.Object]*wasmMemoryValue), globals: make(map[*goja.Object]*wasmGlobalValue), tables: make(map[*goja.Object]*wasmTableValue),
	}
}

func (api *wasmAPI) install(vm *goja.Runtime) error {
	namespace := vm.NewObject()
	if err := namespace.Set("validate", func(call goja.FunctionCall) goja.Value {
		binary, err := wasmBytes(vm, call.Argument(0))
		if err != nil {
			return vm.ToValue(false)
		}
		_, err = api.compile(binary)
		return vm.ToValue(err == nil)
	}); err != nil {
		return err
	}
	if err := namespace.Set("Module", func(call goja.ConstructorCall) *goja.Object {
		binary, err := wasmBytes(vm, call.Argument(0))
		if err != nil {
			panic(vm.NewTypeError("WebAssembly.Module: %v", err))
		}
		module, err := api.compile(binary)
		if err != nil {
			panic(vm.NewGoError(fmt.Errorf("WebAssembly.CompileError: %w", err)))
		}
		api.modules[call.This] = module
		return call.This
	}); err != nil {
		return err
	}
	if err := namespace.Set("Instance", func(call goja.ConstructorCall) *goja.Object {
		module := api.requireModule(vm, call.Argument(0))
		instance, err := api.instantiate(vm, module, call.Argument(1))
		if err != nil {
			panic(vm.NewGoError(fmt.Errorf("WebAssembly.LinkError: %w", err)))
		}
		api.instances[call.This] = instance
		api.defineInstanceExports(vm, call.This, instance, module)
		return call.This
	}); err != nil {
		return err
	}
	if err := namespace.Set("Memory", func(call goja.ConstructorCall) *goja.Object {
		memory, err := api.newMemory(vm, call.Argument(0))
		if err != nil {
			panic(vm.NewTypeError("WebAssembly.Memory: %v", err))
		}
		api.memories[call.This] = memory
		api.defineMemory(vm, call.This, memory)
		return call.This
	}); err != nil {
		return err
	}
	if err := namespace.Set("Global", func(call goja.ConstructorCall) *goja.Object {
		global, err := api.newGlobal(vm, call.Argument(0), call.Argument(1))
		if err != nil {
			panic(vm.NewTypeError("WebAssembly.Global: %v", err))
		}
		api.globals[call.This] = global
		api.defineGlobal(vm, call.This, global)
		return call.This
	}); err != nil {
		return err
	}
	if err := namespace.Set("Table", func(call goja.ConstructorCall) *goja.Object {
		table, err := newWasmTable(vm, call.Argument(0), call.Argument(1))
		if err != nil {
			panic(vm.NewTypeError("WebAssembly.Table: %v", err))
		}
		api.tables[call.This] = table
		api.defineTable(vm, call.This, table)
		return call.This
	}); err != nil {
		return err
	}

	api.moduleConstructor = namespace.Get("Module").ToObject(vm)
	api.instanceConstructor = namespace.Get("Instance").ToObject(vm)
	api.memoryConstructor = namespace.Get("Memory").ToObject(vm)
	api.globalConstructor = namespace.Get("Global").ToObject(vm)
	api.tableConstructor = namespace.Get("Table").ToObject(vm)
	if err := namespace.Set("compile", func(call goja.FunctionCall) goja.Value {
		promise, resolve, reject := vm.NewPromise()
		binary, err := wasmBytes(vm, call.Argument(0))
		if err == nil {
			var module *wasmModuleValue
			module, err = api.compile(binary)
			if err == nil {
				_ = resolve(api.moduleObject(vm, module))
			}
		}
		if err != nil {
			_ = reject(vm.NewGoError(fmt.Errorf("WebAssembly.CompileError: %w", err)))
		}
		return vm.ToValue(promise)
	}); err != nil {
		return err
	}
	if err := namespace.Set("instantiate", func(call goja.FunctionCall) goja.Value {
		promise, resolve, reject := vm.NewPromise()
		value := call.Argument(0)
		module, fromModule := api.modules[value.ToObject(vm)]
		var err error
		if !fromModule {
			var source []byte
			source, err = wasmBytes(vm, value)
			if err == nil {
				module, err = api.compile(source)
			}
		}
		if err == nil {
			var instance *wasmInstanceValue
			instance, err = api.instantiate(vm, module, call.Argument(1))
			if err == nil {
				instanceObject := api.instanceObject(vm, instance, module)
				if fromModule {
					_ = resolve(instanceObject)
				} else {
					result := vm.NewObject()
					_ = result.Set("module", api.moduleObject(vm, module))
					_ = result.Set("instance", instanceObject)
					_ = resolve(result)
				}
			}
		}
		if err != nil {
			_ = reject(vm.NewGoError(fmt.Errorf("WebAssembly.LinkError: %w", err)))
		}
		return vm.ToValue(promise)
	}); err != nil {
		return err
	}
	return vm.Set("WebAssembly", namespace)
}

func (api *wasmAPI) compile(source []byte) (*wasmModuleValue, error) {
	decoded, err := wabinbinary.DecodeModule(source, wabinwasm.CoreFeaturesV2)
	if err != nil {
		return nil, err
	}
	runtime := api.newRuntime()
	compiled, err := runtime.CompileModule(api.ctx, source)
	if err != nil {
		_ = runtime.Close(api.ctx)
		return nil, err
	}
	_ = compiled.Close(api.ctx)
	_ = runtime.Close(api.ctx)
	return &wasmModuleValue{binary: append([]byte(nil), source...), decoded: decoded}, nil
}

func (api *wasmAPI) instantiate(vm *goja.Runtime, module *wasmModuleValue, imports goja.Value) (*wasmInstanceValue, error) {
	if module == nil {
		return nil, errors.New("first argument must be a WebAssembly.Module")
	}
	runtime := api.newRuntime()
	if err := api.installImportProviders(vm, runtime, module.decoded, imports); err != nil {
		_ = runtime.Close(api.ctx)
		return nil, err
	}
	compiled, err := runtime.CompileModule(api.ctx, module.binary)
	if err != nil {
		_ = runtime.Close(api.ctx)
		return nil, err
	}
	instance, err := runtime.InstantiateModule(api.ctx, compiled, wazero.NewModuleConfig().WithName("").WithStartFunctions())
	if err != nil {
		_ = compiled.Close(api.ctx)
		_ = runtime.Close(api.ctx)
		return nil, err
	}
	api.runtimes = append(api.runtimes, runtime)
	return &wasmInstanceValue{runtime: runtime, module: instance}, nil
}

func (api *wasmAPI) newRuntime() wazero.Runtime {
	config := wazero.NewRuntimeConfig().WithMemoryLimitPages(wasmMemoryLimitPages).WithCloseOnContextDone(true)
	return wazero.NewRuntimeWithConfig(api.ctx, config)
}

func (api *wasmAPI) requireModule(vm *goja.Runtime, value goja.Value) *wasmModuleValue {
	if module := api.modules[value.ToObject(vm)]; module != nil {
		return module
	}
	panic(vm.NewTypeError("first argument must be a WebAssembly.Module"))
}

func (api *wasmAPI) moduleObject(vm *goja.Runtime, module *wasmModuleValue) *goja.Object {
	object := vm.NewObject()
	_ = object.SetPrototype(api.moduleConstructor.Get("prototype").ToObject(vm))
	api.modules[object] = module
	return object
}

func (api *wasmAPI) instanceObject(vm *goja.Runtime, instance *wasmInstanceValue, module *wasmModuleValue) *goja.Object {
	object := vm.NewObject()
	_ = object.SetPrototype(api.instanceConstructor.Get("prototype").ToObject(vm))
	api.instances[object] = instance
	api.defineInstanceExports(vm, object, instance, module)
	return object
}

func (api *wasmAPI) defineInstanceExports(vm *goja.Runtime, object *goja.Object, instance *wasmInstanceValue, module *wasmModuleValue) {
	exports := vm.NewObject()
	for _, export := range module.decoded.ExportSection {
		switch export.Type {
		case wabinwasm.ExternTypeFunc:
			function := instance.module.ExportedFunction(export.Name)
			if function != nil {
				_ = exports.Set(export.Name, api.functionValue(vm, function))
			}
		case wabinwasm.ExternTypeMemory:
			memory := instance.module.ExportedMemory(export.Name)
			if memory != nil {
				_ = exports.Set(export.Name, api.memoryObject(vm, &wasmMemoryValue{memory: memory}))
			}
		case wabinwasm.ExternTypeGlobal:
			global := instance.module.ExportedGlobal(export.Name)
			if global != nil {
				_, mutable := global.(wazeroapi.MutableGlobal)
				_ = exports.Set(export.Name, api.globalObject(vm, &wasmGlobalValue{valueType: global.Type(), mutable: mutable, global: global}))
			}
		case wabinwasm.ExternTypeTable:
			if table := exportedTableDescriptor(module.decoded, export.Index); table != nil {
				_ = exports.Set(export.Name, api.tableObject(vm, table))
			}
		}
	}
	_ = object.DefineDataProperty("exports", exports, goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE)
}

func (api *wasmAPI) functionValue(vm *goja.Runtime, function wazeroapi.Function) goja.Value {
	return vm.ToValue(func(call goja.FunctionCall) goja.Value {
		definition := function.Definition()
		params := make([]uint64, len(definition.ParamTypes()))
		for index, valueType := range definition.ParamTypes() {
			value, err := encodeWasmValue(call.Argument(index), valueType)
			if err != nil {
				panic(vm.NewTypeError("WebAssembly function argument %d: %v", index, err))
			}
			params[index] = value
		}
		results, err := function.Call(api.ctx, params...)
		if err != nil {
			panic(vm.NewGoError(fmt.Errorf("WebAssembly.RuntimeError: %w", err)))
		}
		values := make([]goja.Value, len(results))
		for index, result := range results {
			values[index] = decodeWasmValue(vm, result, definition.ResultTypes()[index])
		}
		if len(values) == 0 {
			return goja.Undefined()
		}
		if len(values) == 1 {
			return values[0]
		}
		return vm.ToValue(values)
	})
}

func wasmBytes(vm *goja.Runtime, value goja.Value) ([]byte, error) {
	var result []byte
	if err := vm.ExportTo(value, &result); err != nil {
		return nil, errors.New("source must be an ArrayBuffer or typed array")
	}
	return append([]byte(nil), result...), nil
}

func encodeWasmValue(value goja.Value, valueType wazeroapi.ValueType) (uint64, error) {
	switch valueType {
	case wazeroapi.ValueTypeI32:
		return wazeroapi.EncodeI32(int32(value.ToInteger())), nil
	case wazeroapi.ValueTypeI64:
		if integer, ok := value.Export().(*big.Int); ok {
			return uint64(integer.Int64()), nil
		}
		return uint64(value.ToInteger()), nil
	case wazeroapi.ValueTypeF32:
		return wazeroapi.EncodeF32(float32(value.ToFloat())), nil
	case wazeroapi.ValueTypeF64:
		return wazeroapi.EncodeF64(value.ToFloat()), nil
	default:
		return 0, fmt.Errorf("unsupported value type 0x%x", valueType)
	}
}

func decodeWasmValue(vm *goja.Runtime, value uint64, valueType wazeroapi.ValueType) goja.Value {
	switch valueType {
	case wazeroapi.ValueTypeI32:
		return vm.ToValue(int32(value))
	case wazeroapi.ValueTypeI64:
		return vm.ToValue(big.NewInt(int64(value)))
	case wazeroapi.ValueTypeF32:
		return vm.ToValue(wazeroapi.DecodeF32(value))
	case wazeroapi.ValueTypeF64:
		return vm.ToValue(wazeroapi.DecodeF64(value))
	default:
		return goja.Undefined()
	}
}

func (api *wasmAPI) close() {
	for _, runtime := range api.runtimes {
		_ = runtime.Close(context.Background())
	}
	api.runtimes = nil
}

func descriptorObject(value goja.Value) (*goja.Object, error) {
	object, ok := value.(*goja.Object)
	if !ok || goja.IsNull(value) || goja.IsUndefined(value) {
		return nil, errors.New("descriptor must be an object")
	}
	return object, nil
}

func descriptorLimit(object *goja.Object, name string, required bool) (uint32, bool, error) {
	value := object.Get(name)
	if goja.IsUndefined(value) {
		if required {
			return 0, false, fmt.Errorf("descriptor.%s is required", name)
		}
		return 0, false, nil
	}
	integer := value.ToInteger()
	if integer < 0 || integer > math.MaxUint32 {
		return 0, false, fmt.Errorf("descriptor.%s is out of range", name)
	}
	return uint32(integer), true, nil
}

func constantExpression(valueType wabinwasm.ValueType, bits uint64) (*wabinwasm.ConstantExpression, error) {
	switch valueType {
	case wabinwasm.ValueTypeI32:
		return &wabinwasm.ConstantExpression{Opcode: wabinwasm.OpcodeI32Const, Data: leb128.EncodeInt32(int32(bits))}, nil
	case wabinwasm.ValueTypeI64:
		return &wabinwasm.ConstantExpression{Opcode: wabinwasm.OpcodeI64Const, Data: leb128.EncodeInt64(int64(bits))}, nil
	case wabinwasm.ValueTypeF32:
		data := make([]byte, 4)
		binary.LittleEndian.PutUint32(data, uint32(bits))
		return &wabinwasm.ConstantExpression{Opcode: wabinwasm.OpcodeF32Const, Data: data}, nil
	case wabinwasm.ValueTypeF64:
		data := make([]byte, 8)
		binary.LittleEndian.PutUint64(data, bits)
		return &wabinwasm.ConstantExpression{Opcode: wabinwasm.OpcodeF64Const, Data: data}, nil
	default:
		return nil, fmt.Errorf("unsupported global type 0x%x", valueType)
	}
}

func sortedImportModules(module *wabinwasm.Module) []string {
	set := make(map[string]struct{})
	for _, imported := range module.ImportSection {
		set[imported.Module] = struct{}{}
	}
	result := make([]string, 0, len(set))
	for name := range set {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}
