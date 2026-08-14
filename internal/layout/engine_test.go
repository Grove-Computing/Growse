package layout

import (
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/css"
	"github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/style"
)

func TestBuildCreatesVisibleVerticalBoxes(t *testing.T) {
	document := dom.NewDocument()
	html := document.CreateElement("html", nil)
	head := document.CreateElement("head", nil)
	title := document.CreateElement("title", nil)
	body := document.CreateElement("body", nil)
	h1 := document.CreateElement("h1", nil)
	p := document.CreateElement("p", nil)
	appendNodes(t, document,
		[2]*dom.Node{document.Root, html},
		[2]*dom.Node{html, head},
		[2]*dom.Node{head, title},
		[2]*dom.Node{title, document.CreateText("Hidden title")},
		[2]*dom.Node{html, body},
		[2]*dom.Node{body, h1},
		[2]*dom.Node{h1, document.CreateText("Hello")},
		[2]*dom.Node{body, p},
		[2]*dom.Node{p, document.CreateText("World")},
	)

	tree := Build(document, nil, 800)
	if got, want := len(tree.Boxes), 2; got != want {
		t.Fatalf("box count = %d, want %d", got, want)
	}
	if tree.Boxes[0].Text != "Hello" || tree.Boxes[0].Tag != "h1" || !tree.Boxes[0].Bold {
		t.Fatalf("first box = %#v, want bold h1 Hello", tree.Boxes[0])
	}
	if tree.Boxes[1].Text != "World" || tree.Boxes[1].Y <= tree.Boxes[0].Y {
		t.Fatalf("second box = %#v, want World below first box", tree.Boxes[1])
	}
}

func TestBuildCreatesMultilineTextareaFromTextContent(t *testing.T) {
	document := dom.NewDocument()
	textarea := document.CreateElement("textarea", nil)
	appendNodes(t, document,
		[2]*dom.Node{document.Root, textarea},
		[2]*dom.Node{textarea, document.CreateText("first line\nsecond line")},
	)

	tree := Build(document, style.Compute(document, nil), 800)
	if len(tree.Boxes) != 1 {
		t.Fatalf("textarea box count = %d, want 1", len(tree.Boxes))
	}
	box := tree.Boxes[0]
	if !box.Input || !box.Multiline || box.Text != "first line\nsecond line" || box.Height != textareaHeight {
		t.Fatalf("textarea layout = %#v", box)
	}
}

func TestBuildWrapsLongText(t *testing.T) {
	document := dom.NewDocument()
	p := document.CreateElement("p", nil)
	appendNodes(t, document,
		[2]*dom.Node{document.Root, p},
		[2]*dom.Node{p, document.CreateText("one two three four five six seven eight nine ten")},
	)

	tree := Build(document, nil, 160)
	if len(tree.Boxes) < 2 {
		t.Fatalf("box count = %d, want wrapped text", len(tree.Boxes))
	}
}

func TestBuildUsesComputedTextStyle(t *testing.T) {
	document := dom.NewDocument()
	p := document.CreateElement("p", map[string]string{"class": "notice"})
	appendNodes(t, document,
		[2]*dom.Node{document.Root, p},
		[2]*dom.Node{p, document.CreateText("Styled")},
	)
	stylesheet, err := css.Parse(strings.NewReader(`.notice { color: #abcdef; font-size: 24px; font-weight: bold }`))
	if err != nil {
		t.Fatal(err)
	}

	tree := Build(document, style.Compute(document, stylesheet), 800)
	if len(tree.Boxes) != 1 {
		t.Fatalf("box count = %d, want 1", len(tree.Boxes))
	}
	box := tree.Boxes[0]
	if box.Color != 0xabcdefff || box.FontSize != 24 || !box.Bold {
		t.Fatalf("box style = %#v, want CSS color, size and weight", box)
	}
}

