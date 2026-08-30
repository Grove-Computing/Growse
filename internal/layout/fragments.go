package layout

import (
	"reflect"

	"github.com/Grove-Computing/Growse/internal/dom"
)

func assignFragmentIdentities(tree *Tree) {
	if tree == nil {
		return
	}
	boxOccurrences := make(map[dom.NodeID]uint64)
	for index := range tree.Boxes {
		box := &tree.Boxes[index]
		box.FragmentID = stableFragmentID(box.NodeID, 1, boxOccurrences[box.NodeID])
		boxOccurrences[box.NodeID]++
	}
	decorationOccurrences := make(map[dom.NodeID]uint64)
	for index := range tree.Decorations {
		decoration := &tree.Decorations[index]
		decoration.FragmentID = stableFragmentID(decoration.NodeID, 2, decorationOccurrences[decoration.NodeID])
		decorationOccurrences[decoration.NodeID]++
	}
}

func stableFragmentID(nodeID dom.NodeID, kind, occurrence uint64) uint64 {
	// DOM IDs are page-generation local; the low bits distinguish fragment
	// kind and repeated inline line boxes for one node.
	return uint64(nodeID)<<24 ^ kind<<20 ^ occurrence
}

// ReuseStableFragments restores immutable fragment storage for unaffected
// nodes whose geometry and paint inputs stayed identical.
func ReuseStableFragments(previous, current *Tree, dirty map[dom.NodeID]bool) int {
	if previous == nil || current == nil {
		return 0
	}
	previousBoxes := make(map[uint64]Box, len(previous.Boxes))
	for _, box := range previous.Boxes {
		previousBoxes[box.FragmentID] = box
	}
	previousDecorations := make(map[uint64]Decoration, len(previous.Decorations))
	for _, decoration := range previous.Decorations {
		previousDecorations[decoration.FragmentID] = decoration
	}
	reused := 0
	for index, box := range current.Boxes {
		candidate, exists := previousBoxes[box.FragmentID]
		if exists && !dirty[box.NodeID] && reflect.DeepEqual(candidate, box) {
			current.Boxes[index] = candidate
			reused++
		}
	}
	for index, decoration := range current.Decorations {
		candidate, exists := previousDecorations[decoration.FragmentID]
		if exists && !dirty[decoration.NodeID] && reflect.DeepEqual(candidate, decoration) {
			current.Decorations[index] = candidate
			reused++
		}
	}
	return reused
}
