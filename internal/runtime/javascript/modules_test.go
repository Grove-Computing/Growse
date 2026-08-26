package javascript

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Grove-Computing/Growse/internal/network"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
)

func TestRuntimeLinksAndEvaluatesStaticModuleGraph(t *testing.T) {
	pageURL := moduleTestURL(t, "https://app.example/page/index.html")
	rootURL := moduleTestURL(t, "https://app.example/assets/main.js")
	sources := map[string]string{
		"https://app.example/assets/barrel.js": `
			export { value as renamed } from "./value.js";
			export { default } from "./value.js";`,
		"https://app.example/assets/value.js":   `export const value = 7; export default "default";`,
		"https://app.example/assets/cycle-a.js": `import "./cycle-b.js"; export const a = "a";`,
		"https://app.example/assets/cycle-b.js": `import "./cycle-a.js"; export const b = "b";`,
	}
	fetches := make(map[string]int)
	var fetchMu sync.Mutex
	var records [][2]string
	environment := runtimemodel.Environment{
		BaseURL: pageURL,
		Fetch: func(_ context.Context, request *network.Request) (*network.Response, error) {
			if request.Kind != network.RequestModule || !request.CORS || request.Credentials != network.CredentialsSameOrigin {
				return nil, fmt.Errorf("invalid module request: %#v", request)
			}
			key := request.URL.String()
			fetchMu.Lock()
			fetches[key]++
			fetchMu.Unlock()
			source, ok := sources[key]
			if !ok {
				return &network.Response{URL: request.URL, StatusCode: 404, ContentType: "text/javascript"}, nil
			}
			return &network.Response{URL: request.URL, StatusCode: 200, ContentType: "text/javascript", Body: []byte(source)}, nil
		},
		ConsoleRecord: func(level, message string) { records = append(records, [2]string{level, message}) },
	}
	root := runtimemodel.Script{
		Engine: runtimemodel.EngineJavaScript, Kind: runtimemodel.ScriptModule,
		SourceURL: rootURL, Schedule: runtimemodel.ScriptDefer,
		Source: `
			import primary, { renamed } from "./barrel.js";
			import * as cycleA from "./cycle-a.js";
			import * as cycleB from "./cycle-b.js";
			console.log([primary, renamed, cycleA.a, cycleB.b].join(","));`,
	}
	runtime := New()
	t.Cleanup(func() { _ = runtime.Stop() })
	if err := runtime.Load(context.Background(), []runtimemodel.Script{root}, environment); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got, want := records, [][2]string{{"log", "default,7,a,b"}}; !equalRecords(got, want) {
		t.Fatalf("module records = %v, want %v", got, want)
	}
	fetchMu.Lock()
	defer fetchMu.Unlock()
	if len(fetches) != len(sources) {
		t.Fatalf("module fetches = %v, want every dependency", fetches)
	}
	for moduleURL := range sources {
		if fetches[moduleURL] != 1 {
			t.Fatalf("fetch count for %s = %d, want 1", moduleURL, fetches[moduleURL])
		}
	}
}

func TestRuntimeResolvesInlineModuleFromDocumentURL(t *testing.T) {
	pageURL := moduleTestURL(t, "https://app.example/nested/page.html")
	var requested string
	var records [][2]string
	environment := runtimemodel.Environment{
		BaseURL: pageURL,
		Fetch: func(_ context.Context, request *network.Request) (*network.Response, error) {
			requested = request.URL.String()
			return &network.Response{URL: request.URL, StatusCode: 200, ContentType: "application/javascript", Body: []byte(`export const answer = 42`)}, nil
		},
		ConsoleRecord: func(level, message string) { records = append(records, [2]string{level, message}) },
	}
	script := runtimemodel.Script{
		Engine: runtimemodel.EngineJavaScript, Kind: runtimemodel.ScriptModule, Inline: true,
		SourceURL: pageURL, Schedule: runtimemodel.ScriptDefer,
		Source: `import { answer } from "./answer.js"; console.log(String(answer));`,
	}
	runtime := New()
	t.Cleanup(func() { _ = runtime.Stop() })
	if err := runtime.Load(context.Background(), []runtimemodel.Script{script}, environment); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if requested != "https://app.example/nested/answer.js" || len(records) != 1 || records[0] != [2]string{"log", "42"} {
		t.Fatalf("requested=%q records=%v", requested, records)
	}
}

func TestRuntimeContainsModuleLinkErrorAndContinues(t *testing.T) {
	pageURL := moduleTestURL(t, "https://app.example/page.html")
	var records [][2]string
	environment := runtimemodel.Environment{
		BaseURL: pageURL,
		Fetch: func(_ context.Context, request *network.Request) (*network.Response, error) {
			return &network.Response{URL: request.URL, StatusCode: 200, ContentType: "text/javascript", Body: []byte(`export const present = true`)}, nil
		},
		ConsoleRecord: func(level, message string) { records = append(records, [2]string{level, message}) },
	}
	scripts := []runtimemodel.Script{
		{Engine: runtimemodel.EngineJavaScript, Kind: runtimemodel.ScriptModule, SourceURL: pageURL, Schedule: runtimemodel.ScriptDefer, Source: `import { missing } from "./dependency.js"; console.log(missing);`},
		{Engine: runtimemodel.EngineJavaScript, SourceURL: pageURL, Schedule: runtimemodel.ScriptDefer, Source: `console.log("continued")`},
	}
	runtime := New()
	t.Cleanup(func() { _ = runtime.Stop() })
	if err := runtime.Load(context.Background(), scripts, environment); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || records[0][0] != "error" || !strings.Contains(records[0][1], "No matching export") || records[1] != [2]string{"log", "continued"} {
		t.Fatalf("module failure records = %v", records)
	}
}

