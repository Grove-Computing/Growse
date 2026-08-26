package javascript

import (
	"context"
	"errors"
	"fmt"

	"github.com/dop251/goja"
	wabinbinary "github.com/tetratelabs/wabin/binary"
	wabinwasm "github.com/tetratelabs/wabin/wasm"
	"github.com/tetratelabs/wazero"
	wazeroapi "github.com/tetratelabs/wazero/api"
)

func (api *wasmAPI) installImportProviders(vm *goja.Runtime, runtime wazero.Runtime, module *wabinwasm.Module, imports goja.Value) error {
	for moduleIndex, moduleName := range sortedImportModules(module) {
		entries := importsForModule(module, moduleName)
		provider := &wabinwasm.Module{}
		hostName := fmt.Sprintf("__growse_wasm_host_%d", moduleIndex)
		host := runtime.NewHostModuleBuilder(hostName)
		hostFunctions := 0
		functionIndex, tableIndex, globalIndex := uint32(0), uint32(0), uint32(0)
		memoryDefined := false
		memoryBindings := make(map[string]*wasmMemoryValue)
		globalBindings := make(map[string]*wasmGlobalValue)

		for _, imported := range entries {
			if err := validateImportLimits(imported); err != nil {
				return fmt.Errorf("import %s.%s: %w", moduleName, imported.Name, err)
			}
			value, err := importMember(vm, imports, moduleName, imported.Name)
			if err != nil {
				return err
			}
			switch imported.Type {
			case wabinwasm.ExternTypeFunc:
				callable, ok := goja.AssertFunction(value)
				if !ok {
					return fmt.Errorf("import %s.%s must be a JavaScript function", moduleName, imported.Name)
				}
				if int(imported.DescFunc) >= len(module.TypeSection) {
					return errors.New("WebAssembly function import has invalid type index")
				}
				functionType := module.TypeSection[imported.DescFunc]
				hostExport := fmt.Sprintf("f%d", hostFunctions)
				host.NewFunctionBuilder().WithGoFunction(api.hostFunction(vm, callable, functionType),
					toWazeroTypes(functionType.Params), toWazeroTypes(functionType.Results)).Export(hostExport)
				typeIndex := uint32(len(provider.TypeSection))
				provider.TypeSection = append(provider.TypeSection, &wabinwasm.FunctionType{
					Params: append([]wabinwasm.ValueType(nil), functionType.Params...), Results: append([]wabinwasm.ValueType(nil), functionType.Results...),
				})
				provider.ImportSection = append(provider.ImportSection, &wabinwasm.Import{
					Type: wabinwasm.ExternTypeFunc, Module: hostName, Name: hostExport, DescFunc: typeIndex,
				})
				provider.ExportSection = append(provider.ExportSection, &wabinwasm.Export{Type: wabinwasm.ExternTypeFunc, Name: imported.Name, Index: functionIndex})
				functionIndex++
				hostFunctions++
			case wabinwasm.ExternTypeMemory:
				memory := api.memories[value.ToObject(vm)]
				if memory == nil || memoryDefined {
					return fmt.Errorf("import %s.%s must be the only WebAssembly.Memory", moduleName, imported.Name)
				}
				provider.MemorySection = &wabinwasm.Memory{Min: memory.initial, Max: memory.maximum, IsMaxEncoded: true}
				provider.ExportSection = append(provider.ExportSection, &wabinwasm.Export{Type: wabinwasm.ExternTypeMemory, Name: imported.Name, Index: 0})
				memoryDefined = true
				memoryBindings[imported.Name] = memory
			case wabinwasm.ExternTypeGlobal:
				global := api.globals[value.ToObject(vm)]
				if global == nil {
					return fmt.Errorf("import %s.%s must be a WebAssembly.Global", moduleName, imported.Name)
				}
				constant, err := constantExpression(global.valueType, global.current())
				if err != nil {
					return err
				}
				provider.GlobalSection = append(provider.GlobalSection, &wabinwasm.Global{
					Type: &wabinwasm.GlobalType{ValType: global.valueType, Mutable: global.mutable}, Init: constant,
				})
				provider.ExportSection = append(provider.ExportSection, &wabinwasm.Export{Type: wabinwasm.ExternTypeGlobal, Name: imported.Name, Index: globalIndex})
				globalBindings[imported.Name] = global
				globalIndex++
			case wabinwasm.ExternTypeTable:
				table := api.tables[value.ToObject(vm)]
				if table == nil {
					return fmt.Errorf("import %s.%s must be a WebAssembly.Table", moduleName, imported.Name)
				}
				maximum := table.maximum
				provider.TableSection = append(provider.TableSection, &wabinwasm.Table{
					Min: uint32(len(table.values)), Max: &maximum, Type: tableRefType(table.element),
				})
				provider.ExportSection = append(provider.ExportSection, &wabinwasm.Export{Type: wabinwasm.ExternTypeTable, Name: imported.Name, Index: tableIndex})
				tableIndex++
			default:
				return fmt.Errorf("unsupported WebAssembly import kind 0x%x", imported.Type)
			}
		}
		if hostFunctions != 0 {
			if _, err := host.Instantiate(api.ctx); err != nil {
				return fmt.Errorf("instantiate JavaScript function imports: %w", err)
			}
		}
		compiled, err := runtime.CompileModule(api.ctx, wabinbinary.EncodeModule(provider))
		if err != nil {
			return fmt.Errorf("compile import provider %q: %w", moduleName, err)
		}
		providerInstance, err := runtime.InstantiateModule(api.ctx, compiled, wazero.NewModuleConfig().WithName(moduleName).WithStartFunctions())
		if err != nil {
			return fmt.Errorf("instantiate import provider %q: %w", moduleName, err)
		}
		for name, memory := range memoryBindings {
			newMemory := providerInstance.ExportedMemory(name)
			if newMemory == nil {
				return fmt.Errorf("import provider did not export memory %s.%s", moduleName, name)
			}
			copyWasmMemory(memory, newMemory)
			memory.memory = newMemory
			if memory.buffered {
				memory.buffer.Detach()
				memory.buffered = false
			}
		}
		for name, global := range globalBindings {
			newGlobal := providerInstance.ExportedGlobal(name)
			if newGlobal == nil {
				return fmt.Errorf("import provider did not export global %s.%s", moduleName, name)
			}
			global.global = newGlobal
		}
	}
	return nil
}

