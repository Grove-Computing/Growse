package paint

import (
	"testing"

	"github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/layout"
)

func TestBuildIncrementalReusesStableDisplayCommands(t *testing.T) {
	tree := &layout.Tree{Width: 800, Height: 600, Boxes: []layout.Box{
		{NodeID: 1, FragmentID: 101, Text: "dirty", Width: 40, Height: 20, Runs: []layout.TextRun{{NodeID: 1, Text: "dirty", Width: 40}}},
		{NodeID: 2, FragmentID: 202, Text: "stable", Y: 20, Width: 48, Height: 20, Runs: []layout.TextRun{{NodeID: 2, Text: "stable", Width: 48}}},
	}}
	previous := Build(tree)
	next, reused := BuildIncremental(previous, tree, map[dom.NodeID]bool{1: true})
	if reused != 1 || len(next.Commands) != 2 {
		t.Fatalf("incremental display list reused=%d commands=%d", reused, len(next.Commands))
	}
	oldStable := previous.Commands[1].(DrawText)
	newStable := next.Commands[1].(DrawText)
	if &oldStable.Runs[0] != &newStable.Runs[0] {
		t.Fatal("stable display command storage was rebuilt")
	}
}
