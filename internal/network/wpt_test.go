package network

import (
	"net/http"
	"testing"
)

// WPT source: url/url-origin.any.js.
func TestWPTURLOriginNormalizesDefaultPorts(t *testing.T) {
	if !SameOrigin(parseOriginURL(t, "https://example.test/path"), parseOriginURL(t, "https://example.test:443/other")) {
		t.Fatal("default HTTPS port did not produce the same origin")
	}
}

// WPT source: cookies/attributes/resources/path-redirect-shared.js.
func TestWPTCookiePathDoesNotMatchPrefixLookalike(t *testing.T) {
	jar := newPolicyCookieJar()
	origin := parseOriginURL(t, "https://example.test/cookies/attributes/path/one.html")
	jar.SetCookies(origin, []*http.Cookie{{Name: "path", Value: "yes", Path: "/cookies/attributes/path"}})
	if got := jar.Cookies(parseOriginURL(t, "https://example.test/cookies/attributes/pathfakeout/one.html")); len(got) != 0 {
		t.Fatalf("path lookalike received Cookies: %v", got)
	}
}

// WPT source: cors/cors-safelisted-request-header.any.js.
func TestWPTCORSSafelistedContentTypes(t *testing.T) {
	for _, contentType := range []string{"text/plain;charset=UTF-8", "application/x-www-form-urlencoded", "multipart/form-data; boundary=x"} {
		request, err := http.NewRequest(http.MethodPost, "https://api.example.test", nil)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", contentType)
		if !isSimpleCORSRequest(request) {
			t.Fatalf("Content-Type %q was not CORS-safelisted", contentType)
		}
	}
}