func (api *wasmAPI) hostFunction(vm *goja.Runtime, callable goja.Callable, functionType *wabinwasm.FunctionType) wazeroapi.GoFunction {
	return wazeroapi.GoFunc(func(_ context.Context, stack []uint64) {
		arguments := make([]goja.Value, len(functionType.Params))
		for index, valueType := range functionType.Params {
			arguments[index] = decodeWasmValue(vm, stack[index], valueType)
		}
		result, err := callable(goja.Undefined(), arguments...)
		if err != nil {
			panic(err)
		}
		if len(functionType.Results) == 0 {
			return
		}
		if len(functionType.Results) == 1 {
			encoded, err := encodeWasmValue(result, functionType.Results[0])
			if err != nil {
				panic(err)
			}
			stack[0] = encoded
			return
		}
		object := result.ToObject(vm)
		for index, valueType := range functionType.Results {
			encoded, err := encodeWasmValue(object.Get(fmt.Sprintf("%d", index)), valueType)
			if err != nil {
				panic(err)
			}
			stack[index] = encoded
		}
	})
}

func importsForModule(module *wabinwasm.Module, moduleName string) []*wabinwasm.Import {
	var result []*wabinwasm.Import
	for _, imported := range module.ImportSection {
		if imported.Module == moduleName {
			result = append(result, imported)
		}
	}
	return result
}

func importMember(vm *goja.Runtime, imports goja.Value, moduleName, name string) (goja.Value, error) {
	if goja.IsUndefined(imports) || goja.IsNull(imports) {
		return nil, fmt.Errorf("missing import object for %s.%s", moduleName, name)
	}
	root := imports.ToObject(vm)
	module := root.Get(moduleName)
	if goja.IsUndefined(module) || goja.IsNull(module) {
		return nil, fmt.Errorf("missing import module %q", moduleName)
	}
	value := module.ToObject(vm).Get(name)
	if goja.IsUndefined(value) {
		return nil, fmt.Errorf("missing import %s.%s", moduleName, name)
	}
	return value, nil
}

func toWazeroTypes(types []wabinwasm.ValueType) []wazeroapi.ValueType {
	result := make([]wazeroapi.ValueType, len(types))
	copy(result, types)
	return result
}

func tableRefType(element string) wabinwasm.RefType {
	if element == "externref" {
		return wabinwasm.RefTypeExternref
	}
	return wabinwasm.RefTypeFuncref
}

func (global *wasmGlobalValue) current() uint64 {
	if global.global != nil {
		return global.global.Get()
	}
	return global.initial
}

func copyWasmMemory(source *wasmMemoryValue, target wazeroapi.Memory) {
	if source == nil || source.memory == nil || target == nil {
		return
	}
	length := source.memory.Size()
	if target.Size() < length {
		length = target.Size()
	}
	data, ok := source.memory.Read(0, length)
	if ok {
		_ = target.Write(0, append([]byte(nil), data...))
	}
}

func validateImportLimits(imported *wabinwasm.Import) error {
	if imported.Type == wabinwasm.ExternTypeMemory && imported.DescMem != nil {
		return validateWasmMemoryLimits(imported.DescMem.Min, imported.DescMem.Max, imported.DescMem.IsMaxEncoded)
	}
	if imported.Type == wabinwasm.ExternTypeTable {
		return validateWasmTableLimits(imported.DescTable)
	}
	return nil
}
