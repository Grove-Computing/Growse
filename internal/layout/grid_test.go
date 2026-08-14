package layout

import (
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/css"
	"github.com/Grove-Computing/Growse/internal/dom"
	stylemodel "github.com/Grove-Computing/Growse/internal/style"
)

func TestBuildGridEstablishesFormattingContexts(t *testing.T) {
	document := dom.NewDocument()
	grid := document.CreateElement("div", map[string]string{"class": "grid"})
	first := document.CreateElement("span", map[string]string{"class": "item"})
	second := document.CreateElement("span", map[string]string{"class": "item"})
	paragraph := document.CreateElement("p", nil)
	inlineGrid := document.CreateElement("span", map[string]string{"class": "inline-grid"})
	inlineItem := document.CreateElement("span", map[string]string{"class": "inline-item"})
	appendNodes(t, document,
		[2]*dom.Node{document.Root, grid},
		[2]*dom.Node{grid, first}, [2]*dom.Node{first, document.CreateText("first")},
		[2]*dom.Node{grid, second}, [2]*dom.Node{second, document.CreateText("second")},
		[2]*dom.Node{document.Root, paragraph}, [2]*dom.Node{paragraph, document.CreateText("before ")},
		[2]*dom.Node{paragraph, inlineGrid}, [2]*dom.Node{inlineGrid, inlineItem},
		[2]*dom.Node{inlineItem, document.CreateText("grid")}, [2]*dom.Node{paragraph, document.CreateText(" after")},
	)
	stylesheet, err := css.Parse(strings.NewReader(`
.grid { display:grid; width:240px }
.item { display:inline; height:20px; background-color:#ddd }
.inline-grid { display:inline-grid }
.inline-item { height:20px; background-color:#ccc }
`))
	if err != nil {
		t.Fatal(err)
	}
	tree := Build(document, stylemodel.Compute(document, stylesheet), 500)
	firstRect, secondRect := decorationForNode(t, tree, first.ID), decorationForNode(t, tree, second.ID)
	if firstRect.Width != 240 || secondRect.Width != 240 || secondRect.Y <= firstRect.Y {
		t.Fatalf("grid items were not independently blockified: first=%#v second=%#v", firstRect.Rect, secondRect.Rect)
	}
	line := boxForNode(t, tree, paragraph.ID)
	inlineRect := decorationForNode(t, tree, inlineItem.ID)
	if inlineRect.X <= line.X || inlineRect.Y < line.Y || inlineRect.Y >= line.Y+line.Height {
		t.Fatalf("inline-grid did not participate atomically in inline flow: line=(%v,%v,%v,%v) item=%#v", line.X, line.Y, line.Width, line.Height, inlineRect.Rect)
	}
}

func TestBuildGridGeneratesExplicitAndImplicitTracks(t *testing.T) {
	document := dom.NewDocument()
	grid := document.CreateElement("div", map[string]string{"class": "grid"})
	items := make([]*dom.Node, 3)
	appendNodes(t, document, [2]*dom.Node{document.Root, grid})
	for index := range items {
		items[index] = document.CreateElement("div", map[string]string{"class": "item"})
		appendNodes(t, document, [2]*dom.Node{grid, items[index]}, [2]*dom.Node{items[index], document.CreateText("item")})
	}
	stylesheet, err := css.Parse(strings.NewReader(`
.grid { display:grid; width:300px; grid-template-columns:100px 200px; grid-template-rows:30px; grid-auto-rows:40px }
.item { background-color:#ddd }
`))
	if err != nil {
		t.Fatal(err)
	}
	tree := Build(document, stylemodel.Compute(document, stylesheet), 500)
	first, second, third := decorationForNode(t, tree, items[0].ID), decorationForNode(t, tree, items[1].ID), decorationForNode(t, tree, items[2].ID)
	if first.Width != 100 || first.Height != 30 || second.X != first.X+100 || second.Width != 200 || second.Height != 30 {
		t.Fatalf("explicit tracks = first %#v second %#v", first.Rect, second.Rect)
	}
	if third.X != first.X || third.Y != first.Y+30 || third.Width != 100 || third.Height != 40 {
		t.Fatalf("implicit row = %#v", third.Rect)
	}
}

