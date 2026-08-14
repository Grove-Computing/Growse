package navigation

import (
	"errors"
	"net/url"
	"testing"
)

func TestCurrentReturnsDocumentURLComponentsWithoutCredentials(t *testing.T) {
	documentURL, err := url.Parse("https://alice:secret@example.test:8443/app/a%20b?mode=edit#details")
	if err != nil {
		t.Fatal(err)
	}

	got := New(documentURL).Current()
	if got.Href != "https://example.test:8443/app/a%20b?mode=edit#details" {
		t.Fatalf("Href = %q", got.Href)
	}
	if got.Origin != "https://example.test:8443" {
		t.Fatalf("Origin = %q", got.Origin)
	}
	if got.Scheme != "https" || got.Host != "example.test:8443" || got.Hostname != "example.test" || got.Port != "8443" {
		t.Fatalf("authority components = %#v", got)
	}
	if got.Path != "/app/a%20b" || got.Query != "mode=edit" || got.Fragment != "details" {
		t.Fatalf("resource components = %#v", got)
	}
}

func TestCurrentWithoutDocumentURLReturnsZeroValue(t *testing.T) {
	if got := New(nil).Current(); got != (Location{}) {
		t.Fatalf("Current() = %#v, want zero value", got)
	}
}

func TestResolveAndNavigateUseDocumentURLAsBase(t *testing.T) {
	base, err := url.Parse("https://example.test/app/pages/index.html")
	if err != nil {
		t.Fatal(err)
	}
	var navigated *url.URL
	api := NewPage(base, func(target *url.URL) error {
		navigated = target
		return nil
	})

	resolved, err := api.Resolve("../next?mode=full#result")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got, want := resolved.Href, "https://example.test/app/next?mode=full#result"; got != want {
		t.Fatalf("Resolve().Href = %q, want %q", got, want)
	}
	if got, want := resolved.Port, "443"; got != want {
		t.Fatalf("Resolve().Port = %q, want effective port %q", got, want)
	}
	if err := api.Navigate("../next?mode=full#result"); err != nil {
		t.Fatalf("Navigate() error = %v", err)
	}
	if navigated == nil || navigated.String() != resolved.Href {
		t.Fatalf("navigated URL = %v, want %q", navigated, resolved.Href)
	}
}

func TestNavigationRejectsUnsafeURLsBeforeCallback(t *testing.T) {
	base, _ := url.Parse("https://example.test/app/")
	called := false
	api := NewPage(base, func(*url.URL) error { called = true; return nil })
	for _, rawURL := range []string{
		"javascript:alert(1)",
		"data:text/plain,hello",
		"file:///tmp/secret",
		"https://alice:secret@example.test/private",
		"https://example.test:invalid/path",
		" /space",
	} {
		if err := api.Navigate(rawURL); !errors.Is(err, ErrInvalidURL) {
			t.Errorf("Navigate(%q) error = %v, want ErrInvalidURL", rawURL, err)
		}
	}
	if called {
		t.Fatal("unsafe URL reached Browser callback")
	}
}

func TestUpdateCurrentChangesLocationAndResolutionBase(t *testing.T) {
	initial, _ := url.Parse("https://example.test/notes#first")
	updated, _ := url.Parse("https://example.test/archive#second")
	api := New(initial)

	api.UpdateCurrent(updated)
	if got := api.Current().Href; got != updated.String() {
		t.Fatalf("Current().Href = %q, want %q", got, updated)
	}
	resolved, err := api.Resolve("next")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := resolved.Href, "https://example.test/next"; got != want {
		t.Fatalf("Resolve().Href = %q, want %q", got, want)
	}
}
