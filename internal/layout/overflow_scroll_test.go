package layout

import (
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/css"
	"github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/style"
)

func TestOverflowModesShareAxisClipExtentOffsetAndHitGeometry(t *testing.T) {
	t.Parallel()
	document := dom.NewDocument()
	scroller := document.CreateElement("div", map[string]string{"class": "scroller"})
	content := document.CreateElement("div", map[string]string{"class": "content"})
	clip := document.CreateElement("div", map[string]string{"class": "clip"})
	clipContent := document.CreateElement("div", map[string]string{"class": "clip-content"})
	visible := document.CreateElement("div", map[string]string{"class": "visible"})
	visibleContent := document.CreateElement("div", map[string]string{"class": "visible-content"})
	xClip := document.CreateElement("div", map[string]string{"class": "x-clip"})
	xClipContent := document.CreateElement("div", map[string]string{"class": "x-clip-content"})
	appendNodes(t, document,
		[2]*dom.Node{document.Root, scroller}, [2]*dom.Node{scroller, content},
		[2]*dom.Node{document.Root, clip}, [2]*dom.Node{clip, clipContent},
		[2]*dom.Node{document.Root, visible}, [2]*dom.Node{visible, visibleContent},
		[2]*dom.Node{document.Root, xClip}, [2]*dom.Node{xClip, xClipContent},
	)
	stylesheet, err := css.Parse(strings.NewReader(`
.scroller, .clip, .visible, .x-clip { position:relative; width:100px; height:80px; background:#eee }
.scroller { overflow-x:auto; overflow-y:hidden }
.clip { overflow:clip }
.visible { overflow:visible }
.x-clip { overflow-x:clip; overflow-y:visible }
.content, .clip-content, .visible-content, .x-clip-content { width:240px; height:160px; background:#777 }
`))
	if err != nil {
		t.Fatal(err)
	}
	computed := style.Compute(document, stylesheet)
	tree := BuildWithViewport(document, computed, 640, 480)
	scrollerRect := tree.Bounds[scroller.ID]
	container, exists := tree.ScrollContainers[scroller.ID]
	if !exists || container.ScrollWidth != 240 || container.ScrollHeight != 160 || container.Viewport.Width != 100 || container.Viewport.Height != 80 {
		t.Fatalf("scroll container geometry = %#v exists:%v", container, exists)
	}
	if _, exists := tree.ScrollContainers[clip.ID]; exists {
		t.Fatal("overflow:clip created a scroll container")
	}
	if hit, ok := HitTest(tree, scrollerRect.X+150, scrollerRect.Y+20); ok && hit == content.ID {
		t.Fatal("overflow clip exposed off-scrollport content to hit testing")
	}
	visibleRect := tree.Bounds[visible.ID]
	if hit, ok := HitTest(tree, visibleRect.X+150, visibleRect.Y+20); !ok || hit != visibleContent.ID {
		t.Fatalf("overflow:visible hit = %d/%v", hit, ok)
	}
	xClipRect := tree.Bounds[xClip.ID]
	if hit, ok := HitTest(tree, xClipRect.X+150, xClipRect.Y+20); ok && hit == xClipContent.ID {
		t.Fatal("overflow-x:clip exposed horizontal overflow")
	}
	if hit, ok := HitTest(tree, xClipRect.X+20, xClipRect.Y+120); !ok || hit != xClipContent.ID {
		t.Fatalf("overflow-y:visible vertical hit = %d/%v", hit, ok)
	}
	dirty := ApplyScrollContainerOffset(tree, computed, scroller.ID, 500, 500)
	container = tree.ScrollContainers[scroller.ID]
	if container.Offset != (ScrollOffset{X: 140, Y: 80}) || !containsNodeID(dirty, content.ID) {
		t.Fatalf("clamped scroll offset/dirty = %#v / %v", container, dirty)
	}
	contentRect := tree.Bounds[content.ID]
	if contentRect.X != scrollerRect.X-140 || contentRect.Y != scrollerRect.Y-80 {
		t.Fatalf("scrolled content geometry = scroller:%#v content:%#v", scrollerRect, contentRect)
	}
	if hit, ok := HitTest(tree, scrollerRect.X+10, scrollerRect.Y+10); !ok || hit != content.ID {
		t.Fatalf("scrolled content hit = %d/%v", hit, ok)
	}
	if dirty := ApplyScrollContainerOffset(tree, computed, clip.ID, 20, 20); len(dirty) != 0 {
		t.Fatalf("overflow:clip accepted scroll offset: %v", dirty)
	}
}