func TestBuildAppliesBoxModelAndDisplay(t *testing.T) {
	document := dom.NewDocument()
	visible := document.CreateElement("p", map[string]string{"class": "visible"})
	hidden := document.CreateElement("p", map[string]string{"class": "hidden"})
	link := document.CreateElement("a", nil)
	appendNodes(t, document,
		[2]*dom.Node{document.Root, visible},
		[2]*dom.Node{visible, document.CreateText("Hello ")},
		[2]*dom.Node{visible, link},
		[2]*dom.Node{link, document.CreateText("world")},
		[2]*dom.Node{visible, document.CreateText("!")},
		[2]*dom.Node{document.Root, hidden},
		[2]*dom.Node{hidden, document.CreateText("Secret")},
	)
	stylesheet, err := css.Parse(strings.NewReader(`
.visible { margin: 10px 30px 14px 20px; padding: 4px 12px 6px 8px; }
.hidden { display: none; }
`))
	if err != nil {
		t.Fatal(err)
	}

	tree := Build(document, style.Compute(document, stylesheet), 800)
	if got, want := len(tree.Boxes), 1; got != want {
		t.Fatalf("box count = %d, want %d", got, want)
	}
	box := tree.Boxes[0]
	if box.Text != "Hello world!" {
		t.Fatalf("text = %q, want inline content on one flow", box.Text)
	}
	if box.X != 60 || box.Y != 46 || box.Width != 666 {
		t.Fatalf("box geometry = (%v, %v, %v), want (60, 46, 666)", box.X, box.Y, box.Width)
	}
}

func TestBuildAppliesSizingConstraintsAndBoxSizing(t *testing.T) {
	document := dom.NewDocument()
	contentBox := document.CreateElement("div", map[string]string{"class": "content"})
	borderBox := document.CreateElement("div", map[string]string{"class": "border"})
	following := document.CreateElement("p", nil)
	appendNodes(t, document,
		[2]*dom.Node{document.Root, contentBox},
		[2]*dom.Node{contentBox, document.CreateText("content")},
		[2]*dom.Node{document.Root, borderBox},
		[2]*dom.Node{borderBox, document.CreateText("border")},
		[2]*dom.Node{document.Root, following},
		[2]*dom.Node{following, document.CreateText("after")},
	)
	stylesheet, err := css.Parse(strings.NewReader(`
.content { width: 50%; min-width: 300px; max-width: 320px; padding: 10px; height: 40px; }
.border { width: 50%; padding: 10px; border: 3px solid red; box-sizing: border-box; height: 60px; }
`))
	if err != nil {
		t.Fatal(err)
	}
	tree := BuildWithViewport(document, style.ComputeWithEnvironment(document, stylesheet, style.InteractionState{}, style.Environment{
		ViewportWidth: 800, ViewportHeight: 600, RootFontSize: 16,
	}), 800, 600)
	if got, want := tree.Boxes[0].Width, float32(320); got != want {
		t.Fatalf("content-box text width = %v, want %v", got, want)
	}
	if got, want := tree.Boxes[1].Width, float32(342); got != want {
		t.Fatalf("border-box text width = %v, want %v", got, want)
	}
	if got, want := tree.Boxes[2].Y, float32(32+60+60); got != want {
		t.Fatalf("following y = %v, want %v", got, want)
	}
}

func TestBuildCollapsesAdjacentBlockMargins(t *testing.T) {
	document := dom.NewDocument()
	body := document.CreateElement("body", nil)
	first := document.CreateElement("div", map[string]string{"class": "first"})
	second := document.CreateElement("div", map[string]string{"class": "second"})
	third := document.CreateElement("div", map[string]string{"class": "third"})
	appendNodes(t, document,
		[2]*dom.Node{document.Root, body},
		[2]*dom.Node{body, first}, [2]*dom.Node{first, document.CreateText("first")},
		[2]*dom.Node{body, second}, [2]*dom.Node{second, document.CreateText("second")},
		[2]*dom.Node{body, third}, [2]*dom.Node{third, document.CreateText("third")},
	)
	stylesheet, err := css.Parse(strings.NewReader(`
.first { margin-bottom: 20px; }
.second { margin-top: 10px; margin-bottom: 20px; }
.third { margin-top: -5px; }
`))
	if err != nil {
		t.Fatal(err)
	}
	tree := Build(document, style.Compute(document, stylesheet), 800)
	firstGap := tree.Boxes[1].Y - (tree.Boxes[0].Y + tree.Boxes[0].Height)
	secondGap := tree.Boxes[2].Y - (tree.Boxes[1].Y + tree.Boxes[1].Height)
	if firstGap != 20 || secondGap != 15 {
		t.Fatalf("collapsed gaps = (%v, %v), want (20, 15)", firstGap, secondGap)
	}
}

