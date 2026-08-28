// Package dom はWebGoスクリプト向けのDOM APIを提供する。
package dom

import (
	"sort"
	"strings"

	"github.com/Grove-Computing/Growse/internal/css"
	dommodel "github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/events"
	"github.com/Grove-Computing/Growse/internal/forms"
	htmlparser "github.com/Grove-Computing/Growse/internal/html"
	stylemodel "github.com/Grove-Computing/Growse/internal/style"
)

const (
	maxDOMCollectionResults = 4_096
	maxDOMInnerHTMLBytes    = 256 << 10
	maxDOMOwnedNodes        = dommodel.MaxNodesPerDocument
	maxDOMTextBytes         = dommodel.MaxDOMStringBytes
)

// API は1ページのDocumentへアクセスする。
type API struct {
	document   *dommodel.Document
	events     *events.Dispatcher
	onMutation func()
}

// Element はDocument内の要素をNodeIDで参照する。
type Element struct {
	document   *dommodel.Document
	id         dommodel.NodeID
	events     *events.Dispatcher
	onMutation func()
}

// Event はWebGoへ公開するDOMイベント情報である。
type Event struct {
	Type             string
	TargetID         string
	TargetNodeID     dommodel.NodeID
	CurrentTargetID  dommodel.NodeID
	Value            string
	X                float32
	Y                float32
	Bubbles          bool
	Cancelable       bool
	DefaultPrevented func() bool
	Phase            events.Phase
	RuntimeID        uint64
	preventDefault   func()
	stopPropagation  func()
	stopImmediate    func()
}

// ID はElementが参照するPage内Node IDを返す。
func (element *Element) ID() dommodel.NodeID {
	if element == nil {
		return 0
	}
	return element.id
}

// NodeType returns the browser-owned node kind.
func (element *Element) NodeType() dommodel.NodeType {
	node, ok := element.node()
	if !ok {
		return dommodel.NodeDocument
	}
	return node.Type
}

// NodeName returns the DOM-facing node name.
func (element *Element) NodeName() string {
	node, ok := element.node()
	if !ok {
		return ""
	}
	switch node.Type {
	case dommodel.NodeDocument:
		return "#document"
	case dommodel.NodeDocumentFragment:
		return "#document-fragment"
	case dommodel.NodeText:
		return "#text"
	case dommodel.NodeElement:
		return strings.ToUpper(node.TagName)
	default:
		return ""
	}
}

// PreventDefault cancels a cancelable browser default action such as form submission.
func (event Event) PreventDefault() {
	if event.preventDefault != nil {
		event.preventDefault()
	}
}

// StopPropagation prevents this event from reaching later nodes on its path.
func (event Event) StopPropagation() {
	if event.stopPropagation != nil {
		event.stopPropagation()
	}
}

// StopImmediatePropagation prevents later listeners and further propagation.
func (event Event) StopImmediatePropagation() {
	if event.stopImmediate != nil {
		event.stopImmediate()
	}
}

// New はDocumentに結び付いたDOM APIを生成する。
func New(document *dommodel.Document, dispatcher *events.Dispatcher, onMutation func()) *API {
	return &API{document: document, events: dispatcher, onMutation: onMutation}
}

// GetElementByID は指定したidを持つ最初の要素を返す。
func (api *API) GetElementByID(id string) *Element {
	if api == nil || api.document == nil {
		return nil
	}
	node, ok := api.document.GetElementByID(id)
	if !ok || node.Type != dommodel.NodeElement {
		return nil
	}
	return api.element(node)
}

// QuerySelector は対応する単純セレクターに最初に一致する要素を返す。
func (api *API) QuerySelector(selector string) *Element {
	if api == nil || api.document == nil {
		return nil
	}
	selectors := css.ParseSelectorList(selector)
	node := firstMatchingDescendant(api.document.Root, selectors, true, documentScope(api.document.Root))
	if node == nil {
		return nil
	}
	return api.element(node)
}

// QuerySelectorAll returns a bounded static collection for a supported selector.
func (api *API) QuerySelectorAll(selector string) []*Element {
	if api == nil || api.document == nil {
		return nil
	}
	selectors := css.ParseSelectorList(selector)
	return api.elements(matchingDescendants(api.document.Root, selectors, true, nil, documentScope(api.document.Root)))
}

// GetElementsByClassName returns a bounded static collection for class tokens.
func (api *API) GetElementsByClassName(classNames string) []*Element {
	if api == nil || api.document == nil {
		return nil
	}
	return api.elements(api.document.GetElementsByClassName(classNames))
}

// GetElementsByTagName returns a bounded static collection for a tag name.
func (api *API) GetElementsByTagName(tagName string) []*Element {
	if api == nil || api.document == nil {
		return nil
	}
	return api.elements(api.document.GetElementsByTagName(tagName))
}

// DocumentElement returns the connected root HTML element.
func (api *API) DocumentElement() *Element { return api.firstConnectedElement("html") }

// Head returns the connected head element.
func (api *API) Head() *Element { return api.firstConnectedElement("head") }

// Body returns the connected body element.
func (api *API) Body() *Element { return api.firstConnectedElement("body") }

func (api *API) firstConnectedElement(tagName string) *Element {
	if api == nil || api.document == nil {
		return nil
	}
	nodes := api.document.GetElementsByTagName(tagName)
	if len(nodes) == 0 {
		return nil
	}
	return api.element(nodes[0])
}

// ElementByNodeID returns a connected node handle by its document-scoped identity.
func (api *API) ElementByNodeID(id dommodel.NodeID) *Element {
	if api == nil || api.document == nil || id == 0 {
		return nil
	}
	node, ok := api.document.NodeByID(id)
	if !ok || !api.document.IsConnected(node) {
		return nil
	}
	return api.element(node)
}

// NodeByID returns a handle for connected or detached nodes owned by this Document.
func (api *API) NodeByID(id dommodel.NodeID) *Element {
	if api == nil || api.document == nil || id == 0 {
		return nil
	}
	node, ok := api.document.NodeByID(id)
	if !ok {
		return nil
	}
	return api.element(node)
}

