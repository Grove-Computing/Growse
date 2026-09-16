package layout

import (
	"strings"
	"testing"

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
			if run.NodeID == badge.ID {
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

func hasFallbackReason(tree *Tree, reason string) bool {
	for _, fallback := range tree.Fallbacks {
		if fallback.Reason == reason {
			return true
		}
	}
	return false
}
