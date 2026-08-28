package style

import (
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/css"
	"github.com/Grove-Computing/Growse/internal/dom"
)

func TestComputeModernTypographyAndFontShorthand(t *testing.T) {
	document := dom.NewDocument()
	parent := document.CreateElement("div", map[string]string{"class": "copy"})
	child := document.CreateElement("span", nil)
	appendNode(t, document, document.Root, parent)
	appendNode(t, document, parent, child)
	stylesheet, err := css.Parse(strings.NewReader(`
.copy {
  font: italic 700 expanded 20px/30px "Fixture", sans-serif;
  text-align: center; text-transform: uppercase; text-indent: 10%;
  letter-spacing: 2px; word-spacing: 3px; word-break: break-all;
  overflow-wrap: anywhere; text-overflow: ellipsis;
}
.copy > span { vertical-align: 4px }
`))
	if err != nil {
		t.Fatal(err)
	}
	computed := Compute(document, stylesheet)
	copyStyle, _ := computed.For(parent)
	if copyStyle.FontSize != 20 || copyStyle.FontWeight != 700 || copyStyle.FontStyle != "italic" || copyStyle.FontStretch != "expanded" || copyStyle.LineHeight != 30 {
		t.Fatalf("font shorthand = %#v", copyStyle)
	}
	if len(copyStyle.FontFamilies) != 2 || copyStyle.FontFamilies[0] != "Fixture" || copyStyle.TextAlign != TextAlignCenter || copyStyle.TextTransform != TextTransformUppercase {
		t.Fatalf("font/text values = %#v", copyStyle)
	}
	if copyStyle.TextIndent.Percentage != 10 || copyStyle.LetterSpacing != 2 || copyStyle.WordSpacing != 3 || copyStyle.WordBreak != WordBreakBreakAll || copyStyle.OverflowWrap != OverflowWrapAnywhere || copyStyle.TextOverflow != TextOverflowEllipsis {
		t.Fatalf("spacing/wrapping values = %#v", copyStyle)
	}
	childStyle, _ := computed.For(child)
	if childStyle.TextAlign != TextAlignCenter || childStyle.LetterSpacing != 2 || childStyle.VerticalAlign.Kind != VerticalAlignLength || childStyle.VerticalAlign.Value != 4 {
		t.Fatalf("inherited/vertical values = %#v", childStyle)
	}
}

func TestSupportsModernTypographyOnlyForImplementedValues(t *testing.T) {
	for _, declaration := range [][2]string{
		{"font", `italic 700 16px/1.5 sans-serif`}, {"text-transform", "capitalize"},
		{"word-break", "break-all"}, {"overflow-wrap", "anywhere"}, {"vertical-align", "super"}, {"text-overflow", "ellipsis"},
	} {
		if !supportsDeclaration(declaration[0], declaration[1]) {
			t.Errorf("supports(%s:%s) = false", declaration[0], declaration[1])
		}
	}
	if supportsDeclaration("word-break", "pretty-much-anywhere") {
		t.Fatal("unknown word-break was reported as supported")
	}
}
