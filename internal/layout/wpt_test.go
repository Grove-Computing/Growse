package layout

import (
	"testing"

	stylemodel "github.com/saku0512/growse/internal/style"
)

// Adapted from css/css-flexbox/flex-grow-001.xht at the revision recorded in
// docs/wpt.md. Geometry assertions replace the upstream visual reference.
func TestWPTFlexGrow001DistributesPositiveFreeSpaceByFactor(t *testing.T) {
	line := flexLine{items: []*flexItem{
		{base: 30, hypothetical: 30, maximum: -1, grow: 0, shrink: 1},
		{base: 30, hypothetical: 30, maximum: -1, grow: 1, shrink: 1},
		{base: 30, hypothetical: 30, maximum: -1, grow: 2, shrink: 1},
	}}
	resolveFlexibleLengths(&line, 240, 0)
	want := []float32{30, 80, 130}
	for index, item := range line.items {
		if item.target != want[index] {
			t.Fatalf("item %d width = %v, want %v", index, item.target, want[index])
		}
	}
}

// Adapted from css/css-flexbox/flex-wrap-002.html. The upstream reftest uses
// two 100px-tall items to require two lines in a 100px column main axis.
func TestWPTFlexWrap002FormsTwoColumnFlexLines(t *testing.T) {
	items := []*flexItem{{hypothetical: 100}, {hypothetical: 100}}
	lines := formFlexLines(items, 100, 0, stylemodel.FlexWrapLines)
	if len(lines) != 2 || len(lines[0].items) != 1 || len(lines[1].items) != 1 {
		t.Fatalf("lines = %#v, want two single-item lines", lines)
	}
}

// Adapted from css/css-flexbox/flex-direction-row-reverse.html. Numeric axis
// assertions replace the upstream reference image.
func TestWPTFlexDirectionRowReverseMapsMainAxis(t *testing.T) {
	axis := axisFor(stylemodel.FlexDirectionRowReverse, stylemodel.FlexNoWrap)
	if !axis.horizontal || !axis.reverse || axis.crossFlip {
		t.Fatalf("row-reverse axis = %#v", axis)
	}
}
