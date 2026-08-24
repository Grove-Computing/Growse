package devtools

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Grove-Computing/Growse/internal/dom"
	layoutmodel "github.com/Grove-Computing/Growse/internal/layout"
	stylemodel "github.com/Grove-Computing/Growse/internal/style"
)

const (
	MaxDOMNodes       = 2000
	MaxDOMDepth       = 128
	MaxDOMAttributes  = 64
	MaxInspectorBytes = 4 * 1024
)

// Attribute is one sorted, bounded public DOM attribute.
type Attribute struct {
	Name  string
	Value string
}

// DOMNode is a flattened, read-only DOM tree row.
type DOMNode struct {
	ID         dom.NodeID
	ParentID   dom.NodeID
	Depth      int
	Kind       string
	Name       string
	Text       string
	Attributes []Attribute
}

// StyleProperty is one human-readable computed style value.
type StyleProperty struct {
	Name  string
	Value string
}

// LayoutBox is the selected node's document-coordinate bounding box.
type LayoutBox struct {
	X, Y, Width, Height float32
}

// InspectorSnapshot is a bounded point-in-time view of the active document.
type InspectorSnapshot struct {
	Nodes        []DOMNode
	Selected     dom.NodeID
	SelectedNode *DOMNode
	Styles       []StyleProperty
	Layout       *LayoutBox
	Truncated    bool
}

// SnapshotInspector creates a bounded snapshot without retaining browser-owned objects.
func SnapshotInspector(document *dom.Document, styles stylemodel.Map, tree *layoutmodel.Tree, selected dom.NodeID) InspectorSnapshot {
	if document == nil || document.Root == nil {
		return InspectorSnapshot{}
	}
	type pendingNode struct {
		node     *dom.Node
		parentID dom.NodeID
		depth    int
	}
	pending := []pendingNode{{node: document.Root}}
	snapshot := InspectorSnapshot{Nodes: make([]DOMNode, 0, min(document.NodeCount(), MaxDOMNodes))}
	for len(pending) > 0 && len(snapshot.Nodes) < MaxDOMNodes {
		index := len(pending) - 1
		item := pending[index]
		pending = pending[:index]
		if item.node == nil || item.depth > MaxDOMDepth {
			snapshot.Truncated = true
			continue
		}
		row := snapshotDOMNode(item.node, item.parentID, item.depth)
		snapshot.Nodes = append(snapshot.Nodes, row)
		for childIndex := len(item.node.Children) - 1; childIndex >= 0; childIndex-- {
			pending = append(pending, pendingNode{node: item.node.Children[childIndex], parentID: item.node.ID, depth: item.depth + 1})
		}
	}
	if len(pending) > 0 {
		snapshot.Truncated = true
	}
	for index := range snapshot.Nodes {
		if snapshot.Nodes[index].ID != selected {
			continue
		}
		snapshot.Selected = selected
		selectedCopy := snapshot.Nodes[index]
		snapshot.SelectedNode = &selectedCopy
		if node, ok := document.NodeByID(selected); ok && document.IsConnected(node) {
			if computed, ok := styles.For(node); ok {
				snapshot.Styles = inspectorStyles(computed)
			}
			if tree != nil {
				if bounds, ok := tree.Bounds[selected]; ok {
					snapshot.Layout = &LayoutBox{X: bounds.X, Y: bounds.Y, Width: bounds.Width, Height: bounds.Height}
				}
			}
		}
		break
	}
	return snapshot
}

func snapshotDOMNode(node *dom.Node, parentID dom.NodeID, depth int) DOMNode {
	row := DOMNode{ID: node.ID, ParentID: parentID, Depth: depth}
	switch node.Type {
	case dom.NodeDocument:
		row.Kind, row.Name = "document", "#document"
	case dom.NodeText:
		row.Kind, row.Name, row.Text = "text", "#text", truncateUTF8(node.Text, MaxInspectorBytes)
	default:
		row.Kind, row.Name = "element", truncateUTF8(node.TagName, MaxInspectorBytes)
		keys := make([]string, 0, len(node.Attributes))
		for name := range node.Attributes {
			keys = append(keys, name)
		}
		sort.Strings(keys)
		if len(keys) > MaxDOMAttributes {
			keys = keys[:MaxDOMAttributes]
		}
		password := node.TagName == "input" && strings.EqualFold(node.Attributes["type"], "password")
		for _, name := range keys {
			value := node.Attributes[name]
			if password && name == "value" {
				value = "[REDACTED]"
			}
			row.Attributes = append(row.Attributes, Attribute{Name: truncateUTF8(name, MaxInspectorBytes), Value: truncateUTF8(value, MaxInspectorBytes)})
		}
	}
	return row
}

func inspectorStyles(style stylemodel.ComputedStyle) []StyleProperty {
	return []StyleProperty{
		{Name: "display", Value: displayName(style.Display)},
		{Name: "position", Value: positionName(style.Position)},
		{Name: "font-size", Value: fmt.Sprintf("%.2fpx", style.FontSize)},
		{Name: "font-weight", Value: fmt.Sprint(style.FontWeight)},
		{Name: "line-height", Value: fmt.Sprintf("%.2fpx", style.LineHeight)},
		{Name: "color", Value: fmt.Sprintf("#%08x", style.Color)},
		{Name: "background-color", Value: fmt.Sprintf("#%08x", style.BackgroundColor)},
		{Name: "opacity", Value: fmt.Sprintf("%.3g", style.Opacity)},
		{Name: "margin", Value: edgesValue(style.Margin)},
		{Name: "padding", Value: edgesValue(style.Padding)},
		{Name: "z-index", Value: fmt.Sprint(style.ZIndex)},
	}
}

func displayName(value stylemodel.Display) string {
	names := [...]string{"inline", "block", "inline-block", "none", "flex", "inline-flex", "grid", "inline-grid"}
	if int(value) >= 0 && int(value) < len(names) {
		return names[value]
	}
	return "unknown"
}

func positionName(value stylemodel.Position) string {
	names := [...]string{"static", "relative", "absolute", "fixed", "sticky"}
	if int(value) >= 0 && int(value) < len(names) {
		return names[value]
	}
	return "unknown"
}

func edgesValue(edges stylemodel.Edges) string {
	return fmt.Sprintf("%.2fpx %.2fpx %.2fpx %.2fpx", edges.Top, edges.Right, edges.Bottom, edges.Left)
}
