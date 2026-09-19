package layout

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/Grove-Computing/Growse/internal/css"
	"github.com/Grove-Computing/Growse/internal/dom"
	stylemodel "github.com/Grove-Computing/Growse/internal/style"
)

func TestBuildCollapsesParentChildMarginsAndCentersAutoBlock(t *testing.T) {
	document := dom.NewDocument()
	body := document.CreateElement("body", nil)
	parent := document.CreateElement("section", map[string]string{"class": "parent"})
	child := document.CreateElement("div", map[string]string{"class": "child"})
	next := document.CreateElement("div", map[string]string{"class": "next"})
	centered := document.CreateElement("div", map[string]string{"class": "centered"})
	appendNodes(t, document,
		[2]*dom.Node{document.Root, body}, [2]*dom.Node{body, parent}, [2]*dom.Node{parent, child},
		[2]*dom.Node{body, next}, [2]*dom.Node{body, centered},
	)
	stylesheet, err := css.Parse(strings.NewReader(`
.parent { display:block; margin-top:10px; margin-bottom:12px }
.child { display:block; height:20px; margin-top:-4px; margin-bottom:-5px; background:#ddd }
.next { display:block; height:10px; margin-top:8px; background:#ccc }
.centered { display:block; width:200px; height:10px; margin-left:auto; margin-right:auto; background:#bbb }
`))
	if err != nil {
		t.Fatal(err)
	}
	tree := Build(document, stylemodel.Compute(document, stylesheet), 464)
	parentRect, childRect := tree.Bounds[parent.ID], tree.Bounds[child.ID]
	nextRect, centeredRect := tree.Bounds[next.ID], tree.Bounds[centered.ID]
	if parentRect.Y != 38 || childRect.Y != parentRect.Y {
		t.Fatalf("collapsed parent/child top margins = parent %#v child %#v, want y=38", parentRect, childRect)
	}
	if parentRect.Height != 20 {
		t.Fatalf("collapsed child bottom must not inflate parent: %#v", parentRect)
	}
	if gap := nextRect.Y - (parentRect.Y + parentRect.Height); gap != 7 {
		t.Fatalf("collapsed parent/child/next bottom group gap = %v, want 7; parent=%#v child=%#v next=%#v", gap, parentRect, childRect, nextRect)
	}
	if centeredRect.X != 132 || centeredRect.Width != 200 {
		t.Fatalf("auto margins did not center fixed block: %#v", centeredRect)
	}
}

// Adapted from css/CSS2/box-display/block-in-inline-001.xht.
func TestWPTInlineCustomWrapperSplitsAroundBlockContent(t *testing.T) {
	document := dom.NewDocument()
	body := document.CreateElement("body", nil)
	wrapper := document.CreateElement("react-partial", map[string]string{"class": "custom"})
	shell := document.CreateElement("div", map[string]string{"class": "shell"})
	header := document.CreateElement("header", map[string]string{"class": "header"})
	bar := document.CreateElement("div", map[string]string{"class": "bar"})
	label := document.CreateElement("span", nil)
	main := document.CreateElement("main", map[string]string{"class": "main"})
	appendNodes(t, document,
		[2]*dom.Node{document.Root, body}, [2]*dom.Node{body, wrapper},
		[2]*dom.Node{wrapper, shell}, [2]*dom.Node{shell, header},
		[2]*dom.Node{header, bar}, [2]*dom.Node{bar, label},
		[2]*dom.Node{label, document.CreateText("Navigation")}, [2]*dom.Node{body, main},
	)
	stylesheet, err := css.Parse(strings.NewReader(`
.custom { display:inline }
.shell, .header { display:block }
.header { padding:16px }
.bar { display:flex; height:100% }
.main { display:block; height:20px }
`))
	if err != nil {
		t.Fatal(err)
	}
	tree := BuildWithViewport(document, stylemodel.Compute(document, stylesheet), 800, 600)
	headerRect, barRect, mainRect := tree.Bounds[header.ID], tree.Bounds[bar.ID], tree.Bounds[main.ID]
	if headerRect.Height <= 0 || barRect.Height <= 0 {
		t.Fatalf("block content inside inline wrapper was flattened into text: header=%#v bar=%#v", headerRect, barRect)
	}
	if headerRect.Height >= 200 || mainRect.Y >= 200 {
		t.Fatalf("indefinite percentage height expanded against viewport: header=%#v main=%#v", headerRect, mainRect)
	}
	if mainRect.Y < headerRect.Y+headerRect.Height {
		t.Fatalf("following flow overlaps split block: header=%#v main=%#v", headerRect, mainRect)
	}
}

