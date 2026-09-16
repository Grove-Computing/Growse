package layout

import (
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/css"
	"github.com/Grove-Computing/Growse/internal/dom"
	stylemodel "github.com/Grove-Computing/Growse/internal/style"
)

// Adapted from css/CSS2/margin-padding-clear/margin-collapse-001.xht at the
// revision recorded in docs/wpt.md. Numeric inline geometry replaces Ahem
// stripes while preserving the assertion that horizontal margins do not
// collapse.
func TestWPTV019CSS2HorizontalMarginsDoNotCollapse(t *testing.T) {
	document := dom.NewDocument()
	line := document.CreateElement("div", map[string]string{"class": "line"})
	first := document.CreateElement("span", map[string]string{"class": "item"})
	second := document.CreateElement("span", map[string]string{"class": "item"})
	appendNodes(t, document, [2]*dom.Node{document.Root, line}, [2]*dom.Node{line, first}, [2]*dom.Node{line, second})
	stylesheet, err := css.Parse(strings.NewReader(`
.line { display:block; width:300px }
.item { display:inline-block; width:50px; height:20px; margin:0 20px; background:#080 }
`))
	if err != nil {
		t.Fatal(err)
	}
	tree := Build(document, stylemodel.Compute(document, stylesheet), 400)
	left, right := tree.Bounds[first.ID], tree.Bounds[second.ID]
	if gap := right.X - (left.X + left.Width); gap != 40 {
		t.Fatalf("horizontal inline margins collapsed: first=%#v second=%#v gap=%v", left, right, gap)
	}
}

// Adapted from css/css-flexbox/flex-margin-no-collapse.html. The upstream
// reftest's two 50px adjoining margins remain a 100px flex-item gap.
func TestWPTV019FlexItemMarginsDoNotCollapse(t *testing.T) {
	document := dom.NewDocument()
	container := document.CreateElement("div", map[string]string{"class": "flex"})
	first := document.CreateElement("div", map[string]string{"class": "item"})
	second := document.CreateElement("div", map[string]string{"class": "item"})
	appendNodes(t, document, [2]*dom.Node{document.Root, container}, [2]*dom.Node{container, first}, [2]*dom.Node{container, second})
	stylesheet, err := css.Parse(strings.NewReader(`
.flex { display:flex; flex-direction:column; width:200px; height:400px }
.item { flex:none; width:100px; height:100px; margin:50px 0; background:#080 }
`))
	if err != nil {
		t.Fatal(err)
	}
	tree := Build(document, stylemodel.Compute(document, stylesheet), 400)
	left, right := tree.Bounds[first.ID], tree.Bounds[second.ID]
	if gap := right.Y - (left.Y + left.Height); gap != 100 {
		t.Fatalf("flex item margins collapsed: first=%#v second=%#v gap=%v", left, right, gap)
	}
}

// Adapted from css/css-grid/subgrid/grid-gap-001.html. The full visual matrix
// is reduced to inherited parent tracks plus an explicit subgrid gap.
func TestWPTV019SubgridUsesInheritedTracksAndOwnGap(t *testing.T) {
	document := dom.NewDocument()
	parent := document.CreateElement("div", map[string]string{"class": "parent"})
	subgrid := document.CreateElement("div", map[string]string{"class": "subgrid"})
	first := document.CreateElement("div", map[string]string{"class": "first"})
	second := document.CreateElement("div", map[string]string{"class": "second"})
	appendNodes(t, document,
		[2]*dom.Node{document.Root, parent}, [2]*dom.Node{parent, subgrid},
		[2]*dom.Node{subgrid, first}, [2]*dom.Node{subgrid, second},
	)
	stylesheet, err := css.Parse(strings.NewReader(`
.parent { display:grid; width:220px; grid-template-columns:60px 60px 60px; grid-template-rows:40px; column-gap:20px }
.subgrid { display:grid; grid-column:1 / 4; grid-row:1; grid-template-columns:subgrid; column-gap:10px }
.first { grid-column:1; background:#444 }
.second { grid-column:2; background:#ccc }
`))
	if err != nil {
		t.Fatal(err)
	}
	tree := Build(document, stylemodel.Compute(document, stylesheet), 400)
	parentBounds := tree.Bounds[parent.ID]
	firstBounds, secondBounds := tree.Bounds[first.ID], tree.Bounds[second.ID]
	if firstBounds.X != parentBounds.X || firstBounds.Width != 65 || secondBounds.X != parentBounds.X+75 || secondBounds.Width != 70 {
		t.Fatalf("subgrid inherited track/gap geometry = parent:%#v first:%#v second:%#v", parentBounds, firstBounds, secondBounds)
	}
}

