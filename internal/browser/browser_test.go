package browser

import (
	"net/url"
	"testing"
)

func TestNewHasNoActivePage(t *testing.T) {
	browser := New()

	if browser.Page() != nil {
		t.Fatal("a new browser should not have an active page")
	}
}

func TestSetPageReplacesActivePage(t *testing.T) {
	browser := New()
	first := NewPage(mustParseURL(t, "https://example.com/first"))
	second := NewPage(mustParseURL(t, "https://example.com/second"))

	browser.SetPage(first)
	browser.SetPage(second)

	if got := browser.Page(); got != second {
		t.Fatalf("active page = %p, want %p", got, second)
	}
}

func TestSetPageCanClearActivePage(t *testing.T) {
	browser := New()
	browser.SetPage(NewPage(mustParseURL(t, "https://example.com")))

	browser.SetPage(nil)

	if browser.Page() != nil {
		t.Fatal("active page should be cleared")
	}
}

func TestNewPageCopiesURL(t *testing.T) {
	pageURL := mustParseURL(t, "https://example.com/original")
	page := NewPage(pageURL)
	pageURL.Path = "/changed"

	if got, want := page.URL.String(), "https://example.com/original"; got != want {
		t.Fatalf("page URL = %q, want %q", got, want)
	}
}

func mustParseURL(t *testing.T, rawURL string) *url.URL {
	t.Helper()

	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("url.Parse(%q): %v", rawURL, err)
	}
	return parsed
}