// CreateElement はDocumentが所有する未接続の要素を作成する。
func (api *API) CreateElement(tagName string) *Element {
	if api == nil || api.document == nil || api.document.OwnedNodeCount() >= maxDOMOwnedNodes {
		return nil
	}
	tagName = strings.TrimSpace(tagName)
	if !validTagName(tagName) {
		return nil
	}
	if strings.EqualFold(tagName, "template") && api.document.OwnedNodeCount()+2 > maxDOMOwnedNodes {
		return nil
	}
	node := api.document.CreateElement(tagName, nil)
	if strings.EqualFold(tagName, "template") {
		_ = api.document.AppendChild(node, api.document.CreateDocumentFragment())
	}
	return api.element(node)
}

// CreateElementNS supports the HTML and SVG namespaces used by hydration fixtures.
func (api *API) CreateElementNS(namespace, tagName string) *Element {
	namespace = strings.TrimSpace(namespace)
	if namespace != "" && namespace != "http://www.w3.org/1999/xhtml" && namespace != "http://www.w3.org/2000/svg" {
		return nil
	}
	return api.CreateElement(tagName)
}

// CreateDocumentFragment returns a detached fragment owned by this Document.
func (api *API) CreateDocumentFragment() *Element {
	if api == nil || api.document == nil || api.document.OwnedNodeCount() >= maxDOMOwnedNodes {
		return nil
	}
	return api.element(api.document.CreateDocumentFragment())
}

// ImportNode clones a node into this Document without connecting it.
func (api *API) ImportNode(source *Element, deep bool) *Element {
	if api == nil || api.document == nil || source == nil || source.document != api.document {
		return nil
	}
	node, ok := source.node()
	if !ok || node.Type == dommodel.NodeDocument {
		return nil
	}
	needed := 1
	if deep {
		needed = countSubtreeNodes(node)
	}
	if needed > maxDOMCollectionResults || api.document.OwnedNodeCount()+needed > maxDOMOwnedNodes {
		return nil
	}
	return api.element(cloneNode(api.document, node, deep))
}

// CreateTextNode creates a bounded detached text node owned by the Document.
func (api *API) CreateTextNode(value string) *Element {
	if api == nil || api.document == nil || len(value) > maxDOMTextBytes || api.document.OwnedNodeCount() >= maxDOMOwnedNodes {
		return nil
	}
	return api.element(api.document.CreateText(value))
}

// OnClick は要素のクリックイベントへハンドラーを登録する。
func (element *Element) OnClick(handler func()) {
	if handler == nil {
		return
	}
	element.addEventListener(events.Click, func(events.Event) {
		handler()
	})
}

// OnClickEvent はクリックイベント情報を受け取るハンドラーを登録する。
func (element *Element) OnClickEvent(handler func(Event)) {
	if handler == nil {
		return
	}
	element.addEventListener(events.Click, func(event events.Event) {
		handler(element.publicEvent(event))
	})
}

// OnInput はユーザー操作による入力値変更のハンドラーを登録する。
func (element *Element) OnInput(handler func(Event)) {
	if handler == nil {
		return
	}
	element.addEventListener(events.Input, func(event events.Event) {
		handler(element.publicEvent(event))
	})
}

// OnChange は入力の編集確定ハンドラーを登録する。
func (element *Element) OnChange(handler func(Event)) {
	if handler == nil {
		return
	}
	element.addEventListener(events.Change, func(event events.Event) {
		handler(element.publicEvent(event))
	})
}

// OnSubmit はformの送信操作ハンドラーを登録する。
func (element *Element) OnSubmit(handler func(Event)) {
	if handler == nil {
		return
	}
	element.addEventListener(events.Submit, func(event events.Event) {
		handler(element.publicEvent(event))
	})
}

// OnReset はformのresetハンドラーを登録する。
func (element *Element) OnReset(handler func(Event)) {
	if handler == nil {
		return
	}
	element.addEventListener(events.Reset, func(event events.Event) {
		handler(element.publicEvent(event))
	})
}

// OnFocus は要素がfocusを得たハンドラーを登録する。
func (element *Element) OnFocus(handler func(Event)) {
	if handler == nil {
		return
	}
	element.addEventListener(events.Focus, func(event events.Event) {
		handler(element.publicEvent(event))
	})
}

// OnBlur は要素がfocusを失ったハンドラーを登録する。
func (element *Element) OnBlur(handler func(Event)) {
	if handler == nil {
		return
	}
	element.addEventListener(events.Blur, func(event events.Event) {
		handler(element.publicEvent(event))
	})
}

// OnMouseEnter はポインターが要素へ入ったときのハンドラーを登録する。
func (element *Element) OnMouseEnter(handler func(Event)) {
	if handler == nil {
		return
	}
	element.addEventListener(events.MouseEnter, func(event events.Event) {
		handler(element.publicEvent(event))
	})
}

// OnMouseLeave はポインターが要素から外れたときのハンドラーを登録する。
func (element *Element) OnMouseLeave(handler func(Event)) {
	if handler == nil {
		return
	}
	element.addEventListener(events.MouseLeave, func(event events.Event) {
		handler(element.publicEvent(event))
	})
}

// AddEventListener はJavaScript hostを含むRuntime adapter向けに対応Eventを登録する。
func (element *Element) AddEventListener(eventType string, handler func(Event)) bool {
	return element.AddEventListenerWithCapture(eventType, false, handler) != 0
}

// AddEventListenerWithCapture registers a removable listener with propagation options.
func (element *Element) AddEventListenerWithCapture(eventType string, capture bool, handler func(Event)) events.ListenerID {
	return element.addPublicEventListener(eventType, capture, handler, true)
}

// AddEventListenerForJavaScript registers EventTarget listeners on Document-owned
// detached nodes so JavaScript can attach handlers before connecting an element.
func (element *Element) AddEventListenerForJavaScript(eventType string, capture bool, handler func(Event)) events.ListenerID {
	return element.AddEventListenerForJavaScriptWithOptions(eventType, capture, false, false, handler)
}

// AddEventListenerForJavaScriptWithOptions registers a detached-capable JS listener.
func (element *Element) AddEventListenerForJavaScriptWithOptions(eventType string, capture, once, passive bool, handler func(Event)) events.ListenerID {
	return element.addPublicEventListenerWithOptions(eventType, capture, once, passive, handler, false)
}

func (element *Element) addPublicEventListener(eventType string, capture bool, handler func(Event), requireConnected bool) events.ListenerID {
	return element.addPublicEventListenerWithOptions(eventType, capture, false, false, handler, requireConnected)
}

