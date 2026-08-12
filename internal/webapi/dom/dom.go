// Package dom はWebGoスクリプト向けのDOM APIを提供する。
package dom

import dommodel "github.com/saku0512/growse/internal/dom"

// API は1ページのDocumentへアクセスする。
type API struct {
	document   *dommodel.Document
	onMutation func()
}

// Element はDocument内の要素をNodeIDで参照する。
type Element struct {
	document   *dommodel.Document
	id         dommodel.NodeID
	onMutation func()
}

// New はDocumentに結び付いたDOM APIを生成する。
func New(document *dommodel.Document, onMutation func()) *API {
	return &API{document: document, onMutation: onMutation}
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
	return &Element{document: api.document, id: node.ID, onMutation: api.onMutation}
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
