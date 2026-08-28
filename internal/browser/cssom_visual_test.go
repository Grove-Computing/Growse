package browser

import (
	"testing"

	stylemodel "github.com/Grove-Computing/Growse/internal/style"
)

func TestCSSOMSerializesImplementedVisualProperties(t *testing.T) {
	computed := stylemodel.ComputedStyle{
		ObjectFit: stylemodel.ObjectFitCover, ObjectPosition: stylemodel.BackgroundPosition{X: stylemodel.LengthPercentage{Percentage: 100}, Y: stylemodel.LengthPercentage{Percentage: 25}},
		ListStyleType: stylemodel.ListStyleSquare, ListStylePosition: stylemodel.ListStyleInside, ListStyleImage: "marker.png",
		Appearance: stylemodel.AppearanceNone, AccentColor: 0x12ab34ff, Cursor: stylemodel.CursorPointer,
		Filters:         []stylemodel.Filter{{Kind: stylemodel.FilterBlur, Radius: 4}, {Kind: stylemodel.FilterBrightness, Amount: 1.2}},
		BackdropFilters: []stylemodel.Filter{{Kind: stylemodel.FilterContrast, Amount: .8}}, MixBlendMode: stylemodel.BlendMultiply,
	}
	properties := cssomProperties(computed, 100, 50)
	want := map[string]string{
		"object-fit": "cover", "object-position": "100% 25%", "list-style-type": "square", "list-style-position": "inside", "list-style-image": `url("marker.png")`,
		"appearance": "none", "accent-color": "rgb(18, 171, 52)", "cursor": "pointer", "filter": "blur(4px) brightness(1.2)", "backdrop-filter": "contrast(0.8)", "mix-blend-mode": "multiply",
	}
	for property, expected := range want {
		if properties[property] != expected {
			t.Errorf("%s = %q, want %q", property, properties[property], expected)
		}
	}
}
