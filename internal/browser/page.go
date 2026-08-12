package browser

import (
	"net/url"

	"github.com/saku0512/growse/internal/dom"
)

// Page holds the state of one loaded document.
//
// Document, stylesheet, layout tree, display list, scripts, and runtime state
// are intentionally added only after their corresponding packages exist.
type Page struct {
	URL         *url.URL
	StatusCode  int
	ContentType string
	Source      []byte
	Document    *dom.Document
}

// NewPage creates a page for pageURL. A nil URL is allowed for documents such
// as an in-memory error page that do not have a network location.
func NewPage(pageURL *url.URL) *Page {
	return &Page{URL: cloneURL(pageURL)}
}

func cloneURL(source *url.URL) *url.URL {
	if source == nil {
		return nil
	}

	copy := *source
	return &copy
}
