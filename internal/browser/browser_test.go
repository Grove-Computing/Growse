package browser

import (
	"context"
	"errors"
	"net/url"
	"testing"

	"github.com/saku0512/growse/internal/dom"
	"github.com/saku0512/growse/internal/events"
	"github.com/saku0512/growse/internal/network"
)

type stubLoader struct {
	response *network.Response
	err      error
}

type routeLoader struct {
	responses map[string]*network.Response
	requested []string
}

func (loader *routeLoader) Get(_ context.Context, resourceURL *url.URL) (*network.Response, error) {
	loader.requested = append(loader.requested, resourceURL.String())
	response, ok := loader.responses[resourceURL.String()]
	if !ok {
		return nil, errors.New("unexpected URL: " + resourceURL.String())
	}
	return response, nil
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

func TestSetInputValueUpdatesActiveTextInput(t *testing.T) {
	document := dom.NewDocument()
	input := document.CreateElement("input", map[string]string{"type": "text"})
	if err := document.AppendChild(document.Root, input); err != nil {
		t.Fatal(err)
	}
	page := NewPage(mustParseURL(t, "http://localhost"))
	page.Document = document
	page.Events = events.NewDispatcher()
	var inputEvent events.Event
	page.Events.AddEventListener(input.ID, events.Input, func(event events.Event) { inputEvent = event })
	browser := New(nil)
	browser.SetPage(page)
	mutations := 0
	browser.SetOnMutation(func() { mutations++ })

	if !browser.SetInputValue(input.ID, "hello") {
		t.Fatal("SetInputValue() = false, want true")
	}
	if got, ok := input.Attribute("value"); !ok || got != "hello" {
		t.Fatalf("input value = (%q, %v), want (hello, true)", got, ok)
	}
	if browser.SetInputValue(input.ID, "hello") {
		t.Fatal("SetInputValue() = true for unchanged value")
	}
	if got, want := mutations, 1; got != want {
		t.Fatalf("mutation count = %d, want %d", got, want)
	}
	if inputEvent.Type != events.Input || inputEvent.Target != input.ID || inputEvent.Value != "hello" {
		t.Fatalf("input event = %#v, want updated value", inputEvent)
	}
}

func TestSetInputValueRejectsUnsupportedOrInactiveNode(t *testing.T) {
	document := dom.NewDocument()
	checkbox := document.CreateElement("input", map[string]string{"type": "checkbox"})
	paragraph := document.CreateElement("p", nil)
	detached := document.CreateElement("input", nil)
	for _, node := range []*dom.Node{checkbox, paragraph} {
		if err := document.AppendChild(document.Root, node); err != nil {
			t.Fatal(err)
		}
	}
	page := NewPage(mustParseURL(t, "http://localhost"))
	page.Document = document
	browser := New(nil)
	browser.SetPage(page)

	for _, node := range []*dom.Node{checkbox, paragraph, detached} {
		if browser.SetInputValue(node.ID, "value") {
			t.Fatalf("SetInputValue(%s) = true, want false", node.TagName)
		}
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

func TestBackAndForwardLoadHistoryEntries(t *testing.T) {
	firstURL := mustParseURL(t, "https://example.com/first")
	secondURL := mustParseURL(t, "https://example.com/second")
	loader := &routeLoader{responses: map[string]*network.Response{
		firstURL.String():  {URL: firstURL, StatusCode: 200, ContentType: "text/html", Body: []byte("<p>First</p>")},
		secondURL.String(): {URL: secondURL, StatusCode: 200, ContentType: "text/html", Body: []byte("<p>Second</p>")},
	}}
	browser := New(loader)
	if _, err := browser.Navigate(context.Background(), firstURL.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := browser.Navigate(context.Background(), secondURL.String()); err != nil {
		t.Fatal(err)
	}
	if !browser.CanBack() || browser.CanForward() {
		t.Fatalf("history state after navigation = back:%v forward:%v", browser.CanBack(), browser.CanForward())
	}

	page, err := browser.Back(context.Background())
	if err != nil || page.URL.String() != firstURL.String() {
		t.Fatalf("Back() = (%v, %v), want first page", page, err)
	}
	if browser.CanBack() || !browser.CanForward() {
		t.Fatalf("history state after Back = back:%v forward:%v", browser.CanBack(), browser.CanForward())
	}

	page, err = browser.Forward(context.Background())
	if err != nil || page.URL.String() != secondURL.String() {
		t.Fatalf("Forward() = (%v, %v), want second page", page, err)
	}
}

func TestFailedBackPreservesPageAndHistoryIndex(t *testing.T) {
	firstURL := mustParseURL(t, "https://example.com/first")
	secondURL := mustParseURL(t, "https://example.com/second")
	loader := &routeLoader{responses: map[string]*network.Response{
		firstURL.String():  {URL: firstURL, StatusCode: 200, ContentType: "text/html", Body: []byte("<p>First</p>")},
		secondURL.String(): {URL: secondURL, StatusCode: 200, ContentType: "text/html", Body: []byte("<p>Second</p>")},
	}}
	browser := New(loader)
	if _, err := browser.Navigate(context.Background(), firstURL.String()); err != nil {
		t.Fatal(err)
	}
	current, err := browser.Navigate(context.Background(), secondURL.String())
	if err != nil {
		t.Fatal(err)
	}
	delete(loader.responses, firstURL.String())

	if _, err := browser.Back(context.Background()); err == nil {
		t.Fatal("Back() error = nil, want load failure")
	}
	if browser.Page() != current || !browser.CanBack() || browser.CanForward() {
		t.Fatal("failed Back changed the active page or history position")
	}
}

func TestReloadDoesNotAddHistoryEntry(t *testing.T) {
	pageURL := mustParseURL(t, "https://example.com/page")
	loader := &routeLoader{responses: map[string]*network.Response{
		pageURL.String(): {URL: pageURL, StatusCode: 200, ContentType: "text/html", Body: []byte("<p>Page</p>")},
	}}
	browser := New(loader)
	if _, err := browser.Navigate(context.Background(), pageURL.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := browser.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got, want := len(browser.history.entries), 1; got != want {
		t.Fatalf("history entries after Reload = %d, want %d", got, want)
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

func TestNavigateLoadsInlineAndSameOriginStylesheets(t *testing.T) {
	pageURL := mustParseURL(t, "https://example.com/index.html")
	cssURL := mustParseURL(t, "https://example.com/site.css")
	loader := &routeLoader{responses: map[string]*network.Response{
		pageURL.String(): {
			URL: pageURL, StatusCode: 200, ContentType: "text/html",
			Body: []byte(`<html><head>
<link rel="stylesheet" href="/site.css">
<link rel="stylesheet" href="https://cdn.example.org/ignored.css">
<style>#title { font-size: 30px; }</style>
</head><body><h1 id="title" class="hero">Hello</h1></body></html>`),
		},
		cssURL.String(): {
			URL: cssURL, StatusCode: 200, ContentType: "text/css",
			Body: []byte(`.hero { color: #123456; }`),
		},
	}}
	browser := New(loader)

	page, err := browser.Navigate(context.Background(), pageURL.String())
	if err != nil {
		t.Fatalf("Navigate() error = %v", err)
	}
	if got, want := len(loader.requested), 2; got != want {
		t.Fatalf("request count = %d, want %d (%v)", got, want, loader.requested)
	}
	title, ok := page.Document.GetElementByID("title")
	if !ok {
		t.Fatal("title element was not indexed")
	}
	computed, ok := page.ComputedStyles.For(title)
	if !ok {
		t.Fatal("title element has no computed style")
	}
	if got, want := computed.Color, uint32(0x123456ff); got != want {
		t.Fatalf("title color = %#x, want %#x", got, want)
	}
	if got, want := computed.FontSize, float32(30); got != want {
		t.Fatalf("title font size = %v, want %v", got, want)
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

func TestPageLinkURLResolvesNearestAnchor(t *testing.T) {
	document := dom.NewDocument()
	anchor := document.CreateElement("a", map[string]string{"href": "../next?q=1"})
	span := document.CreateElement("span", nil)
	text := document.CreateText("Next")
	for _, edge := range [][2]*dom.Node{{document.Root, anchor}, {anchor, span}, {span, text}} {
		if err := document.AppendChild(edge[0], edge[1]); err != nil {
			t.Fatal(err)
		}
	}
	page := &Page{URL: mustParseURL(t, "https://example.com/docs/current"), Document: document}

	resolved, ok := page.LinkURL(span.ID)
	if !ok || resolved.String() != "https://example.com/next?q=1" {
		t.Fatalf("LinkURL() = (%v, %v), want resolved relative URL", resolved, ok)
	}
}

func TestPageLinkURLRejectsUnsupportedScheme(t *testing.T) {
	document := dom.NewDocument()
	anchor := document.CreateElement("a", map[string]string{"href": "javascript:alert(1)"})
	if err := document.AppendChild(document.Root, anchor); err != nil {
		t.Fatal(err)
	}
	page := &Page{URL: mustParseURL(t, "https://example.com"), Document: document}
	if resolved, ok := page.LinkURL(anchor.ID); ok || resolved != nil {
		t.Fatalf("LinkURL() = (%v, %v), want unsupported scheme rejected", resolved, ok)
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
