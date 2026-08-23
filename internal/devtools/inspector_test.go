package devtools

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/dom"
	layoutmodel "github.com/Grove-Computing/Growse/internal/layout"
	stylemodel "github.com/Grove-Computing/Growse/internal/style"
)

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

func attributeValue(attributes []Attribute, name string) string {
	for _, attribute := range attributes {
		if attribute.Name == name {
			return attribute.Value
		}
	}
	return ""
}