func (element *Element) addPublicEventListenerWithOptions(eventType string, capture, once, passive bool, handler func(Event), requireConnected bool) events.ListenerID {
	if handler == nil {
		return 0
	}
	internal, ok := supportedEventType(eventType)
	if !ok {
		return 0
	}
	return element.addEventListenerWithConnectionPolicy(internal, capture, once, passive, requireConnected, func(event events.Event) {
		handler(element.publicEvent(event))
	})
}

// RemoveEventListener removes one listener token returned by AddEventListenerWithCapture.
func (element *Element) RemoveEventListener(eventType string, id events.ListenerID) bool {
	internal, ok := supportedEventType(eventType)
	if !ok || element == nil || element.events == nil {
		return false
	}
	return element.events.RemoveEventListener(element.id, internal, id)
}

func supportedEventType(eventType string) (events.Type, bool) {
	switch strings.ToLower(strings.TrimSpace(eventType)) {
	case string(events.Click):
		return events.Click, true
	case string(events.Input):
		return events.Input, true
	case string(events.Change):
		return events.Change, true
	case string(events.Submit):
		return events.Submit, true
	case string(events.Reset):
		return events.Reset, true
	case string(events.Focus):
		return events.Focus, true
	case string(events.Blur):
		return events.Blur, true
	case string(events.MouseEnter):
		return events.MouseEnter, true
	case string(events.MouseLeave):
		return events.MouseLeave, true
	case string(events.Load):
		return events.Load, true
	case string(events.Error):
		return events.Error, true
	default:
		if eventType == "" || len(eventType) > 128 {
			return "", false
		}
		for _, character := range eventType {
			if character <= ' ' || character == 0x7f {
				return "", false
			}
		}
		return events.Type(eventType), true
	}
}

// AppendChild は未接続の子要素を接続済みの要素の末尾へ追加する。
func (element *Element) AppendChild(child *Element) bool {
	return element.Append(child)
}

// IsConnected reports whether this handle currently participates in its Document tree.
func (element *Element) IsConnected() bool {
	node, ok := element.node()
	return ok && element.document.IsConnected(node)
}

// Append moves nodes to the end of this element's child list.
func (element *Element) Append(children ...*Element) bool {
	return element.insert(false, children...)
}

// Prepend moves nodes to the beginning of this element's child list.
func (element *Element) Prepend(children ...*Element) bool {
	return element.insert(true, children...)
}

// RemoveChild detaches a direct child while retaining its Runtime identity.
func (element *Element) RemoveChild(child *Element) bool {
	parentNode, childNode, ok := element.relationship(child)
	if !ok || !element.document.DetachChild(parentNode, childNode) {
		return false
	}
	element.notifyMutation()
	return true
}

// InsertBefore inserts or moves child immediately before reference. A nil reference appends.
func (element *Element) InsertBefore(child, reference *Element) bool {
	parent, nodes, ok := element.mutationNodes([]*Element{child})
	if !ok {
		return false
	}
	var referenceNode *dommodel.Node
	if reference != nil {
		if reference.document != element.document {
			return false
		}
		referenceNode, ok = reference.node()
		if !ok || referenceNode.Parent != parent {
			return false
		}
		if len(nodes) == 1 && nodes[0] == referenceNode {
			return true
		}
	}
	wanted := withoutNodes(parent.Children, nodes, nil)
	index := len(wanted)
	if referenceNode != nil {
		index = nodeIndex(wanted, referenceNode)
		if index < 0 {
			return false
		}
	}
	wanted = insertNodes(wanted, index, nodes)
	if !element.document.ReplaceChildren(parent, wanted) {
		return false
	}
	element.notifyMutation()
	return true
}

// ReplaceChild replaces a direct child while preserving the removed subtree identity.
func (element *Element) ReplaceChild(child, replaced *Element) bool {
	if replaced == nil || replaced.document != element.document {
		return false
	}
	parent, nodes, ok := element.mutationNodes([]*Element{child})
	if !ok {
		return false
	}
	replacedNode, ok := replaced.node()
	if !ok || replacedNode.Parent != parent {
		return false
	}
	if len(nodes) == 1 && nodes[0] == replacedNode {
		return true
	}
	index := 0
	for _, current := range parent.Children {
		if current == replacedNode {
			break
		}
		if !nodeIncluded(nodes, current) {
			index++
		}
	}
	wanted := withoutNodes(parent.Children, nodes, replacedNode)
	wanted = insertNodes(wanted, index, nodes)
	if !element.document.ReplaceChildren(parent, wanted) {
		return false
	}
	element.notifyMutation()
	return true
}

// ReplaceChildren atomically replaces this element's children with supplied nodes.
func (element *Element) ReplaceChildren(children ...*Element) bool {
	parent, nodes, ok := element.mutationNodes(children)
	if !ok || !element.document.ReplaceChildren(parent, nodes) {
		return false
	}
	element.notifyMutation()
	return true
}

// Remove は要素自身とその子孫をDocumentから削除する。
func (element *Element) Remove() bool {
	node, ok := element.node()
	if !ok || node == element.document.Root || node.Parent == nil {
		return false
	}
	parent := node.Parent
	if !element.document.DetachChild(parent, node) {
		return false
	}
	if element.events != nil {
		element.events.RemoveEventListeners(subtreeNodeIDs(node, nil)...)
	}
	if element.onMutation != nil {
		element.onMutation()
	}
	return true
}

// GetAttribute は要素に設定された属性値を返す。
func (element *Element) GetAttribute(name string) (string, bool) {
	if element == nil || element.document == nil {
		return "", false
	}
	name = strings.ToLower(strings.TrimSpace(name))
	if !validAttributeName(name) {
		return "", false
	}
	node, ok := element.document.NodeByID(element.id)
	if !ok || node.Type != dommodel.NodeElement {
		return "", false
	}
	return node.Attribute(name)
}

// SetAttribute は要素の属性値を変更する。
func (element *Element) SetAttribute(name, value string) bool {
	if element == nil || element.document == nil {
		return false
	}
	name = strings.ToLower(strings.TrimSpace(name))
	if !validAttributeName(name) || !element.document.SetAttribute(element.id, name, value) {
		return false
	}
	if element.onMutation != nil {
		element.onMutation()
	}
	return true
}

