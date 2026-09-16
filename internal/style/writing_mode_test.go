package style

import (
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/css"
	"github.com/Grove-Computing/Growse/internal/dom"
)

func TestWritingModeAndDirectionComputeAndInherit(t *testing.T) {
	t.Parallel()
	document := dom.NewDocument()
	parent := document.CreateElement("section", map[string]string{"class": "vertical"})
	child := document.CreateElement("p", nil)
	horizontal := document.CreateElement("aside", map[string]string{"class": "horizontal"})
	appendNode(t, document, document.Root, parent)
	appendNode(t, document, parent, child)
	appendNode(t, document, document.Root, horizontal)
	stylesheet, err := css.Parse(strings.NewReader(`
.vertical { writing-mode: vertical-lr; direction: rtl }
.horizontal { writing-mode: horizontal-tb; direction: ltr }
`))
	if err != nil {
		t.Fatal(err)
	}
	computed := Compute(document, stylesheet)
	parentStyle, _ := computed.For(parent)
	childStyle, _ := computed.For(child)
	horizontalStyle, _ := computed.For(horizontal)
	if parentStyle.WritingMode != WritingModeVerticalLR || parentStyle.Direction != DirectionRTL {
		t.Fatalf("parent writing mode/direction = %v/%v", parentStyle.WritingMode, parentStyle.Direction)
	}
	if childStyle.WritingMode != WritingModeVerticalLR || childStyle.Direction != DirectionRTL {
		t.Fatalf("inherited writing mode/direction = %v/%v", childStyle.WritingMode, childStyle.Direction)
	}
	if horizontalStyle.WritingMode != WritingModeHorizontalTB || horizontalStyle.Direction != DirectionLTR {
		t.Fatalf("horizontal writing mode/direction = %v/%v", horizontalStyle.WritingMode, horizontalStyle.Direction)
	}
}
