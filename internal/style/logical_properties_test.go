package style

import (
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/css"
	"github.com/Grove-Computing/Growse/internal/dom"
)

func TestHorizontalLogicalPropertiesMapToPhysicalCascadeOrder(t *testing.T) {
	document := dom.NewDocument()
	physicalLast := document.CreateElement("div", map[string]string{"class": "physical-last"})
	logicalLast := document.CreateElement("div", map[string]string{"class": "logical-last"})
	appendNode(t, document, document.Root, physicalLast)
	appendNode(t, document, document.Root, logicalLast)
	stylesheet, err := css.Parse(strings.NewReader(`
.physical-last {
  margin-inline-start: 10px; margin-left: 20px;
  padding-block: 3px 5px;
  border-inline: 2px solid red;
  border-start-start-radius: 6px;
  inline-size: 100px; block-size: 40px;
  min-inline-size: 80px; max-block-size: 60px;
  position: relative; inset-inline: 7px 9px;
}
.logical-last { margin-left: 20px; margin-inline-start: 10px; }
`))
	if err != nil {
		t.Fatal(err)
	}
	computed := Compute(document, stylesheet)
	first, _ := computed.For(physicalLast)
	second, _ := computed.For(logicalLast)
	if first.Margin.Left != 20 || second.Margin.Left != 10 || first.Padding.Top != 3 || first.Padding.Bottom != 5 {
		t.Fatalf("logical edges = first margin:%#v padding:%#v second margin:%#v", first.Margin, first.Padding, second.Margin)
	}
	if first.Border.Left.Width != 2 || first.Border.Right.Width != 2 || first.Border.Left.Color != 0xff0000ff || first.Border.Right.Color != 0xff0000ff {
		t.Fatalf("logical borders = %#v", first.Border)
	}
	if first.BorderRadius.TopLeft.X.Pixels != 6 || first.BorderRadius.TopLeft.Y.Pixels != 6 {
		t.Fatalf("logical radius = %#v", first.BorderRadius)
	}
	if first.Width.Value.Pixels != 100 || first.Height.Value.Pixels != 40 || first.MinWidth.Value.Pixels != 80 || first.MaxHeight.Value.Pixels != 60 {
		t.Fatalf("logical sizes = width:%#v height:%#v min:%#v max:%#v", first.Width, first.Height, first.MinWidth, first.MaxHeight)
	}
	if first.Inset.Left.Value.Pixels != 7 || first.Inset.Right.Value.Pixels != 9 {
		t.Fatalf("logical inset = %#v", first.Inset)
	}
}
