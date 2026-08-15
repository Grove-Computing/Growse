package browser

import (
	"context"
	"errors"
	"testing"

	"github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/events"
	"github.com/Grove-Computing/Growse/internal/network"
)

// WPT source: html/semantics/links/links-created-by-a-and-area-elements/target_blank_implicit_noopener.html.
func TestWPTTargetBlankCreatesDistinctTopLevelContext(t *testing.T) {
	document := dom.NewDocument()
	form := document.CreateElement("form", map[string]string{"action": "/target", "target": "_blank"})
	if err := document.AppendChild(document.Root, form); err != nil {
		t.Fatal(err)
	}
	sourceURL := mustURL(t, "https://example.test/source")
	targetURL := mustURL(t, "https://example.test/target")
	source := New(nil)
	source.SetPage(&Page{URL: sourceURL, Document: document, Events: events.NewDispatcher()})
	loader := &routeLoader{responses: map[string]*network.Response{
		targetURL.String(): {URL: targetURL, StatusCode: 200, ContentType: "text/html", Body: []byte("<title>Target</title>")},
	}}
	created := 0
	session := NewSession(func() *Browser {
		created++
		if created == 1 {
			return source
		}
		return New(loader)
	})
	defer session.Close()
	sourceTab, err := session.NewTab(nil)
	if err != nil {
		t.Fatal(err)
	}
	opened, page, err := session.SubmitFormToNewTab(context.Background(), sourceTab.ID, form.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if opened.ID == sourceTab.ID || page.URL.String() != targetURL.String() || source.Page().URL.String() != sourceURL.String() {
		t.Fatalf("target blank contexts = source:%+v opened:%+v page:%s", sourceTab, opened, page.URL)
	}
}

// WPT source: html/browsers/windows/auxiliary-browsing-contexts/opener-closed.html.
func TestWPTClosedBrowsingContextRejectsFutureWork(t *testing.T) {
	session := NewSession()
	closed, err := session.NewTab(nil)
	if err != nil {
		t.Fatal(err)
	}
	live, err := session.NewTab(nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := session.CloseTab(closed.ID); err != nil {
		t.Fatal(err)
	}
	if session.dispatchToTab(closed.ID, func(*Tab) { t.Fatal("closed browsing context received work") }) {
		t.Fatal("closed browsing context remained dispatchable")
	}
	if _, err := session.SelectTab(closed.ID); !errors.Is(err, ErrTabNotFound) {
		t.Fatalf("closed context selection error = %v", err)
	}
	if _, err := session.SelectTab(live.ID); err != nil {
		t.Fatalf("live sibling context was affected: %v", err)
	}
}