func TestBuildGridSizesFixedPercentageIntrinsicAndFlexibleTracks(t *testing.T) {
	document := dom.NewDocument()
	grid := document.CreateElement("div", map[string]string{"class": "grid"})
	items := make([]*dom.Node, 4)
	appendNodes(t, document, [2]*dom.Node{document.Root, grid})
	for index := range items {
		items[index] = document.CreateElement("div", map[string]string{"class": "item"})
		appendNodes(t, document, [2]*dom.Node{grid, items[index]})
	}
	stylesheet, err := css.Parse(strings.NewReader(`
.grid { display:grid; width:400px; grid-template-columns:100px 25% 1fr 2fr; grid-template-rows:20px }
.item { background-color:#ddd }
`))
	if err != nil {
		t.Fatal(err)
	}
	tree := Build(document, stylemodel.Compute(document, stylesheet), 600)
	want := []float32{100, 100, 200.0 / 3.0, 400.0 / 3.0}
	for index, item := range items {
		actual := decorationForNode(t, tree, item.ID).Width
		if difference := actual - want[index]; difference < -0.01 || difference > 0.01 {
			t.Fatalf("track %d width = %v, want %v", index, actual, want[index])
		}
	}

	intrinsicDocument := dom.NewDocument()
	intrinsicGrid := intrinsicDocument.CreateElement("div", map[string]string{"class": "intrinsic"})
	minItem := intrinsicDocument.CreateElement("div", map[string]string{"class": "intrinsic-item"})
	maxItem := intrinsicDocument.CreateElement("div", map[string]string{"class": "intrinsic-item"})
	flexItem := intrinsicDocument.CreateElement("div", map[string]string{"class": "intrinsic-item"})
	appendNodes(t, intrinsicDocument,
		[2]*dom.Node{intrinsicDocument.Root, intrinsicGrid},
		[2]*dom.Node{intrinsicGrid, minItem}, [2]*dom.Node{minItem, intrinsicDocument.CreateText("aa bbbbb")},
		[2]*dom.Node{intrinsicGrid, maxItem}, [2]*dom.Node{maxItem, intrinsicDocument.CreateText("aa bbbbb")},
		[2]*dom.Node{intrinsicGrid, flexItem},
	)
	intrinsicStyles, err := css.Parse(strings.NewReader(`
.intrinsic { display:grid; width:400px; grid-template-columns:min-content max-content 1fr; grid-template-rows:20px }
.intrinsic-item { background-color:#ccc }
`))
	if err != nil {
		t.Fatal(err)
	}
	intrinsicTree := Build(intrinsicDocument, stylemodel.Compute(intrinsicDocument, intrinsicStyles), 600)
	minRect, maxRect, flexRect := decorationForNode(t, intrinsicTree, minItem.ID), decorationForNode(t, intrinsicTree, maxItem.ID), decorationForNode(t, intrinsicTree, flexItem.ID)
	if !(minRect.Width > 0 && maxRect.Width > minRect.Width && flexRect.Width > maxRect.Width) {
		t.Fatalf("intrinsic/flexible widths = %v, %v, %v", minRect.Width, maxRect.Width, flexRect.Width)
	}
}

func TestBuildGridResolvesMinmaxFitContentAndRepeat(t *testing.T) {
	document := dom.NewDocument()
	grid := document.CreateElement("div", map[string]string{"class": "grid"})
	items := make([]*dom.Node, 4)
	appendNodes(t, document, [2]*dom.Node{document.Root, grid})
	for index := range items {
		items[index] = document.CreateElement("div", map[string]string{"class": "item"})
		appendNodes(t, document, [2]*dom.Node{grid, items[index]})
	}
	appendNodes(t, document, [2]*dom.Node{items[3], document.CreateText("aa aa aa aa aa aa aa aa")})
	stylesheet, err := css.Parse(strings.NewReader(`
.grid { display:grid; width:400px; grid-template-columns:repeat(2, 40px) minmax(60px, 1fr) fit-content(80px); grid-template-rows:20px }
.item { background-color:#ddd }
`))
	if err != nil {
		t.Fatal(err)
	}
	tree := Build(document, stylemodel.Compute(document, stylesheet), 600)
	want := []float32{40, 40, 240, 80}
	for index, item := range items {
		actual := decorationForNode(t, tree, item.ID).Width
		if difference := actual - want[index]; difference < -0.01 || difference > 0.01 {
			t.Fatalf("track %d width = %v, want %v", index, actual, want[index])
		}
	}
}

