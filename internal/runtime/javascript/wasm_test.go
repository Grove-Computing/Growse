package javascript

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Grove-Computing/Growse/internal/network"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
	fetchapi "github.com/Grove-Computing/Growse/internal/webapi/fetch"
	"github.com/dop251/goja"
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

func TestWebAssemblyStreamingUsesFetchMIMEAndDoesNotProvideWASI(t *testing.T) {
	moduleBinary := exportedWasmBinary(t)
	wasiBinary := wasiImportWasmBinary()
	baseURL, _ := url.Parse("https://page.example/app/index.html")
	requests := make(chan *network.Request, 4)
	messages := make(chan string, 4)
	environment := runtimemodel.Environment{
		BaseURL:      baseURL,
		FetchLimiter: fetchapi.NewLimiter(8),
		Fetch: func(_ context.Context, request *network.Request) (*network.Response, error) {
			requests <- request
			body := moduleBinary
			contentType := "application/wasm; charset=binary"
			if request.URL.Path == "/app/wrong.wasm" {
				contentType = "application/octet-stream"
			}
			if request.URL.Path == "/app/wasi.wasm" {
				body = wasiBinary
			}
			return &network.Response{
				URL: request.URL, StatusCode: http.StatusOK, Status: "OK",
				Header: http.Header{"Content-Type": {contentType}}, Body: body,
			}, nil
		},
		ConsoleRecord: func(_, message string) { messages <- message },
	}
	source := `
		WebAssembly.compileStreaming(fetch("compile.wasm", {credentials: "omit"})).then(function (module) {
			console.log("compile:" + new WebAssembly.Instance(module).exports.add(20, 22));
		});
		WebAssembly.instantiateStreaming(fetch("instantiate.wasm")).then(function (result) {
			console.log("instantiate:" + result.instance.exports.add(21, 21));
		});
		WebAssembly.compileStreaming(fetch("wrong.wasm")).catch(function (error) {
			console.log("mime:" + error.message);
		});
		WebAssembly.instantiateStreaming(fetch("wasi.wasm")).catch(function (error) {
			console.log("wasi:" + error.message);
		});`
	runtime := New()
	t.Cleanup(func() { _ = runtime.Stop() })
	startJavaScriptRuntime(t, runtime, source, environment)

	var got []string
	for range 4 {
		select {
		case message := <-messages:
			got = append(got, message)
		case <-time.After(time.Second):
			t.Fatalf("streaming results timed out: %v", got)
		}
	}
	joined := strings.Join(got, "\n")
	for _, want := range []string{"compile:42", "instantiate:42", "mime:WebAssembly streaming response MIME must be application/wasm", `wasi:WebAssembly.LinkError: missing import object for wasi_snapshot_preview1.proc_exit`} {
		if !strings.Contains(joined, want) {
			t.Fatalf("streaming records = %v, missing %q", got, want)
		}
	}
	seenOmit := false
	for range 4 {
		select {
		case request := <-requests:
			if request.URL.String() == "https://page.example/app/compile.wasm" && request.Credentials == network.CredentialsOmit {
				seenOmit = true
			}
		case <-time.After(time.Second):
			t.Fatal("streaming Fetch request timed out")
		}
	}
	if !seenOmit {
		t.Fatal("compileStreaming did not use the page Fetch policy")
	}
}

func TestWebAssemblyResourceLimits(t *testing.T) {
	api := newWasmAPI(context.Background(), nil)
	t.Cleanup(api.close)
	valid := exportedWasmBinary(t)

	if _, err := api.validateBinary(make([]byte, maxWasmBinaryBytes+1)); err == nil || !strings.Contains(err.Error(), "binary exceeds") {
		t.Fatalf("oversized validate error = %v", err)
	}
	api.moduleBytes = maxPageWasmBinaryBytes - len(valid) + 1
	if _, err := api.compile(valid); err == nil || !strings.Contains(err.Error(), "Page binary total") {
		t.Fatalf("Page binary quota error = %v", err)
	}
	api.moduleBytes = 0
	api.moduleCount = maxWasmModules
	if _, err := api.compile(valid); err == nil || !strings.Contains(err.Error(), "Module limit") {
		t.Fatalf("Module quota error = %v", err)
	}
	api.moduleCount = 0
	module, err := api.validateBinary(valid)
	if err != nil {
		t.Fatalf("validateBinary() error = %v", err)
	}
	api.instanceCount = maxWasmInstances
	if _, err := api.instantiate(nil, module, nil); err == nil || !strings.Contains(err.Error(), "Instance limit") {
		t.Fatalf("Instance quota error = %v", err)
	}

	tooMuchMemory := wabinbinary.EncodeModule(&wabinwasm.Module{MemorySection: &wabinwasm.Memory{Min: wasmInitialMemoryLimitPages + 1}})
	if _, err := api.validateBinary(tooMuchMemory); err == nil || !strings.Contains(err.Error(), "initial memory") {
		t.Fatalf("initial memory quota error = %v", err)
	}
	tableMaximum := uint32(maxWasmTableElements + 1)
	tooMuchTable := wabinbinary.EncodeModule(&wabinwasm.Module{TableSection: []*wabinwasm.Table{{Min: 1, Max: &tableMaximum, Type: wabinwasm.RefTypeFuncref}}})
	if _, err := api.validateBinary(tooMuchTable); err == nil || !strings.Contains(err.Error(), "maximum table") {
		t.Fatalf("Table quota error = %v", err)
	}

	vmRuntime := New()
	t.Cleanup(func() { _ = vmRuntime.Stop() })
	if err := vmRuntime.Load(context.Background(), []runtimemodel.Script{javaScript(`console.log("loaded")`)}, runtimemodel.Environment{}); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if _, err := vmRuntime.wasmAPI.newMemory(vmRuntime.vm, vmRuntime.vm.ToValue(map[string]any{"initial": wasmInitialMemoryLimitPages + 1})); err == nil {
		t.Fatal("WebAssembly.Memory accepted an initial size over 64 MiB")
	}
	if _, err := newWasmTable(vmRuntime.vm, vmRuntime.vm.ToValue(map[string]any{"element": "funcref", "initial": maxWasmTableElements + 1}), goja.Undefined()); err == nil {
		t.Fatal("WebAssembly.Table accepted more than 65,536 elements")
	}
}

