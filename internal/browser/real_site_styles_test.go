package browser

import (
	"context"
	"reflect"
	"testing"

	"github.com/Grove-Computing/Growse/internal/network"
	"github.com/Grove-Computing/Growse/internal/style"
)

// Adapted from CSS Conditional 3 and CSS Cascading 5 integration assertions.
// One unsupported construct must not discard imported or following real-site
// rules, and a supported generated-content declaration must be queryable.
func TestRealSiteStylesLocalizeUnknownRulesAcrossImportMediaSupportsAndFontFace(t *testing.T) {
	pageURL := mustParseURL(t, "https://site.example/index.html")
	siteURL := mustParseURL(t, "https://site.example/site.css")
	baseURL := mustParseURL(t, "https://site.example/base.css")
	loader := &routeLoader{responses: map[string]*network.Response{
		pageURL.String(): {
			URL: pageURL, StatusCode: 200, ContentType: "text/html",
			Body: []byte(`<link rel="stylesheet" href="site.css"><h1 class="hero" data-label="Members">Directory</h1>`),
		},
		siteURL.String(): {
			URL: siteURL, StatusCode: 200, ContentType: "text/css",
			Body: []byte(`
@import "base.css" screen and (min-width: 600px);
@font-face { font-family: Fixture; src: url("fixture.woff2") format("woff2"); }
@unknown future(syntax) { .hero { display: none } }
@supports (content: attr(data-label)) { .hero { color: #d21d51 } }
@supports (future-property: future-value) { .hero { display: none } }
.hero:future-state, .hero { display: none }
.hero { font-size: 20px }
`),
		},
		baseURL.String(): {
			URL: baseURL, StatusCode: 200, ContentType: "text/css",
			Body: []byte(`.hero { background-color: #5876a0 }`),
		},
	}}
	page, err := New(loader).Navigate(context.Background(), pageURL.String())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loader.requested, []string{pageURL.String(), siteURL.String(), baseURL.String()}) {
		t.Fatalf("stylesheet request order = %v", loader.requested)
	}
	heading, ok := page.Document.QuerySelector("h1")
	if !ok {
		t.Fatal("heading is missing")
	}
	computed, _ := page.ComputedStyles.For(heading)
	if computed.Display == style.DisplayNone || computed.Color != 0xd21d51ff || computed.BackgroundColor != 0x5876a0ff || computed.FontSize != 20 {
		t.Fatalf("localized stylesheet result = %#v", computed)
	}
	if len(page.Stylesheet.FontFaces) != 1 || page.Stylesheet.FontFaces[0].Family != "Fixture" {
		t.Fatalf("font-face was lost: %#v", page.Stylesheet.FontFaces)
	}
}
