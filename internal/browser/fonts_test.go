package browser

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/binary"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/Grove-Computing/Growse/internal/css"
	layoutmodel "github.com/Grove-Computing/Growse/internal/layout"
	"github.com/Grove-Computing/Growse/internal/network"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
	woff2 "github.com/pgaskin/go-woff2"
	"golang.org/x/image/font/gofont/goregular"
)

type timeoutFontLoader struct {
	pageURL *url.URL
	body    []byte
}

func (loader timeoutFontLoader) Get(ctx context.Context, target *url.URL) (*network.Response, error) {
	if target.String() == loader.pageURL.String() {
		return &network.Response{URL: loader.pageURL, StatusCode: http.StatusOK, ContentType: "text/html", Body: loader.body}, nil
	}
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestNavigateLoadsWebFontsOnlyForExplicitJavaScriptEngine(t *testing.T) {
	pageURL := "https://example.com/index.html"
	fontURL := "https://example.com/fixture.woff"
	body := testWOFF(t, goregular.TTF)
	newLoader := func() *routeLoader {
		return &routeLoader{responses: map[string]*network.Response{
			pageURL: {URL: mustParseURL(t, pageURL), StatusCode: http.StatusOK, ContentType: "text/html", Body: []byte(`<style>@font-face { font-family: Fixture; src: url("fixture.woff") format("woff"); }</style><p>hello</p>`)},
			fontURL: {URL: mustParseURL(t, fontURL), StatusCode: http.StatusOK, ContentType: "font/woff", Body: body},
		}}
	}
	factory := func(runtimemodel.Engine) runtimemodel.Runtime { return &runtimeStub{} }
	goLoader := newLoader()
	goBrowser := NewWithEngineFactory(goLoader, factory)
	t.Cleanup(func() { _ = goBrowser.Close() })
	goPage, err := goBrowser.Navigate(context.Background(), pageURL)
	if err != nil {
		t.Fatal(err)
	}
	if goPage.Fonts != nil || len(goLoader.requested) != 1 {
		t.Fatalf("Go page loaded web fonts: fonts=%#v requested=%#v", goPage.Fonts, goLoader.requested)
	}

	jsLoader := newLoader()
	jsBrowser := NewWithEngineFactory(jsLoader, factory)
	t.Cleanup(func() { _ = jsBrowser.Close() })
	if _, err := jsBrowser.SetEngine(context.Background(), runtimemodel.EngineJavaScript); err != nil {
		t.Fatal(err)
	}
	jsPage, err := jsBrowser.Navigate(context.Background(), pageURL)
	if err != nil {
		t.Fatal(err)
	}
	if len(jsPage.Fonts) != 1 || !jsPage.Fonts[0].Decoded || len(jsLoader.requested) != 2 {
		t.Fatalf("JavaScript page fonts=%#v requested=%#v", jsPage.Fonts, jsLoader.requested)
	}
}

func TestWebFontTimeoutKeepsPageVisibleWithBundledFallback(t *testing.T) {
	pageURL := mustParseURL(t, "https://example.com/index.html")
	loader := timeoutFontLoader{pageURL: pageURL, body: []byte(`<style>
		@font-face { font-family: Fixture; src: url("fixture.woff2") format("woff2"); font-display: optional; }
		p { font-family: Fixture, sans-serif; }
	</style><p>fallback remains visible</p>`)}
	browserState := NewWithEngineFactory(loader, func(runtimemodel.Engine) runtimemodel.Runtime { return &runtimeStub{} })
	t.Cleanup(func() { _ = browserState.Close() })
	if _, err := browserState.SetEngine(context.Background(), runtimemodel.EngineJavaScript); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	page, err := browserState.Navigate(context.Background(), pageURL.String())
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("optional font timeout took %v", elapsed)
	}
	if page.Document == nil || page.WebFonts == nil || len(page.FontErrors) != 1 || page.Fonts[0].Error != "font load timed out" {
		t.Fatalf("page font fallback state = fonts:%#v errors:%#v", page.Fonts, page.FontErrors)
	}
	tree := layoutmodel.BuildWithScrollAndResources(page.Document, page.ComputedStyles, page.ImageResources, page.WebFonts, 800, 600, 0, 0)
	if len(tree.Boxes) == 0 {
		t.Fatal("fallback page produced no visible layout boxes")
	}
}

