package dom

import (
	"errors"
	"strings"
)

// Document owns a DOM tree and its node indexes.
type Document struct {
	Root *Node

	byID   map[string]*Node
	nextID NodeID
}

// NewDocument creates an empty document with a document root node.
func NewDocument() *Document {
	document := &Document{
		byID:   make(map[string]*Node),
		nextID: 1,
	}
	document.Root = document.newNode(NodeDocument)
	return document
}

// CreateElement creates an unattached element owned by the document.
func (d *Document) CreateElement(tagName string, attributes map[string]string) *Node {
	node := d.newNode(NodeElement)
	node.TagName = strings.ToLower(tagName)
	node.Attributes = cloneAttributes(attributes)
	return node
}

// CreateText creates an unattached text node owned by the document.
func (d *Document) CreateText(text string) *Node {
	node := d.newNode(NodeText)
	node.Text = text
	return node
}

// AppendChild attaches child beneath parent.
func (d *Document) AppendChild(parent, child *Node) error {
	if parent == nil || child == nil {
		return errors.New("parent and child must not be nil")
	}
	if parent.document != d || child.document != d {
		return errors.New("parent and child must belong to the document")
	}
	if child.Parent != nil {
		return errors.New("child already has a parent")
	}

	child.Parent = parent
	parent.Children = append(parent.Children, child)
	if id, ok := child.Attribute("id"); ok && id != "" {
		if _, exists := d.byID[id]; !exists {
			d.byID[id] = child
		}
	}
	return nil
}

// GetElementByID returns the first element with the given id attribute.
func (d *Document) GetElementByID(id string) (*Node, bool) {
	node, ok := d.byID[id]
	return node, ok
}

// NodeCount returns the number of nodes, including the document root.
func (d *Document) NodeCount() int {
	return countNodes(d.Root)
}

// ElementCount returns the number of element nodes.
func (d *Document) ElementCount() int {
	return countElements(d.Root)
}

// Title returns the normalized text of the first title element.
func (d *Document) Title() string {
	title := findElement(d.Root, "title")
	if title == nil {
		return ""
	}
	return strings.TrimSpace(title.TextContent())
}

func (d *Document) newNode(nodeType NodeType) *Node {
	node := &Node{ID: d.nextID, Type: nodeType, document: d}
	d.nextID++
	return node
}

func cloneAttributes(attributes map[string]string) map[string]string {
	if len(attributes) == 0 {
		return nil
	}
	result := make(map[string]string, len(attributes))
	for name, value := range attributes {
		result[name] = value
	}
	return result
}

func countNodes(node *Node) int {
	if node == nil {
		return 0
	}
	count := 1
	for _, child := range node.Children {
		count += countNodes(child)
	}
	return count
}

func countElements(node *Node) int {
	if node == nil {
		return 0
	}
	count := 0
	if node.Type == NodeElement {
		count = 1
	}
	for _, child := range node.Children {
		count += countElements(child)
	}
	return count
}

func findElement(node *Node, tagName string) *Node {
	if node == nil {
		return nil
	}
	if node.Type == NodeElement && node.TagName == tagName {
		return node
	}
	for _, child := range node.Children {
		if found := findElement(child, tagName); found != nil {
			return found
		}
	}
	return nil
}
