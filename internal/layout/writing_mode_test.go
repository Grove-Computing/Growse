package layout

import (
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/css"
	"github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/style"
)

func TestWritingModeAndDirectionSurviveLayoutTree(t *testing.T) {
	t.Parallel()
	document := dom.NewDocument()
	container := document.CreateElement("div", map[string]string{"class": "vertical"})
	text := document.CreateText("縦書き")
	if err := document.AppendChild(document.Root, container); err != nil {
		t.Fatal(err)
	}
	if err := document.AppendChild(container, text); err != nil {
		t.Fatal(err)
	}
	stylesheet, err := css.Parse(strings.NewReader(`.vertical { display:block; writing-mode:vertical-rl; direction:rtl; background:red }`))
	if err != nil {
		t.Fatal(err)
	}
	tree := Build(document, style.Compute(document, stylesheet), 400)

	decorationFound := false
	for _, decoration := range tree.Decorations {
		if decoration.NodeID != container.ID {
			continue
		}
		decorationFound = true
		if decoration.WritingMode != style.WritingModeVerticalRL || decoration.Direction != style.DirectionRTL {
			t.Fatalf("decoration writing mode/direction = %v/%v", decoration.WritingMode, decoration.Direction)
		}
	}
	if !decorationFound {
		t.Fatal("container decoration is missing")
	}
	boxFound := false
	for _, box := range tree.Boxes {
		if box.NodeID != container.ID {
			continue
		}
		boxFound = true
		if box.WritingMode != style.WritingModeVerticalRL || box.Direction != style.DirectionRTL {
			t.Fatalf("box writing mode/direction = %v/%v", box.WritingMode, box.Direction)
		}
		if len(box.Runs) == 0 || box.Runs[0].WritingMode != style.WritingModeVerticalRL || box.Runs[0].Direction != style.DirectionRTL {
			t.Fatalf("text run writing metadata = %#v", box.Runs)
		}
	}
	if !boxFound {
		t.Fatal("container text box is missing")
	}
}

func TestVerticalTextUsesColumnGeometryForPaintHitAndScroll(t *testing.T) {
	t.Parallel()
	document := dom.NewDocument()
	container := document.CreateElement("div", map[string]string{"class": "vertical"})
	text := document.CreateText(strings.Repeat("縦書き", 24))
	if err := document.AppendChild(document.Root, container); err != nil {
		t.Fatal(err)
	}
	if err := document.AppendChild(container, text); err != nil {
		t.Fatal(err)
	}
	stylesheet, err := css.Parse(strings.NewReader(`.vertical {
  display:block; writing-mode:vertical-lr; inline-size:48px; block-size:24px;
  overflow:auto; font-size:16px; line-height:18px;
}`))
	if err != nil {
		t.Fatal(err)
	}
	tree := BuildWithViewport(document, style.Compute(document, stylesheet), 220, 160)
	var first *Box
	columns := make(map[float32]struct{})
	for index := range tree.Boxes {
		box := &tree.Boxes[index]
		if box.NodeID == container.ID && box.WritingMode == style.WritingModeVerticalLR && len(box.Runs) != 0 {
			if first == nil {
				first = box
			}
			for _, run := range box.Runs {
				columns[run.OffsetX] = struct{}{}
			}
		}
	}
	if first == nil || len(columns) < 2 || first.Height > 48 || first.Width < 16 {
		t.Fatalf("vertical columns = count:%d first:%#v", len(columns), first)
	}
	firstRun := first.Runs[0]
	if hit, ok := HitTest(tree, first.X+firstRun.OffsetX+firstRun.CrossSize/2, first.Y+firstRun.OffsetY+firstRun.Width/2); !ok || hit != container.ID {
		t.Fatalf("vertical glyph hit = %d/%t, want %d", hit, ok, container.ID)
	}
	scrollContainer, exists := tree.ScrollContainers[container.ID]
	if !exists || scrollContainer.ScrollWidth <= scrollContainer.Viewport.Width {
		t.Fatalf("vertical scroll container = %#v exists:%t", scrollContainer, exists)
	}
	if tree.ScrollWidth != tree.Width {
		t.Fatalf("nested vertical overflow leaked into document scroll width = %v, viewport %v", tree.ScrollWidth, tree.Width)
	}
}

