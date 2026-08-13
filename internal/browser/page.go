package browser

import (
	"net/url"
	"strings"

	"github.com/saku0512/growse/internal/css"
	"github.com/saku0512/growse/internal/dom"
	"github.com/saku0512/growse/internal/events"
	"github.com/saku0512/growse/internal/style"
)

// Page holds the state of one loaded document.
//
// Runtimeの状態はスクリプト取得エラーと分けて保持し、Goコードを実行できない場合も
// ページの表示を継続できるようにする。
type Page struct {
	URL            *url.URL
	StatusCode     int
	ContentType    string
	Source         []byte
	Document       *dom.Document
	Events         *events.Dispatcher
	Stylesheet     *css.Stylesheet
	ComputedStyles style.Map
	Scripts        []Script
	ScriptErrors   []string
	RuntimeStarted bool
	RuntimeError   string
	HoverTarget    dom.NodeID
	HoverPath      []dom.NodeID
	FocusTarget    dom.NodeID
	ViewportWidth  float32
	ViewportHeight float32
}

// NewPage creates a page for pageURL. A nil URL is allowed for documents such
// as an in-memory error page that do not have a network location.
func NewPage(pageURL *url.URL) *Page {
	return &Page{URL: cloneURL(pageURL)}
}

// LinkURL resolves the nearest anchor at nodeID against the page URL.
func (p *Page) LinkURL(nodeID dom.NodeID) (*url.URL, bool) {
	if p == nil || p.URL == nil || p.Document == nil {
		return nil, false
	}
	node, ok := p.Document.NodeByID(nodeID)
	if !ok {
		return nil, false
	}
	for current := node; current != nil; current = current.Parent {
		if current.Type != dom.NodeElement || current.TagName != "a" {
			continue
		}
		href, ok := current.Attribute("href")
		href = strings.TrimSpace(href)
		if !ok || href == "" {
			return nil, false
		}
		reference, err := url.Parse(href)
		if err != nil {
			return nil, false
		}
		resolved := p.URL.ResolveReference(reference)
		if resolved.Scheme != "http" && resolved.Scheme != "https" {
			return nil, false
		}
		return resolved, true
	}
	return nil, false
}

func cloneURL(source *url.URL) *url.URL {
	if source == nil {
		return nil
	}

	copy := *source
	return &copy
}