func TestBuildGridPlacesNamedLinesSpansAndTemplateAreas(t *testing.T) {
	document := dom.NewDocument()
	grid := document.CreateElement("div", map[string]string{"class": "grid"})
	named := document.CreateElement("div", map[string]string{"class": "named"})
	spanned := document.CreateElement("div", map[string]string{"class": "spanned"})
	appendNodes(t, document, [2]*dom.Node{document.Root, grid}, [2]*dom.Node{grid, named}, [2]*dom.Node{grid, spanned})
	stylesheet, err := css.Parse(strings.NewReader(`
.grid { display:grid; width:200px; grid-template-columns:[left] 100px [middle] 100px [right]; grid-template-rows:[top] 30px [middle] 40px [bottom] }
.named { grid-column:left / right; grid-row:top / middle; background-color:#ddd }
.spanned { grid-column:1 / span 2; grid-row:2; background-color:#ccc }
`))
	if err != nil {
		t.Fatal(err)
	}
	tree := Build(document, stylemodel.Compute(document, stylesheet), 400)
	namedRect, spanRect := decorationForNode(t, tree, named.ID), decorationForNode(t, tree, spanned.ID)
	if namedRect.Width != 200 || namedRect.Height != 30 || spanRect.X != namedRect.X || spanRect.Y != namedRect.Y+30 || spanRect.Width != 200 || spanRect.Height != 40 {
		t.Fatalf("named/span placement = named %#v span %#v", namedRect.Rect, spanRect.Rect)
	}

	areaDocument := dom.NewDocument()
	areaGrid := areaDocument.CreateElement("div", map[string]string{"class": "areas"})
	hero := areaDocument.CreateElement("div", map[string]string{"class": "hero"})
	main := areaDocument.CreateElement("div", map[string]string{"class": "main"})
	appendNodes(t, areaDocument, [2]*dom.Node{areaDocument.Root, areaGrid}, [2]*dom.Node{areaGrid, hero}, [2]*dom.Node{areaGrid, main})
	areaStyles, err := css.Parse(strings.NewReader(`
.areas { display:grid; width:200px; grid-template-columns:80px 120px; grid-template-rows:20px 30px; grid-template-areas:"hero hero" "side main" }
.hero { grid-area:hero; background-color:#ddd }
.main { grid-area:main; background-color:#ccc }
`))
	if err != nil {
		t.Fatal(err)
	}
	areaTree := Build(areaDocument, stylemodel.Compute(areaDocument, areaStyles), 400)
	heroRect, mainRect := decorationForNode(t, areaTree, hero.ID), decorationForNode(t, areaTree, main.ID)
	if heroRect.Width != 200 || heroRect.Height != 20 || mainRect.X != heroRect.X+80 || mainRect.Y != heroRect.Y+20 || mainRect.Width != 120 || mainRect.Height != 30 {
		t.Fatalf("area placement = hero %#v main %#v", heroRect.Rect, mainRect.Rect)
	}
}

func TestBuildGridSparseAndDenseAutoPlacement(t *testing.T) {
	build := func(t *testing.T, dense bool) (*Tree, []*dom.Node) {
		t.Helper()
		document := dom.NewDocument()
		grid := document.CreateElement("div", map[string]string{"class": "grid"})
		items := make([]*dom.Node, 3)
		appendNodes(t, document, [2]*dom.Node{document.Root, grid})
		for index := range items {
			class := "item"
			if index < 2 {
				class += " wide"
			}
			items[index] = document.CreateElement("div", map[string]string{"class": class})
			appendNodes(t, document, [2]*dom.Node{grid, items[index]})
		}
		flow := "row"
		if dense {
			flow += " dense"
		}
		stylesheet, err := css.Parse(strings.NewReader(`
.grid { display:grid; width:300px; grid-template-columns:repeat(3, 100px); grid-auto-rows:20px; grid-auto-flow:` + flow + ` }
.item { background-color:#ddd }
.wide { grid-column:span 2 }
`))
		if err != nil {
			t.Fatal(err)
		}
		return Build(document, stylemodel.Compute(document, stylesheet), 500), items
	}
	sparseTree, sparseItems := build(t, false)
	sparseFirst, sparseSecond, sparseThird := decorationForNode(t, sparseTree, sparseItems[0].ID), decorationForNode(t, sparseTree, sparseItems[1].ID), decorationForNode(t, sparseTree, sparseItems[2].ID)
	if sparseFirst.Width != 200 || sparseSecond.Width != 200 || sparseThird.Y != sparseSecond.Y || sparseThird.X != sparseSecond.X+200 {
		t.Fatalf("sparse placement = first %#v second %#v third %#v", sparseFirst.Rect, sparseSecond.Rect, sparseThird.Rect)
	}
	denseTree, denseItems := build(t, true)
	denseFirst, denseSecond, denseThird := decorationForNode(t, denseTree, denseItems[0].ID), decorationForNode(t, denseTree, denseItems[1].ID), decorationForNode(t, denseTree, denseItems[2].ID)
	if denseThird.Y != denseFirst.Y || denseThird.X != denseFirst.X+200 || denseSecond.Y <= denseFirst.Y {
		t.Fatalf("dense placement = first %#v second %#v third %#v", denseFirst.Rect, denseSecond.Rect, denseThird.Rect)
	}
}

