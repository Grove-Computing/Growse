package browser

import (
	"bytes"
	"context"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/png"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/css"
	"github.com/Grove-Computing/Growse/internal/network"
	"golang.org/x/image/font/gofont/goregular"
)

func TestImageLimitsRejectDimensionSurfaceCountAndDecoderPanic(t *testing.T) {
	encoded := encodeLimitTestPNG(t)
	oversizedDimension := append([]byte(nil), encoded...)
	binary.BigEndian.PutUint32(oversizedDimension[16:20], maxImageDimension+1)
	binary.BigEndian.PutUint32(oversizedDimension[29:33], crc32.ChecksumIEEE(oversizedDimension[12:29]))
	if _, _, _, err := decodeImageResponse(oversizedDimension, "image/png"); err == nil {
		t.Fatal("oversized image dimension was accepted")
	}

	budget := newImageDecodeBudget()
	budget.surfaceBytes = maxPageImageSurfaceBytes - 4
	if _, _, _, err := decodeImageResponseWithBudget(encoded, "image/png", budget); err != nil {
		t.Fatalf("last bounded surface failed: %v", err)
	}
	if _, _, _, err := decodeImageResponseWithBudget(encoded, "image/png", budget); err == nil {
		t.Fatal("page image surface overflow was accepted")
	}
	for index := 0; index < maxPageImageResources; index++ {
		if !budget.claim(string(rune(index + 1))) {
			t.Fatalf("resource %d was rejected before the limit", index)
		}
	}
	if budget.claim("overflow") {
		t.Fatal("image resource count overflow was accepted")
	}

	image.RegisterFormat("growse-panic-test", "GROWSEPANIC", panicImageDecode, panicImageConfig)
	if _, _, _, err := decodeImageResponse([]byte("GROWSEPANIC"), "image/png"); err == nil || !strings.Contains(err.Error(), "decoder panic") {
		t.Fatalf("decoder panic error = %v", err)
	}
}

func TestSVGAndFontLimitsRejectOnlyAffectedResource(t *testing.T) {
	budget := newImageDecodeBudget()
	budget.surfaceBytes = maxPageImageSurfaceBytes
	if _, _, _, err := rasterizeSVGWithBudget([]byte(`<svg width="1" height="1"><rect width="1" height="1"/></svg>`), budget); err == nil {
		t.Fatal("SVG page surface overflow was accepted")
	}

	woff := testWOFF(t, goregular.TTF)
	tableOverflow := append([]byte(nil), woff...)
	binary.BigEndian.PutUint16(tableOverflow[12:14], maxFontTables+1)
	if _, _, err := decodeWebFont(tableOverflow, "woff"); err == nil {
		t.Fatal("WOFF table count overflow was accepted")
	}
	if err := validateSFNTLimits(testSFNTWithGlyphCount(maxFontGlyphs + 1)); err == nil {
		t.Fatal("font glyph count overflow was accepted")
	}

	pageURL := mustParseURL(t, "https://example.com/")
	badURL := "https://example.com/bad.woff"
	goodURL := "https://example.com/good.woff"
	stylesheet, err := css.Parse(strings.NewReader(`
		@font-face { font-family: Broken; src: url("bad.woff") format("woff"); }
		@font-face { font-family: Good; src: url("good.woff") format("woff"); }
	`))
	if err != nil {
		t.Fatal(err)
	}
	loader := &routeLoader{responses: map[string]*network.Response{
		badURL:  {URL: mustParseURL(t, badURL), StatusCode: http.StatusOK, ContentType: "font/woff", Body: []byte("broken")},
		goodURL: {URL: mustParseURL(t, goodURL), StatusCode: http.StatusOK, ContentType: "font/woff", Body: woff},
	}}
	resources, failures := loadWebFonts(context.Background(), loader, pageURL, stylesheet)
	if len(resources) != 2 || resources[0].Loaded || !resources[1].Decoded || len(failures) != 1 {
		t.Fatalf("localized font failure resources=%#v failures=%#v", resources, failures)
	}
}

func TestEncodedResourceLimitsMatchReleasePolicy(t *testing.T) {
	pageURL := mustParseURL(t, "https://example.com/")
	response := &network.Response{URL: pageURL, Header: make(http.Header), ContentType: "font/woff", Body: make([]byte, maxFontBytes+1)}
	if err := validateFontResponse(pageURL, pageURL, response, "woff", 0); err == nil {
		t.Fatal("oversized encoded font was accepted")
	}
	if maxImageBytes != 16<<20 || maxImagePixels != 100_000_000 || maxImageDimension != 16_384 || maxPageImageResources != 512 || maxPageImageSurfaceBytes != 256<<20 ||
		maxSVGBytes != 4<<20 || maxSVGSurfaceBytes != 64<<20 || maxFontBytes != 8<<20 || maxPageFontBytes != 64<<20 || maxFontFaces != 64 || maxFontTables != 128 || maxFontGlyphs != 50_000 {
		t.Fatal("resource limits no longer match docs/v0.15.0.md")
	}
}

func TestFontFaceCountLimitStopsAdditionalRequests(t *testing.T) {
	stylesheet := &css.Stylesheet{FontFaces: make([]css.FontFaceRule, maxFontFaces+1)}
	for index := range stylesheet.FontFaces {
		stylesheet.FontFaces[index] = css.FontFaceRule{
			Family: "Fixture", Source: `url("missing.woff") format("woff")`, Style: "normal", Weight: "normal", Stretch: "normal", Display: "auto",
		}
	}
	resources, failures := loadWebFonts(context.Background(), &routeLoader{responses: map[string]*network.Response{}}, mustParseURL(t, "https://example.com/"), stylesheet)
	if len(resources) != maxFontFaces || len(failures) != maxFontFaces+1 || failures[0] != "font face limit exceeded" {
		t.Fatalf("font face limit resources=%d failures=%#v", len(resources), failures)
	}
}

func encodeLimitTestPNG(t *testing.T) []byte {
	t.Helper()
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewNRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	return encoded.Bytes()
}

func panicImageDecode(io.Reader) (image.Image, error) { panic("fixture decode panic") }

func panicImageConfig(io.Reader) (image.Config, error) { panic("fixture config panic") }

func testSFNTWithGlyphCount(glyphs int) []byte {
	source := make([]byte, 34)
	copy(source[:4], []byte{0, 1, 0, 0})
	binary.BigEndian.PutUint16(source[4:6], 1)
	copy(source[12:16], "maxp")
	binary.BigEndian.PutUint32(source[20:24], 28)
	binary.BigEndian.PutUint32(source[24:28], 6)
	copy(source[28:32], []byte{0, 1, 0, 0})
	binary.BigEndian.PutUint16(source[32:34], uint16(glyphs))
	return source
}