func TestBuildPlacesInlineBlockAsAtomicInline(t *testing.T) {
	document := dom.NewDocument()
	paragraph := document.CreateElement("p", nil)
	badge := document.CreateElement("span", map[string]string{"class": "badge"})
	appendNodes(t, document,
		[2]*dom.Node{document.Root, paragraph},
		[2]*dom.Node{paragraph, document.CreateText("before ")},
		[2]*dom.Node{paragraph, badge}, [2]*dom.Node{badge, document.CreateText("badge")},
		[2]*dom.Node{paragraph, document.CreateText(" after")},
	)
	stylesheet, err := css.Parse(strings.NewReader(`
.badge { display: inline-block; width: 100px; height: 40px; padding: 4px; border: 2px solid red; }
`))
	if err != nil {
		t.Fatal(err)
	}
	tree := Build(document, style.Compute(document, stylesheet), 800)
	if got, want := len(tree.Boxes), 1; got != want {
		t.Fatalf("line count = %d, want %d", got, want)
	}
	line := tree.Boxes[0]
	if len(line.Runs) != 3 || line.Runs[1].NodeID != badge.ID || line.Runs[1].Width != 112 {
		t.Fatalf("inline-block runs = %#v", line.Runs)
	}
	if line.Height != 52 {
		t.Fatalf("line height = %v, want 52", line.Height)
	}
}

func TestBuildUsesFontMetricsLineHeightBaselineAndWhiteSpace(t *testing.T) {
	document := dom.NewDocument()
	pre := document.CreateElement("p", map[string]string{"class": "pre"})
	normal := document.CreateElement("p", map[string]string{"class": "normal"})
	wide := document.CreateElement("span", nil)
	narrow := document.CreateElement("span", nil)
	appendNodes(t, document,
		[2]*dom.Node{document.Root, pre}, [2]*dom.Node{pre, document.CreateText("a  b\nc")},
		[2]*dom.Node{document.Root, normal}, [2]*dom.Node{normal, wide},
		[2]*dom.Node{wide, document.CreateText("WWW")}, [2]*dom.Node{normal, narrow},
		[2]*dom.Node{narrow, document.CreateText("iii")},
	)
	stylesheet, err := css.Parse(strings.NewReader(`.pre { font-size: 20px; line-height: 40px; white-space: pre; }`))
	if err != nil {
		t.Fatal(err)
	}
	tree := Build(document, style.Compute(document, stylesheet), 800)
	if tree.Boxes[0].Text != "a  b" || tree.Boxes[1].Text != "c" || tree.Boxes[0].Height != 40 {
		t.Fatalf("preformatted lines = %#v", tree.Boxes[:2])
	}
	if tree.Boxes[0].Baseline <= tree.Boxes[0].Y || tree.Boxes[0].Baseline >= tree.Boxes[0].Y+tree.Boxes[0].Height {
		t.Fatalf("baseline = %v outside line %#v", tree.Boxes[0].Baseline, tree.Boxes[0])
	}
	normalLine := tree.Boxes[2]
	if len(normalLine.Runs) != 2 || normalLine.Runs[0].Width <= normalLine.Runs[1].Width {
		t.Fatalf("measured proportional runs = %#v", normalLine.Runs)
	}
}