// Adapted from css/css-writing-modes/logical-physical-mapping-001.html. The
// source's colored border comparison is expressed as physical border mapping.
func TestWPTV019BlockStartMapsAcrossWritingModes(t *testing.T) {
	document := dom.NewDocument()
	horizontal := document.CreateElement("div", map[string]string{"class": "horizontal"})
	verticalRL := document.CreateElement("div", map[string]string{"class": "vertical-rl"})
	verticalLR := document.CreateElement("div", map[string]string{"class": "vertical-lr"})
	appendNodes(t, document, [2]*dom.Node{document.Root, horizontal}, [2]*dom.Node{document.Root, verticalRL}, [2]*dom.Node{document.Root, verticalLR})
	stylesheet, err := css.Parse(strings.NewReader(`
div { display:block; width:40px; height:40px; border-block-start:2px solid orange }
.horizontal { writing-mode:horizontal-tb }
.vertical-rl { writing-mode:vertical-rl }
.vertical-lr { writing-mode:vertical-lr }
`))
	if err != nil {
		t.Fatal(err)
	}
	computed := stylemodel.Compute(document, stylesheet)
	horizontalStyle, _ := computed.For(horizontal)
	rightStyle, _ := computed.For(verticalRL)
	leftStyle, _ := computed.For(verticalLR)
	if horizontalStyle.Border.Top.Width != 2 || rightStyle.Border.Right.Width != 2 || leftStyle.Border.Left.Width != 2 {
		t.Fatalf("block-start mapping = horizontal:%#v vertical-rl:%#v vertical-lr:%#v", horizontalStyle.Border, rightStyle.Border, leftStyle.Border)
	}
}

// Adapted from css/css-position/sticky/position-sticky-top.html. Direct axis
// assertions preserve the three upstream phases: normal flow, stuck, and
// containing-block end.
func TestWPTV019StickyTopRespectsConstraintAndContainerEnd(t *testing.T) {
	length := func(value float32) stylemodel.SizeValue {
		return stylemodel.SizeValue{Kind: stylemodel.SizeLength, Value: stylemodel.LengthPercentage{Pixels: value}}
	}
	for _, test := range []struct {
		name                     string
		flow, scroll, want       float32
		containingBlockBlockSize float32
	}{
		{name: "before sticking", flow: 150, scroll: 100, want: 150, containingBlockBlockSize: 400},
		{name: "stuck", flow: 150, scroll: 200, want: 250, containingBlockBlockSize: 400},
		{name: "container end", flow: 150, scroll: 300, want: 300, containingBlockBlockSize: 330},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := stickyAxisPosition(test.flow, 30, test.scroll, 100, 0, test.containingBlockBlockSize, length(50), stylemodel.SizeValue{})
			if got != test.want {
				t.Fatalf("sticky top position = %v, want %v", got, test.want)
			}
		})
	}
}

// Adapted from css/css-overflow/overflow-clip-cant-scroll.html. Programmatic
// scrolling is reduced to the shared layout scroll-offset API.
func TestWPTV019OverflowClipCannotScroll(t *testing.T) {
	document := dom.NewDocument()
	parent := document.CreateElement("div", map[string]string{"class": "parent"})
	first := document.CreateElement("div", map[string]string{"class": "child"})
	second := document.CreateElement("div", map[string]string{"class": "child"})
	appendNodes(t, document, [2]*dom.Node{document.Root, parent}, [2]*dom.Node{parent, first}, [2]*dom.Node{parent, second})
	stylesheet, err := css.Parse(strings.NewReader(`
.parent { display:block; width:100px; height:100px; overflow:clip }
.child { display:block; width:100px; height:100px; background:#080 }
`))
	if err != nil {
		t.Fatal(err)
	}
	computed := stylemodel.Compute(document, stylesheet)
	tree := BuildWithViewport(document, computed, 400, 300)
	before := tree.Bounds[second.ID]
	if _, exists := tree.ScrollContainers[parent.ID]; exists {
		t.Fatal("overflow:clip created a scroll container")
	}
	if dirty := ApplyScrollContainerOffset(tree, computed, parent.ID, 0, 100); len(dirty) != 0 || tree.Bounds[second.ID] != before {
		t.Fatalf("overflow:clip accepted scroll: dirty=%v before=%#v after=%#v", dirty, before, tree.Bounds[second.ID])
	}
}

// Adapted from css/css-multicol/multicol-margin-001.xht. The first child's
// block-start margin must remain inside the multicol container rather than
// collapsing through its parent.
func TestWPTV019MultiColumnFirstChildMarginDoesNotCollapse(t *testing.T) {
	document := dom.NewDocument()
	columns := document.CreateElement("div", map[string]string{"class": "columns"})
	first := document.CreateElement("div", map[string]string{"class": "first"})
	appendNodes(t, document, [2]*dom.Node{document.Root, columns}, [2]*dom.Node{columns, first})
	stylesheet, err := css.Parse(strings.NewReader(`
.columns { display:block; width:200px; height:200px; column-count:2; column-fill:auto; column-gap:0; background:#f00 }
.first { display:block; height:50px; margin-top:100px; background:#080 }
`))
	if err != nil {
		t.Fatal(err)
	}
	tree := BuildWithViewport(document, stylemodel.Compute(document, stylesheet), 400, 300)
	columnBounds, firstBounds := tree.Bounds[columns.ID], tree.Bounds[first.ID]
	if firstBounds.Y != columnBounds.Y+100 {
		t.Fatalf("first multicol child margin collapsed: columns=%#v first=%#v", columnBounds, firstBounds)
	}
}