func TestLoadWebFontsValidatesDescriptorsAndDecodesWOFF(t *testing.T) {
	pageURL := mustParseURL(t, "https://example.com/app/index.html")
	stylesheet := parseFontStylesheet(t, `@font-face {
		font-family: "Fixture";
		src: local("Fixture"), url("../fonts/fixture.woff") format("woff");
		font-style: italic;
		font-weight: 700;
		font-stretch: condensed;
		unicode-range: U+0020-007F, U+4??;
		font-display: swap;
	}`)
	css.ResolveResourceURLs(stylesheet, pageURL)
	fontURL := "https://example.com/fonts/fixture.woff"
	loader := &routeLoader{responses: map[string]*network.Response{
		fontURL: {URL: mustParseURL(t, fontURL), StatusCode: http.StatusOK, ContentType: "font/woff", Body: testWOFF(t, goregular.TTF)},
	}}

	resources, failures := loadWebFonts(context.Background(), loader, pageURL, stylesheet)
	if len(failures) != 0 || len(resources) != 1 {
		t.Fatalf("loadWebFonts() resources = %#v, failures = %#v", resources, failures)
	}
	font := resources[0]
	if !font.Loaded || !font.Decoded || font.Face == nil || font.URL != fontURL || font.FinalURL != fontURL || font.Format != "woff" {
		t.Fatalf("font = %#v", font)
	}
	if font.Family != "Fixture" || font.Style != "italic" || font.Weight != "700" || font.Stretch != "condensed" || font.Display != "swap" {
		t.Fatalf("descriptors = %#v", font)
	}
	if len(font.UnicodeRanges) != 2 || font.UnicodeRanges[0] != (FontRange{Start: 0x20, End: 0x7f}) || font.UnicodeRanges[1] != (FontRange{Start: 0x400, End: 0x4ff}) {
		t.Fatalf("unicode ranges = %#v", font.UnicodeRanges)
	}
}

func TestLoadWebFontsAcceptsBoundedWOFF2Resource(t *testing.T) {
	pageURL := mustParseURL(t, "https://example.com/")
	stylesheet := parseFontStylesheet(t, `@font-face { font-family: Fixture; src: url("fixture.woff2") format("woff2"); }`)
	fontURL := "https://example.com/fixture.woff2"
	body, err := woff2.Encode(goregular.TTF, nil)
	if err != nil {
		t.Fatal(err)
	}
	loader := &routeLoader{responses: map[string]*network.Response{
		fontURL: {URL: mustParseURL(t, fontURL), StatusCode: http.StatusOK, ContentType: "font/woff2", Body: body},
	}}

	resources, failures := loadWebFonts(context.Background(), loader, pageURL, stylesheet)
	if len(failures) != 0 || len(resources) != 1 || !resources[0].Loaded || !resources[0].Decoded || resources[0].Face == nil || resources[0].Format != "woff2" {
		t.Fatalf("loadWebFonts() resources = %#v, failures = %#v", resources, failures)
	}
}

