package layout_test

import (
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/css"
	"github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/layout"
	"github.com/Grove-Computing/Growse/internal/style"
)

func TestTableFixedColumnsCaptionGroupsSpacingAndSpansShareGeometry(t *testing.T) {
	document := dom.NewDocument()
	table := document.CreateElement("table", map[string]string{"class": "matrix"})
	caption := document.CreateElement("caption", nil)
	colgroup := document.CreateElement("colgroup", nil)
	firstCol := document.CreateElement("col", map[string]string{"class": "first-col"})
	secondCol := document.CreateElement("col", map[string]string{"class": "second-col"})
	tbody := document.CreateElement("tbody", nil)
	firstRow := document.CreateElement("tr", nil)
	secondRow := document.CreateElement("tr", nil)
	first := document.CreateElement("td", map[string]string{"class": "first"})
	second := document.CreateElement("td", map[string]string{"class": "second", "rowspan": "2"})
	wide := document.CreateElement("td", map[string]string{"colspan": "1"})
	for _, edge := range [][2]*dom.Node{
		{document.Root, table}, {table, caption}, {caption, document.CreateText("Matrix caption")},
		{table, colgroup}, {colgroup, firstCol}, {colgroup, secondCol}, {table, tbody},
		{tbody, firstRow}, {firstRow, first}, {first, document.CreateText("A")}, {firstRow, second}, {second, document.CreateText("B")},
		{tbody, secondRow}, {secondRow, wide}, {wide, document.CreateText("later content must not resize the fixed first column")},
	} {
		if err := document.AppendChild(edge[0], edge[1]); err != nil {
			t.Fatal(err)
		}
	}
	stylesheet, err := css.Parse(strings.NewReader(`
.matrix { display:table; table-layout:fixed; width:260px; border-spacing:10px 6px; background:#eee }
caption { display:table-caption; caption-side:bottom; height:24px }
colgroup { display:table-column-group }
col { display:table-column }
.first-col { width:80px }
.second-col { width:120px }
tbody { display:table-row-group }
tr { display:table-row }
td { display:table-cell; height:30px; border:1px solid #333 }
`))
	if err != nil {
		t.Fatal(err)
	}
	tree := layout.Build(document, style.Compute(document, stylesheet), 500)
	firstRect, secondRect := tree.Bounds[first.ID], tree.Bounds[second.ID]
	captionRect, tableRect := tree.Bounds[caption.ID], tree.Bounds[table.ID]
	firstColumnRect, secondColumnRect := tree.Bounds[firstCol.ID], tree.Bounds[secondCol.ID]
	if gap := secondRect.X - (firstRect.X + firstRect.Width); gap != 10 {
		t.Fatalf("separated cell gap = %v, want 10; first=%#v second=%#v", gap, firstRect, secondRect)
	}
	if secondRect.Height <= firstRect.Height || captionRect.Y < secondRect.Y+secondRect.Height || tableRect.Height < captionRect.Y+captionRect.Height-tableRect.Y {
		t.Fatalf("rowspan/caption/table geometry = first:%#v second:%#v caption:%#v table:%#v", firstRect, secondRect, captionRect, tableRect)
	}
	if firstColumnRect.X != firstRect.X || secondColumnRect.X != secondRect.X || firstColumnRect.Width >= secondColumnRect.Width {
		t.Fatalf("column/group geometry = first:%#v second:%#v cells:%#v/%#v", firstColumnRect, secondColumnRect, firstRect, secondRect)
	}
	if groupRect := tree.Bounds[colgroup.ID]; groupRect.Width < firstColumnRect.Width+secondColumnRect.Width {
		t.Fatalf("column group bounds = %#v", groupRect)
	}
}

func TestCollapsedTableSuppressesSpacingAndSharesAdjacentEdges(t *testing.T) {
	document := dom.NewDocument()
	table := document.CreateElement("table", nil)
	row := document.CreateElement("tr", nil)
	left := document.CreateElement("td", nil)
	right := document.CreateElement("td", nil)
	for _, edge := range [][2]*dom.Node{{document.Root, table}, {table, row}, {row, left}, {left, document.CreateText("left")}, {row, right}, {right, document.CreateText("right")}} {
		if err := document.AppendChild(edge[0], edge[1]); err != nil {
			t.Fatal(err)
		}
	}
	stylesheet, err := css.Parse(strings.NewReader(`
table { display:table; width:200px; border-collapse:collapse; border-spacing:20px }
tr { display:table-row }
td { display:table-cell; border:2px solid #111 }
`))
	if err != nil {
		t.Fatal(err)
	}
	tree := layout.Build(document, style.Compute(document, stylesheet), 300)
	leftRect, rightRect := tree.Bounds[left.ID], tree.Bounds[right.ID]
	if rightRect.X != leftRect.X+leftRect.Width {
		t.Fatalf("collapsed edge geometry = left:%#v right:%#v", leftRect, rightRect)
	}
}

