package html

import (
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/dom"
)

func TestParseBuildsGrowseDOM(t *testing.T) {
	document, err := Parse(strings.NewReader(`<!doctype html>
<html><head><title>Growse Demo</title></head><body>
<main id="app"><h1>Hello</h1><p class="lead">World</p></main>
</body></html>`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if got, want := document.Title(), "Growse Demo"; got != want {
		t.Fatalf("Title() = %q, want %q", got, want)
	}
	main, ok := document.GetElementByID("app")
	if !ok {
		t.Fatal("element #app was not indexed")
	}
	if main.Type != dom.NodeElement || main.TagName != "main" {
		t.Fatalf("#app = (%v, %q), want element main", main.Type, main.TagName)
	}
	if got, want := strings.TrimSpace(main.TextContent()), "HelloWorld"; got != want {
		t.Fatalf("main text = %q, want %q", got, want)
	}
	if document.ElementCount() < 7 {
		t.Fatalf("ElementCount() = %d, want at least 7", document.ElementCount())
	}
}

func TestParseRecoversMalformedHTML(t *testing.T) {
	document, err := Parse(strings.NewReader(`<html><body><p>first<p>second`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if document.ElementCount() == 0 {
		t.Fatal("malformed HTML produced no elements")
	}
}