func TestBuildCarriesOverflowClipAndScrollExtentIntoHitTesting(t *testing.T) {
	document := dom.NewDocument()
	container := document.CreateElement("div", map[string]string{"class": "clip"})
	first := document.CreateElement("p", nil)
	second := document.CreateElement("p", nil)
	appendNodes(t, document,
		[2]*dom.Node{document.Root, container},
		[2]*dom.Node{container, first}, [2]*dom.Node{first, document.CreateText(strings.Repeat("long line ", 20))},
		[2]*dom.Node{container, second}, [2]*dom.Node{second, document.CreateText("outside")},
	)
	stylesheet, err := css.Parse(strings.NewReader(`.clip { width: 80px; height: 20px; overflow: hidden; white-space: nowrap; }`))
	if err != nil {
		t.Fatal(err)
	}
	tree := Build(document, style.Compute(document, stylesheet), 800)
	if len(tree.Boxes) != 2 || tree.Boxes[0].Clip == nil || tree.Boxes[1].Clip == nil {
		t.Fatalf("clipped boxes = %#v", tree.Boxes)
	}
	if tree.ScrollWidth <= tree.Width {
		t.Fatalf("scroll width = %v, want greater than viewport %v", tree.ScrollWidth, tree.Width)
	}
	outside := tree.Boxes[1]
	if got, ok := HitTest(tree, outside.X+1, outside.Y+1); ok || got != 0 {
		t.Fatalf("hit outside clip = (%d, %v), want miss", got, ok)
	}
}

func TestBuildCreatesElementBackgroundBeforeItsContent(t *testing.T) {
	document := dom.NewDocument()
	box := document.CreateElement("div", map[string]string{"class": "card"})
	appendNodes(t, document,
		[2]*dom.Node{document.Root, box},
		[2]*dom.Node{box, document.CreateText("card")},
	)
	stylesheet, err := css.Parse(strings.NewReader(`.card { width: 120px; padding: 8px; background-color: #123456; }`))
	if err != nil {
		t.Fatal(err)
	}
	tree := Build(document, style.Compute(document, stylesheet), 800)
	if len(tree.Decorations) != 1 || len(tree.Boxes) != 1 {
		t.Fatalf("decorations/boxes = %d/%d", len(tree.Decorations), len(tree.Boxes))
	}
	decoration := tree.Decorations[0]
	if decoration.NodeID != box.ID || decoration.Background != 0x123456ff || decoration.Width != 136 || decoration.Height <= tree.Boxes[0].Height {
		t.Fatalf("background decoration = %#v", decoration)
	}
	if decoration.Order >= tree.Boxes[0].Order {
		t.Fatalf("background order = %d, content order = %d", decoration.Order, tree.Boxes[0].Order)
	}
	if got, ok := HitTest(tree, decoration.X+2, decoration.Y+2); !ok || got != box.ID {
		t.Fatalf("background hit = (%d, %v), want %d", got, ok, box.ID)
	}
}

func TestBuildCarriesLinearGradientIntoDecoration(t *testing.T) {
	document := dom.NewDocument()
	box := document.CreateElement("div", nil)
	appendNodes(t, document, [2]*dom.Node{document.Root, box})
	stylesheet, err := css.Parse(strings.NewReader(`div { width: 100px; height: 30px; background-image: linear-gradient(90deg, red, blue); }`))
	if err != nil {
		t.Fatal(err)
	}
	tree := Build(document, style.Compute(document, stylesheet), 800)
	if len(tree.Decorations) != 1 || tree.Decorations[0].Image.Kind != style.BackgroundImageLinearGradient || len(tree.Decorations[0].Image.GradientStops) != 2 {
		t.Fatalf("gradient decoration = %#v", tree.Decorations)
	}
}

func TestBuildCarriesBorderRadiusDecorationAndEffectiveOpacity(t *testing.T) {
	document := dom.NewDocument()
	box := document.CreateElement("div", nil)
	span := document.CreateElement("span", nil)
	appendNodes(t, document,
		[2]*dom.Node{document.Root, box}, [2]*dom.Node{box, span},
		[2]*dom.Node{span, document.CreateText("decorated")},
	)
	stylesheet, err := css.Parse(strings.NewReader(`
div { width: 100px; height: 40px; border: 4px solid red; border-radius: 60px; opacity: .5; }
span { text-decoration-line: underline; text-decoration-color: blue; opacity: .5; }
`))
	if err != nil {
		t.Fatal(err)
	}
	tree := Build(document, style.Compute(document, stylesheet), 800)
	if len(tree.Decorations) != 1 || len(tree.Boxes) == 0 {
		t.Fatalf("decorations/boxes = %#v / %#v", tree.Decorations, tree.Boxes)
	}
	decoration := tree.Decorations[0]
	if decoration.Border.Top.Width != 4 || decoration.Border.Top.Color != 0xff0000ff || decoration.Opacity != .5 || decoration.Radius.TopLeft.X != 24 || decoration.Radius.TopLeft.Y != 24 {
		t.Fatalf("decoration = %#v", decoration)
	}
	run := tree.Boxes[0].Runs[0]
	if run.Decoration != style.TextDecorationUnderline || run.DecorationColor != 0x0000ffff || run.Opacity != .25 {
		t.Fatalf("text effect run = %#v", run)
	}
}

