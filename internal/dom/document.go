package dom

import (
	"errors"
	"strings"
)

// Document owns a DOM tree and its node indexes.
type Document struct {
	Root       *Node
	readyState string

	byID   map[string]*Node
	nodes  map[NodeID]*Node
	nextID NodeID
}

// NewDocument creates an empty document with a document root node.
func NewDocument() *Document {
	document := &Document{
		byID:       make(map[string]*Node),
		nodes:      make(map[NodeID]*Node),
		nextID:     1,
		readyState: "loading",
	}
	document.Root = document.newNode(NodeDocument)
	return document
}

// ReadyState reports the document loading lifecycle exposed to scripts.
func (d *Document) ReadyState() string {
	if d == nil || d.readyState == "" {
		return "loading"
	}
	return d.readyState
}

// SetReadyState advances the document loading lifecycle.
func (d *Document) SetReadyState(state string) bool {
	if d == nil || state != "loading" && state != "interactive" && state != "complete" || d.ReadyState() == state {
		return false
	}
	d.readyState = state
	return true
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
	index := 0
	if parent != nil {
		index = len(parent.Children)
	}
	return d.InsertChild(parent, child, index)
}

// InsertChild attaches or moves child beneath parent at index.
func (d *Document) InsertChild(parent, child *Node, index int) error {
	if parent == nil || child == nil {
		return errors.New("parent and child must not be nil")
	}
	if parent.document != d || child.document != d {
		return errors.New("parent and child must belong to the document")
	}
	if parent.Type != NodeDocument && parent.Type != NodeElement || child == d.Root || index < 0 || index > len(parent.Children) {
		return errors.New("invalid child insertion")
	}
	for current := parent; current != nil; current = current.Parent {
		if current == child {
			return errors.New("child insertion would create a cycle")
		}
	}
	if child.Parent != nil {
		oldParent := child.Parent
		oldIndex := childIndex(oldParent, child)
		if oldIndex < 0 {
			return errors.New("child parent relationship is invalid")
		}
		oldParent.Children = append(oldParent.Children[:oldIndex], oldParent.Children[oldIndex+1:]...)
		if oldParent == parent && oldIndex < index {
			index--
		}
	}
	child.Parent = parent
	parent.Children = append(parent.Children, nil)
	copy(parent.Children[index+1:], parent.Children[index:])
	parent.Children[index] = child
	d.rebuildIDIndex()
	return nil
}

// DetachChild removes a direct child without destroying its subtree identity.
func (d *Document) DetachChild(parent, child *Node) bool {
	if d == nil || parent == nil || child == nil || parent.document != d || child.document != d || child.Parent != parent {
		return false
	}
	index := childIndex(parent, child)
	if index < 0 {
		return false
	}
	parent.Children = append(parent.Children[:index], parent.Children[index+1:]...)
	child.Parent = nil
	d.rebuildIDIndex()
	return true
}

