package browser

import (
	"fmt"
	"net/url"
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/css"
	"github.com/Grove-Computing/Growse/internal/devtools"
	htmlparser "github.com/Grove-Computing/Growse/internal/html"
	"github.com/Grove-Computing/Growse/internal/network"
)

func TestCompatibilityDiagnosticsExplainResourcesStylesFallbacksAndRuntimeErrors(t *testing.T) {
	document, err := htmlparser.Parse(strings.NewReader(`<main class="matched">SSR</main>`))
	if err != nil {
		t.Fatal(err)
	}
	stylesheet, err := css.Parse(strings.NewReader(`
@layer framework { .matched { color: blue } .missing { color: red } }
@media (max-width: 100px) { .matched { display: none } }
@supports selector(:unknown-state) { .matched { opacity: 0 } }`))
	if err != nil {
		t.Fatal(err)
	}
	pageURL, _ := url.Parse("https://user:password@app.example/page?token=secret")
	requested, _ := url.Parse("https://user:password@cdn.example/chunk.mjs?token=secret")
	finalURL, _ := url.Parse("https://cdn.example/chunk-v2.mjs?signature=secret")
	page := NewPage(pageURL)
	page.Document = document
	page.Stylesheet = stylesheet
	page.ViewportWidth, page.ViewportHeight = 800, 600
	page.FontErrors = []string{"font CORS rejected token=secret"}
	page.ImageErrors = []string{"image decode failed /private/source.png"}
	page.ScriptErrors = []string{"dynamic chunk load failed token=secret", "hydration exception secret", "observer loop limit secret"}
	page.DevTools.ObserveNetwork(network.Observation{
		Method: "GET", URL: requested, FinalURL: finalURL, Kind: network.RequestModule, Engine: "javascript",
		Initiator: "module-graph", Schedule: "module", StatusCode: 503, ErrorCategory: "http",
	})

	contexts := page.RuntimeDiagnostics()
	if len(contexts) != 1 {
		t.Fatalf("runtime context count = %d", len(contexts))
	}
	diagnostics := contexts[0].Diagnostics
	for _, expected := range [][3]string{
		{"resource/module", "error", "http"},
		{"style", "applied", "matched"},
		{"style", "ignored", "selector-unmatched"},
		{"style", "ignored", "media-condition"},
		{"font", "fallback", "cors"},
		{"image", "fallback", "decode"},
		{"runtime", "error", "chunk"},
		{"runtime", "error", "hydration"},
		{"runtime", "error", "observer"},
	} {
		if !hasCompatibilityDiagnostic(diagnostics, expected[0], expected[1], expected[2]) {
			t.Errorf("missing diagnostic %v in %+v", expected, diagnostics)
		}
	}
	resource := diagnostics[0]
	if resource.Initiator != "module-graph" || resource.Schedule != "module" || !strings.Contains(resource.Subject, "chunk-v2.mjs") {
		t.Fatalf("dynamic resource diagnostic = %+v", resource)
	}
	encoded := fmt.Sprintf("%+v", contexts)
	for _, secret := range []string{"password", "secret", "/private/"} {
		if strings.Contains(encoded, secret) {
			t.Fatalf("compatibility diagnostics leaked %q: %s", secret, encoded)
		}
	}
}

func hasCompatibilityDiagnostic(diagnostics []devtools.CompatibilityDiagnostic, category, state, reason string) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Category == category && diagnostic.State == state && diagnostic.Reason == reason {
			return true
		}
	}
	return false
}
