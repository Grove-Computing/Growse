// Package browser contains the state and navigation lifecycle of a Growse
// browser window.
package browser

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"mime"
	"net/url"
	"strings"
	"sync"

	htmlparser "github.com/saku0512/growse/internal/html"
	"github.com/saku0512/growse/internal/network"
	"github.com/saku0512/growse/internal/style"
)

// ResourceLoader retrieves a resource for navigation.
type ResourceLoader interface {
	Get(ctx context.Context, resourceURL *url.URL) (*network.Response, error)
}

// Browser owns the state for one browser window.
//
// The MVP supports one active page and a linear navigation history. The Go
// runtime will be added as a separate responsibility in a later step.
type Browser struct {
	mu           sync.RWMutex
	page         *Page
	client       ResourceLoader
	navigationID uint64
	history      history
}

// New creates a browser with no page loaded.
func New(client ResourceLoader) *Browser {
	return &Browser{client: client, history: newHistory()}
}

// Page returns the currently active page, or nil before the first successful
// navigation.
func (b *Browser) Page() *Page {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.page
}

// SetPage replaces the active page. Passing nil clears the active page.
func (b *Browser) SetPage(page *Page) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.navigationID++
	b.page = page
	if page == nil {
		b.history = newHistory()
	} else {
		b.history.reset(page.URL)
	}
}

// Navigate retrieves an HTML document and makes it the active page. The
// current page is preserved if validation or loading fails.
func (b *Browser) Navigate(ctx context.Context, rawURL string) (*Page, error) {
	pageURL, err := normalizeURL(rawURL)
	if err != nil {
		return nil, err
	}
	return b.load(ctx, pageURL, historyPush, -1)
}

// Back loads the previous successful navigation entry.
func (b *Browser) Back(ctx context.Context) (*Page, error) {
	return b.traverse(ctx, -1)
}

// Forward loads the next navigation entry after a successful Back.
func (b *Browser) Forward(ctx context.Context) (*Page, error) {
	return b.traverse(ctx, 1)
}

// Reload refreshes the active page without adding a history entry.
func (b *Browser) Reload(ctx context.Context) (*Page, error) {
	b.mu.RLock()
	if b.page == nil || b.page.URL == nil {
		b.mu.RUnlock()
		return nil, errors.New("no active page to reload")
	}
	pageURL := cloneURL(b.page.URL)
	index := b.history.index
	b.mu.RUnlock()
	return b.load(ctx, pageURL, historyReplace, index)
}

// CanBack reports whether Back has a target entry.
func (b *Browser) CanBack() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.history.canBack()
}

// CanForward reports whether Forward has a target entry.
func (b *Browser) CanForward() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.history.canForward()
}

func (b *Browser) traverse(ctx context.Context, delta int) (*Page, error) {
	b.mu.RLock()
	target, index, ok := b.history.target(delta)
	b.mu.RUnlock()
	if !ok {
		return nil, errors.New("no history entry in requested direction")
	}
	return b.load(ctx, target, historyTraverse, index)
}

type historyCommit uint8

const (
	historyPush historyCommit = iota
	historyTraverse
	historyReplace
)

func (b *Browser) load(ctx context.Context, pageURL *url.URL, commit historyCommit, historyIndex int) (*Page, error) {
	b.mu.Lock()
	b.navigationID++
	navigationID := b.navigationID
	client := b.client
	b.mu.Unlock()

	if client == nil {
		return nil, errors.New("network client is not configured")
	}

	response, err := client.Get(ctx, pageURL)
	if err != nil {
		return nil, fmt.Errorf("navigate to %s: %w", pageURL.Redacted(), err)
	}

	mediaType, _, err := mime.ParseMediaType(response.ContentType)
	if err != nil {
		return nil, fmt.Errorf("invalid Content-Type %q: %w", response.ContentType, err)
	}
	if mediaType != "text/html" && mediaType != "application/xhtml+xml" {
		return nil, fmt.Errorf("unsupported Content-Type %q", mediaType)
	}
	document, err := htmlparser.Parse(bytes.NewReader(response.Body))
	if err != nil {
		return nil, fmt.Errorf("build DOM for %s: %w", pageURL.Redacted(), err)
	}
	stylesheet, err := b.loadStyles(ctx, client, response.URL, document)
	if err != nil {
		return nil, fmt.Errorf("load styles for %s: %w", pageURL.Redacted(), err)
	}
	computedStyles := style.Compute(document, stylesheet)

	page := &Page{
		URL:            cloneURL(response.URL),
		StatusCode:     response.StatusCode,
		ContentType:    response.ContentType,
		Source:         append([]byte(nil), response.Body...),
		Document:       document,
		Stylesheet:     stylesheet,
		ComputedStyles: computedStyles,
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if navigationID != b.navigationID {
		return nil, context.Canceled
	}
	b.page = page
	switch commit {
	case historyPush:
		b.history.push(page.URL)
	case historyTraverse:
		b.history.index = historyIndex
		b.history.replace(page.URL)
	case historyReplace:
		b.history.index = historyIndex
		b.history.replace(page.URL)
	}
	return page, nil
}

func normalizeURL(rawURL string) (*url.URL, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil, errors.New("URL is empty")
	}
	if !strings.Contains(rawURL, "://") {
		if strings.HasPrefix(rawURL, "localhost") || strings.HasPrefix(rawURL, "127.0.0.1") || strings.HasPrefix(rawURL, "[::1]") {
			rawURL = "http://" + rawURL
		} else {
			rawURL = "https://" + rawURL
		}
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parse URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("unsupported URL scheme %q", parsed.Scheme)
	}
	if parsed.Host == "" {
		return nil, errors.New("URL host is empty")
	}
	parsed.Fragment = ""
	return parsed, nil
}
