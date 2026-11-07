package browser

import (
	"github.com/Grove-Computing/Growse/internal/dom"
	layoutmodel "github.com/Grove-Computing/Growse/internal/layout"
	"github.com/Grove-Computing/Growse/internal/style"
)

// PageRenderSnapshot is a detached input set for layout and display-list work.
// Its DOM preserves live Node IDs so hit testing can still target the page.
type PageRenderSnapshot struct {
	Document       *dom.Document
	ComputedStyles style.Map
	ImageResources map[dom.NodeID]layoutmodel.ImageResource
	WebFonts       *layoutmodel.FontSet
	Revision       uint64
}

// CaptureRenderSnapshot copies the mutable page inputs needed by the renderer.
func (p *Page) CaptureRenderSnapshot() (PageRenderSnapshot, bool) {
	if p == nil || p.Document == nil {
		return PageRenderSnapshot{}, false
	}
	document, err := dom.NewDocumentFromSnapshot(p.Document.Snapshot())
	if err != nil {
		return PageRenderSnapshot{}, false
	}
	p.styleMu.Lock()
	styles := make(style.Map, len(p.ComputedStyles))
	for nodeID, computed := range p.ComputedStyles {
		styles[nodeID] = computed
	}
	revision := p.StyleRevision
	p.styleMu.Unlock()
	p.imageMu.Lock()
	var images map[dom.NodeID]layoutmodel.ImageResource
	if p.ImageResources != nil {
		images = make(map[dom.NodeID]layoutmodel.ImageResource, len(p.ImageResources))
		for nodeID, resource := range p.ImageResources {
			images[nodeID] = resource
		}
	}
	p.imageMu.Unlock()
	p.fontMu.Lock()
	fonts := p.WebFonts
	p.fontMu.Unlock()
	return PageRenderSnapshot{Document: document, ComputedStyles: styles, ImageResources: images, WebFonts: fonts, Revision: revision}, true
}
