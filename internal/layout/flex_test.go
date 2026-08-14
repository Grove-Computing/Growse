package layout

import (
	"strings"
	"testing"

	"github.com/saku0512/growse/internal/css"
	"github.com/saku0512/growse/internal/dom"
	stylemodel "github.com/saku0512/growse/internal/style"
)

func TestFlexAxisMapsDirectionAndWrap(t *testing.T) {
	tests := []struct {
		direction stylemodel.FlexDirection
		wrap      stylemodel.FlexWrap
		expected  flexAxis
	}{
		{stylemodel.FlexDirectionRow, stylemodel.FlexNoWrap, flexAxis{horizontal: true}},
		{stylemodel.FlexDirectionRowReverse, stylemodel.FlexWrapReverse, flexAxis{horizontal: true, reverse: true, crossFlip: true}},
		{stylemodel.FlexDirectionColumn, stylemodel.FlexWrapLines, flexAxis{}},
		{stylemodel.FlexDirectionColumnReverse, stylemodel.FlexNoWrap, flexAxis{reverse: true}},
	}
	for _, test := range tests {
		if actual := axisFor(test.direction, test.wrap); actual != test.expected {
			t.Fatalf("axisFor(%v, %v) = %#v, want %#v", test.direction, test.wrap, actual, test.expected)
		}
	}
}

func TestOrderFlexItemsIsStableForEqualOrder(t *testing.T) {
	items := []*flexItem{{index: 0, order: 2}, {index: 1, order: -1}, {index: 2, order: 2}, {index: 3, order: 0}}
	orderFlexItems(items)
	indices := []int{items[0].index, items[1].index, items[2].index, items[3].index}
	expected := []int{1, 3, 0, 2}
	for index := range expected {
		if indices[index] != expected[index] {
			t.Fatalf("order = %v, want %v", indices, expected)
		}
	}
}

func TestFormFlexLinesUsesHypotheticalSizeAndGap(t *testing.T) {
	items := []*flexItem{{hypothetical: 40}, {hypothetical: 50}, {hypothetical: 20}}
	lines := formFlexLines(items, 100, 10, stylemodel.FlexWrapLines)
	if len(lines) != 2 || len(lines[0].items) != 2 || len(lines[1].items) != 1 {
		t.Fatalf("lines = %#v", lines)
	}
	if nowrap := formFlexLines(items, 50, 10, stylemodel.FlexNoWrap); len(nowrap) != 1 || len(nowrap[0].items) != 3 {
		t.Fatalf("nowrap lines = %#v", nowrap)
	}
}

func TestResolveFlexibleLengthsGrowsAndAssignsRemainder(t *testing.T) {
	line := flexLine{items: []*flexItem{
		{base: 20, hypothetical: 20, maximum: -1, grow: 1, shrink: 1},
		{base: 20, hypothetical: 20, maximum: -1, grow: 2, shrink: 1},
		{base: 20, hypothetical: 20, maximum: -1, grow: 1, shrink: 1},
	}}
	resolveFlexibleLengths(&line, 101, 0)
	if line.items[0].target != 30.25 || line.items[1].target != 40.5 || line.items[2].target != 30.25 {
		t.Fatalf("grown targets = %v, %v, %v", line.items[0].target, line.items[1].target, line.items[2].target)
	}
}

func TestResolveFlexibleLengthsShrinksByScaledFactor(t *testing.T) {
	line := flexLine{items: []*flexItem{
		{base: 100, hypothetical: 100, maximum: -1, shrink: 1},
		{base: 50, hypothetical: 50, maximum: -1, shrink: 1},
	}}
	resolveFlexibleLengths(&line, 120, 0)
	if line.items[0].target != 80 || line.items[1].target != 40 {
		t.Fatalf("shrunk targets = %v, %v", line.items[0].target, line.items[1].target)
	}
}

func TestResolveFlexibleLengthsFreezesMinMaxViolations(t *testing.T) {
	line := flexLine{items: []*flexItem{
		{base: 40, hypothetical: 40, minimum: 0, maximum: 45, grow: 1, shrink: 1},
		{base: 40, hypothetical: 40, minimum: 0, maximum: -1, grow: 1, shrink: 1},
	}}
	resolveFlexibleLengths(&line, 100, 0)
	if line.items[0].target != 45 || line.items[1].target != 55 {
		t.Fatalf("constrained targets = %v, %v", line.items[0].target, line.items[1].target)
	}
}

