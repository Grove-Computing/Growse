// Package dom defines Growse's browser-owned document model.
package dom

// NodeID uniquely identifies a node within one document.
type NodeID uint64

// NodeType describes the role of a DOM node.
type NodeType uint8

const (
	NodeDocument NodeType = iota
	NodeElement
	NodeText
)

// Node is one item in a Growse DOM tree.
type Node struct {
	ID NodeID

	Type NodeType

	TagName string
	Text    string

	Attributes map[string]string

	// ControlValue and ControlChecked hold browser-owned live form state. The
	// corresponding HTML attributes remain the reset defaults.
	ControlValue        string
	ControlValueDirty   bool
	ControlChecked      bool
	ControlCheckedDirty bool

	Parent   *Node
	Children []*Node

	document *Document
}

// Attribute returns an attribute value from an element node.
func (n *Node) Attribute(name string) (string, bool) {
	if n == nil {
		return "", false
	}
	value, ok := n.Attributes[name]
	return value, ok
}

// TextContent returns the concatenated text of the node and its descendants.
func (n *Node) TextContent() string {
	if n == nil {
		return ""
	}
	if n.Type == NodeText {
		return n.Text
	}

	var result string
	for _, child := range n.Children {
		result += child.TextContent()
	}
	return result
}
