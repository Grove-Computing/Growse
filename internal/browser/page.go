package browser

import (
	"image"
	"net/url"
	"strings"
	"time"

	"github.com/Grove-Computing/Growse/internal/css"
	"github.com/Grove-Computing/Growse/internal/devtools"
	"github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/events"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
	"github.com/Grove-Computing/Growse/internal/style"
)

// Page holds the state of one loaded document.
//
// Runtimeの状態はスクリプト取得エラーと分けて保持し、Goコードを実行できない場合も
// ページの表示を継続できるようにする。
type Page struct {
	HistoryID        uint64
	HistoryState     string
	ScrollFirst      int
	ScrollOffset     int
	ScrollRevision   uint64
	URL              *url.URL
	StatusCode       int
	ContentType      string
	Source           []byte
	Document         *dom.Document
	Events           *events.Dispatcher
	Stylesheet       *css.Stylesheet
	ComputedStyles   style.Map
	Animations       *style.AnimationRegistry
	Transitions      *style.TransitionRegistry
	BackgroundImages map[string]image.Image
	BackgroundErrors []string
	Engine           runtimemodel.Engine
	Scripts          []Script
	ImportMap        map[string]string
	ScriptErrors     []string
	RuntimeStarted   bool
	RuntimeError     string
	Sandbox          runtimemodel.SandboxStatus
	HoverTarget      dom.NodeID
	HoverPath        []dom.NodeID
	FocusTarget      dom.NodeID
	Submitter        dom.NodeID
	ViewportWidth    float32
	ViewportHeight   float32
	ReducedMotion    bool
	StyleRevision    uint64
	DevTools         *devtools.PageStore
}

// AnimatedStyles samples this page's CSS Animations and Transitions at current without
// changing its underlying computed style map.
func (p *Page) AnimatedStyles(current time.Time) style.Map {
	if p == nil {
		return nil
	}
	result := p.ComputedStyles
	if p.Animations != nil {
		result = p.Animations.AnimatedStyles(p.ComputedStyles, p.Stylesheet, current)
	}
	if p.Transitions != nil {
		result = p.Transitions.Apply(result, current)
	}
	return result
}

// ActiveAnimations reports whether this page needs another animation frame.
func (p *Page) ActiveAnimations(current time.Time) bool {
	return p != nil && ((p.Animations != nil && p.Animations.Active(current)) ||
		(p.Transitions != nil && p.Transitions.Active(current)))
}

// NewPage creates a page for pageURL. A nil URL is allowed for documents such
// as an in-memory error page that do not have a network location.
func NewPage(pageURL *url.URL) *Page {
	return &Page{URL: cloneURL(pageURL), DevTools: devtools.NewPageStore()}
}

func (p *Page) ensureDevTools() *devtools.PageStore {
	if p.DevTools == nil {
		p.DevTools = devtools.NewPageStore()
	}
	return p.DevTools
}

func (p *Page) closeDevTools() {
	if p != nil && p.DevTools != nil {
		p.DevTools.Close()
	}
}

// LinkURL resolves the nearest anchor at nodeID against the page URL.
func (p *Page) LinkURL(nodeID dom.NodeID) (*url.URL, bool) {
	linkURL, _, ok := p.LinkDestination(nodeID)
	return linkURL, ok
}

// LinkDestination resolves the nearest anchor URL and normalized target.
func (p *Page) LinkDestination(nodeID dom.NodeID) (*url.URL, string, bool) {
	if p == nil || p.URL == nil || p.Document == nil {
		return nil, "", false
	}
	node, ok := p.Document.NodeByID(nodeID)
	if !ok {
		return nil, "", false
	}
	for current := node; current != nil; current = current.Parent {
		if current.Type != dom.NodeElement || current.TagName != "a" {
			continue
		}
		href, ok := current.Attribute("href")
		href = strings.TrimSpace(href)
		if !ok || href == "" {
			return nil, "", false
		}
		reference, err := url.Parse(href)
		if err != nil {
			return nil, "", false
		}
		resolved := p.URL.ResolveReference(reference)
		if resolved.Scheme != "http" && resolved.Scheme != "https" {
			return nil, "", false
		}
		target, _ := current.Attribute("target")
		return resolved, strings.ToLower(strings.TrimSpace(target)), true
	}
	return nil, "", false
}

func cloneURL(source *url.URL) *url.URL {
	if source == nil {
		return nil
	}

	copy := *source
	return &copy
}