func TestWebAssemblyFunctionCallTimesOut(t *testing.T) {
	runtime := New()
	t.Cleanup(func() { _ = runtime.Stop() })
	var records [][2]string
	environment := runtimemodel.Environment{ConsoleRecord: func(level, message string) {
		records = append(records, [2]string{level, message})
	}}
	source := fmt.Sprintf(`new WebAssembly.Instance(new WebAssembly.Module(new Uint8Array(%s))).exports.spin();`, javascriptArray(t, spinningWasmBinary(false)))
	if err := runtime.Load(context.Background(), []runtimemodel.Script{javaScript(source)}, environment); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	runtime.wasmAPI.callTimeout = 15 * time.Millisecond
	started := time.Now()
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("WASM timeout took %s", elapsed)
	}
	if len(records) != 1 || records[0][0] != "error" || !strings.Contains(records[0][1], "deadline exceeded") {
		t.Fatalf("WASM timeout records = %v", records)
	}
}

func TestWebAssemblyStopsWithPageLifecycle(t *testing.T) {
	runtime := New()
	entered := make(chan struct{})
	var once sync.Once
	environment := runtimemodel.Environment{ConsoleRecord: func(_, message string) {
		if message == "entered" {
			once.Do(func() { close(entered) })
		}
	}}
	source := fmt.Sprintf(`
		const module = new WebAssembly.Module(new Uint8Array(%s));
		new WebAssembly.Instance(module, {env: {notify: function () { console.log("entered"); }}}).exports.spin();`, javascriptArray(t, spinningWasmBinary(true)))
	if err := runtime.Load(context.Background(), []runtimemodel.Script{javaScript(source)}, environment); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	runtime.wasmAPI.callTimeout = time.Minute
	finished := make(chan error, 1)
	go func() { finished <- runtime.Start(context.Background()) }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("WASM function did not start")
	}
	if err := runtime.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("Page Stop did not cancel the running WASM function")
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

func wasiImportWasmBinary() []byte {
	return wabinbinary.EncodeModule(&wabinwasm.Module{
		TypeSection: []*wabinwasm.FunctionType{{Params: []wabinwasm.ValueType{wabinwasm.ValueTypeI32}}},
		ImportSection: []*wabinwasm.Import{{
			Module: "wasi_snapshot_preview1", Name: "proc_exit", Type: wabinwasm.ExternTypeFunc, DescFunc: 0,
		}},
	})
}

func spinningWasmBinary(notify bool) []byte {
	module := &wabinwasm.Module{
		TypeSection:     []*wabinwasm.FunctionType{{}},
		FunctionSection: []wabinwasm.Index{0},
		ExportSection:   []*wabinwasm.Export{{Name: "spin", Type: wabinwasm.ExternTypeFunc, Index: 0}},
	}
	body := []byte{wabinwasm.OpcodeLoop, 0x40, wabinwasm.OpcodeBr, 0, wabinwasm.OpcodeEnd, wabinwasm.OpcodeEnd}
	if notify {
		module.ImportSection = []*wabinwasm.Import{{Module: "env", Name: "notify", Type: wabinwasm.ExternTypeFunc, DescFunc: 0}}
		module.ExportSection[0].Index = 1
		body = append([]byte{wabinwasm.OpcodeCall, 0}, body...)
	}
	module.CodeSection = []*wabinwasm.Code{{Body: body}}
	return wabinbinary.EncodeModule(module)
}
