package browser

import (
	"fmt"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

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

func TestCompatibilityDiagnosticsAggregateAndBoundMetadataWithoutPayloadCopies(t *testing.T) {
	page := NewPage(nil)
	page.FontErrors = make([]string, 2500)
	for index := range page.FontErrors {
		page.FontErrors[index] = "font CORS rejected credential=must-not-survive"
	}
	diagnostics := compatibilityDiagnostics(page)
	if len(diagnostics) != 1 || diagnostics[0].Category != "font" || diagnostics[0].Reason != "cors" || diagnostics[0].Count != 2500 {
		t.Fatalf("aggregated diagnostics = %+v", diagnostics)
	}
	if strings.Contains(fmt.Sprint(diagnostics), "must-not-survive") {
		t.Fatalf("aggregated diagnostics retained the raw failure: %+v", diagnostics)
	}
	limitedPage := NewPage(nil)
	for index := 0; index < maxCompatibilityDiagnostics+100; index++ {
		target, _ := url.Parse(fmt.Sprintf("https://fixture.test/resource-%d.mjs", index))
		limitedPage.DevTools.ObserveNetwork(network.Observation{Method: "GET", URL: target, Kind: network.RequestModule, StatusCode: 200})
	}
	if got := len(compatibilityDiagnostics(limitedPage)); got != maxCompatibilityDiagnostics {
		t.Fatalf("compatibility diagnostic limit = %d, want %d", got, maxCompatibilityDiagnostics)
	}

	long := strings.Repeat("界", maxCompatibilityDiagnosticBytes)
	bounded := normalizeCompatibilityDiagnostic(devtools.CompatibilityDiagnostic{Category: "runtime", Subject: long, Count: maxCompatibilityDiagnosticCount + 1})
	if len(bounded.Subject) > maxCompatibilityDiagnosticBytes || !utf8.ValidString(bounded.Subject) || bounded.Count != maxCompatibilityDiagnosticCount {
		t.Fatalf("bounded diagnostic bytes/count = %d / %d", len(bounded.Subject), bounded.Count)
	}

	typeOfDiagnostic := reflect.TypeOf(devtools.CompatibilityDiagnostic{})
	for index := range typeOfDiagnostic.NumField() {
		field := typeOfDiagnostic.Field(index)
		if field.Type.Kind() != reflect.String && field.Type.Kind() != reflect.Int {
			t.Fatalf("diagnostic field %s can retain a payload: %s", field.Name, field.Type)
		}
		name := strings.ToLower(field.Name)
		if strings.Contains(name, "body") || strings.Contains(name, "source") || strings.Contains(name, "bytes") || strings.Contains(name, "image") || strings.Contains(name, "font") {
			t.Fatalf("diagnostic model exposes payload-like field %s", field.Name)
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
