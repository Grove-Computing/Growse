package devtools

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/css"
	"github.com/Grove-Computing/Growse/internal/dom"
	layoutmodel "github.com/Grove-Computing/Growse/internal/layout"
	stylemodel "github.com/Grove-Computing/Growse/internal/style"
)

func TestSnapshotInspectorListsMultiColumnVisualFragments(t *testing.T) {
	document := dom.NewDocument()
	paragraph := document.CreateElement("p", map[string]string{"class": "columns"})
	if err := document.AppendChild(document.Root, paragraph); err != nil {
		t.Fatal(err)
	}
	if err := document.AppendChild(paragraph, document.CreateText(strings.Repeat("fragmented inspector content ", 30))); err != nil {
		t.Fatal(err)
	}
	stylesheet, err := css.Parse(strings.NewReader(`
.columns { display:block; width:360px; margin:0; column-count:3; column-gap:18px; line-height:18px }
`))
	if err != nil {
		t.Fatal(err)
	}
	styles := stylemodel.Compute(document, stylesheet)
	tree := layoutmodel.BuildWithViewport(document, styles, 520, 600)
	snapshot := SnapshotInspector(document, styles, tree, paragraph.ID)
	boxFragments := 0
	identities := make(map[uint64]bool)
	xPositions := make(map[int]bool)
	for _, fragment := range snapshot.Fragments {
		if fragment.Kind != "box" {
			continue
		}
		boxFragments++
		identities[fragment.ID] = true
		xPositions[int(fragment.X+0.5)] = true
	}
	if boxFragments < 3 || len(identities) != boxFragments || len(xPositions) != 3 {
		t.Fatalf("inspector fragments = count:%d ids:%d columns:%v all:%+v", boxFragments, len(identities), xPositions, snapshot.Fragments)
	}
	if snapshot.Layout == nil || snapshot.Layout.Width != 360 {
		t.Fatalf("aggregate layout = %+v", snapshot.Layout)
	}
}

func TestSnapshotInspectorPreservesTreeAndRedactsPassword(t *testing.T) {
	document := dom.NewDocument()
	form := document.CreateElement("form", map[string]string{"id": "login"})
	password := document.CreateElement("input", map[string]string{"type": "password", "value": "secret"})
	text := document.CreateText("Sign in")
	for _, edge := range [][2]*dom.Node{{document.Root, form}, {form, password}, {form, text}} {
		if err := document.AppendChild(edge[0], edge[1]); err != nil {
			t.Fatal(err)
		}
	}
	styles := stylemodel.Map{password.ID: {Display: stylemodel.DisplayInlineBlock, Position: stylemodel.PositionRelative, FontSize: 16, FontWeight: 400, Opacity: 1}}
	tree := &layoutmodel.Tree{Bounds: map[dom.NodeID]layoutmodel.Rect{password.ID: {X: 10, Y: 20, Width: 100, Height: 30}}}

	snapshot := SnapshotInspector(document, styles, tree, password.ID)
	if len(snapshot.Nodes) != 4 || snapshot.Nodes[2].ParentID != form.ID || snapshot.Nodes[2].Depth != 2 {
		t.Fatalf("DOM snapshot = %+v", snapshot.Nodes)
	}
	if snapshot.SelectedNode == nil || attributeValue(snapshot.SelectedNode.Attributes, "value") != "[REDACTED]" {
		t.Fatalf("selected password = %+v", snapshot.SelectedNode)
	}
	if snapshot.Layout == nil || snapshot.Layout.Width != 100 || len(snapshot.Styles) == 0 {
		t.Fatalf("details = styles:%+v layout:%+v", snapshot.Styles, snapshot.Layout)
	}
}

func TestSnapshotInspectorEnforcesLimitsAndClearsDisconnectedSelection(t *testing.T) {
	document := dom.NewDocument()
	parent := document.Root
	var selected dom.NodeID
	for depth := 0; depth < MaxDOMDepth+5; depth++ {
		attributes := make(map[string]string, MaxDOMAttributes+5)
		for index := 0; index < MaxDOMAttributes+5; index++ {
			attributes[fmt.Sprintf("data-%03d", index)] = strings.Repeat("x", MaxInspectorBytes+10)
		}
		node := document.CreateElement("div", attributes)
		if err := document.AppendChild(parent, node); err != nil {
			t.Fatal(err)
		}
		parent = node
		selected = node.ID
	}
	snapshot := SnapshotInspector(document, nil, nil, selected)
	if !snapshot.Truncated || len(snapshot.Nodes) != MaxDOMDepth+1 || snapshot.Selected != 0 {
		t.Fatalf("limited snapshot = nodes:%d truncated:%v selected:%d", len(snapshot.Nodes), snapshot.Truncated, snapshot.Selected)
	}
	if got := len(snapshot.Nodes[1].Attributes); got != MaxDOMAttributes {
		t.Fatalf("attributes = %d, want %d", got, MaxDOMAttributes)
	}
	if got := len(snapshot.Nodes[1].Attributes[0].Value); got > MaxInspectorBytes {
		t.Fatalf("attribute bytes = %d", got)
	}
}

func TestSnapshotInspectorLimitsNodeCount(t *testing.T) {
	document := dom.NewDocument()
	for index := 0; index < MaxDOMNodes+5; index++ {
		if err := document.AppendChild(document.Root, document.CreateElement("span", nil)); err != nil {
			t.Fatal(err)
		}
	}
	snapshot := SnapshotInspector(document, nil, nil, 0)
	if !snapshot.Truncated || len(snapshot.Nodes) != MaxDOMNodes {
		t.Fatalf("snapshot = nodes:%d truncated:%v", len(snapshot.Nodes), snapshot.Truncated)
	}
}

func TestSnapshotInspectorLimitsLayoutFragments(t *testing.T) {
	document := dom.NewDocument()
	selected := document.CreateElement("p", nil)
	if err := document.AppendChild(document.Root, selected); err != nil {
		t.Fatal(err)
	}
	tree := &layoutmodel.Tree{Bounds: map[dom.NodeID]layoutmodel.Rect{selected.ID: {Width: 10, Height: 10}}}
	for index := 0; index < MaxLayoutFragments+1; index++ {
		tree.Boxes = append(tree.Boxes, layoutmodel.Box{NodeID: selected.ID, FragmentID: uint64(index + 1), Width: 10, Height: 10})
	}
	snapshot := SnapshotInspector(document, nil, tree, selected.ID)
	if !snapshot.Truncated || len(snapshot.Fragments) != MaxLayoutFragments {
		t.Fatalf("fragment snapshot = count:%d truncated:%v", len(snapshot.Fragments), snapshot.Truncated)
	}
}

func attributeValue(attributes []Attribute, name string) string {
	for _, attribute := range attributes {
		if attribute.Name == name {
			return attribute.Value
		}
	}
	return ""
}
