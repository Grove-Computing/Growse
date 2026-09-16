package layout

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/css"
	"github.com/Grove-Computing/Growse/internal/dom"
	stylemodel "github.com/Grove-Computing/Growse/internal/style"
)

func TestMultiColumnBalancesBlockAndInlineFragmentsWithRules(t *testing.T) {
	document := dom.NewDocument()
	container := document.CreateElement("section", map[string]string{"class": "columns"})
	appendNodes(t, document, [2]*dom.Node{document.Root, container})
	items := make([]*dom.Node, 6)
	for index := range items {
		items[index] = document.CreateElement("p", map[string]string{"class": "item"})
		appendNodes(t, document, [2]*dom.Node{container, items[index]}, [2]*dom.Node{items[index], document.CreateText(fmt.Sprintf("column item %d with wrapping text", index+1))})
	}
	stylesheet, err := css.Parse(strings.NewReader(`
.columns { display:block; width:600px; column-count:3; column-gap:30px; column-rule:4px solid #ff00aa; background:#101820 }
.item { display:block; height:54px; margin:0; background:#24405f; line-height:18px }
`))
	if err != nil {
		t.Fatal(err)
	}
	tree := BuildWithViewport(document, stylemodel.Compute(document, stylesheet), 760, 600)
	containerRect := tree.Bounds[container.ID]
	if containerRect.Width != 600 {
		t.Fatalf("column container = %#v", containerRect)
	}
	columnX := make(map[int]bool)
	for _, item := range items {
		bounds := tree.Bounds[item.ID]
		columnX[int(bounds.X+0.5)] = true
		if hit, ok := HitTest(tree, bounds.X+2, bounds.Y+2); !ok || hit != item.ID {
			t.Fatalf("fragment hit for item %d = (%d,%v), bounds %#v", item.ID, hit, ok, bounds)
		}
	}
	if len(columnX) != 3 {
		t.Fatalf("balanced column x positions = %v, want 3 columns", columnX)
	}
	rules := 0
	for _, decoration := range tree.Decorations {
		if decoration.NodeID == container.ID && decoration.Background == 0xff00aaff && decoration.Width == 4 {
			rules++
		}
	}
	if rules != 2 {
		t.Fatalf("column rules = %d, want 2", rules)
	}
}

func TestMultiColumnSpanAndForcedBreakStartNewFragmentainers(t *testing.T) {
	document := dom.NewDocument()
	container := document.CreateElement("section", map[string]string{"class": "columns"})
	first := document.CreateElement("div", map[string]string{"class": "item first"})
	forced := document.CreateElement("div", map[string]string{"class": "item forced"})
	spanner := document.CreateElement("h2", map[string]string{"class": "spanner"})
	last := document.CreateElement("div", map[string]string{"class": "item last"})
	appendNodes(t, document,
		[2]*dom.Node{document.Root, container}, [2]*dom.Node{container, first}, [2]*dom.Node{first, document.CreateText("first")},
		[2]*dom.Node{container, forced}, [2]*dom.Node{forced, document.CreateText("forced")},
		[2]*dom.Node{container, spanner}, [2]*dom.Node{spanner, document.CreateText("all columns")},
		[2]*dom.Node{container, last}, [2]*dom.Node{last, document.CreateText("last")},
	)
	stylesheet, err := css.Parse(strings.NewReader(`
.columns { display:block; width:420px; column-count:2; column-gap:20px }
.item { display:block; height:48px; margin:0; background:#335577 }
.forced { break-before:column }
.spanner { display:block; column-span:all; height:32px; margin:6px 0; background:#ddaa33 }
`))
	if err != nil {
		t.Fatal(err)
	}
	tree := BuildWithViewport(document, stylemodel.Compute(document, stylesheet), 600, 500)
	firstRect, forcedRect := tree.Bounds[first.ID], tree.Bounds[forced.ID]
	if forcedRect.X <= firstRect.X {
		t.Fatalf("forced column break = first:%#v forced:%#v", firstRect, forcedRect)
	}
	spanRect, lastRect := tree.Bounds[spanner.ID], tree.Bounds[last.ID]
	if spanRect.Width != 420 || spanRect.Y < max(firstRect.Y+firstRect.Height, forcedRect.Y+forcedRect.Height) || lastRect.Y < spanRect.Y+spanRect.Height {
		t.Fatalf("column span geometry = first:%#v forced:%#v span:%#v last:%#v", firstRect, forcedRect, spanRect, lastRect)
	}
}