// RemoveAttribute removes an element attribute and notifies the active Page once.
func (element *Element) RemoveAttribute(name string) bool {
	if element == nil || element.document == nil {
		return false
	}
	name = strings.ToLower(strings.TrimSpace(name))
	if !validAttributeName(name) || !element.document.RemoveAttribute(element.id, name) {
		return false
	}
	if element.onMutation != nil {
		element.onMutation()
	}
	return true
}

// HasAttribute reports whether an attribute is present.
func (element *Element) HasAttribute(name string) bool {
	_, present := element.GetAttribute(name)
	return present
}

// ToggleAttribute adds or removes a boolean attribute and returns its resulting presence.
func (element *Element) ToggleAttribute(name string, force *bool) (bool, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	if !validAttributeName(name) {
		return false, false
	}
	present := element.HasAttribute(name)
	wanted := !present
	if force != nil {
		wanted = *force
	}
	if wanted == present {
		return present, true
	}
	if wanted {
		return true, element.SetAttribute(name, "")
	}
	return false, element.RemoveAttribute(name)
}

// AttributeNames returns a stable sorted snapshot of attribute names.
func (element *Element) AttributeNames() []string {
	node, ok := element.node()
	if !ok || node.Type != dommodel.NodeElement {
		return nil
	}
	result := make([]string, 0, len(node.Attributes))
	for name := range node.Attributes {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

// Matches evaluates a supported selector list against this element.
func (element *Element) Matches(selector string) bool {
	node, ok := element.node()
	if !ok || node.Type != dommodel.NodeElement {
		return false
	}
	return matchesAnySelector(node, css.ParseSelectorList(selector), node.ID)
}

// Closest returns this element or its nearest element ancestor matching selector.
func (element *Element) Closest(selector string) *Element {
	selectors := css.ParseSelectorList(selector)
	if len(selectors) == 0 {
		return nil
	}
	node, ok := element.node()
	if !ok {
		return nil
	}
	api := &API{document: element.document, events: element.events, onMutation: element.onMutation}
	for current := node; current != nil; current = current.Parent {
		if current.Type == dommodel.NodeDocumentFragment {
			break
		}
		if current.Type == dommodel.NodeElement && matchesAnySelector(current, selectors, node.ID) {
			return api.element(current)
		}
	}
	return nil
}

// QuerySelector returns the first matching descendant.
func (element *Element) QuerySelector(selector string) *Element {
	selectors := css.ParseSelectorList(selector)
	node, ok := element.node()
	if !ok || len(selectors) == 0 {
		return nil
	}
	match := firstMatchingDescendant(node, selectors, false, node.ID)
	return (&API{document: element.document, events: element.events, onMutation: element.onMutation}).element(match)
}

// QuerySelectorAll returns a bounded static descendant snapshot.
func (element *Element) QuerySelectorAll(selector string) []*Element {
	selectors := css.ParseSelectorList(selector)
	node, ok := element.node()
	if !ok || len(selectors) == 0 {
		return nil
	}
	return (&API{document: element.document, events: element.events, onMutation: element.onMutation}).elements(
		matchingDescendants(node, selectors, false, nil, node.ID),
	)
}

// AddClass は重複を避けてクラスを追加する。
func (element *Element) AddClass(className string) bool {
	if !validClassName(className) {
		return false
	}
	classes, _ := element.GetAttribute("class")
	for _, class := range strings.Fields(classes) {
		if class == className {
			return false
		}
	}
	if classes = strings.Join(strings.Fields(classes), " "); classes != "" {
		classes += " "
	}
	return element.SetAttribute("class", classes+className)
}

// RemoveClass は指定したクラスを要素から削除する。
func (element *Element) RemoveClass(className string) bool {
	if !validClassName(className) {
		return false
	}
	classes, ok := element.GetAttribute("class")
	if !ok {
		return false
	}
	fields := strings.Fields(classes)
	result := fields[:0]
	removed := false
	for _, class := range fields {
		if class == className {
			removed = true
			continue
		}
		result = append(result, class)
	}
	if !removed {
		return false
	}
	return element.SetAttribute("class", strings.Join(result, " "))
}

// ContainsClass reports whether className is present.
func (element *Element) ContainsClass(className string) bool {
	if !validClassName(className) {
		return false
	}
	classes, _ := element.GetAttribute("class")
	for _, class := range strings.Fields(classes) {
		if class == className {
			return true
		}
	}
	return false
}

// ToggleClass applies optional force and returns the resulting membership.
func (element *Element) ToggleClass(className string, force *bool) bool {
	if !validClassName(className) {
		return false
	}
	present := element.ContainsClass(className)
	wanted := !present
	if force != nil {
		wanted = *force
	}
	if wanted == present {
		return present
	}
	if wanted {
		element.AddClass(className)
	} else {
		element.RemoveClass(className)
	}
	return wanted
}

// ParentElement returns the connected element parent, excluding the Document node.
func (element *Element) ParentElement() *Element {
	node, ok := element.node()
	if !ok || node.Parent == nil || node.Parent.Type != dommodel.NodeElement {
		return nil
	}
	return (&API{document: element.document, events: element.events, onMutation: element.onMutation}).element(node.Parent)
}

// ParentNode returns the parent Node, including a DocumentFragment or Document.
func (element *Element) ParentNode() *Element {
	node, ok := element.node()
	if !ok || node.Parent == nil || node.Type == dommodel.NodeDocumentFragment {
		return nil
	}
	return (&API{document: element.document, events: element.events, onMutation: element.onMutation}).element(node.Parent)
}

// ChildNodes returns a bounded static snapshot of direct DOM children.
func (element *Element) ChildNodes() []*Element {
	node, ok := element.node()
	if !ok || node.Type == dommodel.NodeElement && node.TagName == "template" {
		return nil
	}
	api := &API{document: element.document, events: element.events, onMutation: element.onMutation}
	children := node.Children
	if len(children) > maxDOMCollectionResults {
		children = children[:maxDOMCollectionResults]
	}
	result := make([]*Element, len(children))
	for index, child := range children {
		result[index] = api.element(child)
	}
	return result
}

// FirstChild returns the first child Node.
func (element *Element) FirstChild() *Element {
	children := element.ChildNodes()
	if len(children) == 0 {
		return nil
	}
	return children[0]
}

// LastChild returns the last child Node.
func (element *Element) LastChild() *Element {
	children := element.ChildNodes()
	if len(children) == 0 {
		return nil
	}
	return children[len(children)-1]
}

// NextSibling returns the following sibling Node.
func (element *Element) NextSibling() *Element { return element.sibling(1) }

// PreviousSibling returns the preceding sibling Node.
func (element *Element) PreviousSibling() *Element { return element.sibling(-1) }

func (element *Element) sibling(offset int) *Element {
	node, ok := element.node()
	if !ok || node.Parent == nil {
		return nil
	}
	index := nodeIndex(node.Parent.Children, node)
	if index < 0 || index+offset < 0 || index+offset >= len(node.Parent.Children) {
		return nil
	}
	return (&API{document: element.document, events: element.events, onMutation: element.onMutation}).element(node.Parent.Children[index+offset])
}

// Contains reports inclusive descendant membership without crossing template fragments.
func (element *Element) Contains(candidate *Element) bool {
	if element == nil || candidate == nil || element.document == nil || candidate.document != element.document {
		return false
	}
	wanted, wantedOK := element.node()
	current, currentOK := candidate.node()
	if !wantedOK || !currentOK {
		return false
	}
	for current != nil {
		if current == wanted {
			return true
		}
		if current.Type == dommodel.NodeDocumentFragment {
			return false
		}
		current = current.Parent
	}
	return false
}

// CloneNode returns a detached shallow or deep clone owned by the same Document.
func (element *Element) CloneNode(deep bool) *Element {
	if element == nil || element.document == nil {
		return nil
	}
	return (&API{document: element.document, events: element.events, onMutation: element.onMutation}).ImportNode(element, deep)
}

// Children returns a bounded static collection of direct element children.
func (element *Element) Children() []*Element {
	node, ok := element.node()
	if !ok {
		return nil
	}
	api := &API{document: element.document, events: element.events, onMutation: element.onMutation}
	var nodes []*dommodel.Node
	for _, child := range node.Children {
		if child.Type == dommodel.NodeElement {
			nodes = append(nodes, child)
		}
	}
	return api.elements(nodes)
}

// TemplateContent returns the detached DocumentFragment owned by a template.
func (element *Element) TemplateContent() *Element {
	node, ok := element.node()
	if !ok || node.Type != dommodel.NodeElement || node.TagName != "template" {
		return nil
	}
	for _, child := range node.Children {
		if child.Type == dommodel.NodeDocumentFragment {
			return (&API{document: element.document, events: element.events, onMutation: element.onMutation}).element(child)
		}
	}
	return nil
}

// IDValue returns the id attribute.
func (element *Element) IDValue() string { value, _ := element.GetAttribute("id"); return value }

// SetIDValue updates the id attribute.
func (element *Element) SetIDValue(value string) bool { return element.SetAttribute("id", value) }

// ClassName returns the normalized class attribute string.
func (element *Element) ClassName() string { value, _ := element.GetAttribute("class"); return value }

// SetClassName updates the class attribute.
func (element *Element) SetClassName(value string) bool { return element.SetAttribute("class", value) }

// TagName returns the HTML tag name in ASCII uppercase.
func (element *Element) TagName() string {
	node, ok := element.node()
	if !ok || node.Type != dommodel.NodeElement {
		return ""
	}
	return strings.ToUpper(node.TagName)
}

// InnerHTML serializes this element's children.
func (element *Element) InnerHTML() string {
	node, ok := element.node()
	if !ok || node.Type != dommodel.NodeElement {
		return ""
	}
	if node.TagName == "template" {
		if content := element.TemplateContent(); content != nil {
			node, _ = content.node()
		}
	}
	value, _ := htmlparser.SerializeChildren(node)
	return value
}

// SetInnerHTML parses and atomically replaces a bounded HTML fragment.
func (element *Element) SetInnerHTML(value string) bool {
	parent, ok := element.node()
	if !ok || parent.Type != dommodel.NodeElement || len(value) > maxDOMInnerHTMLBytes {
		return false
	}
	contextTag := parent.TagName
	if parent.TagName == "template" {
		content := element.TemplateContent()
		if content == nil {
			return false
		}
		parent, _ = content.node()
		contextTag = "template"
	}
	fragment, err := htmlparser.ParseFragment(value, contextTag)
	if err != nil || fragment.NodeCount()-1 > maxDOMCollectionResults || element.document.OwnedNodeCount()+fragment.NodeCount()-1 > maxDOMOwnedNodes {
		return false
	}
	children := make([]*dommodel.Node, 0, len(fragment.Root.Children))
	for _, child := range fragment.Root.Children {
		children = append(children, cloneNode(element.document, child, true))
	}
	if !element.document.ReplaceChildren(parent, children) {
		return false
	}
	element.notifyMutation()
	return true
}

// OuterHTML serializes this element and its subtree.
func (element *Element) OuterHTML() string {
	node, ok := element.node()
	if !ok || node.Type != dommodel.NodeElement {
		return ""
	}
	value, _ := htmlparser.SerializeNode(node)
	return value
}

// SetOuterHTML replaces this element with a bounded parsed fragment.
func (element *Element) SetOuterHTML(value string) ([]*Element, bool) {
	node, ok := element.node()
	if !ok || node.Type != dommodel.NodeElement || node.Parent == nil || len(value) > maxDOMInnerHTMLBytes {
		return nil, false
	}
	parent := node.Parent
	contextTag := "div"
	if parent.Type == dommodel.NodeElement {
		contextTag = parent.TagName
	}
	nodes, ok := element.parseFragmentNodes(value, contextTag)
	if !ok {
		return nil, false
	}
	index := nodeIndex(parent.Children, node)
	if index < 0 {
		return nil, false
	}
	wanted := append([]*dommodel.Node(nil), parent.Children[:index]...)
	wanted = append(wanted, nodes...)
	wanted = append(wanted, parent.Children[index+1:]...)
	if !element.document.ReplaceChildren(parent, wanted) {
		return nil, false
	}
	element.notifyMutation()
	api := &API{document: element.document, events: element.events, onMutation: element.onMutation}
	result := make([]*Element, len(nodes))
	for index, inserted := range nodes {
		result[index] = api.element(inserted)
	}
	return result, true
}

// InsertAdjacentHTML parses and inserts HTML at one of the four standard positions.
func (element *Element) InsertAdjacentHTML(position, value string) ([]*Element, bool) {
	node, ok := element.node()
	if !ok || node.Type != dommodel.NodeElement || len(value) > maxDOMInnerHTMLBytes {
		return nil, false
	}
	position = strings.ToLower(strings.TrimSpace(position))
	parent, index, contextTag := node, 0, node.TagName
	switch position {
	case "beforebegin", "afterend":
		if node.Parent == nil {
			return nil, false
		}
		parent = node.Parent
		if parent.Type == dommodel.NodeElement {
			contextTag = parent.TagName
		} else {
			contextTag = "div"
		}
		index = nodeIndex(parent.Children, node)
		if position == "afterend" {
			index++
		}
	case "afterbegin":
		index = 0
	case "beforeend":
		index = len(parent.Children)
	default:
		return nil, false
	}
	if node.TagName == "template" && (position == "afterbegin" || position == "beforeend") {
		content := element.TemplateContent()
		if content == nil {
			return nil, false
		}
		parent, _ = content.node()
		contextTag = "template"
		if position == "beforeend" {
			index = len(parent.Children)
		}
	}
	nodes, ok := element.parseFragmentNodes(value, contextTag)
	if !ok || index < 0 || index > len(parent.Children) {
		return nil, false
	}
	wanted := insertNodes(parent.Children, index, nodes)
	if !element.document.ReplaceChildren(parent, wanted) {
		return nil, false
	}
	element.notifyMutation()
	api := &API{document: element.document, events: element.events, onMutation: element.onMutation}
	result := make([]*Element, len(nodes))
	for index, inserted := range nodes {
		result[index] = api.element(inserted)
	}
	return result, true
}

func (element *Element) parseFragmentNodes(value, contextTag string) ([]*dommodel.Node, bool) {
	fragment, err := htmlparser.ParseFragment(value, contextTag)
	if err != nil || fragment.NodeCount()-1 > maxDOMCollectionResults || element.document.OwnedNodeCount()+fragment.NodeCount()-1 > maxDOMOwnedNodes {
		return nil, false
	}
	nodes := make([]*dommodel.Node, 0, len(fragment.Root.Children))
	for _, child := range fragment.Root.Children {
		nodes = append(nodes, cloneNode(element.document, child, true))
	}
	return nodes, true
}

// InlineStyleProperty returns the last inline declaration value and priority.
func (element *Element) InlineStyleProperty(name string) (value, priority string) {
	name = normalizeCSSPropertyName(name)
	for _, declaration := range element.inlineStyleDeclarations() {
		if declaration.Property == name {
			value = declaration.Value.Raw
			if declaration.Important {
				priority = "important"
			}
		}
	}
	return value, priority
}

// SetInlineStyleProperty validates and updates one inline declaration.
func (element *Element) SetInlineStyleProperty(name, value, priority string) bool {
	name, value, priority = normalizeCSSPropertyName(name), strings.TrimSpace(value), strings.ToLower(strings.TrimSpace(priority))
	if name == "" || value == "" || priority != "" && priority != "important" {
		return false
	}
	parsed, err := css.ParseDeclarations(name + ":" + value + map[bool]string{true: " !important"}[priority == "important"])
	if err != nil || len(parsed) != 1 || parsed[0].Property != name {
		return false
	}
	declarations := element.inlineStyleDeclarations()
	result := make([]css.Declaration, 0, len(declarations)+1)
	for _, declaration := range declarations {
		if declaration.Property != name {
			result = append(result, declaration)
		}
	}
	result = append(result, parsed[0])
	return element.setInlineStyleDeclarations(result)
}

// RemoveInlineStyleProperty removes one declaration and returns its old value.
func (element *Element) RemoveInlineStyleProperty(name string) string {
	name = normalizeCSSPropertyName(name)
	declarations := element.inlineStyleDeclarations()
	result := make([]css.Declaration, 0, len(declarations))
	removed := ""
	for _, declaration := range declarations {
		if declaration.Property == name {
			removed = declaration.Value.Raw
			continue
		}
		result = append(result, declaration)
	}
	if removed != "" {
		element.setInlineStyleDeclarations(result)
	}
	return removed
}

// InlineStyleText returns the normalized serialized style attribute.
func (element *Element) InlineStyleText() string {
	return serializeDeclarations(element.inlineStyleDeclarations())
}

// InlineStylePropertyNames returns declaration names in source order.
func (element *Element) InlineStylePropertyNames() []string {
	declarations := element.inlineStyleDeclarations()
	result := make([]string, 0, len(declarations))
	seen := make(map[string]bool, len(declarations))
	for index := len(declarations) - 1; index >= 0; index-- {
		name := declarations[index].Property
		if !seen[name] {
			seen[name] = true
			result = append(result, name)
		}
	}
	for left, right := 0, len(result)-1; left < right; left, right = left+1, right-1 {
		result[left], result[right] = result[right], result[left]
	}
	return result
}

// SetInlineStyleText replaces the complete inline declaration list.
func (element *Element) SetInlineStyleText(value string) bool {
	if len(value) > maxDOMTextBytes {
		return false
	}
	declarations, err := css.ParseDeclarations(value)
	if err != nil {
		return false
	}
	return element.setInlineStyleDeclarations(declarations)
}

func (element *Element) inlineStyleDeclarations() []css.Declaration {
	value, _ := element.GetAttribute("style")
	declarations, _ := css.ParseDeclarations(value)
	return declarations
}

func (element *Element) setInlineStyleDeclarations(declarations []css.Declaration) bool {
	serialized := serializeDeclarations(declarations)
	if serialized == "" {
		if element.HasAttribute("style") {
			return element.RemoveAttribute("style")
		}
		return true
	}
	if len(serialized) > maxDOMTextBytes {
		return false
	}
	if current, _ := element.GetAttribute("style"); current == serialized {
		return true
	}
	return element.SetAttribute("style", serialized)
}

// Value はテキストinputの現在値を返す。
func (element *Element) Value() string {
	node, ok := element.textInputNode()
	if !ok {
		return ""
	}
	return forms.CurrentValue(node)
}

// SetValue はテキストinputの現在値を変更する。
func (element *Element) SetValue(value string) bool {
	node, ok := element.textInputNode()
	if !ok || forms.Disabled(node) || forms.ReadOnly(node) || !forms.SetCurrentValue(node, value) {
		return false
	}
	if element.onMutation != nil {
		element.onMutation()
	}
	return true
}

// Reset restores this form's controls to their default state.
func (element *Element) Reset() bool {
	if element == nil || element.document == nil {
		return false
	}
	node, ok := element.document.NodeByID(element.id)
	if !ok {
		return false
	}
	if element.events != nil {
		event := events.Cancelable(events.Reset, node.ID)
		element.events.DispatchTree(element.document, event)
		if event.DefaultPrevented() {
			return false
		}
	}
	if !forms.Reset(node) {
		return false
	}
	if element.onMutation != nil {
		element.onMutation()
	}
	return true
}

// Text は要素と子孫のテキストを返す。
func (element *Element) Text() string {
	if element == nil || element.document == nil {
		return ""
	}
	node, ok := element.document.NodeByID(element.id)
	if !ok {
		return ""
	}
	return node.TextContent()
}

// SetText は要素の内容を指定したテキストへ置き換える。
func (element *Element) SetText(value string) bool {
	if element == nil || element.document == nil {
		return false
	}
	node, ok := element.node()
	if !ok {
		return false
	}
	parentTag := node.TagName
	if node.Type == dommodel.NodeText && node.Parent != nil {
		parentTag = node.Parent.TagName
	}
	if len(value) > dommodel.TextLimitForParent(parentTag) {
		return false
	}
	if node.Type == dommodel.NodeText {
		if node.Text == value {
			return true
		}
		node.Text = value
	} else {
		removed := 0
		for _, child := range node.Children {
			removed += countSubtreeNodes(child)
		}
		if element.document.OwnedNodeCount()-removed+1 > maxDOMOwnedNodes || !element.document.SetTextContent(element.id, value) {
			return false
		}
	}
	if element.onMutation != nil {
		element.onMutation()
	}
	return true
}

func (api *API) element(node *dommodel.Node) *Element {
	if api == nil || node == nil {
		return nil
	}
	return &Element{document: api.document, id: node.ID, events: api.events, onMutation: api.onMutation}
}

func (api *API) elements(nodes []*dommodel.Node) []*Element {
	if len(nodes) > maxDOMCollectionResults {
		nodes = nodes[:maxDOMCollectionResults]
	}
	result := make([]*Element, 0, len(nodes))
	for _, node := range nodes {
		if node.Type == dommodel.NodeElement {
			result = append(result, api.element(node))
		}
	}
	return result
}

func (element *Element) node() (*dommodel.Node, bool) {
	if element == nil || element.document == nil {
		return nil, false
	}
	node, ok := element.document.NodeByID(element.id)
	return node, ok
}

func (element *Element) relationship(child *Element) (*dommodel.Node, *dommodel.Node, bool) {
	if element == nil || child == nil || element.document == nil || child.document != element.document {
		return nil, nil, false
	}
	parentNode, parentOK := element.node()
	childNode, childOK := child.node()
	return parentNode, childNode, parentOK && childOK && (parentNode.Type == dommodel.NodeElement || parentNode.Type == dommodel.NodeDocumentFragment)
}

func (element *Element) mutationNodes(children []*Element) (*dommodel.Node, []*dommodel.Node, bool) {
	if element == nil || element.document == nil {
		return nil, nil, false
	}
	parent, ok := element.node()
	if !ok || parent.Type != dommodel.NodeElement && parent.Type != dommodel.NodeDocumentFragment {
		return nil, nil, false
	}
	nodes := make([]*dommodel.Node, 0, len(children))
	seen := make(map[dommodel.NodeID]bool, len(children))
	for _, child := range children {
		if child == nil || child.document != element.document {
			return nil, nil, false
		}
		node, ok := child.node()
		if !ok || node == element.document.Root {
			return nil, nil, false
		}
		candidates := []*dommodel.Node{node}
		if node.Type == dommodel.NodeDocumentFragment {
			candidates = append([]*dommodel.Node(nil), node.Children...)
		}
		for _, candidate := range candidates {
			if seen[candidate.ID] {
				return nil, nil, false
			}
			seen[candidate.ID] = true
			nodes = append(nodes, candidate)
		}
	}
	return parent, nodes, true
}

func (element *Element) insert(prepend bool, children ...*Element) bool {
	parent, nodes, ok := element.mutationNodes(children)
	if !ok || len(nodes) == 0 {
		return false
	}
	wanted := make([]*dommodel.Node, 0, len(parent.Children)+len(nodes))
	inserted := make(map[dommodel.NodeID]bool, len(nodes))
	for _, node := range nodes {
		inserted[node.ID] = true
	}
	if prepend {
		wanted = append(wanted, nodes...)
	}
	for _, current := range parent.Children {
		if !inserted[current.ID] {
			wanted = append(wanted, current)
		}
	}
	if !prepend {
		wanted = append(wanted, nodes...)
	}
	if !element.document.ReplaceChildren(parent, wanted) {
		return false
	}
	element.notifyMutation()
	return true
}

func (element *Element) notifyMutation() {
	if element != nil && element.onMutation != nil {
		element.onMutation()
	}
}

func cloneNode(document *dommodel.Document, source *dommodel.Node, deep bool) *dommodel.Node {
	if source.Type == dommodel.NodeText {
		return document.CreateText(source.Text)
	}
	var target *dommodel.Node
	if source.Type == dommodel.NodeDocumentFragment {
		target = document.CreateDocumentFragment()
	} else {
		target = document.CreateElement(source.TagName, source.Attributes)
	}
	if deep {
		for _, child := range source.Children {
			_ = document.AppendChild(target, cloneNode(document, child, true))
		}
	}
	return target
}

func countSubtreeNodes(node *dommodel.Node) int {
	if node == nil {
		return 0
	}
	count := 1
	for _, child := range node.Children {
		count += countSubtreeNodes(child)
	}
	return count
}

func subtreeNodeIDs(node *dommodel.Node, result []dommodel.NodeID) []dommodel.NodeID {
	if node == nil {
		return result
	}
	result = append(result, node.ID)
	for _, child := range node.Children {
		result = subtreeNodeIDs(child, result)
	}
	return result
}

func nodeIndex(nodes []*dommodel.Node, wanted *dommodel.Node) int {
	for index, node := range nodes {
		if node == wanted {
			return index
		}
	}
	return -1
}

func nodeIncluded(nodes []*dommodel.Node, wanted *dommodel.Node) bool {
	return nodeIndex(nodes, wanted) >= 0
}

func withoutNodes(source, moving []*dommodel.Node, removed *dommodel.Node) []*dommodel.Node {
	result := make([]*dommodel.Node, 0, len(source))
	for _, node := range source {
		if node != removed && !nodeIncluded(moving, node) {
			result = append(result, node)
		}
	}
	return result
}

func insertNodes(source []*dommodel.Node, index int, inserted []*dommodel.Node) []*dommodel.Node {
	if index < 0 || index > len(source) {
		return source
	}
	result := make([]*dommodel.Node, 0, len(source)+len(inserted))
	result = append(result, source[:index]...)
	result = append(result, inserted...)
	result = append(result, source[index:]...)
	return result
}

func firstMatchingDescendant(root *dommodel.Node, selectors []css.Selector, includeRoot bool, scope dommodel.NodeID) *dommodel.Node {
	if len(selectors) == 0 || root == nil {
		return nil
	}
	if includeRoot && root.Type == dommodel.NodeElement && matchesAnySelector(root, selectors, scope) {
		return root
	}
	if root.Type == dommodel.NodeElement && root.TagName == "template" {
		return nil
	}
	for _, child := range root.Children {
		if child.Type == dommodel.NodeDocumentFragment && root.Type != dommodel.NodeDocumentFragment {
			continue
		}
		if match := firstMatchingDescendant(child, selectors, true, scope); match != nil {
			return match
		}
	}
	return nil
}

func matchingDescendants(root *dommodel.Node, selectors []css.Selector, includeRoot bool, result []*dommodel.Node, scope dommodel.NodeID) []*dommodel.Node {
	if len(selectors) == 0 || root == nil || len(result) >= maxDOMCollectionResults {
		return result
	}
	if includeRoot && root.Type == dommodel.NodeElement && matchesAnySelector(root, selectors, scope) {
		result = append(result, root)
	}
	if root.Type == dommodel.NodeElement && root.TagName == "template" {
		return result
	}
	for _, child := range root.Children {
		if child.Type == dommodel.NodeDocumentFragment && root.Type != dommodel.NodeDocumentFragment {
			continue
		}
		result = matchingDescendants(child, selectors, true, result, scope)
		if len(result) >= maxDOMCollectionResults {
			break
		}
	}
	return result
}

func matchesAnySelector(node *dommodel.Node, selectors []css.Selector, scope dommodel.NodeID) bool {
	for _, selector := range selectors {
		if stylemodel.MatchesSelector(node, selector, stylemodel.InteractionState{Scope: scope}) {
			return true
		}
	}
	return false
}

func documentScope(root *dommodel.Node) dommodel.NodeID {
	if root == nil {
		return 0
	}
	for _, child := range root.Children {
		if child.Type == dommodel.NodeElement {
			return child.ID
		}
	}
	return 0
}

func normalizeCSSPropertyName(name string) string {
	name = strings.TrimSpace(name)
	if strings.HasPrefix(name, "--") {
		return name
	}
	return strings.ToLower(name)
}

func serializeDeclarations(declarations []css.Declaration) string {
	parts := make([]string, 0, len(declarations))
	for _, declaration := range declarations {
		property, value := normalizeCSSPropertyName(declaration.Property), strings.TrimSpace(declaration.Value.Raw)
		if property == "" || value == "" {
			continue
		}
		if declaration.Important {
			value += " !important"
		}
		parts = append(parts, property+": "+value)
	}
	return strings.Join(parts, "; ")
}

func (element *Element) textInputNode() (*dommodel.Node, bool) {
	if element == nil || element.document == nil {
		return nil, false
	}
	node, ok := element.document.NodeByID(element.id)
	if !ok || !forms.IsEditableTextControl(node) {
		return nil, false
	}
	return node, true
}

func (element *Element) addEventListener(eventType events.Type, listener events.Listener) bool {
	return element.addEventListenerWithCapture(eventType, false, listener) != 0
}

func (element *Element) addEventListenerWithCapture(eventType events.Type, capture bool, listener events.Listener) events.ListenerID {
	return element.addEventListenerWithConnectionPolicy(eventType, capture, false, false, true, listener)
}

func (element *Element) addEventListenerWithConnectionPolicy(eventType events.Type, capture, once, passive, requireConnected bool, listener events.Listener) events.ListenerID {
	if element == nil || element.document == nil || element.events == nil || listener == nil {
		return 0
	}
	node, ok := element.document.NodeByID(element.id)
	if !ok || requireConnected && !element.document.IsConnected(node) {
		return 0
	}
	return element.events.AddEventListenerWithOptions(element.id, eventType, capture, once, passive, listener)
}

func (element *Element) publicEvent(event events.Event) Event {
	result := Event{
		Type: string(event.Type), TargetNodeID: event.Target, CurrentTargetID: event.CurrentTarget(), Value: event.Value, X: event.X, Y: event.Y,
		Bubbles: event.Bubbles(), Cancelable: event.IsCancelable(), DefaultPrevented: event.DefaultPrevented, Phase: event.EventPhase(),
		stopPropagation: event.StopPropagation,
		stopImmediate:   event.StopImmediatePropagation,
		RuntimeID:       event.RuntimeID,
	}
	if event.IsCancelable() {
		result.preventDefault = event.PreventDefault
	}
	if element == nil || element.document == nil {
		return result
	}
	node, ok := element.document.NodeByID(event.Target)
	if !ok {
		return result
	}
	result.TargetID, _ = node.Attribute("id")
	if forms.IsEditableTextControl(node) {
		result.Value = forms.CurrentValue(node)
	}
	return result
}

func validTagName(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if character != '-' &&
			(character < 'a' || character > 'z') &&
			(character < 'A' || character > 'Z') &&
			(character < '0' || character > '9') {
			return false
		}
	}
	return true
}

func validAttributeName(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if character != '-' && character != '_' && character != ':' &&
			(character < 'a' || character > 'z') &&
			(character < '0' || character > '9') {
			return false
		}
	}
	return true
}

func validClassName(value string) bool {
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
