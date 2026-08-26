// Package dom はWebGoスクリプト向けのDOM APIを提供する。
package dom

import (
	"strings"

	dommodel "github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/events"
	"github.com/Grove-Computing/Growse/internal/forms"
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
	Type           string
	TargetID       string
	Value          string
	X              float32
	Y              float32
	preventDefault func()
}

// ID はElementが参照するPage内Node IDを返す。
func (element *Element) ID() dommodel.NodeID {
	if element == nil {
		return 0
	}
	return element.id
}

// PreventDefault cancels a cancelable browser default action such as form submission.
func (event Event) PreventDefault() {
	if event.preventDefault != nil {
		event.preventDefault()
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
	node, ok := api.document.QuerySelector(selector)
	if !ok {
		return nil
	}
	return api.element(node)
}

// ElementByNodeID returns a connected element by its document-scoped identity.
func (api *API) ElementByNodeID(id dommodel.NodeID) *Element {
	if api == nil || api.document == nil || id == 0 {
		return nil
	}
	node, ok := api.document.NodeByID(id)
	if !ok || node.Type != dommodel.NodeElement || !api.document.IsConnected(node) {
		return nil
	}
	return api.element(node)
}

// CreateElement はDocumentが所有する未接続の要素を作成する。
func (api *API) CreateElement(tagName string) *Element {
	if api == nil || api.document == nil {
		return nil
	}
	tagName = strings.TrimSpace(tagName)
	if !validTagName(tagName) {
		return nil
	}
	return api.element(api.document.CreateElement(tagName, nil))
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
	if handler == nil {
		return false
	}
	var internal events.Type
	switch strings.ToLower(strings.TrimSpace(eventType)) {
	case string(events.Click):
		internal = events.Click
	case string(events.Input):
		internal = events.Input
	case string(events.Change):
		internal = events.Change
	case string(events.Submit):
		internal = events.Submit
	case string(events.Reset):
		internal = events.Reset
	case string(events.Focus):
		internal = events.Focus
	case string(events.Blur):
		internal = events.Blur
	case string(events.MouseEnter):
		internal = events.MouseEnter
	case string(events.MouseLeave):
		internal = events.MouseLeave
	default:
		return false
	}
	return element.addEventListener(internal, func(event events.Event) {
		handler(element.publicEvent(event))
	})
}

// AppendChild は未接続の子要素を接続済みの要素の末尾へ追加する。
func (element *Element) AppendChild(child *Element) bool {
	if element == nil || child == nil || element.document == nil || child.document != element.document {
		return false
	}
	parentNode, parentOK := element.document.NodeByID(element.id)
	childNode, childOK := element.document.NodeByID(child.id)
	if !parentOK || !childOK || parentNode.Type != dommodel.NodeElement || childNode.Type != dommodel.NodeElement {
		return false
	}
	if !element.document.IsConnected(parentNode) || element.document.IsConnected(childNode) || childNode.Parent != nil {
		return false
	}
	if err := element.document.AppendChild(parentNode, childNode); err != nil {
		return false
	}
	if element.onMutation != nil {
		element.onMutation()
	}
	return true
}

// Remove は要素自身とその子孫をDocumentから削除する。
func (element *Element) Remove() bool {
	if element == nil || element.document == nil {
		return false
	}
	removed, ok := element.document.Remove(element.id)
	if !ok {
		return false
	}
	if element.events != nil {
		element.events.RemoveEventListeners(removed...)
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
	if !ok || !forms.Reset(node) {
		return false
	}
	if element.events != nil {
		element.events.Dispatch(events.Event{Type: events.Reset, Target: node.ID})
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
func (element *Element) SetText(value string) {
	if element == nil || element.document == nil || !element.document.SetTextContent(element.id, value) {
		return
	}
	if element.onMutation != nil {
		element.onMutation()
	}
}

func (api *API) element(node *dommodel.Node) *Element {
	if api == nil || node == nil {
		return nil
	}
	return &Element{document: api.document, id: node.ID, events: api.events, onMutation: api.onMutation}
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
	if element == nil || element.document == nil || element.events == nil || listener == nil {
		return false
	}
	node, ok := element.document.NodeByID(element.id)
	if !ok || !element.document.IsConnected(node) {
		return false
	}
	element.events.AddEventListener(element.id, eventType, listener)
	return true
}

func (element *Element) publicEvent(event events.Event) Event {
	result := Event{Type: string(event.Type), Value: event.Value, X: event.X, Y: event.Y}
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
