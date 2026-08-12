// Package browser contains the state and navigation lifecycle of a Growse
// browser window.
package browser

import (
	"context"
	"errors"
	"fmt"
	"mime"
	"net/url"
	"strings"
	"sync"

	"github.com/saku0512/growse/internal/network"
)

// ResourceLoader retrieves a resource for navigation.
type ResourceLoader interface {
	Get(ctx context.Context, resourceURL *url.URL) (*network.Response, error)
}

// Browser owns the state for one browser window.
//
// The MVP supports one active page. Network loading, history, and the Go
// runtime will be added as separate responsibilities in later steps.
type Browser struct {
	mu           sync.RWMutex
	page         *Page
	client       ResourceLoader
	navigationID uint64
}

// New creates a browser with no page loaded.
func New(client ResourceLoader) *Browser {
	return &Browser{client: client}
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
}

// Navigate retrieves an HTML document and makes it the active page. The
// current page is preserved if validation or loading fails.
func (b *Browser) Navigate(ctx context.Context, rawURL string) (*Page, error) {
	b.mu.Lock()
	b.navigationID++
	navigationID := b.navigationID
	client := b.client
	b.mu.Unlock()

	if client == nil {
		return nil, errors.New("network client is not configured")
	}

	pageURL, err := normalizeURL(rawURL)
	if err != nil {
		return nil, err
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

	page := &Page{
		URL:         cloneURL(response.URL),
		StatusCode:  response.StatusCode,
		ContentType: response.ContentType,
		Source:      append([]byte(nil), response.Body...),
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if navigationID != b.navigationID {
		return nil, context.Canceled
	}
	b.page = page
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