func TestBuildGridGapContentSelfAlignmentAndAutoMargin(t *testing.T) {
	document := dom.NewDocument()
	grid := document.CreateElement("div", map[string]string{"class": "grid"})
	first := document.CreateElement("div", map[string]string{"class": "item"})
	second := document.CreateElement("div", map[string]string{"class": "item"})
	autoMargin := document.CreateElement("div", map[string]string{"class": "item auto-margin"})
	appendNodes(t, document, [2]*dom.Node{document.Root, grid}, [2]*dom.Node{grid, first}, [2]*dom.Node{grid, second}, [2]*dom.Node{grid, autoMargin})
	stylesheet, err := css.Parse(strings.NewReader(`
.grid { display:grid; width:300px; height:120px; grid-template-columns:50px 50px; grid-template-rows:30px 30px; gap:10px; place-content:flex-end center; place-items:center; background-color:#eee }
.item { width:20px; height:10px; background-color:#ddd }
.auto-margin { margin-left:auto }
`))
	if err != nil {
		t.Fatal(err)
	}
	tree := BuildWithViewport(document, stylemodel.Compute(document, stylesheet), 500, 300)
	containerRect := decorationForNode(t, tree, grid.ID)
	firstRect, secondRect, autoRect := decorationForNode(t, tree, first.ID), decorationForNode(t, tree, second.ID), decorationForNode(t, tree, autoMargin.ID)
	if firstRect.X != containerRect.X+110 || firstRect.Y != containerRect.Y+60 {
		t.Fatalf("content/self alignment = container %#v first %#v", containerRect.Rect, firstRect.Rect)
	}
	if secondRect.X-firstRect.X != 60 || secondRect.Y != firstRect.Y {
		t.Fatalf("column gap = first %#v second %#v", firstRect.Rect, secondRect.Rect)
	}
	if autoRect.X != containerRect.X+125 || autoRect.Y != containerRect.Y+100 {
		t.Fatalf("auto margin alignment = %#v", autoRect.Rect)
	}
}

func TestBuildNestedGridGridInFlexAndFlexInGrid(t *testing.T) {
	document := dom.NewDocument()
	outerGrid := document.CreateElement("div", map[string]string{"class": "outer-grid"})
	innerGrid := document.CreateElement("div", map[string]string{"class": "inner-grid"})
	innerFirst := document.CreateElement("div", map[string]string{"class": "cell"})
	innerSecond := document.CreateElement("div", map[string]string{"class": "cell"})
	flexHost := document.CreateElement("div", map[string]string{"class": "flex-host"})
	gridInFlex := document.CreateElement("div", map[string]string{"class": "grid-in-flex"})
	gridFlexFirst := document.CreateElement("div", map[string]string{"class": "cell"})
	gridFlexSecond := document.CreateElement("div", map[string]string{"class": "cell"})
	gridHost := document.CreateElement("div", map[string]string{"class": "grid-host"})
	flexInGrid := document.CreateElement("div", map[string]string{"class": "flex-in-grid"})
	flexFirst := document.CreateElement("div", map[string]string{"class": "flex-cell"})
	flexSecond := document.CreateElement("div", map[string]string{"class": "flex-cell"})
	appendNodes(t, document,
		[2]*dom.Node{document.Root, outerGrid}, [2]*dom.Node{outerGrid, innerGrid},
		[2]*dom.Node{innerGrid, innerFirst}, [2]*dom.Node{innerGrid, innerSecond},
		[2]*dom.Node{document.Root, flexHost}, [2]*dom.Node{flexHost, gridInFlex},
		[2]*dom.Node{gridInFlex, gridFlexFirst}, [2]*dom.Node{gridInFlex, gridFlexSecond},
		[2]*dom.Node{document.Root, gridHost}, [2]*dom.Node{gridHost, flexInGrid},
		[2]*dom.Node{flexInGrid, flexFirst}, [2]*dom.Node{flexInGrid, flexSecond},
	)
	stylesheet, err := css.Parse(strings.NewReader(`
.outer-grid { display:grid; width:300px; grid-template-columns:150px 150px; grid-template-rows:60px }
.inner-grid { display:grid; grid-template-columns:50px 100px; grid-template-rows:60px }
.flex-host { display:flex; width:300px }
.grid-in-flex { display:grid; flex:0 0 200px; height:60px; grid-template-columns:80px 120px; grid-template-rows:60px }
.grid-host { display:grid; width:200px; grid-template-columns:200px; grid-template-rows:50px }
.flex-in-grid { display:flex }
.flex-cell { flex:0 0 100px; background-color:#ccc }
.cell { background-color:#ddd }
`))
	if err != nil {
		t.Fatal(err)
	}
	tree := Build(document, stylemodel.Compute(document, stylesheet), 500)
	innerFirstRect, innerSecondRect := decorationForNode(t, tree, innerFirst.ID), decorationForNode(t, tree, innerSecond.ID)
	if innerFirstRect.Width != 50 || innerSecondRect.X != innerFirstRect.X+50 || innerSecondRect.Width != 100 {
		t.Fatalf("nested grid = first %#v second %#v", innerFirstRect.Rect, innerSecondRect.Rect)
	}
	gridFlexFirstRect, gridFlexSecondRect := decorationForNode(t, tree, gridFlexFirst.ID), decorationForNode(t, tree, gridFlexSecond.ID)
	if gridFlexFirstRect.Width != 80 || gridFlexSecondRect.X != gridFlexFirstRect.X+80 || gridFlexSecondRect.Width != 120 {
		t.Fatalf("grid in flex = first %#v second %#v", gridFlexFirstRect.Rect, gridFlexSecondRect.Rect)
	}
	flexFirstRect, flexSecondRect := decorationForNode(t, tree, flexFirst.ID), decorationForNode(t, tree, flexSecond.ID)
	if flexFirstRect.Width != 100 || flexSecondRect.X != flexFirstRect.X+100 || flexSecondRect.Width != 100 {
		t.Fatalf("flex in grid = first %#v second %#v", flexFirstRect.Rect, flexSecondRect.Rect)
	}
}