func TestHitTestUsesPaintedEllipticalBorderRadius(t *testing.T) {
	tree := &Tree{Decorations: []Decoration{{
		NodeID: 9, Rect: Rect{X: 10, Y: 20, Width: 100, Height: 50},
		Radius: BorderRadii{
			TopLeft: CornerRadius{X: 20, Y: 10}, TopRight: CornerRadius{X: 20, Y: 10},
			BottomRight: CornerRadius{X: 20, Y: 10}, BottomLeft: CornerRadius{X: 20, Y: 10},
		},
	}}}
	if nodeID, ok := HitTest(tree, 11, 21); ok || nodeID != 0 {
		t.Fatalf("rounded corner hit = (%d, %v), want miss", nodeID, ok)
	}
	if nodeID, ok := HitTest(tree, 30, 21); !ok || nodeID != 9 {
		t.Fatalf("rounded top hit = (%d, %v), want node 9", nodeID, ok)
	}
}

func TestBuildPreservesInlineRunStyles(t *testing.T) {
	document := dom.NewDocument()
	p := document.CreateElement("p", nil)
	span := document.CreateElement("span", map[string]string{"class": "accent"})
	appendNodes(t, document,
		[2]*dom.Node{document.Root, p},
		[2]*dom.Node{p, document.CreateText("Hello ")},
		[2]*dom.Node{p, span},
		[2]*dom.Node{span, document.CreateText("Growse")},
		[2]*dom.Node{p, document.CreateText("!")},
	)
	stylesheet, err := css.Parse(strings.NewReader(`.accent { color: red; font-size: 24px; font-weight: bold; }`))
	if err != nil {
		t.Fatal(err)
	}

	tree := Build(document, style.Compute(document, stylesheet), 800)
	if got, want := len(tree.Boxes), 1; got != want {
		t.Fatalf("line count = %d, want %d", got, want)
	}
	line := tree.Boxes[0]
	if line.Text != "Hello Growse!" || len(line.Runs) != 3 {
		t.Fatalf("line = %#v, want three styled runs", line)
	}
	accent := line.Runs[1]
	if accent.Text != "Growse" || accent.Color != 0xff0000ff || accent.FontSize != 24 || !accent.Bold {
		t.Fatalf("accent run = %#v, want styled Growse", accent)
	}
	_, wantHeight, _ := measureText("Mg", 24, true)
	if line.Height != wantHeight {
		t.Fatalf("line height = %v, want measured %v", line.Height, wantHeight)
	}
}

func TestBuildIncludesGeneratedContentInLayoutAndHitTesting(t *testing.T) {
	document := dom.NewDocument()
	paragraph := document.CreateElement("p", nil)
	appendNodes(t, document,
		[2]*dom.Node{document.Root, paragraph},
		[2]*dom.Node{paragraph, document.CreateText("middle")},
	)
	stylesheet, err := css.Parse(strings.NewReader(`
p::before { content: "before "; }
p::after { content: " after"; }
`))
	if err != nil {
		t.Fatal(err)
	}
	tree := Build(document, style.Compute(document, stylesheet), 800)
	if got, want := len(tree.Boxes), 1; got != want {
		t.Fatalf("box count = %d, want %d", got, want)
	}
	box := tree.Boxes[0]
	if got, want := box.Text, "before middle after"; got != want {
		t.Fatalf("generated text = %q, want %q", got, want)
	}
	if got, want := len(box.Runs), 3; got != want {
		t.Fatalf("run count = %d, want %d", got, want)
	}
	if box.Runs[0].Tag != "::before" || box.Runs[2].Tag != "::after" {
		t.Fatalf("generated runs = %#v", box.Runs)
	}
	if got, ok := HitTest(tree, box.X+1, box.Y+1); !ok || got != paragraph.ID {
		t.Fatalf("generated content hit = (%d, %v), want paragraph %d", got, ok, paragraph.ID)
	}
}

