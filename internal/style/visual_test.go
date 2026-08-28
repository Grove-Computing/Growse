package style

import (
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/css"
	"github.com/Grove-Computing/Growse/internal/dom"
)

func TestComputeVisualPropertiesAndKeepsSupportedFilterFunctions(t *testing.T) {
	document := dom.NewDocument()
	list := document.CreateElement("ul", map[string]string{"class": "plain"})
	item := document.CreateElement("li", nil)
	card := document.CreateElement("div", map[string]string{"class": "card"})
	appendNode(t, document, document.Root, list)
	appendNode(t, document, list, item)
	appendNode(t, document, document.Root, card)
	stylesheet, err := css.Parse(strings.NewReader(`
.plain { list-style:none inside }
.card {
  object-fit:cover; object-position:right 25%; appearance:none; accent-color:oklch(60% .2 30); cursor:pointer;
  filter:brightness(120%) unsupported(1) blur(4px) blur(101px);
  backdrop-filter:contrast(.8); mix-blend-mode:multiply;
}
`))
	if err != nil {
		t.Fatal(err)
	}
	computed := Compute(document, stylesheet)
	itemStyle, _ := computed.For(item)
	if itemStyle.ListStyleType != ListStyleNone || itemStyle.ListStylePosition != ListStyleInside {
		t.Fatalf("inherited list style = %#v", itemStyle)
	}
	cardStyle, _ := computed.For(card)
	if cardStyle.ObjectFit != ObjectFitCover || cardStyle.ObjectPosition.X.Percentage != 100 || cardStyle.ObjectPosition.Y.Percentage != 25 || cardStyle.Appearance != AppearanceNone || cardStyle.AccentColorAuto || cardStyle.Cursor != CursorPointer || cardStyle.MixBlendMode != BlendMultiply {
		t.Fatalf("visual values = %#v", cardStyle)
	}
	if len(cardStyle.Filters) != 2 || cardStyle.Filters[0].Kind != FilterBrightness || cardStyle.Filters[1].Kind != FilterBlur || cardStyle.Filters[1].Radius != 4 || len(cardStyle.BackdropFilters) != 1 {
		t.Fatalf("bounded filters = %#v / %#v", cardStyle.Filters, cardStyle.BackdropFilters)
	}
}

func TestFilterLimitsAndSolidColorApplication(t *testing.T) {
	context := LengthContext{FontSize: 16, RootFontSize: 16}
	chain := strings.Repeat("brightness(1) ", MaxFilterFunctions+1)
	if _, valid := parseFilterList(chain, context); valid {
		t.Fatal("oversized filter chain was accepted")
	}
	if _, valid := parseFilterList("blur(101px)", context); valid {
		t.Fatal("oversized blur was accepted")
	}
	if FilterSurfaceAllowed(5000, 10) || FilterSurfaceAllowed(4096, 4096.1) || !FilterSurfaceAllowed(100, 100) {
		t.Fatal("filter surface boundary is inconsistent")
	}
	filtered := ApplyColorFilters(0x804020ff, []Filter{{Kind: FilterBrightness, Amount: 2}, {Kind: FilterOpacity, Amount: .5}})
	if filtered != 0xff804080 {
		t.Fatalf("filtered color = %08x", filtered)
	}
}

func TestSupportsVisualSubsetWithoutUnknownValues(t *testing.T) {
	for _, declaration := range [][2]string{{"object-fit", "cover"}, {"list-style", "square inside"}, {"appearance", "none"}, {"accent-color", "red"}, {"cursor", "grab"}, {"filter", "blur(2px)"}, {"backdrop-filter", "contrast(.8)"}, {"mix-blend-mode", "screen"}} {
		if !supportsDeclaration(declaration[0], declaration[1]) {
			t.Errorf("supports(%s:%s) = false", declaration[0], declaration[1])
		}
	}
	if supportsDeclaration("mix-blend-mode", "difference") || supportsDeclaration("cursor", "zoom-to-the-moon") {
		t.Fatal("unknown visual value was reported as supported")
	}
}
