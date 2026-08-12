package paint

import (
	"testing"

	"github.com/saku0512/growse/internal/layout"
)

func TestBuildPreservesPaintOrder(t *testing.T) {
	tree := &layout.Tree{Width: 400, Height: 100, Boxes: []layout.Box{
		{Text: "first", Y: 10, Runs: []layout.TextRun{{Text: "first", Color: 0x123456ff}}},
		{Text: "second", Y: 40},
	}}

	list := Build(tree)
	if got, want := len(list.Commands), 2; got != want {
		t.Fatalf("command count = %d, want %d", got, want)
	}
	first, ok := list.Commands[0].(DrawText)
	if !ok || first.Text != "first" {
		t.Fatalf("first command = %#v, want DrawText first", list.Commands[0])
	}
	if len(first.Runs) != 1 || first.Runs[0].Color != 0x123456ff {
		t.Fatalf("first runs = %#v, want preserved inline style", first.Runs)
	}
}
