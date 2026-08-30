package browser

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/css"
	"github.com/Grove-Computing/Growse/internal/dom"
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

func TestImageQueueSaturationUsesPlaceholdersAndBoundsDiagnostics(t *testing.T) {
	document := dom.NewDocument()
	for index := 0; index < maxResourceQueue+44; index++ {
		node := document.CreateElement("img", map[string]string{"src": fmt.Sprintf("image-%d.png", index), "alt": "placeholder"})
		if err := document.AppendChild(document.Root, node); err != nil {
			t.Fatal(err)
		}
	}
	resources, images, failures := loadReplacedImagesWithCache(
		context.Background(), &routeLoader{responses: map[string]*network.Response{}}, mustParseURL(t, "https://example.com/"), document, 1280, 1, nil, newImageDecodeBudget(), newImageResourceCache(),
	)
	if len(resources) != maxResourceQueue+44 || len(images) != 0 || len(failures) != maxImageDiagnostics {
		t.Fatalf("resources=%d images=%d failures=%d", len(resources), len(images), len(failures))
	}
	for nodeID, resource := range resources {
		if resource.Loaded || resource.Error == "" || resource.Alt != "placeholder" {
			t.Fatalf("resource %d is not a localized placeholder: %#v", nodeID, resource)
		}
	}
}

func TestImageAllocationAndCorruptAnimationFailuresStayLocal(t *testing.T) {
	pageURL := mustParseURL(t, "https://example.com/")
	validURL := "https://example.com/valid.png"
	corruptURL := "https://example.com/corrupt.gif"
	var pngBody bytes.Buffer
	if err := png.Encode(&pngBody, image.NewNRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	loader := &routeLoader{responses: map[string]*network.Response{
		validURL:   {URL: mustParseURL(t, validURL), ContentType: "image/png", Body: pngBody.Bytes()},
		corruptURL: {URL: mustParseURL(t, corruptURL), ContentType: "image/gif", Body: corruptAnimatedGIF(t)},
	}}

	allocationNode := dom.NewDocument().CreateElement("img", map[string]string{"src": validURL, "alt": "allocation fallback"})
	budget := newImageDecodeBudget()
	budget.surfaceBytes = maxPageImageSurfaceBytes
	resource, decoded, failure := loadReplacedImageNodeWithCache(context.Background(), loader, pageURL, allocationNode, 1280, 1, true, budget, newImageResourceCache())
	if resource.Loaded || decoded != nil || failure == "" || resource.Alt != "allocation fallback" {
		t.Fatalf("allocation failure escaped placeholder: resource=%#v decoded=%v failure=%q", resource, decoded, failure)
	}
	previousAllocator := allocateImageNRGBA
	allocateImageNRGBA = func(image.Rectangle) *image.NRGBA { panic("allocation fixture") }
	defer func() { allocateImageNRGBA = previousAllocator }()
	resizeNode := dom.NewDocument().CreateElement("img", map[string]string{"src": validURL, "width": "2", "height": "2", "alt": "resize fallback"})
	resource, decoded, failure = loadReplacedImageNodeWithCache(context.Background(), loader, pageURL, resizeNode, 1280, 1, true, newImageDecodeBudget(), newImageResourceCache())
	if resource.Loaded || decoded != nil || failure == "" || resource.Alt != "resize fallback" {
		t.Fatalf("allocation panic escaped placeholder: resource=%#v decoded=%v failure=%q", resource, decoded, failure)
	}
	allocateImageNRGBA = previousAllocator

	corruptNode := dom.NewDocument().CreateElement("img", map[string]string{"src": corruptURL, "alt": "corrupt fallback"})
	resource, decoded, failure = loadReplacedImageNodeWithCache(context.Background(), loader, pageURL, corruptNode, 1280, 1, true, newImageDecodeBudget(), newImageResourceCache())
	if resource.Loaded || decoded != nil || failure == "" || resource.Alt != "corrupt fallback" {
		t.Fatalf("corrupt animation escaped placeholder: resource=%#v decoded=%v failure=%q", resource, decoded, failure)
	}
	if !strings.Contains(resource.Error, "animated image frame") {
		t.Fatalf("corrupt animation diagnostic = %q", resource.Error)
	}
}

func corruptAnimatedGIF(t *testing.T) []byte {
	t.Helper()
	palette := color.Palette{color.Black, color.White}
	first := image.NewPaletted(image.Rect(0, 0, 2, 1), palette)
	second := image.NewPaletted(image.Rect(0, 0, 2, 1), palette)
	second.SetColorIndex(0, 0, 1)
	var encoded bytes.Buffer
	if err := gif.EncodeAll(&encoded, &gif.GIF{Image: []*image.Paletted{first, second}, Delay: []int{10, 10}, Config: image.Config{Width: 2, Height: 1, ColorModel: palette}}); err != nil {
		t.Fatal(err)
	}
	body := encoded.Bytes()
	for cut := len(body) - 1; cut > 0; cut-- {
		candidate := append([]byte(nil), body[:cut]...)
		if _, staticErr := gif.Decode(bytes.NewReader(candidate)); staticErr != nil {
			continue
		}
		if _, animationErr := gif.DecodeAll(bytes.NewReader(candidate)); animationErr != nil {
			return candidate
		}
	}
	t.Fatal("could not construct a GIF with a valid first frame and corrupt animation tail")
	return nil
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
