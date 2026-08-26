package browser

import (
	"fmt"
	"net/url"
	"strings"
	"testing"

	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
)

func TestRuntimeDiagnosticsRedactsSecretsAndCategorizesFailures(t *testing.T) {
	pageURL, err := url.Parse("https://user:password@example.test/app?token=top-secret#private")
	if err != nil {
		t.Fatal(err)
	}
	scriptURL, err := url.Parse("https://cdn.test/app.mjs?key=script-secret#source")
	if err != nil {
		t.Fatal(err)
	}
	frameURL, err := url.Parse("https://frame.test/view?session=frame-secret")
	if err != nil {
		t.Fatal(err)
	}
	page := &Page{
		URL: pageURL, Engine: runtimemodel.EngineJavaScript, RuntimeError: "WebAssembly compile failed",
		ScriptErrors: []string{"module import failed"},
		Scripts:      []Script{{Engine: runtimemodel.EngineJavaScript, Kind: runtimemodel.ScriptModule, Schedule: runtimemodel.ScriptDefer, SourceURL: scriptURL}},
		Sandbox:      runtimemodel.SandboxStatus{Ready: true, ProcessBoundary: true, BrokeredHostIO: true, Generation: 7, Constraints: []string{"process"}},
		Frames:       []*Frame{{ID: 2, ParentID: 0, Generation: 3, URL: frameURL, LoadError: "blocked", Page: &Page{URL: frameURL, Engine: runtimemodel.EngineGo}}},
	}
	page.window.Self.Generation = 2

	contexts := page.RuntimeDiagnostics()
	if len(contexts) != 2 {
		t.Fatalf("RuntimeDiagnostics() context count = %d, want 2", len(contexts))
	}
	if contexts[0].Kind != "page" || contexts[0].BrowsingGeneration != 2 || contexts[0].Sandbox.Generation != 7 {
		t.Fatalf("page context = %+v", contexts[0])
	}
	if got := strings.Join(contexts[0].ErrorCategories, ","); got != "module,wasm" {
		t.Fatalf("page error categories = %q", got)
	}
	if len(contexts[0].Scripts) != 1 || contexts[0].Scripts[0].Kind != "module" || contexts[0].Scripts[0].Schedule != "defer" {
		t.Fatalf("page scripts = %+v", contexts[0].Scripts)
	}
	if contexts[1].Kind != "frame" || contexts[1].ID != 2 || contexts[1].BrowsingGeneration != 3 || strings.Join(contexts[1].ErrorCategories, ",") != "frame" {
		t.Fatalf("frame context = %+v", contexts[1])
	}
	diagnostics := fmt.Sprintf("%+v", contexts)
	for _, secret := range []string{"user", "password", "top-secret", "private", "script-secret", "frame-secret"} {
		if strings.Contains(diagnostics, secret) {
			t.Fatalf("RuntimeDiagnostics() leaked %q: %s", secret, diagnostics)
		}
	}
}

func TestRuntimeErrorCategoryIsBounded(t *testing.T) {
	for input, want := range map[string]string{
		"dynamic import failed": "module", "WASM trap": "wasm", "sandbox unavailable": "sandbox", "token=do-not-copy": "runtime", "": "",
	} {
		if got := runtimeErrorCategory(input); got != want {
			t.Errorf("runtimeErrorCategory(%q) = %q, want %q", input, got, want)
		}
	}
}