func TestTableAutoUsesLaterIntrinsicContributionsWhileFixedUsesFirstRow(t *testing.T) {
	document := dom.NewDocument()
	makeTable := func(class string) (*dom.Node, *dom.Node) {
		table := document.CreateElement("table", map[string]string{"class": class})
		firstRow, secondRow := document.CreateElement("tr", nil), document.CreateElement("tr", nil)
		firstA, firstB := document.CreateElement("td", nil), document.CreateElement("td", nil)
		longCell, shortCell := document.CreateElement("td", nil), document.CreateElement("td", nil)
		for _, edge := range [][2]*dom.Node{
			{document.Root, table}, {table, firstRow}, {firstRow, firstA}, {firstA, document.CreateText("a")},
			{firstRow, firstB}, {firstB, document.CreateText("b")}, {table, secondRow},
			{secondRow, longCell}, {longCell, document.CreateText("a very long later intrinsic contribution")},
			{secondRow, shortCell}, {shortCell, document.CreateText("x")},
		} {
			if err := document.AppendChild(edge[0], edge[1]); err != nil {
				t.Fatal(err)
			}
		}
		return table, longCell
	}
	_, autoLong := makeTable("auto")
	_, fixedLong := makeTable("fixed")
	stylesheet, err := css.Parse(strings.NewReader(`
table { display:table; width:240px; border-spacing:0 }
.auto { table-layout:auto }
.fixed { table-layout:fixed }
tr { display:table-row }
td { display:table-cell }
`))
	if err != nil {
		t.Fatal(err)
	}
	tree := layout.Build(document, style.Compute(document, stylesheet), 400)
	if tree.Bounds[autoLong.ID].Width <= tree.Bounds[fixedLong.ID].Width {
		t.Fatalf("auto/fixed later contribution widths = %v/%v", tree.Bounds[autoLong.ID].Width, tree.Bounds[fixedLong.ID].Width)
	}
}

func TestAspectRatioSizesBlockFlexGridImageIframeAndFormControls(t *testing.T) {
	document := dom.NewDocument()
	block := document.CreateElement("div", map[string]string{"class": "block"})
	flex := document.CreateElement("div", map[string]string{"class": "flex"})
	flexItem := document.CreateElement("div", map[string]string{"class": "flex-item"})
	grid := document.CreateElement("div", map[string]string{"class": "grid"})
	gridItem := document.CreateElement("div", map[string]string{"class": "grid-item"})
	image := document.CreateElement("img", map[string]string{"class": "image"})
	frame := document.CreateElement("iframe", map[string]string{"class": "frame"})
	input := document.CreateElement("input", map[string]string{"class": "input"})
	contentBox := document.CreateElement("div", map[string]string{"class": "content-box"})
	borderBox := document.CreateElement("div", map[string]string{"class": "border-box"})
	for _, edge := range [][2]*dom.Node{
		{document.Root, block}, {document.Root, flex}, {flex, flexItem}, {document.Root, grid}, {grid, gridItem},
		{document.Root, image}, {document.Root, frame}, {document.Root, input}, {document.Root, contentBox}, {document.Root, borderBox},
	} {
		if err := document.AppendChild(edge[0], edge[1]); err != nil {
			t.Fatal(err)
		}
	}
	stylesheet, err := css.Parse(strings.NewReader(`
.block { display:block; width:160px; height:auto; aspect-ratio:2 }
.flex { display:flex; width:300px }
.flex-item { width:80px; height:auto; aspect-ratio:2 }
.grid { display:grid; width:180px; grid-template-columns:90px; grid-template-rows:auto }
.grid-item { width:90px; height:auto; aspect-ratio:3 }
.image { display:block; width:auto; height:50px; aspect-ratio:3 }
.frame { display:inline-block; width:120px; height:auto; aspect-ratio:2 }
.input { width:100px; height:auto; aspect-ratio:4 }
.content-box { box-sizing:content-box; width:50%; height:auto; aspect-ratio:2; padding:10px; border:2px solid }
.border-box { box-sizing:border-box; width:50%; height:auto; aspect-ratio:2; padding:10px; border:2px solid }
`))
	if err != nil {
		t.Fatal(err)
	}
	computed := style.Compute(document, stylesheet)
	images := map[dom.NodeID]layout.ImageResource{image.ID: {Loaded: true, IntrinsicWidth: 60, IntrinsicHeight: 30}}
	tree := layout.BuildWithScrollAndImages(document, computed, images, 500, 600, 0, 0)
	assertRatioRect(t, "block", tree.Bounds[block.ID], 160, 80)
	assertRatioRect(t, "flex item", tree.Bounds[flexItem.ID], 80, 40)
	assertRatioRect(t, "grid item", tree.Bounds[gridItem.ID], 90, 30)
	assertRatioRect(t, "image", tree.Bounds[image.ID], 150, 50)
	assertRatioRect(t, "iframe", tree.Bounds[frame.ID], 120, 60)
	assertRatioRect(t, "input", tree.Bounds[input.ID], 100, 25)
	contentRect, borderRect := tree.Bounds[contentBox.ID], tree.Bounds[borderBox.ID]
	if contentRect.Height != (contentRect.Width-24)/2+24 {
		t.Fatalf("content-box percentage ratio = %#v", contentRect)
	}
	if borderRect.Height != borderRect.Width/2 || contentRect.Width != borderRect.Width+24 {
		t.Fatalf("box-sizing percentage ratios = content:%#v border:%#v", contentRect, borderRect)
	}
}

func assertRatioRect(t *testing.T, name string, rectangle layout.Rect, width, height float32) {
	t.Helper()
	if rectangle.Width != width || rectangle.Height != height {
		t.Fatalf("%s bounds = %#v, want %vx%v", name, rectangle, width, height)
	}
}
