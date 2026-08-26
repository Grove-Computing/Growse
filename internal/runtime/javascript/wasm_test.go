package javascript

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
	wabinbinary "github.com/tetratelabs/wabin/binary"
	wabinwasm "github.com/tetratelabs/wabin/wasm"
)

func TestWebAssemblyValidateCompileInstantiateAndExports(t *testing.T) {
	binary := exportedWasmBinary(t)
	source := fmt.Sprintf(`
		const bytes = new Uint8Array(%s);
		const invalid = new Uint8Array([0, 1, 2, 3]);
		console.log("validate:" + WebAssembly.validate(bytes) + ":" + WebAssembly.validate(invalid));
		const module = new WebAssembly.Module(bytes);
		const instance = new WebAssembly.Instance(module);
		const memory = instance.exports.memory;
		new Uint8Array(memory.buffer)[0] = 9;
		instance.exports.value.value = 11;
		console.log([
			instance.exports.add(20, 22),
			new Uint8Array(memory.buffer)[0],
			instance.exports.value.value,
			instance.exports.table.length,
			memory.grow(1),
			memory.buffer.byteLength
		].join("|"));
		WebAssembly.compile(bytes).then(function (compiled) {
			return WebAssembly.instantiate(compiled);
		}).then(function (asyncInstance) {
			console.log("async:" + asyncInstance.exports.add(40, 2));
		});`, javascriptArray(t, binary))

	got := runWasmScript(t, source)
	want := []string{"validate:true:false", "42|9|11|1|1|131072", "async:42"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("console records = %v, want %v", got, want)
	}
}

func TestWebAssemblyImportsFunctionsMemoryGlobalAndTable(t *testing.T) {
	binary := importedWasmBinary(t)
	source := fmt.Sprintf(`
		const memory = new WebAssembly.Memory({initial: 1, maximum: 2});
		const global = new WebAssembly.Global({value: "i32", mutable: true}, 3);
		const table = new WebAssembly.Table({element: "anyfunc", initial: 1, maximum: 2});
		const module = new WebAssembly.Module(new Uint8Array(%s));
		const instance = new WebAssembly.Instance(module, {env: {
			twice: function (value) { return value * 2; },
			memory: memory,
			global: global,
			table: table
		}});
		new Uint8Array(memory.buffer)[0] = 17;
		global.value = 4;
		console.log([
			instance.exports.run(5),
			new Uint8Array(instance.exports.memory.buffer)[0],
			instance.exports.global.value,
			instance.exports.table.length,
			table.grow(1),
			table.length,
			global.valueOf()
		].join("|"));`, javascriptArray(t, binary))

	got := runWasmScript(t, source)
	want := []string{"14|17|4|1|1|2|4"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("console records = %v, want %v", got, want)
	}
}

func runWasmScript(t *testing.T, source string) []string {
	t.Helper()
	runtime := New()
	t.Cleanup(func() { _ = runtime.Stop() })
	var records []string
	environment := runtimemodel.Environment{ConsoleRecord: func(_, message string) { records = append(records, message) }}
	if err := runtime.Load(context.Background(), []runtimemodel.Script{javaScript(source)}, environment); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	return records
}

func javascriptArray(t *testing.T, binary []byte) string {
	t.Helper()
	values := make([]int, len(binary))
	for index, value := range binary {
		values[index] = int(value)
	}
	encoded, err := json.Marshal(values)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	return string(encoded)
}

func exportedWasmBinary(t *testing.T) []byte {
	t.Helper()
	maximum := uint32(2)
	module := &wabinwasm.Module{
		TypeSection:     []*wabinwasm.FunctionType{{Params: []wabinwasm.ValueType{wabinwasm.ValueTypeI32, wabinwasm.ValueTypeI32}, Results: []wabinwasm.ValueType{wabinwasm.ValueTypeI32}}},
		FunctionSection: []wabinwasm.Index{0},
		TableSection:    []*wabinwasm.Table{{Min: 1, Max: &maximum, Type: wabinwasm.RefTypeFuncref}},
		MemorySection:   &wabinwasm.Memory{Min: 1, Max: 2, IsMaxEncoded: true},
		GlobalSection: []*wabinwasm.Global{{
			Type: &wabinwasm.GlobalType{ValType: wabinwasm.ValueTypeI32, Mutable: true},
			Init: &wabinwasm.ConstantExpression{Opcode: wabinwasm.OpcodeI32Const, Data: []byte{7}},
		}},
		ExportSection: []*wabinwasm.Export{
			{Name: "add", Type: wabinwasm.ExternTypeFunc, Index: 0},
			{Name: "table", Type: wabinwasm.ExternTypeTable, Index: 0},
			{Name: "memory", Type: wabinwasm.ExternTypeMemory, Index: 0},
			{Name: "value", Type: wabinwasm.ExternTypeGlobal, Index: 0},
		},
		CodeSection: []*wabinwasm.Code{{Body: []byte{
			wabinwasm.OpcodeLocalGet, 0,
			wabinwasm.OpcodeLocalGet, 1,
			wabinwasm.OpcodeI32Add,
			wabinwasm.OpcodeEnd,
		}}},
	}
	return wabinbinary.EncodeModule(module)
}

func importedWasmBinary(t *testing.T) []byte {
	t.Helper()
	maximum := uint32(2)
	module := &wabinwasm.Module{
		TypeSection: []*wabinwasm.FunctionType{
			{Params: []wabinwasm.ValueType{wabinwasm.ValueTypeI32}, Results: []wabinwasm.ValueType{wabinwasm.ValueTypeI32}},
		},
		ImportSection: []*wabinwasm.Import{
			{Module: "env", Name: "twice", Type: wabinwasm.ExternTypeFunc, DescFunc: 0},
			{Module: "env", Name: "memory", Type: wabinwasm.ExternTypeMemory, DescMem: &wabinwasm.Memory{Min: 1, Max: 2, IsMaxEncoded: true}},
			{Module: "env", Name: "global", Type: wabinwasm.ExternTypeGlobal, DescGlobal: &wabinwasm.GlobalType{ValType: wabinwasm.ValueTypeI32, Mutable: true}},
			{Module: "env", Name: "table", Type: wabinwasm.ExternTypeTable, DescTable: &wabinwasm.Table{Min: 1, Max: &maximum, Type: wabinwasm.RefTypeFuncref}},
		},
		FunctionSection: []wabinwasm.Index{0},
		ExportSection: []*wabinwasm.Export{
			{Name: "run", Type: wabinwasm.ExternTypeFunc, Index: 1},
			{Name: "memory", Type: wabinwasm.ExternTypeMemory, Index: 0},
			{Name: "global", Type: wabinwasm.ExternTypeGlobal, Index: 0},
			{Name: "table", Type: wabinwasm.ExternTypeTable, Index: 0},
		},
		CodeSection: []*wabinwasm.Code{{Body: []byte{
			wabinwasm.OpcodeLocalGet, 0,
			wabinwasm.OpcodeCall, 0,
			wabinwasm.OpcodeGlobalGet, 0,
			wabinwasm.OpcodeI32Add,
			wabinwasm.OpcodeEnd,
		}}},
	}
	return wabinbinary.EncodeModule(module)
}
