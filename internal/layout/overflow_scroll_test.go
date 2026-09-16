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