func TestBuildGridReevaluatesAutoFillAndAutoFitAfterResize(t *testing.T) {
	document := dom.NewDocument()
	grid := document.CreateElement("div", map[string]string{"class": "grid"})
	items := make([]*dom.Node, 4)
	appendNodes(t, document, [2]*dom.Node{document.Root, grid})
	for index := range items {
		items[index] = document.CreateElement("div", map[string]string{"class": "item"})
		appendNodes(t, document, [2]*dom.Node{grid, items[index]})
	}
	stylesheet, err := css.Parse(strings.NewReader(`
.grid { display:grid; grid-template-columns:repeat(auto-fill, minmax(100px, 1fr)); grid-auto-rows:20px }
.item { background-color:#ddd }
`))
	if err != nil {
		t.Fatal(err)
	}
	computed := stylemodel.Compute(document, stylesheet)
	narrow, wide := Build(document, computed, 350), Build(document, computed, 564)
	narrowFirst, narrowThird := decorationForNode(t, narrow, items[0].ID), decorationForNode(t, narrow, items[2].ID)
	wideFirst, wideThird := decorationForNode(t, wide, items[0].ID), decorationForNode(t, wide, items[2].ID)
	if narrowThird.Y <= narrowFirst.Y || wideThird.Y != wideFirst.Y || narrowFirst.Width <= wideFirst.Width {
		t.Fatalf("auto-fill resize = narrow first %#v third %#v; wide first %#v third %#v", narrowFirst.Rect, narrowThird.Rect, wideFirst.Rect, wideThird.Rect)
	}

	fitDocument := dom.NewDocument()
	fitGrid := fitDocument.CreateElement("div", map[string]string{"class": "fit"})
	fitItem := fitDocument.CreateElement("div", map[string]string{"class": "item"})
	appendNodes(t, fitDocument, [2]*dom.Node{fitDocument.Root, fitGrid}, [2]*dom.Node{fitGrid, fitItem})
	fitStyles, err := css.Parse(strings.NewReader(`
.fit { display:grid; grid-template-columns:repeat(auto-fit, minmax(100px, 1fr)); grid-template-rows:20px }
.item { background-color:#ddd }
`))
	if err != nil {
		t.Fatal(err)
	}
	fitTree := Build(fitDocument, stylemodel.Compute(fitDocument, fitStyles), 564)
	if fitRect := decorationForNode(t, fitTree, fitItem.ID); fitRect.Width != 500 {
		t.Fatalf("auto-fit did not collapse empty tracks: %#v", fitRect.Rect)
	}
}
