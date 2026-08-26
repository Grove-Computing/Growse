package javascript

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/Grove-Computing/Growse/internal/network"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
)

// WPT source: html/semantics/scripting-1/the-script-element/module/single-evaluation-1.html.
func TestWPTModuleSharedDependencyEvaluatesOnce(t *testing.T) {
	pageURL := moduleTestURL(t, "https://module.example/index.html")
	sources := map[string]string{
		"https://module.example/left.js":   `import { value } from "./shared.js"; export const left = value;`,
		"https://module.example/right.js":  `import { value } from "./shared.js"; export const right = value;`,
		"https://module.example/shared.js": `globalThis.evaluations = (globalThis.evaluations || 0) + 1; export const value = evaluations;`,
	}
	fetches := make(map[string]int)
	var fetchMu sync.Mutex
	var records [][2]string
	environment := runtimemodel.Environment{
		BaseURL: pageURL,
		Fetch: func(_ context.Context, request *network.Request) (*network.Response, error) {
			fetchMu.Lock()
			fetches[request.URL.String()]++
			fetchMu.Unlock()
			source, ok := sources[request.URL.String()]
			if !ok {
				return nil, fmt.Errorf("unexpected module URL: %s", request.URL)
			}
			return &network.Response{URL: request.URL, StatusCode: 200, ContentType: "text/javascript", Body: []byte(source)}, nil
		},
		ConsoleRecord: func(level, message string) { records = append(records, [2]string{level, message}) },
	}
	script := runtimemodel.Script{
		Engine: runtimemodel.EngineJavaScript, Kind: runtimemodel.ScriptModule, SourceURL: pageURL,
		Source: `import { left } from "./left.js"; import { right } from "./right.js"; console.log([left, right, evaluations].join(","));`,
	}
	runtime := New()
	t.Cleanup(func() { _ = runtime.Stop() })
	if err := runtime.Load(context.Background(), []runtimemodel.Script{script}, environment); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got, want := records, [][2]string{{"log", "1,1,1"}}; !equalRecords(got, want) {
		t.Fatalf("module records = %v, want %v", got, want)
	}
	for moduleURL := range sources {
		if fetches[moduleURL] != 1 {
			t.Fatalf("module fetch count for %s = %d, want 1", moduleURL, fetches[moduleURL])
		}
	}
}

// WPT source: wasm/jsapi/module/constructor.any.js.
func TestWPTWebAssemblyModuleConstructorValidatesBytes(t *testing.T) {
	valid := javascriptArray(t, exportedWasmBinary(t))
	source := fmt.Sprintf(`
		const invalid = new Uint8Array([0, 1, 2, 3]);
		let errorName = "";
		try { new WebAssembly.Module(invalid); } catch (error) { errorName = error.name; }
		const valid = new Uint8Array(%s);
		console.log([WebAssembly.validate(invalid), errorName, new WebAssembly.Module(valid) instanceof WebAssembly.Module].join("|"));`, valid)
	if got := fmt.Sprint(runWasmScript(t, source)); got != "[false|CompileError|true]" {
		t.Fatalf("WebAssembly constructor result = %s", got)
	}
}