func TestBuildFlexRowGrowsItemsAndUsesOrderModifiedVisualOrder(t *testing.T) {
	document := dom.NewDocument()
	container := document.CreateElement("div", map[string]string{"class": "container"})
	first := document.CreateElement("div", map[string]string{"class": "item first"})
	second := document.CreateElement("div", map[string]string{"class": "item second"})
	third := document.CreateElement("div", map[string]string{"class": "item third"})
	appendNodes(t, document,
		[2]*dom.Node{document.Root, container},
		[2]*dom.Node{container, first},
		[2]*dom.Node{first, document.CreateText("first")},
		[2]*dom.Node{container, second},
		[2]*dom.Node{second, document.CreateText("second")},
		[2]*dom.Node{container, third},
		[2]*dom.Node{third, document.CreateText("third")},
	)
	stylesheet, err := css.Parse(strings.NewReader(`
.container { display:flex; width:300px; }
.item { flex:1 1 0; height:30px; background-color:#ddd; }
.second { order:-1; }
`))
	if err != nil {
		t.Fatal(err)
	}
	computed := stylemodel.Compute(document, stylesheet)
	if containerStyle, _ := computed.For(container); containerStyle.Display != stylemodel.DisplayFlex {
		t.Fatalf("container display = %v", containerStyle.Display)
	}
	if itemStyle, _ := computed.For(first); itemStyle.BackgroundColor == 0 {
		t.Fatalf("item style = %#v", itemStyle)
	}
	tree := Build(document, computed, 500)
	secondBox := decorationForNode(t, tree, second.ID)
	firstBox := decorationForNode(t, tree, first.ID)
	thirdBox := decorationForNode(t, tree, third.ID)
	if secondBox.Width != 100 || firstBox.Width != 100 || thirdBox.Width != 100 {
		t.Fatalf("flex widths = %v, %v, %v", secondBox.Width, firstBox.Width, thirdBox.Width)
	}
	if !(secondBox.X < firstBox.X && firstBox.X < thirdBox.X) {
		t.Fatalf("visual order x = second %v, first %v, third %v", secondBox.X, firstBox.X, thirdBox.X)
	}
	if container.Children[0] != first || container.Children[1] != second || container.Children[2] != third {
		t.Fatal("flex order changed DOM order")
	}
}

func TestBuildFlexColumnShrinksItems(t *testing.T) {
	document := dom.NewDocument()
	container := document.CreateElement("section", map[string]string{"class": "container"})
	first := document.CreateElement("div", map[string]string{"class": "item"})
	second := document.CreateElement("div", map[string]string{"class": "item"})
	appendNodes(t, document,
		[2]*dom.Node{document.Root, container},
		[2]*dom.Node{container, first},
		[2]*dom.Node{first, document.CreateText("one")},
		[2]*dom.Node{container, second},
		[2]*dom.Node{second, document.CreateText("two")},
	)
	stylesheet, err := css.Parse(strings.NewReader(`
.container { display:flex; flex-direction:column; width:120px; height:120px; }
.item { flex:0 1 80px; background-color:#ddd; }
`))
	if err != nil {
		t.Fatal(err)
	}
	tree := Build(document, stylemodel.Compute(document, stylesheet), 500)
	firstBox := decorationForNode(t, tree, first.ID)
	secondBox := decorationForNode(t, tree, second.ID)
	if firstBox.Height != 60 || secondBox.Height != 60 || secondBox.Y != firstBox.Y+60 {
		t.Fatalf("column geometry = first %#v, second %#v", firstBox.Rect, secondBox.Rect)
	}
}

func TestBuildFlexAppliesJustifyGapAndMainAutoMargin(t *testing.T) {
	document := dom.NewDocument()
	container := document.CreateElement("div", map[string]string{"class": "container"})
	first := document.CreateElement("div", map[string]string{"class": "item"})
	second := document.CreateElement("div", map[string]string{"class": "item push"})
	appendNodes(t, document,
		[2]*dom.Node{document.Root, container},
		[2]*dom.Node{container, first},
		[2]*dom.Node{container, second},
	)
	stylesheet, err := css.Parse(strings.NewReader(`
.container { display:flex; width:300px; justify-content:center; column-gap:20px; }
.item { flex:0 0 50px; height:20px; background-color:#ddd; }
.push { margin-left:auto; }
`))
	if err != nil {
		t.Fatal(err)
	}
	tree := Build(document, stylemodel.Compute(document, stylesheet), 500)
	firstBox, secondBox := decorationForNode(t, tree, first.ID), decorationForNode(t, tree, second.ID)
	if firstBox.X != 32 || secondBox.X != 282 {
		t.Fatalf("auto margin positions = %v, %v", firstBox.X, secondBox.X)
	}
}