func TestFlowRootContainsAndIsolatesFloats(t *testing.T) {
	document := dom.NewDocument()
	flowRoot := document.CreateElement("section", map[string]string{"class": "flow-root"})
	floated := document.CreateElement("div", map[string]string{"class": "float"})
	following := document.CreateElement("p", map[string]string{"class": "following"})
	appendNodes(t, document,
		[2]*dom.Node{document.Root, flowRoot}, [2]*dom.Node{flowRoot, floated},
		[2]*dom.Node{document.Root, following}, [2]*dom.Node{following, document.CreateText("outside the formatting context")},
	)
	stylesheet, err := css.Parse(strings.NewReader(`
.flow-root { display:flow-root; width:240px; background:#eee }
.float { float:left; width:80px; height:90px; background:#ddd }
.following { display:block; margin:0 }
`))
	if err != nil {
		t.Fatal(err)
	}
	computed := stylemodel.Compute(document, stylesheet)
	flowStyle, _ := computed.For(flowRoot)
	if flowStyle.Display != stylemodel.DisplayFlowRoot {
		t.Fatalf("display:flow-root = %v", flowStyle.Display)
	}
	tree := Build(document, computed, 500)
	flowRect, floatRect, followingRect := tree.Bounds[flowRoot.ID], tree.Bounds[floated.ID], tree.Bounds[following.ID]
	if flowRect.Height < floatRect.Height {
		t.Fatalf("flow-root did not contain float: root=%#v float=%#v", flowRect, floatRect)
	}
	if followingRect.Y < flowRect.Y+flowRect.Height {
		t.Fatalf("inner float leaked into following flow: root=%#v following=%#v", flowRect, followingRect)
	}
}

// Adapted from CSS2 floats-142 and containing-block assertions. This mirrors
// an editorial media object with a floated replaced image and BFC body.
func TestRealSiteFloatBFCReplacedElementAndContainingBlockStayDisjoint(t *testing.T) {
	document := dom.NewDocument()
	article := document.CreateElement("article", map[string]string{"class": "article"})
	imageNode := document.CreateElement("img", map[string]string{"class": "thumb", "src": "thumb.png"})
	content := document.CreateElement("section", map[string]string{"class": "content"})
	label := document.CreateElement("span", map[string]string{"class": "label"})
	footer := document.CreateElement("footer", map[string]string{"class": "footer"})
	appendNodes(t, document,
		[2]*dom.Node{document.Root, article}, [2]*dom.Node{article, imageNode},
		[2]*dom.Node{article, content}, [2]*dom.Node{content, document.CreateText("Readable text beside the image")},
		[2]*dom.Node{content, label}, [2]*dom.Node{label, document.CreateText("new")},
		[2]*dom.Node{article, footer}, [2]*dom.Node{footer, document.CreateText("following section")},
	)
	stylesheet, err := css.Parse(strings.NewReader(`
.article { display:block; width:320px }
.thumb { float:left; width:80px; height:auto; margin-right:12px }
.content { display:block; position:relative; overflow:hidden; min-height:48px }
.label { position:absolute; right:0; top:0 }
.footer { display:block; clear:both; margin-top:10px }
`))
	if err != nil {
		t.Fatal(err)
	}
	computed := stylemodel.Compute(document, stylesheet)
	tree := BuildWithScrollAndImages(document, computed, map[dom.NodeID]ImageResource{
		imageNode.ID: {URL: "https://example.test/thumb.png", IntrinsicWidth: 160, IntrinsicHeight: 96, Loaded: true},
	}, 400, 300, 0, 0)
	imageRect, contentRect := tree.Bounds[imageNode.ID], tree.Bounds[content.ID]
	labelRect, footerRect := tree.Bounds[label.ID], tree.Bounds[footer.ID]
	if imageRect.Width != 80 || imageRect.Height != 48 {
		t.Fatalf("replaced float ratio = %#v", imageRect)
	}
	if contentRect.X < imageRect.X+imageRect.Width || contentRect.Width >= 320 {
		t.Fatalf("BFC overlaps float: image=%#v content=%#v", imageRect, contentRect)
	}
	if labelRect.X < contentRect.X || labelRect.X+labelRect.Width > contentRect.X+contentRect.Width {
		t.Fatalf("positioned child escaped BFC containing block: content=%#v label=%#v", contentRect, labelRect)
	}
	if footerRect.Y < imageRect.Y+imageRect.Height {
		t.Fatalf("clear did not pass replaced float: image=%#v footer=%#v", imageRect, footerRect)
	}
}