// ReplaceChildren atomically moves children beneath parent and detaches the old list.
func (d *Document) ReplaceChildren(parent *Node, children []*Node) bool {
	if d == nil || parent == nil || parent.document != d || parent.Type != NodeElement {
		return false
	}
	seen := make(map[NodeID]bool, len(children))
	for _, child := range children {
		if child == nil || child.document != d || child == d.Root || seen[child.ID] {
			return false
		}
		seen[child.ID] = true
		for current := parent; current != nil; current = current.Parent {
			if current == child {
				return false
			}
		}
	}
	for _, child := range children {
		if child.Parent != nil {
			oldParent := child.Parent
			index := childIndex(oldParent, child)
			if index >= 0 {
				oldParent.Children = append(oldParent.Children[:index], oldParent.Children[index+1:]...)
			}
			child.Parent = nil
		}
	}
	for _, child := range parent.Children {
		child.Parent = nil
	}
	parent.Children = append([]*Node(nil), children...)
	for _, child := range parent.Children {
		child.Parent = parent
	}
	d.rebuildIDIndex()
	return true
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

// Remove は接続済み要素とその子孫をDocumentから削除し、削除したNodeIDを返す。
func (d *Document) Remove(id NodeID) ([]NodeID, bool) {
	if d == nil {
		return nil, false
	}
	node, ok := d.nodes[id]
	if !ok || node == d.Root || node.Type != NodeElement || !d.IsConnected(node) || node.Parent == nil {
		return nil, false
	}
	parent := node.Parent
	index := -1
	for childIndex, child := range parent.Children {
		if child == node {
			index = childIndex
			break
		}
	}
	if index < 0 {
		return nil, false
	}
	removed := collectNodeIDs(node, nil)
	parent.Children = append(parent.Children[:index], parent.Children[index+1:]...)
	d.removeSubtree(node)
	d.rebuildIDIndex()
	return removed, true
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

// QuerySelectorAll returns all connected elements matching a supported selector.
func (d *Document) QuerySelectorAll(value string) []*Node {
	if d == nil || d.Root == nil {
		return nil
	}
	selector, ok := parseSimpleSelector(strings.TrimSpace(value))
	if !ok {
		return nil
	}
	return querySelectorAll(d.Root, selector, nil)
}

// GetElementsByClassName returns connected elements containing every class token.
func (d *Document) GetElementsByClassName(value string) []*Node {
	classes := strings.Fields(value)
	if d == nil || d.Root == nil || len(classes) == 0 {
		return nil
	}
	return collectElements(d.Root, nil, func(node *Node) bool {
		present := make(map[string]bool)
		value, _ := node.Attribute("class")
		for _, class := range strings.Fields(value) {
			present[class] = true
		}
		for _, class := range classes {
			if !present[class] {
				return false
			}
		}
		return true
	})
}

// GetElementsByTagName returns connected elements with the requested tag or all for "*".
func (d *Document) GetElementsByTagName(value string) []*Node {
	value = strings.ToLower(strings.TrimSpace(value))
	if d == nil || d.Root == nil || value == "" || value != "*" && !validSelectorName(value) {
		return nil
	}
	return collectElements(d.Root, nil, func(node *Node) bool { return value == "*" || node.TagName == value })
}

// SetAttribute は要素の属性を設定し、値が変化した場合にtrueを返す。
func (d *Document) SetAttribute(id NodeID, name, value string) bool {
	if d == nil || name == "" {
		return false
	}
	node, ok := d.nodes[id]
	if !ok || node.Type != NodeElement {
		return false
	}
	if current, exists := node.Attribute(name); exists && current == value {
		return false
	}
	if node.Attributes == nil {
		node.Attributes = make(map[string]string)
	}
	node.Attributes[name] = value
	if name == "id" {
		d.rebuildIDIndex()
	}
	return true
}

// RemoveAttribute removes an element attribute and reports whether it existed.
func (d *Document) RemoveAttribute(id NodeID, name string) bool {
	if d == nil || name == "" {
		return false
	}
	node, ok := d.nodes[id]
	if !ok || node.Type != NodeElement || node.Attributes == nil {
		return false
	}
	if _, exists := node.Attributes[name]; !exists {
		return false
	}
	delete(node.Attributes, name)
	if name == "id" {
		d.rebuildIDIndex()
	}
	return true
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

// OwnedNodeCount includes detached nodes retained by live Runtime handles.
func (d *Document) OwnedNodeCount() int {
	if d == nil {
		return 0
	}
	return len(d.nodes)
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

func collectNodeIDs(node *Node, ids []NodeID) []NodeID {
	if node == nil {
		return ids
	}
	ids = append(ids, node.ID)
	for _, child := range node.Children {
		ids = collectNodeIDs(child, ids)
	}
	return ids
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

func querySelectorAll(node *Node, selector simpleSelector, result []*Node) []*Node {
	if matchesSimpleSelector(node, selector) {
		result = append(result, node)
	}
	for _, child := range node.Children {
		result = querySelectorAll(child, selector, result)
	}
	return result
}

func collectElements(node *Node, result []*Node, matches func(*Node) bool) []*Node {
	if node == nil {
		return result
	}
	if node.Type == NodeElement && matches(node) {
		result = append(result, node)
	}
	for _, child := range node.Children {
		result = collectElements(child, result, matches)
	}
	return result
}

func childIndex(parent, child *Node) int {
	if parent == nil || child == nil {
		return -1
	}
	for index, candidate := range parent.Children {
		if candidate == child {
			return index
		}
	}
	return -1
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
