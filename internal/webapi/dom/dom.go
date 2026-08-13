// Package dom はWebGoスクリプト向けのDOM APIを提供する。
package dom

import (
	"strings"

	dommodel "github.com/saku0512/growse/internal/dom"
	"github.com/saku0512/growse/internal/events"
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
	if element == nil || element.document == nil || element.events == nil || handler == nil {
		return
	}
	if _, ok := element.document.NodeByID(element.id); !ok {
		return
	}
	element.events.AddEventListener(element.id, events.Click, func(events.Event) {
		handler()
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