func TestAtomicInlineBaselineWrapAndHitGeometryAgree(t *testing.T) {
	document := dom.NewDocument()
	paragraph := document.CreateElement("p", map[string]string{"class": "line"})
	badge := document.CreateElement("span", map[string]string{"class": "badge"})
	appendNodes(t, document,
		[2]*dom.Node{document.Root, paragraph}, [2]*dom.Node{paragraph, document.CreateText("before ")},
		[2]*dom.Node{paragraph, badge}, [2]*dom.Node{badge, document.CreateText("atomic")},
		[2]*dom.Node{paragraph, document.CreateText(" after text that wraps")},
	)
	stylesheet, err := css.Parse(strings.NewReader(`
.line { display:block; width:180px; line-height:24px; margin:0 }
.badge { display:inline-block; width:70px; height:32px; vertical-align:middle; background:#ddd }
`))
	if err != nil {
		t.Fatal(err)
	}
	tree := BuildAtRevision(document, stylemodel.Compute(document, stylesheet), 300, 19)
	var badgeRun TextRun
	var line Box
	for _, candidate := range tree.Boxes {
		for _, run := range candidate.Runs {
			if run.NodeID == badge.ID && run.Atomic {
				badgeRun, line = run, candidate
			}
		}
	}
	if badgeRun.Width != 70 || line.Baseline <= line.Y || len(tree.Boxes) < 2 {
		t.Fatalf("atomic inline baseline/wrap = run %#v line %#v boxes=%d", badgeRun, line, len(tree.Boxes))
	}
	runX := line.X
	for _, run := range line.Runs {
		if run.NodeID == badge.ID {
			break
		}
		runX += run.Width
	}
	hit, ok := HitTestWithRevision(tree, runX+badgeRun.Width/2, line.Y+line.Height/2)
	if !ok || hit.NodeID != badge.ID || hit.Revision != 19 {
		t.Fatalf("atomic inline hit = %#v/%t, want node=%d revision=19", hit, ok, badge.ID)
	}
}