func TestMultiColumnHonorsBreakAfterAndAvoidInside(t *testing.T) {
	document := dom.NewDocument()
	container := document.CreateElement("section", map[string]string{"class": "columns"})
	first := document.CreateElement("div", map[string]string{"class": "item first"})
	kept := document.CreateElement("div", map[string]string{"class": "item kept"})
	last := document.CreateElement("div", map[string]string{"class": "item last"})
	appendNodes(t, document,
		[2]*dom.Node{document.Root, container},
		[2]*dom.Node{container, first}, [2]*dom.Node{first, document.CreateText("first block")},
		[2]*dom.Node{container, kept}, [2]*dom.Node{kept, document.CreateText("kept block")},
		[2]*dom.Node{container, last}, [2]*dom.Node{last, document.CreateText("last block")},
	)
	stylesheet, err := css.Parse(strings.NewReader(`
.columns { display:block; width:340px; column-count:2; column-gap:20px }
.item { display:block; height:44px; margin:0; background:#335577 }
.first { break-after:column }
.kept { height:70px; break-inside:avoid-column }
`))
	if err != nil {
		t.Fatal(err)
	}
	tree := BuildWithViewport(document, stylemodel.Compute(document, stylesheet), 500, 400)
	firstRect, keptRect := tree.Bounds[first.ID], tree.Bounds[kept.ID]
	if keptRect.X <= firstRect.X {
		t.Fatalf("break-after did not advance a fragmentainer: first=%#v kept=%#v", firstRect, keptRect)
	}
	keptFragments := 0
	for _, box := range tree.Boxes {
		if box.NodeID == kept.ID {
			keptFragments++
		}
	}
	if keptFragments != 1 {
		t.Fatalf("break-inside avoid split the kept block into %d visual fragments", keptFragments)
	}
	if lastRect := tree.Bounds[last.ID]; lastRect.X < keptRect.X || lastRect.X == keptRect.X && lastRect.Y < keptRect.Y+keptRect.Height-0.01 {
		t.Fatalf("content order after kept block = %#v, kept=%#v", lastRect, keptRect)
	}
}

func TestMultiColumnRepeatsOneDOMNodeAsStablePaintAndHitFragments(t *testing.T) {
	document := dom.NewDocument()
	paragraph := document.CreateElement("p", map[string]string{"class": "columns"})
	appendNodes(t, document, [2]*dom.Node{document.Root, paragraph}, [2]*dom.Node{paragraph, document.CreateText(strings.Repeat("fragmented inline content ", 30))})
	stylesheet, err := css.Parse(strings.NewReader(`
.columns { display:block; width:360px; column-count:3; column-gap:18px; line-height:18px; margin:0; background:#123 }
`))
	if err != nil {
		t.Fatal(err)
	}
	tree := BuildWithViewport(document, stylemodel.Compute(document, stylesheet), 520, 600)
	fragments := make([]Box, 0)
	identities := make(map[uint64]bool)
	xPositions := make(map[int]bool)
	for _, box := range tree.Boxes {
		if box.NodeID != paragraph.ID {
			continue
		}
		fragments = append(fragments, box)
		identities[box.FragmentID] = true
		xPositions[int(box.X+0.5)] = true
		if hit, ok := HitTest(tree, box.X+1, box.Y+box.Height/2); !ok || hit != paragraph.ID {
			t.Fatalf("inline fragment hit = (%d,%v), box %#v", hit, ok, box)
		}
	}
	if len(fragments) < 3 || len(xPositions) != 3 || len(identities) != len(fragments) {
		t.Fatalf("inline fragments = count:%d columns:%v identities:%d", len(fragments), xPositions, len(identities))
	}
	if bounds := tree.Bounds[paragraph.ID]; bounds.Width != 360 {
		t.Fatalf("fragment union/container bounds = %#v", bounds)
	}
}