func TestNestedScrollMovesOwnedRoundedClipsAndTransformGeometryTogether(t *testing.T) {
	t.Parallel()
	document := dom.NewDocument()
	outer := document.CreateElement("div", map[string]string{"class": "outer"})
	inner := document.CreateElement("div", map[string]string{"class": "inner"})
	content := document.CreateElement("div", map[string]string{"class": "content"})
	appendNodes(t, document,
		[2]*dom.Node{document.Root, outer},
		[2]*dom.Node{outer, inner},
		[2]*dom.Node{inner, content},
	)
	stylesheet, err := css.Parse(strings.NewReader(`
.outer { position:relative; width:180px; height:140px; overflow:hidden; border-radius:30px; transform:translateX(40px) }
.inner { position:relative; width:120px; height:90px; overflow:auto; border-radius:20px }
.content { width:240px; height:180px; background:#777 }
`))
	if err != nil {
		t.Fatal(err)
	}
	computed := style.Compute(document, stylesheet)
	tree := BuildWithViewport(document, computed, 640, 480)
	before := tree.Bounds[content.ID]
	beforeDecoration := decorationForNode(t, tree, content.ID)
	visualX, visualY := beforeDecoration.Transform.TransformPoint(before.X+60, before.Y+45)

	ApplyScrollContainerOffset(tree, computed, inner.ID, 40, 30)
	afterInner := tree.Bounds[content.ID]
	afterInnerDecoration := decorationForNode(t, tree, content.ID)
	afterX, afterY := afterInnerDecoration.Transform.TransformPoint(afterInner.X+60, afterInner.Y+45)
	if afterX != visualX-40 || afterY != visualY-30 {
		t.Fatalf("transformed inner-scroll delta = (%v,%v), want (%v,%v)", afterX, afterY, visualX-40, visualY-30)
	}
	outerClipBefore := clipForOwner(t, afterInnerDecoration.Clips, outer.ID)
	innerClipBefore := clipForOwner(t, afterInnerDecoration.Clips, inner.ID)

	ApplyScrollContainerOffset(tree, computed, outer.ID, 10, 5)
	decoration := decorationForNode(t, tree, content.ID)
	outerClipAfter := clipForOwner(t, decoration.Clips, outer.ID)
	innerClipAfter := clipForOwner(t, decoration.Clips, inner.ID)
	if outerClipAfter.Rect != outerClipBefore.Rect {
		t.Fatalf("outer scrollport moved with its own content: before=%#v after=%#v", outerClipBefore, outerClipAfter)
	}
	if innerClipAfter.X != innerClipBefore.X-10 || innerClipAfter.Y != innerClipBefore.Y-5 {
		t.Fatalf("descendant scrollport did not move with outer scroll: before=%#v after=%#v", innerClipBefore, innerClipAfter)
	}
	innerBounds := tree.Bounds[inner.ID]
	pointX, pointY := decoration.Transform.TransformPoint(innerBounds.X+60, innerBounds.Y+45)
	if hit, ok := HitTest(tree, pointX, pointY); !ok || hit != content.ID {
		t.Fatalf("transformed nested-scroll hit = %d/%v at %v,%v", hit, ok, pointX, pointY)
	}
	cornerX, cornerY := decoration.Transform.TransformPoint(innerClipAfter.X+1, innerClipAfter.Y+1)
	if hit, ok := HitTest(tree, cornerX, cornerY); ok && hit == content.ID {
		t.Fatalf("rounded nested clip exposed content corner: %d/%v", hit, ok)
	}
}

func TestRealSiteAxisOverflowKeepsRoundedTransformClipAcrossResize(t *testing.T) {
	document := dom.NewDocument()
	panel := document.CreateElement("section", map[string]string{"class": "panel"})
	content := document.CreateElement("a", map[string]string{"class": "content"})
	appendNodes(t, document, [2]*dom.Node{document.Root, panel}, [2]*dom.Node{panel, content})
	stylesheet, err := css.Parse(strings.NewReader(`
.panel { position:relative; width:50%; max-height:80px; overflow:hidden; border-radius:24px; transform:translateX(40px) }
.content { display:block; width:240px; height:160px; background:#777 }
`))
	if err != nil {
		t.Fatal(err)
	}
	computed := style.Compute(document, stylesheet)
	wide := BuildWithViewport(document, computed, 600, 400)
	narrow := BuildWithViewport(document, computed, 360, 400)
	if wide.Bounds[panel.ID].Width <= narrow.Bounds[panel.ID].Width {
		t.Fatalf("responsive overflow geometry did not resize: wide=%#v narrow=%#v", wide.Bounds[panel.ID], narrow.Bounds[panel.ID])
	}
	for name, tree := range map[string]*Tree{"wide": wide, "narrow": narrow} {
		panelRect := tree.Bounds[panel.ID]
		decoration := decorationForNode(t, tree, content.ID)
		cornerX, cornerY := decoration.Transform.TransformPoint(panelRect.X+1, panelRect.Y+1)
		if hit, ok := HitTest(tree, cornerX, cornerY); ok && hit == content.ID {
			t.Fatalf("%s rounded overflow corner exposed content: %d/%v", name, hit, ok)
		}
		centerX, centerY := decoration.Transform.TransformPoint(panelRect.X+panelRect.Width/2, panelRect.Y+40)
		if hit, ok := HitTest(tree, centerX, centerY); !ok || hit != content.ID {
			t.Fatalf("%s transformed overflow center hit = %d/%v", name, hit, ok)
		}
		belowX, belowY := decoration.Transform.TransformPoint(panelRect.X+panelRect.Width/2, panelRect.Y+120)
		if hit, ok := HitTest(tree, belowX, belowY); ok && hit == content.ID {
			t.Fatalf("%s max-height overflow exposed content below scrollport: %d/%v", name, hit, ok)
		}
	}
}

func clipForOwner(t *testing.T, clips []ClipRegion, nodeID dom.NodeID) ClipRegion {
	t.Helper()
	for _, region := range clips {
		if region.NodeID == nodeID {
			return region
		}
	}
	t.Fatalf("clip for node %d is missing in %#v", nodeID, clips)
	return ClipRegion{}
}
