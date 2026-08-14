package network

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
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

// WPT source: fetch/http-cache/freshness.any.js.
func TestWPTHTTPCacheMaxAgeOverridesExpiresAndAgeCanMakeItStale(t *testing.T) {
	now := time.Date(2026, time.August, 15, 12, 0, 0, 0, time.UTC)
	request := &Request{Method: http.MethodGet, URL: parseOriginURL(t, "https://example.test/data")}
	cache := NewHTTPCache()
	cache.now = func() time.Time { return now }
	cache.Store(request, &Response{StatusCode: http.StatusOK, Header: http.Header{
		"Cache-Control": []string{"max-age=3600"}, "Expires": []string{now.Add(-time.Hour).Format(http.TimeFormat)},
	}})
	if _, ok := cache.MatchFresh(request); !ok {
		t.Fatal("positive max-age did not override past Expires")
	}
	aged := NewHTTPCache()
	aged.now = func() time.Time { return now }
	aged.Store(request, &Response{StatusCode: http.StatusOK, Header: http.Header{
		"Cache-Control": []string{"max-age=3600"}, "Age": []string{"12000"},
	}})
	if _, ok := aged.MatchFresh(request); ok {
		t.Fatal("Age greater than freshness lifetime was reused")
	}
}

// WPT source: fetch/http-cache/invalidate.any.js.
func TestWPTFailedUnsafeRequestDoesNotInvalidateFreshEntry(t *testing.T) {
	getRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodPost {
			response.WriteHeader(http.StatusInternalServerError)
			return
		}
		getRequests++
		response.Header().Set("Cache-Control", "max-age=60")
		_, _ = response.Write([]byte("fresh"))
	}))
	defer server.Close()
	client := NewClientWithLimits(server.Client(), 1024)
	target := parseOriginURL(t, server.URL+"/data")
	if _, err := client.Get(context.Background(), target); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Do(context.Background(), &Request{Method: http.MethodPost, URL: target}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Get(context.Background(), target); err != nil {
		t.Fatal(err)
	}
	if getRequests != 1 {
		t.Fatalf("GET requests after failed POST = %d, want cached 1", getRequests)
	}
}