func TestVerticalWritingPlacesInlineReplacedElementOnInlineAxis(t *testing.T) {
	t.Parallel()
	document := dom.NewDocument()
	container := document.CreateElement("div", map[string]string{"class": "vertical"})
	image := document.CreateElement("img", map[string]string{"class": "pic", "alt": "fixture"})
	if err := document.AppendChild(document.Root, container); err != nil {
		t.Fatal(err)
	}
	if err := document.AppendChild(container, image); err != nil {
		t.Fatal(err)
	}
	stylesheet, err := css.Parse(strings.NewReader(`
.vertical { display:block; writing-mode:vertical-rl; inline-size:100px; block-size:60px }
.pic { display:inline; inline-size:40px; block-size:30px }
`))
	if err != nil {
		t.Fatal(err)
	}
	tree := Build(document, style.Compute(document, stylesheet), 300)
	for _, box := range tree.Boxes {
		if box.NodeID != image.ID || !box.Image {
			continue
		}
		if box.Width != 30 || box.Height != 40 || box.WritingMode != style.WritingModeVerticalRL {
			t.Fatalf("vertical replaced box = %#v", box)
		}
		if hit, ok := HitTest(tree, box.X+1, box.Y+1); !ok || hit != image.ID {
			t.Fatalf("vertical replaced hit = %d/%t", hit, ok)
		}
		return
	}
	t.Fatal("vertical replaced box is missing")
}

func TestVerticalWritingMapsFlexAndGridLogicalAxes(t *testing.T) {
	t.Parallel()
	document := dom.NewDocument()
	flex := document.CreateElement("div", map[string]string{"class": "flex"})
	flexA := document.CreateElement("span", map[string]string{"class": "item"})
	flexB := document.CreateElement("span", map[string]string{"class": "item"})
	grid := document.CreateElement("div", map[string]string{"class": "grid"})
	gridA := document.CreateElement("span", map[string]string{"class": "a"})
	gridB := document.CreateElement("span", map[string]string{"class": "b"})
	for _, pair := range [][2]*dom.Node{{document.Root, flex}, {flex, flexA}, {flex, flexB}, {document.Root, grid}, {grid, gridA}, {grid, gridB}} {
		if err := document.AppendChild(pair[0], pair[1]); err != nil {
			t.Fatal(err)
		}
	}
	if err := document.AppendChild(flexA, document.CreateText("甲一")); err != nil {
		t.Fatal(err)
	}
	stylesheet, err := css.Parse(strings.NewReader(`
.flex { display:flex; writing-mode:vertical-rl; inline-size:120px; block-size:100px; gap:10px }
.item { display:block; writing-mode:vertical-rl; inline-size:48px; block-size:20px }
.grid { display:grid; writing-mode:vertical-rl; inline-size:100px; block-size:100px; grid-template-columns:30px 40px; grid-template-rows:20px 25px }
.a { grid-column:1; grid-row:1 }
.b { grid-column:2; grid-row:2 }
`))
	if err != nil {
		t.Fatal(err)
	}
	tree := Build(document, style.Compute(document, stylesheet), 420)
	flexFirst, flexSecond := tree.Bounds[flexA.ID], tree.Bounds[flexB.ID]
	if flexSecond.Y <= flexFirst.Y || flexSecond.X != flexFirst.X {
		t.Fatalf("vertical flex geometry = first:%#v second:%#v", flexFirst, flexSecond)
	}
	verticalFlexText := false
	var flexTextBoxes []Box
	for _, box := range tree.Boxes {
		if box.NodeID != flexA.ID || len(box.Runs) < 2 {
			continue
		}
		flexTextBoxes = append(flexTextBoxes, box)
		verticalFlexText = box.WritingMode == style.WritingModeVerticalRL && box.Runs[0].OffsetX == box.Runs[1].OffsetX && box.Runs[1].OffsetY > box.Runs[0].OffsetY
	}
	if !verticalFlexText {
		t.Fatalf("vertical flex item text did not advance on the physical Y axis: %#v", flexTextBoxes)
	}
	gridFirst, gridSecond := tree.Bounds[gridA.ID], tree.Bounds[gridB.ID]
	if gridSecond.Y <= gridFirst.Y || gridSecond.X >= gridFirst.X {
		t.Fatalf("vertical grid axes = first:%#v second:%#v", gridFirst, gridSecond)
	}
	if gridFirst.Width != 20 || gridFirst.Height != 30 || gridSecond.Width != 25 || gridSecond.Height != 40 {
		t.Fatalf("vertical grid track sizes = first:%#v second:%#v", gridFirst, gridSecond)
	}
}
