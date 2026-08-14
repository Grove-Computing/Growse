package layout

import (
	"strings"
	"testing"

	"github.com/saku0512/growse/internal/css"
	"github.com/saku0512/growse/internal/dom"
	stylemodel "github.com/saku0512/growse/internal/style"
)

// Adapted from css/css-flexbox/flex-grow-001.xht at the revision recorded in
// docs/wpt.md. Geometry assertions replace the upstream visual reference.
func TestWPTFlexGrow001DistributesPositiveFreeSpaceByFactor(t *testing.T) {
	line := flexLine{items: []*flexItem{
		{base: 30, hypothetical: 30, maximum: -1, grow: 0, shrink: 1},
		{base: 30, hypothetical: 30, maximum: -1, grow: 1, shrink: 1},
		{base: 30, hypothetical: 30, maximum: -1, grow: 2, shrink: 1},
	}}
	resolveFlexibleLengths(&line, 240, 0)
	want := []float32{30, 80, 130}
	for index, item := range line.items {
		if item.target != want[index] {
			t.Fatalf("item %d width = %v, want %v", index, item.target, want[index])
		}
	}
}

// Adapted from css/css-flexbox/flex-wrap-002.html. The upstream reftest uses
// two 100px-tall items to require two lines in a 100px column main axis.
func TestWPTFlexWrap002FormsTwoColumnFlexLines(t *testing.T) {
	items := []*flexItem{{hypothetical: 100}, {hypothetical: 100}}
	lines := formFlexLines(items, 100, 0, stylemodel.FlexWrapLines)
	if len(lines) != 2 || len(lines[0].items) != 1 || len(lines[1].items) != 1 {
		t.Fatalf("lines = %#v, want two single-item lines", lines)
	}
}

// Adapted from css/css-flexbox/flex-direction-row-reverse.html. Numeric axis
// assertions replace the upstream reference image.
func TestWPTFlexDirectionRowReverseMapsMainAxis(t *testing.T) {
	axis := axisFor(stylemodel.FlexDirectionRowReverse, stylemodel.FlexNoWrap)
	if !axis.horizontal || !axis.reverse || axis.crossFlip {
		t.Fatalf("row-reverse axis = %#v", axis)
	}
}

// Adapted from css/css-grid/computed-grid-column.html at the revision recorded
// in docs/wpt.md. The upstream calc() expressions resolve to row 1 / column 2;
// Growse exercises the equivalent computed placement and numeric geometry.
func TestWPTComputedGridColumnPlacesItemOnSecondColumn(t *testing.T) {
	document := dom.NewDocument()
	grid := document.CreateElement("div", map[string]string{"class": "grid"})
	target := document.CreateElement("div", map[string]string{"id": "target"})
	appendNodes(t, document, [2]*dom.Node{document.Root, grid}, [2]*dom.Node{grid, target})
	stylesheet, err := css.Parse(strings.NewReader(`
.grid { display:inline-grid; grid-template-columns:100px 150px; grid-template-rows:150px 100px; background-color:#fff }
#target { grid-row:1; grid-column:2; width:20px; height:20px; background-color:#008000 }
`))
	if err != nil {
		t.Fatal(err)
	}
	computed := stylemodel.Compute(document, stylesheet)
	targetStyle, ok := computed.For(target)
	if !ok || targetStyle.GridColumn.Start.Index != 2 || targetStyle.GridRow.Start.Index != 1 {
		t.Fatalf("computed placement = %#v, want row 1 / column 2", targetStyle)
	}
	tree := Build(document, computed, 400)
	gridRect := decorationForNode(t, tree, grid.ID)
	targetRect := decorationForNode(t, tree, target.ID)
	if targetRect.X != gridRect.X+100 || targetRect.Y != gridRect.Y {
		t.Fatalf("target geometry = %#v, grid = %#v", targetRect.Rect, gridRect.Rect)
	}
}

// Adapted from css/CSS2/abspos/abspos-containing-block-initial-005b.xht.
// Numeric viewport-relative geometry replaces the upstream reference image.
func TestWPTAbsoluteWithoutPositionedAncestorUsesInitialContainingBlock(t *testing.T) {
	document := dom.NewDocument()
	target := document.CreateElement("div", map[string]string{"id": "target"})
	appendNodes(t, document, [2]*dom.Node{document.Root, target})
	stylesheet, err := css.Parse(strings.NewReader(`
#target { position:absolute; right:25px; bottom:30px; width:100px; height:80px; background-color:#ffff00 }
`))
	if err != nil {
		t.Fatal(err)
	}
	tree := BuildWithViewport(document, stylemodel.Compute(document, stylesheet), 500, 300)
	rect := decorationForNode(t, tree, target.ID)
	if rect.X != 375 || rect.Y != 190 || rect.Width != 100 || rect.Height != 80 {
		t.Fatalf("initial containing block geometry = %#v", rect.Rect)
	}
}

// Adapted from css/css-transforms/transform-origin-001.html. The WPT is a
// mismatch reftest; Growse compares the resulting matrices directly.
func TestWPTTransformOriginZeroDiffersFromDefaultCenter(t *testing.T) {
	matrixFor := func(origin string) stylemodel.Matrix {
		document := dom.NewDocument()
		target := document.CreateElement("div", map[string]string{"id": "target"})
		appendNodes(t, document, [2]*dom.Node{document.Root, target})
		declaration := ""
		if origin != "" {
			declaration = " transform-origin:" + origin + ";"
		}
		stylesheet, err := css.Parse(strings.NewReader(
			"#target { width:200px; height:100px; background-color:#fff; transform:rotate(45deg);" + declaration + " }",
		))
		if err != nil {
			t.Fatal(err)
		}
		return decorationForNode(t, Build(document, stylemodel.Compute(document, stylesheet), 400), target.ID).Transform
	}
	zero, center := matrixFor("0% 0%"), matrixFor("")
	if zero == center {
		t.Fatalf("zero origin matrix = center origin matrix = %#v", zero)
	}
}