func TestBuildCreatesTextInputBox(t *testing.T) {
	document := dom.NewDocument()
	input := document.CreateElement("input", map[string]string{"type": "text", "value": "hello"})
	appendNodes(t, document, [2]*dom.Node{document.Root, input})

	tree := Build(document, style.Compute(document, nil), 800)
	if got, want := len(tree.Boxes), 1; got != want {
		t.Fatalf("box count = %d, want %d", got, want)
	}
	box := tree.Boxes[0]
	if !box.Input || box.NodeID != input.ID || box.Text != "hello" {
		t.Fatalf("input box = %#v, want text input value", box)
	}
	if box.Width != inputWidth || box.Height != inputHeight {
		t.Fatalf("input size = (%v, %v), want (%v, %v)", box.Width, box.Height, inputWidth, inputHeight)
	}
}

func TestBuildIgnoresUnsupportedInputType(t *testing.T) {
	document := dom.NewDocument()
	input := document.CreateElement("input", map[string]string{"type": "checkbox"})
	appendNodes(t, document, [2]*dom.Node{document.Root, input})

	if boxes := Build(document, style.Compute(document, nil), 800).Boxes; len(boxes) != 0 {
		t.Fatalf("boxes = %#v, want unsupported input omitted", boxes)
	}
}

func TestBuildResolvesRelativeAbsoluteFixedAndStickyPositioning(t *testing.T) {
	document := dom.NewDocument()
	parent := document.CreateElement("div", map[string]string{"class": "parent"})
	normal := document.CreateElement("div", map[string]string{"class": "normal"})
	relative := document.CreateElement("div", map[string]string{"class": "relative"})
	sticky := document.CreateElement("div", map[string]string{"class": "sticky"})
	following := document.CreateElement("div", map[string]string{"class": "following"})
	absolute := document.CreateElement("div", map[string]string{"class": "absolute"})
	fixed := document.CreateElement("div", map[string]string{"class": "fixed"})
	appendNodes(t, document,
		[2]*dom.Node{document.Root, parent}, [2]*dom.Node{parent, normal}, [2]*dom.Node{parent, relative},
		[2]*dom.Node{parent, sticky}, [2]*dom.Node{parent, following}, [2]*dom.Node{parent, absolute}, [2]*dom.Node{parent, fixed},
	)
	stylesheet, err := css.Parse(strings.NewReader(`
.parent { position:relative; width:300px; height:200px; background-color:#eee }
.normal, .relative, .sticky, .following { height:20px; background-color:#ddd }
.relative { position:relative; left:15px; top:10px }
.sticky { position:sticky; top:100px }
.absolute { position:absolute; left:20px; top:30px; width:50px; height:40px; background-color:#ccc }
.fixed { position:fixed; right:10px; bottom:20px; width:60px; height:30px; background-color:#bbb }
`))
	if err != nil {
		t.Fatal(err)
	}
	tree := BuildWithViewport(document, style.Compute(document, stylesheet), 400, 300)
	parentRect := decorationForNode(t, tree, parent.ID)
	normalRect, relativeRect := decorationForNode(t, tree, normal.ID), decorationForNode(t, tree, relative.ID)
	stickyRect, followingRect := decorationForNode(t, tree, sticky.ID), decorationForNode(t, tree, following.ID)
	absoluteRect, fixedRect := decorationForNode(t, tree, absolute.ID), decorationForNode(t, tree, fixed.ID)
	if relativeRect.X != normalRect.X+15 || relativeRect.Y != normalRect.Y+30 {
		t.Fatalf("relative position = normal %#v relative %#v", normalRect.Rect, relativeRect.Rect)
	}
	if followingRect.Y != normalRect.Y+60 || stickyRect.Y != 100 {
		t.Fatalf("sticky flow geometry = sticky %#v following %#v", stickyRect.Rect, followingRect.Rect)
	}
	if absoluteRect.X != parentRect.X+20 || absoluteRect.Y != parentRect.Y+30 || absoluteRect.Width != 50 || absoluteRect.Height != 40 {
		t.Fatalf("absolute containing block = parent %#v child %#v", parentRect.Rect, absoluteRect.Rect)
	}
	if fixedRect.X != 330 || fixedRect.Y != 250 || fixedRect.Width != 60 || fixedRect.Height != 30 {
		t.Fatalf("fixed viewport geometry = %#v", fixedRect.Rect)
	}
}

