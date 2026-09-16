package paint

import (
	"testing"

	"github.com/Grove-Computing/Growse/internal/layout"
	"github.com/Grove-Computing/Growse/internal/style"
)

func TestWritingModeAndDirectionSurviveDisplayList(t *testing.T) {
	t.Parallel()
	tree := &layout.Tree{
		Decorations: []layout.Decoration{{
			Order: 1, NodeID: 7, Rect: layout.Rect{Width: 100, Height: 40}, Background: 0xff0000ff,
			WritingMode: style.WritingModeVerticalRL, Direction: style.DirectionRTL,
		}},
		Boxes: []layout.Box{{
			Order: 2, NodeID: 7, Text: "縦", Width: 20, Height: 40,
			WritingMode: style.WritingModeVerticalRL, Direction: style.DirectionRTL,
			Runs: []layout.TextRun{{NodeID: 8, Text: "縦", WritingMode: style.WritingModeVerticalLR, Direction: style.DirectionLTR}},
		}},
	}
	list := Build(tree)
	box, ok := list.Commands[0].(DrawBox)
	if !ok || box.WritingMode != style.WritingModeVerticalRL || box.Direction != style.DirectionRTL {
		t.Fatalf("paint box writing metadata = %#v", list.Commands[0])
	}
	text, ok := list.Commands[1].(DrawText)
	if !ok || text.WritingMode != style.WritingModeVerticalRL || text.Direction != style.DirectionRTL {
		t.Fatalf("paint text writing metadata = %#v", list.Commands[1])
	}
	if len(text.Runs) != 1 || text.Runs[0].WritingMode != style.WritingModeVerticalLR || text.Runs[0].Direction != style.DirectionLTR {
		t.Fatalf("paint run writing metadata = %#v", text.Runs)
	}
}
