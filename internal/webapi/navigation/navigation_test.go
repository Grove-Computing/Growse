package navigation

import (
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
