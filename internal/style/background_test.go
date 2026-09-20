package style

import (
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/css"
	"github.com/Grove-Computing/Growse/internal/dom"
)

func TestConicAndDataBackgroundLayersPreserveOriginAndClip(t *testing.T) {
	document := dom.NewDocument()
	card := document.CreateElement("div", map[string]string{"class": "card"})
	if err := document.AppendChild(document.Root, card); err != nil {
		t.Fatal(err)
	}
	stylesheet, err := css.Parse(strings.NewReader(`
.card {
  background-image: conic-gradient(from 45deg at 25% 75%, red 0deg, blue 360deg), url("data:image/png;base64,AA==");
  background-origin: content-box, padding-box;
  background-clip: padding-box, content-box;
}
`))
	if err != nil {
		t.Fatal(err)
	}
	computed, _ := Compute(document, stylesheet).For(card)
	if len(computed.BackgroundLayers) != 2 {
		t.Fatalf("background layers = %#v", computed.BackgroundLayers)
	}
	conic, data := computed.BackgroundLayers[0], computed.BackgroundLayers[1]
	if conic.Image.Kind != BackgroundImageConicGradient || conic.Image.GradientAngle != 45 || conic.Image.GradientCenter.X.Percentage != 25 || conic.Image.GradientCenter.Y.Percentage != 75 || conic.Origin != BackgroundBoxContent || conic.Clip != BackgroundBoxPadding {
		t.Fatalf("conic layer = %#v", conic)
	}
	if data.Image.Kind != BackgroundImageURL || !strings.HasPrefix(data.Image.URL, "data:image/png") || data.Origin != BackgroundBoxPadding || data.Clip != BackgroundBoxContent {
		t.Fatalf("data layer = %#v", data)
	}
}

func TestMaskImageFallsBackToCenteredSVGInsteadOfSolidSquare(t *testing.T) {
	document := dom.NewDocument()
	icon := document.CreateElement("span", map[string]string{"class": "vector-icon"})
	if err := document.AppendChild(document.Root, icon); err != nil {
		t.Fatal(err)
	}
	stylesheet, err := css.Parse(strings.NewReader(`
.vector-icon {
  width:20px; height:20px; display:inline-block; background-color:#202122;
  -webkit-mask-image:url("https://example.test/menu.svg");
  mask-image:url("https://example.test/menu.svg")
}
`))
	if err != nil {
		t.Fatal(err)
	}
	computed, _ := Compute(document, stylesheet).For(icon)
	if computed.BackgroundColor != transparent || computed.BackgroundImage.Kind != BackgroundImageURL || computed.BackgroundImage.URL != "https://example.test/menu.svg" {
		t.Fatalf("mask fallback = color:%08x image:%#v", computed.BackgroundColor, computed.BackgroundImage)
	}
	if computed.BackgroundRepeat.X || computed.BackgroundRepeat.Y || computed.BackgroundSize.Kind != BackgroundSizeContain || computed.BackgroundPos.X.Percentage != 50 || computed.BackgroundPos.Y.Percentage != 50 {
		t.Fatalf("mask fallback geometry = repeat:%#v size:%#v position:%#v", computed.BackgroundRepeat, computed.BackgroundSize, computed.BackgroundPos)
	}
}