func TestBuildFlexWrapAlignAndGap(t *testing.T) {
	document := dom.NewDocument()
	container := document.CreateElement("div", map[string]string{"class": "container"})
	items := []*dom.Node{
		document.CreateElement("div", map[string]string{"class": "item"}),
		document.CreateElement("div", map[string]string{"class": "item"}),
		document.CreateElement("div", map[string]string{"class": "item"}),
	}
	appendNodes(t, document, [2]*dom.Node{document.Root, container})
	for _, item := range items {
		appendNodes(t, document, [2]*dom.Node{container, item})
	}
	stylesheet, err := css.Parse(strings.NewReader(`
.container { display:flex; width:120px; height:100px; flex-wrap:wrap; align-content:space-between; align-items:center; gap:10px; }
.item { flex:0 0 50px; height:20px; background-color:#ddd; }
`))
	if err != nil {
		t.Fatal(err)
	}
	tree := Build(document, stylemodel.Compute(document, stylesheet), 500)
	first := decorationForNode(t, tree, items[0].ID)
	second := decorationForNode(t, tree, items[1].ID)
	third := decorationForNode(t, tree, items[2].ID)
	if second.X != first.X+60 || third.X != first.X {
		t.Fatalf("wrapped x positions = %v, %v, %v", first.X, second.X, third.X)
	}
	if first.Y != 32 || third.Y != 112 {
		t.Fatalf("align-content positions = first %v, third %v", first.Y, third.Y)
	}
}

func TestBuildFlexAlignsCrossAutoMarginAndBaseline(t *testing.T) {
	document := dom.NewDocument()
	container := document.CreateElement("div", map[string]string{"class": "container"})
	large := document.CreateElement("div", map[string]string{"class": "large"})
	small := document.CreateElement("div", map[string]string{"class": "small"})
	automatic := document.CreateElement("div", map[string]string{"class": "automatic"})
	appendNodes(t, document,
		[2]*dom.Node{document.Root, container},
		[2]*dom.Node{container, large}, [2]*dom.Node{large, document.CreateText("Large")},
		[2]*dom.Node{container, small}, [2]*dom.Node{small, document.CreateText("small")},
		[2]*dom.Node{container, automatic},
	)
	stylesheet, err := css.Parse(strings.NewReader(`
.container { display:flex; width:300px; height:80px; align-items:baseline; }
.large { flex:0 0 80px; font-size:30px; }
.small { flex:0 0 80px; font-size:14px; }
.automatic { flex:0 0 40px; height:20px; margin-top:auto; background-color:#ddd; }
`))
	if err != nil {
		t.Fatal(err)
	}
	tree := Build(document, stylemodel.Compute(document, stylesheet), 500)
	largeText, smallText := boxForNode(t, tree, large.ID), boxForNode(t, tree, small.ID)
	if largeText.Baseline != smallText.Baseline {
		t.Fatalf("baselines = %v, %v", largeText.Baseline, smallText.Baseline)
	}
	autoBox := decorationForNode(t, tree, automatic.ID)
	if autoBox.Y != 92 {
		t.Fatalf("cross auto margin y = %v, want 92", autoBox.Y)
	}
}

func TestBuildFlexReverseAndWrapReverseKeepHitGeometry(t *testing.T) {
	document := dom.NewDocument()
	container := document.CreateElement("div", map[string]string{"class": "container"})
	items := []*dom.Node{
		document.CreateElement("div", map[string]string{"class": "item"}),
		document.CreateElement("div", map[string]string{"class": "item"}),
		document.CreateElement("div", map[string]string{"class": "item"}),
	}
	appendNodes(t, document, [2]*dom.Node{document.Root, container})
	for _, item := range items {
		appendNodes(t, document, [2]*dom.Node{container, item})
	}
	stylesheet, err := css.Parse(strings.NewReader(`
.container { display:flex; width:110px; height:60px; flex-flow:row-reverse wrap-reverse; gap:10px; }
.item { flex:0 0 50px; height:20px; background-color:#ddd; }
`))
	if err != nil {
		t.Fatal(err)
	}
	tree := Build(document, stylemodel.Compute(document, stylesheet), 500)
	first := decorationForNode(t, tree, items[0].ID)
	second := decorationForNode(t, tree, items[1].ID)
	third := decorationForNode(t, tree, items[2].ID)
	if first.X != 92 || second.X != 32 || third.X != 92 {
		t.Fatalf("row-reverse positions = %v, %v, %v", first.X, second.X, third.X)
	}
	if first.Y <= third.Y {
		t.Fatalf("wrap-reverse y = first %v, third %v", first.Y, third.Y)
	}
	if hit, ok := HitTest(tree, first.X+1, first.Y+1); !ok || hit != items[0].ID {
		t.Fatalf("reverse hit = (%d, %v), want %d", hit, ok, items[0].ID)
	}
}

