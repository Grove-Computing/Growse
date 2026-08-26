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
	layoutmodel "github.com/Grove-Computing/Growse/internal/layout"
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
	Frames           []*Frame
	FramePolicy      runtimemodel.FramePolicy
	window           runtimemodel.WindowContext
	windows          *windowRegistry
}

// Frame is one nested browsing context owned by a parent Page.
type Frame struct {
	ID         uint64
	ElementID  dom.NodeID
	ParentID   uint64
	Depth      int
	Generation uint64
	URL        *url.URL
	Page       *Page
	Viewport   FrameViewport
	ScrollX    float32
	ScrollY    float32
	LoadError  string
	Loaded     bool
	Closed     bool
	Sandbox    runtimemodel.FramePolicy
	runtime    runtimemodel.Runtime
	cancel     func()
	state      *frameLoadState
	parentPage *Page
}

// FrameViewport is the iframe border box and its clipped child viewport.
type FrameViewport struct {
	X, Y, Width, Height float32
	ClipX, ClipY        float32
	ClipWidth           float32
	ClipHeight          float32
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

// FrameByElement returns the live top-level Frame hosted by elementID.
func (p *Page) FrameByElement(elementID dom.NodeID) (*Frame, bool) {
	if p == nil {
		return nil, false
	}
	for _, frame := range p.Frames {
		if frame != nil && frame.ElementID == elementID && !frame.Closed {
			return frame, true
		}
	}
	return nil, false
}

// SyncFrameViewports maps the parent layout geometry into clipped child viewports.
func (p *Page) SyncFrameViewports(tree *layoutmodel.Tree) {
	if p == nil || tree == nil {
		return
	}
	for _, frame := range p.Frames {
		if frame == nil || frame.Closed {
			continue
		}
		if bounds, ok := tree.Bounds[frame.ElementID]; ok {
			frame.SetViewport(bounds.X, bounds.Y, bounds.Width, bounds.Height)
			continue
		}
		for _, box := range tree.Boxes {
			x := box.X
			for _, run := range box.Runs {
				if run.NodeID == frame.ElementID {
					frame.SetViewport(x, box.Y, run.Width, box.Height)
					break
				}
				x += run.Width
			}
		}
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