func TestLoadWebFontsEnforcesCORSMixedContentMIMEAndSourceFallback(t *testing.T) {
	pageURL := mustParseURL(t, "https://site.example/")
	crossURL := "https://cdn.example/fixture.woff"
	stylesheet := parseFontStylesheet(t, `@font-face { font-family: Fixture; src: url("https://cdn.example/fixture.woff") format("woff"); }`)
	body := testWOFF(t, goregular.TTF)

	loader := &routeLoader{responses: map[string]*network.Response{
		crossURL: {URL: mustParseURL(t, crossURL), StatusCode: http.StatusOK, ContentType: "font/woff", Body: body},
	}}
	resources, failures := loadWebFonts(context.Background(), loader, pageURL, stylesheet)
	if len(failures) != 1 || resources[0].Loaded || resources[0].Error != "font CORS policy rejected response" {
		t.Fatalf("missing CORS result = %#v, failures = %#v", resources, failures)
	}
	loader.responses[crossURL].Header = http.Header{"Access-Control-Allow-Origin": []string{"https://site.example"}}
	resources, failures = loadWebFonts(context.Background(), loader, pageURL, stylesheet)
	if len(failures) != 0 || !resources[0].Decoded {
		t.Fatalf("allowed CORS result = %#v, failures = %#v", resources, failures)
	}

	mixed := parseFontStylesheet(t, `@font-face { font-family: Fixture; src: url("http://site.example/fixture.woff") format("woff"); }`)
	resources, failures = loadWebFonts(context.Background(), loader, pageURL, mixed)
	if len(failures) != 1 || resources[0].Error != "mixed-content font was blocked" {
		t.Fatalf("mixed-content result = %#v, failures = %#v", resources, failures)
	}

	wrongMIMEURL := "https://site.example/wrong.woff"
	wrongMIME := parseFontStylesheet(t, `@font-face { font-family: Fixture; src: url("wrong.woff") format("woff"); }`)
	loader.responses[wrongMIMEURL] = &network.Response{URL: mustParseURL(t, wrongMIMEURL), StatusCode: http.StatusOK, ContentType: "application/octet-stream", Body: body}
	resources, failures = loadWebFonts(context.Background(), loader, pageURL, wrongMIME)
	if len(failures) != 1 || resources[0].Error != "font MIME type was rejected" {
		t.Fatalf("MIME result = %#v, failures = %#v", resources, failures)
	}

	goodURL := "https://site.example/good.woff"
	fallback := parseFontStylesheet(t, `@font-face { font-family: Fixture; src: url("bad.woff2") format("woff2"), url("good.woff") format("woff"); }`)
	loader.responses["https://site.example/bad.woff2"] = &network.Response{URL: mustParseURL(t, "https://site.example/bad.woff2"), StatusCode: http.StatusOK, ContentType: "font/woff2", Body: []byte("invalid")}
	loader.responses[goodURL] = &network.Response{URL: mustParseURL(t, goodURL), StatusCode: http.StatusOK, ContentType: "font/woff", Body: body}
	resources, failures = loadWebFonts(context.Background(), loader, pageURL, fallback)
	if len(failures) != 0 || !resources[0].Decoded || resources[0].URL != goodURL {
		t.Fatalf("fallback result = %#v, failures = %#v", resources, failures)
	}
}

func parseFontStylesheet(t *testing.T, source string) *css.Stylesheet {
	t.Helper()
	stylesheet, err := css.Parse(bytes.NewBufferString(source))
	if err != nil {
		t.Fatal(err)
	}
	return stylesheet
}

func testWOFF(t *testing.T, sfnt []byte) []byte {
	t.Helper()
	if len(sfnt) < 12 {
		t.Fatal("invalid test SFNT")
	}
	numTables := int(binary.BigEndian.Uint16(sfnt[4:6]))
	if len(sfnt) < 12+numTables*16 {
		t.Fatal("truncated test SFNT table directory")
	}
	result := make([]byte, 44+numTables*20)
	copy(result[:4], "wOFF")
	copy(result[4:8], sfnt[:4])
	binary.BigEndian.PutUint16(result[12:14], uint16(numTables))
	binary.BigEndian.PutUint32(result[16:20], uint32(len(sfnt)))
	for index := 0; index < numTables; index++ {
		sourceDirectory := sfnt[12+index*16 : 12+(index+1)*16]
		sourceOffset := int(binary.BigEndian.Uint32(sourceDirectory[8:12]))
		originalLength := int(binary.BigEndian.Uint32(sourceDirectory[12:16]))
		if sourceOffset < 0 || originalLength < 0 || sourceOffset > len(sfnt)-originalLength {
			t.Fatal("invalid test SFNT table")
		}
		original := sfnt[sourceOffset : sourceOffset+originalLength]
		var compressed bytes.Buffer
		writer := zlib.NewWriter(&compressed)
		if _, err := writer.Write(original); err != nil {
			t.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		payload := original
		if compressed.Len() < len(original) {
			payload = compressed.Bytes()
		}
		for len(result)%4 != 0 {
			result = append(result, 0)
		}
		targetDirectory := result[44+index*20 : 44+(index+1)*20]
		copy(targetDirectory[:4], sourceDirectory[:4])
		binary.BigEndian.PutUint32(targetDirectory[4:8], uint32(len(result)))
		binary.BigEndian.PutUint32(targetDirectory[8:12], uint32(len(payload)))
		binary.BigEndian.PutUint32(targetDirectory[12:16], uint32(originalLength))
		copy(targetDirectory[16:20], sourceDirectory[4:8])
		result = append(result, payload...)
	}
	binary.BigEndian.PutUint32(result[8:12], uint32(len(result)))
	return result
}
