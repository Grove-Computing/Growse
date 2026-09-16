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

func TestVerticalLogicalPropertiesMapThroughWritingAxes(t *testing.T) {
	document := dom.NewDocument()
	verticalRL := document.CreateElement("div", map[string]string{"class": "vertical-rl"})
	verticalLR := document.CreateElement("div", map[string]string{"class": "vertical-lr"})
	appendNode(t, document, document.Root, verticalRL)
	appendNode(t, document, document.Root, verticalLR)
	stylesheet, err := css.Parse(strings.NewReader(`
.vertical-rl {
  writing-mode: vertical-rl; direction: ltr;
  margin-inline: 1px 2px; margin-left: 20px;
  padding-block: 3px 4px;
  border-block-start: 5px solid red;
  border-inline-end-width: 6px; border-inline-end-style: solid;
  border-start-end-radius: 10px;
  inline-size: 100px; block-size: 40px;
  min-inline-size: 80px; max-block-size: 60px;
  position: relative; inset-inline: 7px 9px;
  float: inline-start; clear: inline-end;
}

func TestSupportsLogicalGeometryOnlyForValidValues(t *testing.T) {
	for _, declaration := range [][2]string{
		{"inline-size", "20px"}, {"margin-block", "1px 2px"}, {"padding-inline-start", "4%"},
		{"inset-inline", "auto 10px"}, {"border-block", "2px solid red"},
		{"border-inline-width", "1px 2px"}, {"border-start-end-radius", "5px 10%"},
	} {
		if !supportsDeclaration(declaration[0], declaration[1]) {
			t.Errorf("supportsDeclaration(%q, %q) = false", declaration[0], declaration[1])
		}
	}
	for _, declaration := range [][2]string{
		{"margin-block", "1px 2px 3px"}, {"padding-inline-start", "1px 2px"},
		{"border-inline-width", "1px 2px 3px"}, {"border-start-end-radius", "1px 2px 3px"},
	} {
		if supportsDeclaration(declaration[0], declaration[1]) {
			t.Errorf("supportsDeclaration(%q, %q) = true", declaration[0], declaration[1])
		}
	}
}
.vertical-lr {
  writing-mode: vertical-lr; direction: rtl;
  margin-block-start: 11px; margin-inline-start: 12px;
  border-start-start-radius: 13px;
  float: inline-start; clear: inline-end;
}
`))
	if err != nil {
		t.Fatal(err)
	}
	computed := Compute(document, stylesheet)
	rl, _ := computed.For(verticalRL)
	if rl.Margin.Top != 1 || rl.Margin.Bottom != 2 || rl.Margin.Left != 20 {
		t.Fatalf("vertical-rl margin = %#v", rl.Margin)
	}
	if rl.Padding.Right != 3 || rl.Padding.Left != 4 {
		t.Fatalf("vertical-rl padding = %#v", rl.Padding)
	}
	if rl.Border.Right.Width != 5 || rl.Border.Right.Color != 0xff0000ff || rl.Border.Bottom.Width != 6 {
		t.Fatalf("vertical-rl border = %#v", rl.Border)
	}
	if rl.BorderRadius.BottomRight.X.Pixels != 10 || rl.BorderRadius.BottomRight.Y.Pixels != 10 {
		t.Fatalf("vertical-rl logical corner = %#v", rl.BorderRadius)
	}
	if rl.Width.Value.Pixels != 40 || rl.Height.Value.Pixels != 100 || rl.MinHeight.Value.Pixels != 80 || rl.MaxWidth.Value.Pixels != 60 {
		t.Fatalf("vertical-rl logical sizes = width:%#v height:%#v min-height:%#v max-width:%#v", rl.Width, rl.Height, rl.MinHeight, rl.MaxWidth)
	}
	if rl.Inset.Top.Value.Pixels != 7 || rl.Inset.Bottom.Value.Pixels != 9 {
		t.Fatalf("vertical-rl inset = %#v", rl.Inset)
	}
	if rl.Float != FloatTop || rl.Clear != ClearBottom {
		t.Fatalf("vertical-rl float/clear = %v/%v", rl.Float, rl.Clear)
	}

	lr, _ := computed.For(verticalLR)
	if lr.Margin.Left != 11 || lr.Margin.Bottom != 12 {
		t.Fatalf("vertical-lr rtl margin = %#v", lr.Margin)
	}
	if lr.BorderRadius.BottomLeft.X.Pixels != 13 || lr.BorderRadius.BottomLeft.Y.Pixels != 13 {
		t.Fatalf("vertical-lr rtl logical corner = %#v", lr.BorderRadius)
	}
	if lr.Float != FloatBottom || lr.Clear != ClearTop {
		t.Fatalf("vertical-lr rtl float/clear = %v/%v", lr.Float, lr.Clear)
	}
}
