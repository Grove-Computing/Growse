package browser

import (
	"context"
	"testing"

	"github.com/Grove-Computing/Growse/internal/network"
)

func TestStylesheetImportLayerWrapsImportedRules(t *testing.T) {
	pageURL := mustParseURL(t, "https://example.com/index.html")
	baseURL := mustParseURL(t, "https://example.com/app.css")
	importURL := mustParseURL(t, "https://example.com/reset.css")
	loader := &routeLoader{responses: map[string]*network.Response{
		importURL.String(): {
			URL: importURL, StatusCode: 200, ContentType: "text/css",
			Body: []byte(`@layer defaults { p { color: red } } div { color: blue }`),
		},
	}}
	state := &stylesheetLoadState{client: loader, origin: pageURL, activeURLs: make(map[string]bool)}
	stylesheet, err := state.loadContent(context.Background(), []byte(`@import "reset.css" layer(framework);`), baseURL, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(stylesheet.Rules) != 2 || stylesheet.Rules[0].Layer != "framework.defaults" || stylesheet.Rules[1].Layer != "framework" {
		t.Fatalf("layered import rules = %#v", stylesheet.Rules)
	}
}