func TestRuntimeSupportsImportMapDynamicImportAndTopLevelAwait(t *testing.T) {
	pageURL := moduleTestURL(t, "https://app.example/page.html")
	var records [][2]string
	environment := runtimemodel.Environment{
		BaseURL: pageURL,
		ImportMap: map[string]string{
			"pkg/": "https://cdn.example/modules/",
		},
		Fetch: func(_ context.Context, request *network.Request) (*network.Response, error) {
			if request.URL.String() != "https://cdn.example/modules/dynamic.js" || request.Kind != network.RequestModule || !request.CORS {
				return nil, fmt.Errorf("unexpected dynamic module request: %#v", request)
			}
			return &network.Response{URL: request.URL, StatusCode: 200, ContentType: "text/javascript", Body: []byte(`export const value = "dynamic"`)}, nil
		},
		ConsoleRecord: func(level, message string) { records = append(records, [2]string{level, message}) },
	}
	script := runtimemodel.Script{
		Engine: runtimemodel.EngineJavaScript, Kind: runtimemodel.ScriptModule,
		SourceURL: pageURL, Schedule: runtimemodel.ScriptDefer,
		Source: `
			const namespace = await import("pkg/dynamic.js");
			await new Promise(resolve => setTimeout(resolve, 5));
			console.log(namespace.value + ":awaited");`,
	}
	runtime := New()
	t.Cleanup(func() { _ = runtime.Stop() })
	if err := runtime.Load(context.Background(), []runtimemodel.Script{script}, environment); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got, want := records, [][2]string{{"log", "dynamic:awaited"}}; !equalRecords(got, want) {
		t.Fatalf("dynamic module records = %v, want %v", got, want)
	}
}

func TestRuntimeDeliversDynamicImportFailureAsRejection(t *testing.T) {
	pageURL := moduleTestURL(t, "https://app.example/page.html")
	var records [][2]string
	environment := runtimemodel.Environment{
		BaseURL: pageURL,
		Fetch: func(_ context.Context, request *network.Request) (*network.Response, error) {
			return &network.Response{URL: request.URL, StatusCode: 404, ContentType: "text/javascript"}, nil
		},
		ConsoleRecord: func(level, message string) { records = append(records, [2]string{level, message}) },
	}
	script := runtimemodel.Script{
		Engine: runtimemodel.EngineJavaScript, Kind: runtimemodel.ScriptModule, SourceURL: pageURL,
		Source: `const outcome = await import("./missing.js").then(() => "loaded", () => "rejected"); console.log(outcome);`,
	}
	runtime := New()
	t.Cleanup(func() { _ = runtime.Stop() })
	if err := runtime.Load(context.Background(), []runtimemodel.Script{script}, environment); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got, want := records, [][2]string{{"log", "rejected"}}; !equalRecords(got, want) {
		t.Fatalf("dynamic rejection records = %v, want %v", got, want)
	}
}

func TestRuntimeTimesOutPendingTopLevelAwait(t *testing.T) {
	pageURL := moduleTestURL(t, "https://app.example/page.html")
	var records [][2]string
	runtime := New()
	runtime.moduleTimeout = 20 * time.Millisecond
	t.Cleanup(func() { _ = runtime.Stop() })
	script := runtimemodel.Script{
		Engine: runtimemodel.EngineJavaScript, Kind: runtimemodel.ScriptModule, SourceURL: pageURL,
		Source: `await new Promise(() => {});`,
	}
	environment := runtimemodel.Environment{BaseURL: pageURL, ConsoleRecord: func(level, message string) { records = append(records, [2]string{level, message}) }}
	if err := runtime.Load(context.Background(), []runtimemodel.Script{script}, environment); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0][0] != "error" || !strings.Contains(records[0][1], "exceeded 20ms") {
		t.Fatalf("top-level await timeout records = %v", records)
	}
}

func TestModuleGraphEnforcesCountDepthAndTotalSize(t *testing.T) {
	graph := &moduleGraph{depths: make(map[string]int)}
	for index := range maxModulesPerGraph {
		graph.depths[fmt.Sprintf("https://example.test/%d.js", index)] = 0
	}
	if err := graph.registerModule("https://example.test/0.js", "https://example.test/overflow.js"); err == nil || !strings.Contains(err.Error(), "modules") {
		t.Fatalf("module count error = %v", err)
	}

	graph = &moduleGraph{depths: map[string]int{"https://example.test/deep.js": maxModuleGraphDepth}}
	if err := graph.registerModule("https://example.test/deep.js", "https://example.test/deeper.js"); err == nil || !strings.Contains(err.Error(), "depth") {
		t.Fatalf("module depth error = %v", err)
	}

	target := moduleTestURL(t, "https://example.test/extra.js")
	graph = &moduleGraph{
		ctx: context.Background(), environment: runtimemodel.Environment{
			BaseURL: target,
			Fetch: func(context.Context, *network.Request) (*network.Response, error) {
				return &network.Response{URL: target, StatusCode: 200, ContentType: "text/javascript", Body: []byte("x")}, nil
			},
		},
		sources: make(map[string]string), referrers: make(map[string]string), totalBytes: maxModuleGraphBytes,
	}
	if _, _, err := graph.load(target.String()); err == nil || !strings.Contains(err.Error(), "bytes") {
		t.Fatalf("module total size error = %v", err)
	}
}

func moduleTestURL(t *testing.T, value string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(value)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}
