package layout

import (
	"testing"

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
