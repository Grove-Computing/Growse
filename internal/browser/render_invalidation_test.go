package browser

import (
	"testing"

	"github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/style"
)

func TestComputedStyleMutationClassificationStaysOnAffectedNodes(t *testing.T) {
	base := style.ComputedStyle{Display: style.DisplayBlock, Width: style.SizeValue{Kind: style.SizeAuto}, Opacity: 1, Color: 0x111111ff, Cursor: style.CursorDefault}
	previous := style.Map{1: base, 2: base, 3: base, 4: base, 99: base}
	current := style.Map{1: base, 2: base, 3: base, 4: base, 99: base}
	styleOnly := current[1]
	styleOnly.Cursor = style.CursorPointer
	current[1] = styleOnly
	composite := current[2]
	composite.Opacity = 0.5
	current[2] = composite
	paint := current[3]
	paint.Color = 0xff0000ff
	current[3] = paint
	layout := current[4]
	layout.Width = style.SizeValue{Kind: style.SizeLength, Value: style.LengthPercentage{Pixels: 240}}
	current[4] = layout

	page := &Page{StyleRevision: 8}
	got := page.RecordComputedStyleChanges(previous, current)
	if got.Revision != 9 || got.Damage != RenderDamageLayout {
		t.Fatalf("invalidation header = %#v", got)
	}
	assertNodeIDs(t, got.StyleNodes, []dom.NodeID{1, 2, 3, 4})
	assertNodeIDs(t, got.CompositeNodes, []dom.NodeID{2})
	assertNodeIDs(t, got.PaintNodes, []dom.NodeID{3})
	assertNodeIDs(t, got.LayoutRoots, []dom.NodeID{4})
	for _, nodeID := range got.StyleNodes {
		if nodeID == 99 {
			t.Fatal("unrelated sibling was dirtied")
		}
	}

	snapshot := page.RenderInvalidationSnapshot()
	snapshot.LayoutRoots[0] = 99
	if page.RenderInvalidationSnapshot().LayoutRoots[0] != 4 {
		t.Fatal("render invalidation snapshot aliases page state")
	}
}

func TestComputedStyleMutationBoundsDirtyNodes(t *testing.T) {
	previous, current := style.Map{}, style.Map{}
	for index := 1; index <= maxRenderInvalidationNodes+20; index++ {
		previous[dom.NodeID(index)] = style.ComputedStyle{Display: style.DisplayBlock, Opacity: 1}
		current[dom.NodeID(index)] = style.ComputedStyle{Display: style.DisplayBlock, Opacity: 0.5}
	}
	got := (&Page{}).RecordComputedStyleChanges(previous, current)
	if len(got.StyleNodes) != maxRenderInvalidationNodes || len(got.CompositeNodes) != maxRenderInvalidationNodes {
		t.Fatalf("bounded nodes = style:%d composite:%d", len(got.StyleNodes), len(got.CompositeNodes))
	}
}

func assertNodeIDs(t *testing.T, actual, expected []dom.NodeID) {
	t.Helper()
	if len(actual) != len(expected) {
		t.Fatalf("nodes = %v, want %v", actual, expected)
	}
	seen := make(map[dom.NodeID]bool, len(actual))
	for _, nodeID := range actual {
		seen[nodeID] = true
	}
	for _, nodeID := range expected {
		if !seen[nodeID] {
			t.Fatalf("nodes = %v, missing %d", actual, nodeID)
		}
	}
}