func TestBuildCreatesDeterministicStackingContextsAndPaintOrder(t *testing.T) {
	document := dom.NewDocument()
	parent := document.CreateElement("div", map[string]string{"class": "parent"})
	high := document.CreateElement("div", map[string]string{"class": "high"})
	low := document.CreateElement("div", map[string]string{"class": "low"})
	transparent := document.CreateElement("div", map[string]string{"class": "transparent"})
	appendNodes(t, document, [2]*dom.Node{document.Root, parent}, [2]*dom.Node{parent, high}, [2]*dom.Node{parent, low}, [2]*dom.Node{parent, transparent})
	stylesheet, err := css.Parse(strings.NewReader(`
.parent { position:relative; width:100px; height:100px }
.high, .low { position:absolute; inset:0; width:50px; height:50px; background-color:#ddd }
.high { z-index:2 }
.low { z-index:1 }
.transparent { opacity:.5; width:10px; height:10px; background-color:#ccc }
`))
	if err != nil {
		t.Fatal(err)
	}
	tree := Build(document, style.Compute(document, stylesheet), 300)
	if len(tree.StackingContexts) != 4 {
		t.Fatalf("stacking contexts = %#v", tree.StackingContexts)
	}
	highRect := decorationForNode(t, tree, high.ID)
	if hit, ok := HitTest(tree, highRect.X+1, highRect.Y+1); !ok || hit != high.ID {
		t.Fatalf("z-index hit = (%d, %v), want high node %d", hit, ok, high.ID)
	}
	entries := tree.OrderedPaintEntries()
	lastDecoration := dom.NodeID(0)
	for _, entry := range entries {
		if entry.DecorationIndex >= 0 {
			lastDecoration = tree.Decorations[entry.DecorationIndex].NodeID
		}
	}
	if lastDecoration != high.ID {
		t.Fatalf("last painted decoration = %d, want high node %d", lastDecoration, high.ID)
	}
}

func TestBuildAppliesTransformOriginToDisplayGeometry(t *testing.T) {
	document := dom.NewDocument()
	item := document.CreateElement("div", map[string]string{"class": "item"})
	appendNodes(t, document, [2]*dom.Node{document.Root, item})
	stylesheet, err := css.Parse(strings.NewReader(`.item { width:100px; height:50px; background-color:#ddd; transform:rotate(180deg); transform-origin:center }`))
	if err != nil {
		t.Fatal(err)
	}
	tree := Build(document, style.Compute(document, stylesheet), 300)
	decoration := decorationForNode(t, tree, item.ID)
	want := style.Matrix{A: -1, D: -1, E: 164, F: 114}
	for _, values := range [][2]float32{{decoration.Transform.A, want.A}, {decoration.Transform.B, want.B}, {decoration.Transform.C, want.C}, {decoration.Transform.D, want.D}, {decoration.Transform.E, want.E}, {decoration.Transform.F, want.F}} {
		if difference := values[0] - values[1]; difference < -0.001 || difference > 0.001 {
			t.Fatalf("transform matrix = %#v, want %#v", decoration.Transform, want)
		}
	}
	if len(tree.StackingContexts) != 2 {
		t.Fatalf("transform stacking context = %#v", tree.StackingContexts)
	}
}