func TestInlineButtonAutoBorderBoxIncludesPaddingAndBorder(t *testing.T) {
	document := dom.NewDocument()
	header := document.CreateElement("header", nil)
	button := document.CreateElement("button", map[string]string{"type": "button"})
	status := document.CreateElement("strong", nil)
	appendNodes(t, document,
		[2]*dom.Node{document.Root, header}, [2]*dom.Node{header, button},
		[2]*dom.Node{button, document.CreateText("Flip flow direction")},
		[2]*dom.Node{header, status}, [2]*dom.Node{status, document.CreateText("LEFT FLOAT")},
	)
	stylesheet, err := css.Parse(strings.NewReader(`
header { display:block }
button { box-sizing:border-box; margin:8px 12px 0 0; padding:9px 13px; border:1px solid #66e3d7 }
`))
	if err != nil {
		t.Fatal(err)
	}
	tree := Build(document, stylemodel.ComputeWithEnvironment(document, stylesheet, stylemodel.InteractionState{}, stylemodel.Environment{BrowserDefaults: true}), 800)
	var buttonBox Box
	var buttonRun TextRun
	for _, box := range tree.Boxes {
		if box.NodeID == button.ID && box.Button {
			buttonBox = box
		}
		for _, run := range box.Runs {
			if run.NodeID == button.ID && run.Atomic {
				buttonRun = run
			}
		}
	}
	if buttonBox.Text != "Flip flow direction" || buttonBox.Width <= 120 || buttonBox.Height <= 30 {
		t.Fatalf("auto border-box button omitted intrinsic padding/border: %#v", buttonBox)
	}
	if buttonRun.Width <= buttonBox.Width {
		t.Fatalf("atomic button advance = %v, button width = %v; want margins included", buttonRun.Width, buttonBox.Width)
	}
}

func TestInlineReplacedImageSharesLinePaintAndHitGeometry(t *testing.T) {
	document := dom.NewDocument()
	paragraph := document.CreateElement("p", map[string]string{"class": "line"})
	imageNode := document.CreateElement("img", map[string]string{"src": "badge.png", "alt": "badge", "width": "40", "height": "24"})
	appendNodes(t, document,
		[2]*dom.Node{document.Root, paragraph}, [2]*dom.Node{paragraph, document.CreateText("before ")},
		[2]*dom.Node{paragraph, imageNode}, [2]*dom.Node{paragraph, document.CreateText(" after")},
	)
	stylesheet, err := css.Parse(strings.NewReader(`.line { display:block; margin:0; line-height:20px } img { vertical-align:middle }`))
	if err != nil {
		t.Fatal(err)
	}
	tree := BuildWithScrollAndImages(document, stylemodel.Compute(document, stylesheet), map[dom.NodeID]ImageResource{
		imageNode.ID: {URL: "https://example.test/badge.png", IntrinsicWidth: 80, IntrinsicHeight: 48, Loaded: true},
	}, 320, 200, 0, 0)
	imageRect := tree.Bounds[imageNode.ID]
	if imageRect.Width != 40 || imageRect.Height != 24 {
		t.Fatalf("inline replaced bounds = %#v", imageRect)
	}
	var line Box
	for _, box := range tree.Boxes {
		if !box.Image {
			for _, run := range box.Runs {
				if run.NodeID == imageNode.ID {
					line = box
				}
			}
		}
	}
	if line.Height < imageRect.Height || imageRect.Y < line.Y || imageRect.Y+imageRect.Height > line.Y+line.Height {
		t.Fatalf("inline image not contained by line: line=%#v image=%#v", line, imageRect)
	}
	hit, ok := HitTest(tree, imageRect.X+imageRect.Width/2, imageRect.Y+imageRect.Height/2)
	if !ok || hit != imageNode.ID {
		t.Fatalf("inline image hit = %d/%t, want %d", hit, ok, imageNode.ID)
	}
}

