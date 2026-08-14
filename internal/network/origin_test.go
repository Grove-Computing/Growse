package network

import (
	"net/url"
	"testing"
)

func TestSameOriginUsesSchemeHostAndEffectivePort(t *testing.T) {
	base := parseOriginURL(t, "https://user:secret@Example.COM:443/app?q=1#part")
	tests := []struct {
		name string
		raw  string
		same bool
	}{
		{name: "path query fragment ignored", raw: "https://example.com/other?q=2#next", same: true},
		{name: "explicit default port", raw: "https://example.com:443/", same: true},
		{name: "different scheme", raw: "http://example.com/", same: false},
		{name: "different host", raw: "https://api.example.com/", same: false},
		{name: "different port", raw: "https://example.com:8443/", same: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := SameOrigin(base, parseOriginURL(t, test.raw)); got != test.same {
				t.Fatalf("SameOrigin() = %t, want %t", got, test.same)
			}
		})
	}
}

func TestOriginStringUsesEffectivePort(t *testing.T) {
	for rawURL, want := range map[string]string{
		"http://example.com:80/path":    "http://example.com",
		"https://example.com/path":      "https://example.com",
		"https://example.com:8443/path": "https://example.com:8443",
	} {
		origin, err := OriginFromURL(parseOriginURL(t, rawURL))
		if err != nil {
			t.Fatalf("OriginFromURL(%q) error = %v", rawURL, err)
		}
		if got := origin.String(); got != want {
			t.Fatalf("Origin(%q) = %q, want %q", rawURL, got, want)
		}
	}
}

func parseOriginURL(t *testing.T, rawURL string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("url.Parse(%q): %v", rawURL, err)
	}
	return parsed
}