func TestBuildCarriesNestedRoundedClipsAndOpacityLayer(t *testing.T) {
	document := dom.NewDocument()
	outer := document.CreateElement("div", map[string]string{"class": "outer"})
	inner := document.CreateElement("div", map[string]string{"class": "inner"})
	group := document.CreateElement("div", map[string]string{"class": "group"})
	child := document.CreateElement("div", map[string]string{"class": "child"})
	appendNodes(t, document, [2]*dom.Node{document.Root, outer}, [2]*dom.Node{outer, inner}, [2]*dom.Node{inner, group}, [2]*dom.Node{group, child})
	stylesheet, err := css.Parse(strings.NewReader(`
.outer { width:100px; height:100px; overflow:hidden; border-radius:20px; background-color:#eee }
.inner { width:80px; height:80px; overflow:hidden; border-radius:10px; background-color:#ddd }
.group { opacity:.5 }
.child { width:120px; height:40px; background-color:#ccc }
`))
	if err != nil {
		t.Fatal(err)
	}
	tree := Build(document, style.Compute(document, stylesheet), 300)
	childDecoration := decorationForNode(t, tree, child.ID)
	if len(childDecoration.Clips) != 2 || childDecoration.Clips[0].Radius.TopLeft.X != 20 || childDecoration.Clips[1].Radius.TopLeft.X != 10 {
		t.Fatalf("nested rounded clips = %#v", childDecoration.Clips)
	}
	foundLayer := false
	for _, context := range tree.StackingContexts {
		if context.NodeID == group.ID && context.Offscreen && context.Opacity == .5 {
			foundLayer = true
		}
	}
	if !foundLayer {
		t.Fatalf("opacity compositing contexts = %#v", tree.StackingContexts)
	}
}

func TestBuildWithScrollKeepsFixedStickyGridAndTransformHitGeometryAligned(t *testing.T) {
	document := dom.NewDocument()
	spacer := document.CreateElement("div", map[string]string{"class": "spacer"})
	sticky := document.CreateElement("div", map[string]string{"class": "sticky"})
	grid := document.CreateElement("div", map[string]string{"class": "grid"})
	gridItem := document.CreateElement("div", map[string]string{"class": "grid-item"})
	host := document.CreateElement("div", map[string]string{"class": "host"})
	fixed := document.CreateElement("div", map[string]string{"class": "fixed"})
	appendNodes(t, document, [2]*dom.Node{document.Root, spacer}, [2]*dom.Node{document.Root, sticky}, [2]*dom.Node{document.Root, grid}, [2]*dom.Node{grid, gridItem}, [2]*dom.Node{document.Root, host}, [2]*dom.Node{host, fixed})
	stylesheet, err := css.Parse(strings.NewReader(`
.spacer { height:200px }
.sticky { position:sticky; top:10px; height:20px; background-color:#ddd }
.grid { display:grid; width:100px; grid-template-columns:100px; grid-template-rows:50px; transform:translateX(40px) }
.grid-item { background-color:#ccc }
.host { position:relative; height:100px }
.fixed { position:fixed; left:10px; top:20px; width:60px; height:30px; background-color:#bbb }
`))
	if err != nil {
		t.Fatal(err)
	}
	computed := style.Compute(document, stylesheet)
	tree := BuildWithScroll(document, computed, 400, 300, 0, 300)
	stickyRect, fixedRect := decorationForNode(t, tree, sticky.ID), decorationForNode(t, tree, fixed.ID)
	gridRect := decorationForNode(t, tree, gridItem.ID)
	if stickyRect.Y != 310 || fixedRect.X != 10 || fixedRect.Y != 320 {
		t.Fatalf("scroll positions = sticky %#v fixed %#v", stickyRect.Rect, fixedRect.Rect)
	}
	if hit, ok := HitTest(tree, stickyRect.X+1, stickyRect.Y+1); !ok || hit != sticky.ID {
		t.Fatalf("sticky scroll hit = (%d, %v)", hit, ok)
	}
	if hit, ok := HitTest(tree, fixedRect.X+1, fixedRect.Y+1); !ok || hit != fixed.ID {
		t.Fatalf("fixed scroll hit = (%d, %v)", hit, ok)
	}
	transformedX, transformedY := gridRect.Transform.TransformPoint(gridRect.X+1, gridRect.Y+1)
	if hit, ok := HitTest(tree, transformedX, transformedY); !ok || hit != gridItem.ID {
		t.Fatalf("transformed grid scroll hit = (%d, %v) at (%v,%v)", hit, ok, transformedX, transformedY)
	}
}

func appendNodes(t *testing.T, document *dom.Document, edges ...[2]*dom.Node) {
	t.Helper()
	for _, edge := range edges {
		if err := document.AppendChild(edge[0], edge[1]); err != nil {
			t.Fatal(err)
		}
	}
}
