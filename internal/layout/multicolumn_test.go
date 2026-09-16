package layout

import (
	"fmt"
	"reflect"
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

func TestBlockAvoidCandidateEnclosesSystemFontContent(t *testing.T) {
	document := dom.NewDocument()
	card := document.CreateElement("article", map[string]string{"class": "card"})
	title := document.CreateElement("strong", nil)
	body := document.CreateElement("span", nil)
	appendNodes(t, document,
		[2]*dom.Node{document.Root, card},
		[2]*dom.Node{card, title}, [2]*dom.Node{title, document.CreateText("01 · Balance")},
		[2]*dom.Node{card, body}, [2]*dom.Node{body, document.CreateText("Column count, width, and gap resolve against one available inline size.")},
	)
	stylesheet, err := css.Parse(strings.NewReader(`
* { box-sizing:border-box }
.card { display:block; width:270px; min-height:82px; padding:11px; border:1px solid #416b91 }
.card strong, .card span { display:block }
.card strong { margin-bottom:6px }
`))
	if err != nil {
		t.Fatal(err)
	}
	tree := BuildWithScrollAndResources(document, stylemodel.Compute(document, stylesheet), nil, NewFontSetWithSystemFallback(nil), 500, 400, 0, 0)
	cardBounds := tree.Bounds[card.ID]
	for _, box := range tree.Boxes {
		if box.NodeID != title.ID && box.NodeID != body.ID {
			continue
		}
		if box.Y < cardBounds.Y-0.01 || box.Y+box.Height > cardBounds.Y+cardBounds.Height+0.01 {
			t.Fatalf("system-font content escaped block bounds: card=%#v box=%#v", cardBounds, box)
		}
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
	if tree.ScrollWidth <= 500 {
		t.Fatalf("forced fragmentainer overflow was not included in scroll extent: %v", tree.ScrollWidth)
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

func TestMultiColumnBalancingLimitsAndFallbackAreDeterministic(t *testing.T) {
	document := dom.NewDocument()
	container := document.CreateElement("section", map[string]string{"class": "columns"})
	kept := document.CreateElement("div", map[string]string{"class": "kept"})
	appendNodes(t, document, [2]*dom.Node{document.Root, container}, [2]*dom.Node{container, kept})
	lineIDs := make(map[dom.NodeID]bool)
	for index := 0; index < 200; index++ {
		line := document.CreateElement("div", map[string]string{"class": "line"})
		lineIDs[line.ID] = true
		appendNodes(t, document, [2]*dom.Node{kept, line}, [2]*dom.Node{line, document.CreateText(fmt.Sprintf("line %03d", index))})
	}
	stylesheet, err := css.Parse(strings.NewReader(`
.columns { display:block; width:600px; column-count:999; column-gap:16px }
.kept { display:block; break-inside:avoid-column }
.line { display:block; height:8px; margin:0 }
`))
	if err != nil {
		t.Fatal(err)
	}
	computed := stylemodel.Compute(document, stylesheet)
	containerStyle, _ := computed.For(container)
	if containerStyle.ColumnCount != maxMultiColumnCount {
		t.Fatalf("computed column count = %d, want bounded %d", containerStyle.ColumnCount, maxMultiColumnCount)
	}
	first := BuildWithViewport(document, computed, 760, 600)
	second := BuildWithViewport(document, computed, 760, 600)
	if !reflect.DeepEqual(first.Boxes, second.Boxes) || !reflect.DeepEqual(first.Decorations, second.Decorations) || !reflect.DeepEqual(first.Fallbacks, second.Fallbacks) {
		t.Fatal("bounded multi-column fallback was not deterministic")
	}
	columns := make(map[int]bool)
	for _, box := range first.Boxes {
		if !lineIDs[box.NodeID] {
			continue
		}
		columns[int(box.X+0.5)] = true
	}
	if len(columns) != maxMultiColumnCount {
		t.Fatalf("generated fragmentainers = %d, want %d", len(columns), maxMultiColumnCount)
	}
	iterationFallback, finiteFallback := false, false
	for _, fallback := range first.Fallbacks {
		if fallback.NodeID != container.ID {
			t.Fatalf("fallback owner = %d, want %d: %#v", fallback.NodeID, container.ID, fallback)
		}
		iterationFallback = iterationFallback || fallback.Reason == "multi-column balance iteration limit exceeded"
		finiteFallback = finiteFallback || fallback.Reason == "multi-column balance used finite fallback"
	}
	if !iterationFallback || !finiteFallback {
		t.Fatalf("bounded balancing diagnostics = %#v", first.Fallbacks)
	}
}

func TestMultiColumnAutoFillCapsFragmentainers(t *testing.T) {
	document := dom.NewDocument()
	container := document.CreateElement("section", map[string]string{"class": "columns"})
	appendNodes(t, document, [2]*dom.Node{document.Root, container})
	for index := 0; index < 80; index++ {
		item := document.CreateElement("div", map[string]string{"class": "item"})
		appendNodes(t, document, [2]*dom.Node{container, item}, [2]*dom.Node{item, document.CreateText("item")})
	}
	stylesheet, err := css.Parse(strings.NewReader(`
.columns { display:block; width:320px; height:10px; column-count:2; column-fill:auto }
.item { display:block; height:8px; margin:0 }
`))
	if err != nil {
		t.Fatal(err)
	}
	tree := BuildWithViewport(document, stylemodel.Compute(document, stylesheet), 480, 300)
	if !hasFallbackReason(tree, "multi-column count limit exceeded") {
		t.Fatalf("auto-fill count fallback = %#v", tree.Fallbacks)
	}
}
