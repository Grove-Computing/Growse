package layout

import (
	"strings"
	"testing"

	"github.com/saku0512/growse/internal/css"
	"github.com/saku0512/growse/internal/dom"
	stylemodel "github.com/saku0512/growse/internal/style"
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
