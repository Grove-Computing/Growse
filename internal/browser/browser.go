// Package browser contains the state and navigation lifecycle of a Growse
// browser window.
package browser

// Browser owns the state for one browser window.
//
// The MVP supports one active page. Network loading, history, and the Go
// runtime will be added as separate responsibilities in later steps.
type Browser struct {
	page *Page
}

// New creates a browser with no page loaded.
func New() *Browser {
	return &Browser{}
}

// Page returns the currently active page, or nil before the first successful
// navigation.
func (b *Browser) Page() *Page {
	return b.page
}

// SetPage replaces the active page. Passing nil clears the active page.
func (b *Browser) SetPage(page *Page) {
	b.page = page
}
