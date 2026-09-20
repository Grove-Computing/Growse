package layout

import (
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/css"
	"github.com/Grove-Computing/Growse/internal/dom"
	stylemodel "github.com/Grove-Computing/Growse/internal/style"
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

// Adapted from CSS Flexbox automatic-minimum, wrapping, baseline and
// percentage sizing assertions. The labels mirror the 1MB Club navigation.
func TestRealSiteNavigationFlexWrapDistributionBaselineAndPercentage(t *testing.T) {
	document := dom.NewDocument()
	navigation := document.CreateElement("nav", map[string]string{"class": "navigation"})
	labels := []string{"Home", "About", "Hall of Fame", "Members", "Blog", "Submit"}
	links := make([]*dom.Node, 0, len(labels))
	appendNodes(t, document, [2]*dom.Node{document.Root, navigation})
	for index, label := range labels {
		class := "link"
		if index == 1 {
			class += " tall"
		}
		if index == 2 {
			class += " wide"
		}
		link := document.CreateElement("a", map[string]string{"class": class})
		links = append(links, link)
		appendNodes(t, document, [2]*dom.Node{navigation, link}, [2]*dom.Node{link, document.CreateText(label)})
	}
	stylesheet, err := css.Parse(strings.NewReader(`
.navigation { display:flex; width:240px; flex-wrap:wrap; justify-content:center; align-items:baseline; gap:12px }
.link { flex:0 1 auto; min-width:0; font-size:16px; line-height:24px }
.tall { font-size:18px }
.wide { flex-basis:25%; font-size:18px }
`))
	if err != nil {
		t.Fatal(err)
	}
	tree := Build(document, stylemodel.Compute(document, stylesheet), 320)
	first, wide, last := tree.Bounds[links[0].ID], tree.Bounds[links[2].ID], tree.Bounds[links[5].ID]
	if wide.Width < 60 || wide.Width > 80 {
		t.Fatalf("percentage flex basis = %#v", wide)
	}
	if last.Y <= first.Y || first.X < tree.Bounds[navigation.ID].X || last.X+last.Width > tree.Bounds[navigation.ID].X+tree.Bounds[navigation.ID].Width {
		t.Fatalf("wrapped navigation geometry = first:%#v last:%#v nav:%#v", first, last, tree.Bounds[navigation.ID])
	}
	var firstBaseline, tallBaseline float32
	for _, box := range tree.Boxes {
		if box.NodeID == links[0].ID {
			firstBaseline = box.Baseline
		}
		if box.NodeID == links[1].ID {
			tallBaseline = box.Baseline
		}
	}
	if firstBaseline == 0 || tallBaseline == 0 || firstBaseline != tallBaseline {
		t.Fatalf("first-line baseline alignment = first:%v tall:%v", firstBaseline, tallBaseline)
	}
}

// Adapted from css/css-flexbox/flex-002.htm. In addition to the upstream
// negative free-space assertion, verify the auto cross-size after the used
// width makes text wrap.
func TestWPTFlexShrinkRecomputesAutoCrossSizeForWrappedText(t *testing.T) {
	document := dom.NewDocument()
	container := document.CreateElement("div", map[string]string{"class": "badge"})
	label := document.CreateElement("strong", nil)
	copy := document.CreateElement("p", nil)
	after := document.CreateElement("div", map[string]string{"class": "after"})
	appendNodes(t, document,
		[2]*dom.Node{document.Root, container},
		[2]*dom.Node{container, label}, [2]*dom.Node{label, document.CreateText("HTTP")},
		[2]*dom.Node{container, copy}, [2]*dom.Node{copy, document.CreateText("Items marked with this clickable badge support out-of-date browsers via HTTP.")},
		[2]*dom.Node{document.Root, after}, [2]*dom.Node{after, document.CreateText("URL")},
	)
	stylesheet, err := css.Parse(strings.NewReader(`
.badge { display:flex; width:300px; gap:14px; align-items:center; padding:14px 4px; }
.badge strong { flex:0 0 auto; }
.badge p { margin:0; min-width:0; font-size:16px; line-height:24px; }
.after { height:20px; }
`))
	if err != nil {
		t.Fatal(err)
	}
	tree := Build(document, stylemodel.Compute(document, stylesheet), 360)
	containerBounds, copyBounds, afterBounds := tree.Bounds[container.ID], tree.Bounds[copy.ID], tree.Bounds[after.ID]
	if copyBounds.Height < 48 || containerBounds.Height < 76 {
		t.Fatalf("wrapped flex cross sizes = container:%#v copy:%#v", containerBounds, copyBounds)
	}
	if afterBounds.Y < containerBounds.Y+containerBounds.Height {
		t.Fatalf("following block overlaps wrapped flex item: container:%#v after:%#v", containerBounds, afterBounds)
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

func TestBuildFlexAbsoluteChildUsesStaticPositionWithoutAffectingItems(t *testing.T) {
	document := dom.NewDocument()
	container := document.CreateElement("div", map[string]string{"class": "container"})
	first := document.CreateElement("div", map[string]string{"class": "item"})
	positioned := document.CreateElement("div", map[string]string{"class": "positioned"})
	second := document.CreateElement("div", map[string]string{"class": "item"})
	appendNodes(t, document,
		[2]*dom.Node{document.Root, container},
		[2]*dom.Node{container, first},
		[2]*dom.Node{container, positioned},
		[2]*dom.Node{container, second},
	)
	stylesheet, err := css.Parse(strings.NewReader(`
.container { position:relative; display:flex; width:300px; height:100px; justify-content:center; align-items:center }
.item { flex:0 0 100px; height:20px; background-color:#ddd }
.positioned { position:absolute; width:40px; height:20px; background-color:#c44 }
`))
	if err != nil {
		t.Fatal(err)
	}
	tree := Build(document, stylemodel.Compute(document, stylesheet), 500)
	containerRect, ok := tree.Bounds[container.ID]
	if !ok {
		t.Fatalf("container bounds missing: %#v", tree.Bounds)
	}
	firstRect, secondRect := decorationForNode(t, tree, first.ID), decorationForNode(t, tree, second.ID)
	positionedRect := decorationForNode(t, tree, positioned.ID)
	if firstRect.X != containerRect.X+50 || secondRect.X != firstRect.X+100 {
		t.Fatalf("absolute child affected flex items: container %#v first %#v second %#v", containerRect, firstRect.Rect, secondRect.Rect)
	}
	if positionedRect.X != containerRect.X+130 || positionedRect.Y != containerRect.Y+40 {
		t.Fatalf("absolute static position = %#v, want centered in %#v", positionedRect.Rect, containerRect)
	}
}

func TestBuildInlineFlexUsesFirstLineBaselineWithSurroundingText(t *testing.T) {
	document := dom.NewDocument()
	paragraph := document.CreateElement("p", nil)
	container := document.CreateElement("span", map[string]string{"class": "inline-flex"})
	first := document.CreateElement("span", map[string]string{"class": "item"})
	second := document.CreateElement("span", map[string]string{"class": "item"})
	appendNodes(t, document,
		[2]*dom.Node{document.Root, paragraph},
		[2]*dom.Node{paragraph, document.CreateText("before ")},
		[2]*dom.Node{paragraph, container},
		[2]*dom.Node{container, first}, [2]*dom.Node{first, document.CreateText("A")},
		[2]*dom.Node{container, second}, [2]*dom.Node{second, document.CreateText("B")},
		[2]*dom.Node{paragraph, document.CreateText(" after")},
	)
	stylesheet, err := css.Parse(strings.NewReader(`
.inline-flex { display:inline-flex; column-gap:5px; }
.item { flex:0 0 30px; height:20px; background-color:#ddd; }
`))
	if err != nil {
		t.Fatal(err)
	}
	tree := Build(document, stylemodel.Compute(document, stylesheet), 500)
	line := boxForNode(t, tree, paragraph.ID)
	firstText := boxForNode(t, tree, first.ID)
	firstDecoration, secondDecoration := decorationForNode(t, tree, first.ID), decorationForNode(t, tree, second.ID)
	if firstDecoration.Width != 30 || secondDecoration.X != firstDecoration.X+35 {
		t.Fatalf("inline-flex geometry = first %#v, second %#v", firstDecoration.Rect, secondDecoration.Rect)
	}
	if firstText.Baseline != line.Baseline {
		t.Fatalf("inline-flex baseline = %v, surrounding baseline = %v", firstText.Baseline, line.Baseline)
	}
}

func TestBuildNestedFlexAndMixedItems(t *testing.T) {
	document := dom.NewDocument()
	outer := document.CreateElement("div", map[string]string{"class": "outer"})
	inner := document.CreateElement("div", map[string]string{"class": "inner"})
	block := document.CreateElement("div", map[string]string{"class": "block"})
	input := document.CreateElement("input", map[string]string{"type": "text", "value": "edit"})
	button := document.CreateElement("button", map[string]string{"class": "button"})
	appendNodes(t, document,
		[2]*dom.Node{document.Root, outer},
		[2]*dom.Node{outer, document.CreateText("text")},
		[2]*dom.Node{outer, input},
		[2]*dom.Node{outer, button}, [2]*dom.Node{button, document.CreateText("button")},
		[2]*dom.Node{outer, inner},
		[2]*dom.Node{inner, block}, [2]*dom.Node{block, document.CreateText("nested")},
	)
	stylesheet, err := css.Parse(strings.NewReader(`
.outer { display:flex; width:500px; gap:10px; }
.outer > * { flex:0 0 80px; height:30px; }
.inner { display:flex; flex-direction:column; background-color:#eee; }
.block { background-color:#ddd; }
`))
	if err != nil {
		t.Fatal(err)
	}
	tree := Build(document, stylemodel.Compute(document, stylesheet), 700)
	inputBox, buttonBox, nestedBox := boxForNode(t, tree, input.ID), boxForNode(t, tree, button.ID), decorationForNode(t, tree, block.ID)
	if !inputBox.Input || !(inputBox.X < buttonBox.X && buttonBox.X < nestedBox.X) {
		t.Fatalf("mixed flex geometry = input %#v, button %#v, nested %#v", inputBox, buttonBox, nestedBox.Rect)
	}
	if hit, ok := HitTest(tree, inputBox.X+1, inputBox.Y+1); !ok || hit != input.ID {
		t.Fatalf("input hit = (%d, %v), want %d", hit, ok, input.ID)
	}
}

func TestBuildFlexRecomputesForViewportAndInteractionChanges(t *testing.T) {
	document := dom.NewDocument()
	container := document.CreateElement("div", map[string]string{"class": "container"})
	first := document.CreateElement("div", map[string]string{"class": "item"})
	second := document.CreateElement("div", map[string]string{"class": "item"})
	appendNodes(t, document, [2]*dom.Node{document.Root, container}, [2]*dom.Node{container, first}, [2]*dom.Node{container, second})
	stylesheet, err := css.Parse(strings.NewReader(`
.container { display:flex; }
.container:hover, .container.column { flex-direction:column; }
.item { flex:1 1 0; height:20px; background-color:#ddd; }
`))
	if err != nil {
		t.Fatal(err)
	}
	wide := Build(document, stylemodel.Compute(document, stylesheet), 500)
	narrow := Build(document, stylemodel.Compute(document, stylesheet), 300)
	if decorationForNode(t, wide, first.ID).Width <= decorationForNode(t, narrow, first.ID).Width {
		t.Fatal("viewport resize did not recompute flex width")
	}
	hovered := stylemodel.ComputeWithState(document, stylesheet, stylemodel.InteractionState{Hovered: map[dom.NodeID]bool{container.ID: true}})
	hoverTree := Build(document, hovered, 500)
	if decorationForNode(t, hoverTree, second.ID).Y <= decorationForNode(t, hoverTree, first.ID).Y {
		t.Fatal("hover did not change flex direction")
	}
	if !document.SetAttribute(container.ID, "class", "container column") {
		t.Fatal("class mutation was not applied")
	}
	mutated := Build(document, stylemodel.Compute(document, stylesheet), 500)
	if decorationForNode(t, mutated, second.ID).Y <= decorationForNode(t, mutated, first.ID).Y {
		t.Fatal("DOM mutation did not update flex layout")
	}
}

func TestBuildFlexOverflowClipScrollAndHitTestingShareGeometry(t *testing.T) {
	document := dom.NewDocument()
	container := document.CreateElement("div", map[string]string{"class": "container"})
	item := document.CreateElement("div", map[string]string{"class": "item"})
	appendNodes(t, document, [2]*dom.Node{document.Root, container}, [2]*dom.Node{container, item})
	stylesheet, err := css.Parse(strings.NewReader(`
.container { display:flex; width:100px; height:30px; overflow:hidden; }
.item { flex:0 0 200px; height:30px; background-color:#ddd; }
`))
	if err != nil {
		t.Fatal(err)
	}
	tree := Build(document, stylemodel.Compute(document, stylesheet), 160)
	itemBox := decorationForNode(t, tree, item.ID)
	if itemBox.Clip == nil || itemBox.Clip.Width != 100 || tree.ScrollWidth >= itemBox.X+itemBox.Width+pagePadding {
		t.Fatalf("overflow geometry = item %#v, scroll width %v", itemBox, tree.ScrollWidth)
	}
	if _, ok := HitTest(tree, itemBox.X+150, itemBox.Y+1); ok {
		t.Fatal("hit testing ignored flex overflow clip")
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
