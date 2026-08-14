package layout

import (
	"sort"

	stylemodel "github.com/saku0512/growse/internal/style"
)

type flexAxis struct {
	horizontal bool
	reverse    bool
	crossFlip  bool
}

func axisFor(direction stylemodel.FlexDirection, wrap stylemodel.FlexWrap) flexAxis {
	axis := flexAxis{horizontal: direction == stylemodel.FlexDirectionRow || direction == stylemodel.FlexDirectionRowReverse}
	axis.reverse = direction == stylemodel.FlexDirectionRowReverse || direction == stylemodel.FlexDirectionColumnReverse
	axis.crossFlip = wrap == stylemodel.FlexWrapReverse
	return axis
}

type flexItem struct {
	index        int
	order        int
	base         float32
	hypothetical float32
	minimum      float32
	maximum      float32
	grow         float32
	shrink       float32
	target       float32
	frozen       bool
}

type flexLine struct {
	items []*flexItem
}

func orderFlexItems(items []*flexItem) {
	sort.SliceStable(items, func(left, right int) bool {
		if items[left].order != items[right].order {
			return items[left].order < items[right].order
		}
		return items[left].index < items[right].index
	})
}

func formFlexLines(items []*flexItem, available, gap float32, wrap stylemodel.FlexWrap) []flexLine {
	if len(items) == 0 {
		return nil
	}
	if wrap == stylemodel.FlexNoWrap || available < 0 {
		return []flexLine{{items: append([]*flexItem(nil), items...)}}
	}
	lines := []flexLine{{}}
	used := float32(0)
	for _, item := range items {
		required := item.hypothetical
		if len(lines[len(lines)-1].items) != 0 {
			required += gap
		}
		if len(lines[len(lines)-1].items) != 0 && used+required > available {
			lines = append(lines, flexLine{})
			used = 0
			required = item.hypothetical
		}
		lines[len(lines)-1].items = append(lines[len(lines)-1].items, item)
		used += required
	}
	return lines
}

// resolveFlexibleLengths distributes free space and repeatedly freezes items
// which violate their min/max constraints. The final item receives any binary
// floating-point remainder so the line sum remains deterministic.
func resolveFlexibleLengths(line *flexLine, available, gap float32) {
	if line == nil || len(line.items) == 0 {
		return
	}
	itemSpace := available - gap*float32(len(line.items)-1)
	sumHypothetical := float32(0)
	for _, item := range line.items {
		item.target = item.base
		item.frozen = false
		sumHypothetical += item.hypothetical
	}
	growing := sumHypothetical < itemSpace
	for _, item := range line.items {
		factor := item.shrink
		if growing {
			factor = item.grow
		}
		if factor == 0 || growing && item.base > item.hypothetical || !growing && item.base < item.hypothetical {
			item.target = item.hypothetical
			item.frozen = true
		}
	}

	for iteration := 0; iteration <= len(line.items); iteration++ {
		used, weight := float32(0), float32(0)
		var unfrozen []*flexItem
		for _, item := range line.items {
			used += item.target
			if item.frozen {
				continue
			}
			factor := item.grow
			if !growing {
				factor = item.shrink * item.base
			}
			weight += factor
			unfrozen = append(unfrozen, item)
		}
		if len(unfrozen) == 0 {
			break
		}
		remaining := itemSpace - used
		assigned := float32(0)
		violations := false
		for index, item := range unfrozen {
			share := float32(0)
			if weight > 0 {
				factor := item.grow
				if !growing {
					factor = item.shrink * item.base
				}
				if index == len(unfrozen)-1 {
					share = remaining - assigned
				} else {
					share = remaining * factor / weight
					assigned += share
				}
			}
			candidate := item.target + share
			clamped := max(item.minimum, candidate)
			if item.maximum >= 0 {
				clamped = min(item.maximum, clamped)
			}
			item.target = clamped
			if clamped != candidate {
				item.frozen = true
				violations = true
			}
		}
		if !violations {
			for _, item := range unfrozen {
				item.frozen = true
			}
			break
		}
	}
}
