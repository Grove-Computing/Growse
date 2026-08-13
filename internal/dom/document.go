package dom

import (
	"errors"
	"strings"
)

// Document owns a DOM tree and its node indexes.
type Document struct {
	Root *Node

	byID   map[string]*Node
	nodes  map[NodeID]*Node
	nextID NodeID
}

// NewDocument creates an empty document with a document root node.
func NewDocument() *Document {
	document := &Document{
		byID:   make(map[string]*Node),
		nodes:  make(map[NodeID]*Node),
		nextID: 1,
	}
	document.Root = document.newNode(NodeDocument)
	return document
}

// NodeByID returns the node with the internal document-scoped identifier.
func (d *Document) NodeByID(id NodeID) (*Node, bool) {
	if d == nil {
		return nil, false
	}
	node, ok := d.nodes[id]
	return node, ok
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
	d.rebuildIDIndex()
	return nil
}

// IsConnected はノードがDocumentのルートツリーに接続されているかを返す。
func (d *Document) IsConnected(node *Node) bool {
	if d == nil || node == nil || node.document != d || d.Root == nil {
		return false
	}
	for current := node; current != nil; current = current.Parent {
		if current == d.Root {
			return true
		}
	}
	return false
}

// GetElementByID returns the first element with the given id attribute.
func (d *Document) GetElementByID(id string) (*Node, bool) {
	node, ok := d.byID[id]
	return node, ok
}

// QuerySelector は対応する単純セレクターに最初に一致する要素を返す。
func (d *Document) QuerySelector(value string) (*Node, bool) {
	if d == nil || d.Root == nil {
		return nil, false
	}
	selector, ok := parseSimpleSelector(strings.TrimSpace(value))
	if !ok {
		return nil, false
	}
	return querySelector(d.Root, selector)
}

// SetTextContent は指定したノードの子を1つのテキストノードへ置き換える。
func (d *Document) SetTextContent(id NodeID, value string) bool {
	if d == nil {
		return false
	}
	node, ok := d.nodes[id]
	if !ok || node.Type != NodeElement {
		return false
	}
	for _, child := range node.Children {
		d.removeSubtree(child)
	}
	node.Children = nil
	text := d.CreateText(value)
	text.Parent = node
	node.Children = append(node.Children, text)
	d.rebuildIDIndex()
	return true
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
	d.nodes[node.ID] = node
	return node
}

func (d *Document) removeSubtree(node *Node) {
	if node == nil {
		return
	}
	for _, child := range node.Children {
		d.removeSubtree(child)
	}
	delete(d.nodes, node.ID)
	node.Children = nil
	node.Parent = nil
}

func (d *Document) rebuildIDIndex() {
	d.byID = make(map[string]*Node)
	var walk func(*Node)
	walk = func(node *Node) {
		if node == nil {
			return
		}
		if id, ok := node.Attribute("id"); ok && id != "" {
			if _, exists := d.byID[id]; !exists {
				d.byID[id] = node
			}
		}
		for _, child := range node.Children {
			walk(child)
		}
	}
	walk(d.Root)
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

type simpleSelector struct {
	tag   string
	id    string
	class string
}

func parseSimpleSelector(value string) (simpleSelector, bool) {
	if value == "" || strings.ContainsAny(value, " >+~[:*") {
		return simpleSelector{}, false
	}
	if strings.HasPrefix(value, "#") && validSelectorName(value[1:]) {
		return simpleSelector{id: value[1:]}, true
	}
	if strings.HasPrefix(value, ".") && validSelectorName(value[1:]) {
		return simpleSelector{class: value[1:]}, true
	}
	if index := strings.IndexByte(value, '.'); index > 0 && strings.Count(value, ".") == 1 {
		tag, class := strings.ToLower(value[:index]), value[index+1:]
		if validSelectorName(tag) && validSelectorName(class) {
			return simpleSelector{tag: tag, class: class}, true
		}
		return simpleSelector{}, false
	}
	if validSelectorName(value) {
		return simpleSelector{tag: strings.ToLower(value)}, true
	}
	return simpleSelector{}, false
}

func validSelectorName(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if character != '-' && character != '_' &&
			(character < 'a' || character > 'z') &&
			(character < 'A' || character > 'Z') &&
			(character < '0' || character > '9') {
			return false
		}
	}
	return true
}

func querySelector(node *Node, selector simpleSelector) (*Node, bool) {
	if matchesSimpleSelector(node, selector) {
		return node, true
	}
	for _, child := range node.Children {
		if found, ok := querySelector(child, selector); ok {
			return found, true
		}
	}
	return nil, false
}

func matchesSimpleSelector(node *Node, selector simpleSelector) bool {
	if node == nil || node.Type != NodeElement {
		return false
	}
	if selector.tag != "" && node.TagName != selector.tag {
		return false
	}
	if selector.id != "" {
		id, _ := node.Attribute("id")
		if id != selector.id {
			return false
		}
	}
	if selector.class != "" {
		classes, _ := node.Attribute("class")
		for _, class := range strings.Fields(classes) {
			if class == selector.class {
				return true
			}
		}
		return false
	}
	return true
}
