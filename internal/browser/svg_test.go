package browser

import (
	"context"
	"image/color"
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/dom"
	htmlparser "github.com/Grove-Computing/Growse/internal/html"
	"github.com/Grove-Computing/Growse/internal/layout"
	"github.com/Grove-Computing/Growse/internal/network"
	"github.com/Grove-Computing/Growse/internal/style"
)

const staticSVGFixture = `<svg xmlns="http://www.w3.org/2000/svg" width="120" height="80" viewBox="0 0 120 80">
<defs>
  <linearGradient id="linear"><stop offset="0" stop-color="#ff0000"/><stop offset="1" stop-color="#0000ff"/></linearGradient>
  <radialGradient id="radial"><stop offset="0" stop-color="#ffffff"/><stop offset="1" stop-color="#00aa00"/></radialGradient>
  <clipPath id="frame"><rect x="10" y="8" width="100" height="64"/></clipPath>
</defs>
<g transform="translate(2 1)" clip-path="url(#frame)" opacity="0.9">
  <rect x="8" y="7" width="42" height="22" fill="url(#linear)" stroke="#111111"/>
  <circle cx="65" cy="20" r="12" fill="url(#radial)"/>
  <ellipse cx="92" cy="20" rx="13" ry="8" fill="#ffaa00"/>
  <line x1="10" y1="38" x2="108" y2="38" stroke="#000000" stroke-width="2"/>
  <polyline points="10,48 22,42 34,48" fill="none" stroke="#0088ff"/>
  <polygon points="42,48 54,42 66,48" fill="#8844ff"/>
  <path d="M72 48 C82 38 94 58 108 46 L108 62 L72 62 Z" fill="#00aaaa"/>
  <text x="14" y="68" fill="#222222">Growse</text>
</g>
</svg>`

func TestRasterizeSVGDrawsStaticSubsetWithViewBoxGradientClipAndText(t *testing.T) {
	decoded, width, height, err := rasterizeSVG([]byte(staticSVGFixture))
	if err != nil {
		t.Fatal(err)
	}
	if width != 120 || height != 80 || decoded.Bounds().Dx() != 120 || decoded.Bounds().Dy() != 80 {
		t.Fatalf("SVG dimensions = %dx%d / %v", width, height, decoded.Bounds())
	}
	if alpha := color.NRGBAModel.Convert(decoded.At(2, 2)).(color.NRGBA).A; alpha != 0 {
		t.Fatalf("clip outside alpha = %d, want 0", alpha)
	}
	painted := 0
	for y := 8; y < 72; y++ {
		for x := 10; x < 110; x++ {
			if color.NRGBAModel.Convert(decoded.At(x, y)).(color.NRGBA).A != 0 {
				painted++
			}
		}
	}
	if painted < 500 {
		t.Fatalf("painted pixels = %d, want representative shapes and text", painted)
	}
}

func TestLoadReplacedImagesRasterizesExternalSVG(t *testing.T) {
	baseURL := mustParseURL(t, "https://example.com/")
	document := dom.NewDocument()
	imageNode := document.CreateElement("img", map[string]string{"src": "icon.svg", "alt": "Icon"})
	if err := document.AppendChild(document.Root, imageNode); err != nil {
		t.Fatal(err)
	}
	loader := &routeLoader{responses: map[string]*network.Response{
		"https://example.com/icon.svg": {URL: mustParseURL(t, "https://example.com/icon.svg"), ContentType: "image/svg+xml", Body: []byte(staticSVGFixture)},
	}}
	resources, images, failures := loadReplacedImages(context.Background(), loader, baseURL, document, 800, 1)
	resource := resources[imageNode.ID]
	if len(failures) != 0 || !resource.Loaded || resource.IntrinsicWidth != 120 || resource.IntrinsicHeight != 80 || images[resource.URL] == nil {
		t.Fatalf("external SVG resource/images/failures = %#v / %#v / %#v", resource, images, failures)
	}
}

func TestInlineSVGRasterParticipatesInLayout(t *testing.T) {
	document, err := htmlparser.Parse(strings.NewReader(`<body><svg id="logo" width="120" height="80" viewBox="0 0 120 80"><rect width="120" height="80" fill="#336699"/><text x="8" y="20">Logo</text></svg></body>`))
	if err != nil {
		t.Fatal(err)
	}
	resources, images, failures := loadInlineSVGImages(document)
	if len(failures) != 0 || len(resources) != 1 || len(images) != 1 {
		t.Fatalf("inline SVG resources/images/failures = %#v / %#v / %#v", resources, images, failures)
	}
	tree := layout.BuildWithScrollAndImages(document, style.Compute(document, nil), resources, 800, 600, 0, 0)
	if len(tree.Boxes) != 1 || !tree.Boxes[0].Image || tree.Boxes[0].Tag != "svg" || tree.Boxes[0].Width != 120 || tree.Boxes[0].Height != 80 {
		t.Fatalf("inline SVG layout = %#v", tree.Boxes)
	}
}
