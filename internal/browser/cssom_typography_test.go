package browser

import (
	"testing"

	stylemodel "github.com/Grove-Computing/Growse/internal/style"
)

func TestCSSOMSerializesImplementedTypography(t *testing.T) {
	properties := cssomProperties(stylemodel.ComputedStyle{
		FontSize: 20, FontWeight: 700, FontFamilies: []string{"Fixture", "sans-serif"}, FontStyle: "italic", FontStretch: "expanded", LineHeight: 30,
		TextAlign: stylemodel.TextAlignCenter, TextTransform: stylemodel.TextTransformUppercase, TextIndent: stylemodel.LengthPercentage{Percentage: 10},
		LetterSpacing: 2, WordSpacing: 3, WordBreak: stylemodel.WordBreakBreakAll, OverflowWrap: stylemodel.OverflowWrapAnywhere,
		VerticalAlign: stylemodel.VerticalAlign{Kind: stylemodel.VerticalAlignLength, Value: 4}, TextOverflow: stylemodel.TextOverflowEllipsis,
	}, 100, 50)
	want := map[string]string{
		"font-family": "Fixture, sans-serif", "font-style": "italic", "font-stretch": "expanded",
		"text-align": "center", "text-transform": "uppercase", "text-indent": "10%", "letter-spacing": "2px", "word-spacing": "3px",
		"word-break": "break-all", "overflow-wrap": "anywhere", "vertical-align": "4px", "text-overflow": "ellipsis",
	}
	for property, expected := range want {
		if properties[property] != expected {
			t.Errorf("%s = %q, want %q", property, properties[property], expected)
		}
	}
}
