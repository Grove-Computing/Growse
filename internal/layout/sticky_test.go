package layout

import (
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/css"
	"github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/style"
)

func TestStickyAxisSupportsStartEndOpposingInsetsAndOversizedBoxes(t *testing.T) {
	t.Parallel()
	length := func(value float32) style.SizeValue {
		return style.SizeValue{Kind: style.SizeLength, Value: style.LengthPercentage{Pixels: value}}
	}
	tests := []struct {
		name                                                 string
		flow, size, scrollStart, scrollSize, cbStart, cbSize float32
		start, end                                           style.SizeValue
		want                                                 float32
	}{
		{name: "top or left", flow: 0, size: 30, scrollStart: 100, scrollSize: 120, cbStart: 0, cbSize: 400, start: length(8), want: 108},
		{name: "bottom or right", flow: 300, size: 30, scrollStart: 100, scrollSize: 120, cbStart: 0, cbSize: 400, end: length(12), want: 178},
		{name: "opposing insets", flow: 150, size: 30, scrollStart: 100, scrollSize: 120, cbStart: 0, cbSize: 400, start: length(8), end: length(12), want: 150},
		{name: "container end", flow: 0, size: 30, scrollStart: 260, scrollSize: 120, cbStart: 0, cbSize: 280, start: length(8), want: 250},
		{name: "oversized", flow: 0, size: 150, scrollStart: 0, scrollSize: 120, cbStart: 0, cbSize: 300, start: length(8), end: length(12), want: 8},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := stickyAxisPosition(test.flow, test.size, test.scrollStart, test.scrollSize, test.cbStart, test.cbSize, test.start, test.end); got != test.want {
				t.Fatalf("sticky axis position = %v, want %v", got, test.want)
			}
		})
	}
}

func TestStickyUsesBothAxesNestedScrollContainerAndContainerEnd(t *testing.T) {
	t.Parallel()
	document := dom.NewDocument()
	scroller := document.CreateElement("div", map[string]string{"class": "scroller"})
	section := document.CreateElement("div", map[string]string{"class": "section"})
	sticky := document.CreateElement("div", map[string]string{"class": "sticky"})
	filler := document.CreateElement("div", map[string]string{"class": "filler"})
	appendNodes(t, document,
		[2]*dom.Node{document.Root, scroller},
		[2]*dom.Node{scroller, section},
		[2]*dom.Node{section, sticky},
		[2]*dom.Node{scroller, filler},
	)
	stylesheet, err := css.Parse(strings.NewReader(`
.scroller { position:relative; width:200px; height:120px; overflow:auto; background:#eee }
.section { position:relative; width:300px; height:180px; background:#ddd }
.sticky { position:sticky; left:10px; right:15px; top:8px; bottom:12px; width:40px; height:30px; background:#aaa }
.filler { width:500px; height:220px; background:#ccc }
`))
	if err != nil {
		t.Fatal(err)
	}
	computed := style.Compute(document, stylesheet)
	tree := BuildWithViewport(document, computed, 640, 480)
	scrollerRect := tree.Bounds[scroller.ID]
	if got := tree.Bounds[sticky.ID]; got.X != scrollerRect.X+10 || got.Y != scrollerRect.Y+8 {
		t.Fatalf("initial sticky geometry = scroller:%#v sticky:%#v", scrollerRect, got)
	}
	if dirty := ApplyScrollContainerOffset(tree, computed, scroller.ID, 50, 60); !containsNodeID(dirty, sticky.ID) {
		t.Fatalf("nested scroll dirty nodes = %v", dirty)
	}
	if got := tree.Bounds[sticky.ID]; got.X != scrollerRect.X+10 || got.Y != scrollerRect.Y+8 {
		t.Fatalf("nested sticky geometry = %#v", got)
	}
	stickyClip := clipForOwner(t, decorationForNode(t, tree, sticky.ID).Clips, scroller.ID)
	if stickyClip.X != scrollerRect.X || stickyClip.Y != scrollerRect.Y || stickyClip.Width != 200 || stickyClip.Height != 120 {
		t.Fatalf("sticky moved its ancestor scrollport clip = %#v", stickyClip)
	}
	ApplyScrollContainerOffset(tree, computed, scroller.ID, 0, 0)
	if got := tree.Bounds[sticky.ID]; got.X != scrollerRect.X+10 || got.Y != scrollerRect.Y+8 {
		t.Fatalf("reverse nested scroll geometry = %#v", got)
	}
	ApplyScrollContainerOffset(tree, computed, scroller.ID, 290, 160)
	sectionRect := tree.Bounds[section.ID]
	wantX, wantY := sectionRect.X+sectionRect.Width-40, sectionRect.Y+sectionRect.Height-30
	if got := tree.Bounds[sticky.ID]; got.X != wantX || got.Y != wantY {
		t.Fatalf("sticky container-end geometry = section:%#v sticky:%#v want:(%v,%v)", sectionRect, got, wantX, wantY)
	}
}

func containsNodeID(values []dom.NodeID, target dom.NodeID) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
