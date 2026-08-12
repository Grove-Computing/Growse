package browser

import (
	"context"
	"errors"
	"net/url"
	"testing"

	"github.com/saku0512/growse/internal/network"
)

type stubLoader struct {
	response *network.Response
	err      error
}

func (loader stubLoader) Get(context.Context, *url.URL) (*network.Response, error) {
	return loader.response, loader.err
}

func TestNewHasNoActivePage(t *testing.T) {
	browser := New(nil)

	if browser.Page() != nil {
		t.Fatal("a new browser should not have an active page")
	}
}

func TestSetPageReplacesActivePage(t *testing.T) {
	browser := New(nil)
	first := NewPage(mustParseURL(t, "https://example.com/first"))
	second := NewPage(mustParseURL(t, "https://example.com/second"))

	browser.SetPage(first)
	browser.SetPage(second)

	if got := browser.Page(); got != second {
		t.Fatalf("active page = %p, want %p", got, second)
	}
}

func TestSetPageCanClearActivePage(t *testing.T) {
	browser := New(nil)
	browser.SetPage(NewPage(mustParseURL(t, "https://example.com")))

	browser.SetPage(nil)

	if browser.Page() != nil {
		t.Fatal("active page should be cleared")
	}
}

func TestNavigateLoadsHTMLAndUpdatesPage(t *testing.T) {
	finalURL := mustParseURL(t, "https://example.com/final")
	browser := New(stubLoader{response: &network.Response{
		URL:         finalURL,
		StatusCode:  200,
		ContentType: "text/html; charset=utf-8",
		Body:        []byte("<h1>Hello</h1>"),
	}})

	page, err := browser.Navigate(context.Background(), "example.com/start#section")
	if err != nil {
		t.Fatalf("Navigate() error = %v", err)
	}
	if browser.Page() != page {
		t.Fatal("successful navigation did not activate the loaded page")
	}
	if got, want := page.URL.String(), finalURL.String(); got != want {
		t.Fatalf("page URL = %q, want %q", got, want)
	}
	if got, want := string(page.Source), "<h1>Hello</h1>"; got != want {
		t.Fatalf("page source = %q, want %q", got, want)
	}
	if page.Document == nil || page.Document.ElementCount() == 0 {
		t.Fatal("successful navigation did not build a DOM")
	}
}

func TestNavigatePreservesPageOnFailure(t *testing.T) {
	loadErr := errors.New("network unavailable")
	browser := New(stubLoader{err: loadErr})
	current := NewPage(mustParseURL(t, "https://example.com/current"))
	browser.SetPage(current)

	_, err := browser.Navigate(context.Background(), "https://example.com/next")
	if !errors.Is(err, loadErr) {
		t.Fatalf("Navigate() error = %v, want %v", err, loadErr)
	}
	if browser.Page() != current {
		t.Fatal("failed navigation replaced the active page")
	}
}

func TestNavigateRejectsUnsupportedContentType(t *testing.T) {
	browser := New(stubLoader{response: &network.Response{
		URL:         mustParseURL(t, "https://example.com/image.png"),
		StatusCode:  200,
		ContentType: "image/png",
	}})

	if _, err := browser.Navigate(context.Background(), "https://example.com/image.png"); err == nil {
		t.Fatal("Navigate() error = nil, want unsupported Content-Type error")
	}
}

func TestNormalizeURL(t *testing.T) {
	tests := []struct {
		name    string
		rawURL  string
		want    string
		wantErr bool
	}{
		{name: "HTTPS default", rawURL: "example.com/path", want: "https://example.com/path"},
		{name: "localhost HTTP", rawURL: "localhost:8080", want: "http://localhost:8080"},
		{name: "trim and remove fragment", rawURL: " https://example.com/#top ", want: "https://example.com/"},
		{name: "empty", rawURL: " ", wantErr: true},
		{name: "unsupported scheme", rawURL: "file:///tmp/index.html", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := normalizeURL(test.rawURL)
			if test.wantErr {
				if err == nil {
					t.Fatalf("normalizeURL(%q) error = nil", test.rawURL)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeURL(%q) error = %v", test.rawURL, err)
			}
			if got.String() != test.want {
				t.Fatalf("normalizeURL(%q) = %q, want %q", test.rawURL, got, test.want)
			}
		})
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