func TestBuildFlexAppliesAutomaticMinimumSizeAndOverflowException(t *testing.T) {
	document := dom.NewDocument()
	container := document.CreateElement("div", map[string]string{"class": "container"})
	automatic := document.CreateElement("div", map[string]string{"class": "item automatic"})
	clipped := document.CreateElement("div", map[string]string{"class": "item clipped"})
	appendNodes(t, document,
		[2]*dom.Node{document.Root, container},
		[2]*dom.Node{container, automatic}, [2]*dom.Node{automatic, document.CreateText("unbreakable-content")},
		[2]*dom.Node{container, clipped}, [2]*dom.Node{clipped, document.CreateText("unbreakable-content")},
	)
	stylesheet, err := css.Parse(strings.NewReader(`
.container { display:flex; width:100px; }
.item { flex:1 1 auto; height:20px; background-color:#ddd; }
.clipped { overflow:hidden; }
`))
	if err != nil {
		t.Fatal(err)
	}
	tree := Build(document, stylemodel.Compute(document, stylesheet), 500)
	autoBox := decorationForNode(t, tree, automatic.ID)
	clippedBox := decorationForNode(t, tree, clipped.ID)
	if autoBox.Width <= clippedBox.Width || autoBox.Width <= 50 {
		t.Fatalf("automatic min widths = visible %v, clipped %v", autoBox.Width, clippedBox.Width)
	}
}

func TestBuildFlexUsesPercentageFallbackAndAspectRatio(t *testing.T) {
	document := dom.NewDocument()
	column := document.CreateElement("div", map[string]string{"class": "column"})
	percentage := document.CreateElement("div", map[string]string{"class": "percentage"})
	ratioContainer := document.CreateElement("div", map[string]string{"class": "ratio-container"})
	ratio := document.CreateElement("div", map[string]string{"class": "ratio"})
	appendNodes(t, document,
		[2]*dom.Node{document.Root, column}, [2]*dom.Node{column, percentage},
		[2]*dom.Node{document.Root, ratioContainer}, [2]*dom.Node{ratioContainer, ratio},
	)
	stylesheet, err := css.Parse(strings.NewReader(`
.column { display:flex; flex-direction:column; width:100px; }
.percentage { flex:0 0 50%; height:40px; background-color:#ddd; }
.ratio-container { display:flex; width:200px; }
.ratio { flex:0 1 auto; height:40px; aspect-ratio:2; background-color:#ddd; }
`))
	if err != nil {
		t.Fatal(err)
	}
	tree := Build(document, stylemodel.Compute(document, stylesheet), 500)
	percentageBox := decorationForNode(t, tree, percentage.ID)
	ratioBox := decorationForNode(t, tree, ratio.ID)
	if percentageBox.Height != 40 {
		t.Fatalf("indefinite percentage basis height = %v, want 40", percentageBox.Height)
	}
	if ratioBox.Width != 80 {
		t.Fatalf("aspect-ratio width = %v, want 80", ratioBox.Width)
	}
}

func decorationForNode(t *testing.T, tree *Tree, nodeID dom.NodeID) Decoration {
	t.Helper()
	for _, decoration := range tree.Decorations {
		if decoration.NodeID == nodeID {
			return decoration
		}
	}
	t.Fatalf("decoration for node %d was not created: %#v", nodeID, tree.Decorations)
	return Decoration{}
}

func boxForNode(t *testing.T, tree *Tree, nodeID dom.NodeID) Box {
	t.Helper()
	for _, box := range tree.Boxes {
		if box.NodeID == nodeID {
			return box
		}
	}
	t.Fatalf("box for node %d was not created: %#v", nodeID, tree.Boxes)
	return Box{}
}
