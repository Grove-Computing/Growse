package browser

import (
	"context"
	"testing"

	"github.com/Grove-Computing/Growse/internal/network"
)

// WPT source: html/browsers/history/the-history-interface/history_pushstate_url.html.
func TestWPTHistoryPushStateSetsURLAndBackRestoresIt(t *testing.T) {
	pageURL := mustParseURL(t, "https://example.test/page")
	loader := &routeLoader{responses: map[string]*network.Response{
		pageURL.String(): {URL: pageURL, StatusCode: 200, ContentType: "text/html", Body: []byte("<p>Page</p>")},
	}}
	browser := New(loader)
	page, err := browser.Navigate(context.Background(), pageURL.String())
	if err != nil {
		t.Fatal(err)
	}
	target := mustParseURL(t, pageURL.String()+"#hash")
	if err := browser.pushHistoryState(page, target, "null"); err != nil {
		t.Fatal(err)
	}
	if page.URL.String() != target.String() {
		t.Fatalf("pushState URL = %s, want %s", page.URL, target)
	}
	if _, err := browser.Back(context.Background()); err != nil {
		t.Fatal(err)
	}
	if page.URL.String() != pageURL.String() {
		t.Fatalf("Back URL = %s, want %s", page.URL, pageURL)
	}
}