func TestLayoutSafetyLimitsReturnFiniteFallbacks(t *testing.T) {
	t.Run("time", func(t *testing.T) {
		expired := time.Unix(100, 0)
		tree := &Tree{}
		state := engine{tree: tree, now: func() time.Time { return expired }, deadline: expired}
		if state.withinBudget(6) {
			t.Fatal("expired layout budget was accepted")
		}
		if !hasFallbackReason(tree, "layout time limit exceeded") {
			t.Fatalf("time fallback = %#v", tree.Fallbacks)
		}
	})

	t.Run("box", func(t *testing.T) {
		tree := &Tree{Boxes: make([]Box, maxLayoutBoxes)}
		state := engine{tree: tree}
		if state.withinBudget(7) {
			t.Fatal("exhausted box budget was accepted")
		}
		if !hasFallbackReason(tree, "layout box limit exceeded") {
			t.Fatalf("box fallback = %#v", tree.Fallbacks)
		}
	})

	t.Run("recursion", func(t *testing.T) {
		document := dom.NewDocument()
		parent := document.Root
		for index := 0; index < maxLayoutDepth+8; index++ {
			child := document.CreateElement("div", map[string]string{"style": "display:block"})
			if err := document.AppendChild(parent, child); err != nil {
				t.Fatal(err)
			}
			parent = child
		}
		tree := Build(document, stylemodel.Compute(document, nil), 400)
		if !hasFallbackReason(tree, "layout recursion limit exceeded") {
			t.Fatalf("recursion fallback = %#v", tree.Fallbacks)
		}
	})

	t.Run("line and float", func(t *testing.T) {
		tree := &Tree{Boxes: make([]Box, maxLineBoxes), Bounds: make(map[dom.NodeID]Rect)}
		state := engine{tree: tree, opacity: 1}
		state.addInlineRuns(7, "p", []inlineRun{{nodeID: 7, tag: "p", text: "bounded", style: defaultStyle(), opacity: 1}}, defaultStyle(), 0, 200)
		if !hasFallbackReason(tree, "line box limit exceeded") {
			t.Fatalf("line fallback = %#v", tree.Fallbacks)
		}
		state.floats = make([]floatRegion, maxFloatBoxes)
		state.addFloat(&dom.Node{ID: 8, Type: dom.NodeElement, TagName: "div"}, blockStyle{float: stylemodel.FloatLeft}, 0, 200, 0, false)
		if !hasFallbackReason(tree, "float box limit exceeded") {
			t.Fatalf("float fallback = %#v", tree.Fallbacks)
		}
	})

	t.Run("fragment", func(t *testing.T) {
		tree := &Tree{Boxes: make([]Box, maxLayoutFragments), Decorations: []Decoration{{}}}
		assignFragmentIdentities(tree)
		if len(tree.Boxes)+len(tree.Decorations) != maxLayoutFragments || !hasFallbackReason(tree, "layout fragment limit exceeded") {
			t.Fatalf("fragment limit = boxes:%d decorations:%d fallbacks:%#v", len(tree.Boxes), len(tree.Decorations), tree.Fallbacks)
		}
	})
}

func TestMalformedCSSValuesStillProduceFiniteLayout(t *testing.T) {
	document := dom.NewDocument()
	body := document.CreateElement("body", nil)
	stress := document.CreateElement("main", map[string]string{"class": "stress"})
	appendNodes(t, document,
		[2]*dom.Node{document.Root, body},
		[2]*dom.Node{body, stress},
		[2]*dom.Node{stress, document.CreateText(strings.Repeat("bounded layout ", 512))},
	)
	stylesheet, err := css.Parse(strings.NewReader(`
.stress {
  display:grid;
  width:calc(100% / 0);
  height:calc(infinity * 1px);
  columns:999999 -4px;
  grid-template-columns:repeat(999999999, minmax(-1px, 1fr));
  gap:-999px;
  writing-mode:sideways-rl;
}`))
	if err != nil {
		t.Fatal(err)
	}
	tree := BuildWithViewport(document, stylemodel.Compute(document, stylesheet), 800, 600)
	values := []float32{tree.Width, tree.Height, tree.ScrollWidth, tree.ScrollHeight}
	for _, box := range tree.Boxes {
		values = append(values, box.X, box.Y, box.Width, box.Height)
	}
	for _, value := range values {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			t.Fatalf("non-finite layout value %v in tree %#v", value, tree)
		}
	}
}

func hasFallbackReason(tree *Tree, reason string) bool {
	for _, fallback := range tree.Fallbacks {
		if fallback.Reason == reason {
			return true
		}
	}
	return false
}
