package network

import (
	"net/http"
	"testing"
	"time"
)

func TestHTTPCacheKeysGETHEADURLPartitionAndVary(t *testing.T) {
	cache := NewHTTPCache()
	target := mustParseURL(t, "https://cdn.test/app.css#first")
	request := &Request{
		Method: http.MethodGet, URL: target, SiteURL: mustParseURL(t, "https://site.test/page"), Kind: RequestSubresource,
		Header: http.Header{"Accept-Language": []string{"ja"}},
	}
	response := &Response{URL: target, StatusCode: http.StatusOK, Header: http.Header{"Vary": []string{"Accept-Language"}}, Body: []byte("ja")}
	if !cache.Store(request, response) {
		t.Fatal("Store() rejected cacheable GET")
	}
	fragmentVariant := *request
	fragmentVariant.URL = mustParseURL(t, "https://cdn.test/app.css#second")
	if got, ok := cache.Match(&fragmentVariant); !ok || string(got.Body) != "ja" {
		t.Fatalf("fragment-insensitive Match() = (%v, %v)", got, ok)
	}
	languageVariant := fragmentVariant
	languageVariant.Header = http.Header{"Accept-Language": []string{"en"}}
	if _, ok := cache.Match(&languageVariant); ok {
		t.Fatal("Vary mismatch produced a cache hit")
	}
	head := fragmentVariant
	head.Method = http.MethodHead
	if _, ok := cache.Match(&head); ok {
		t.Fatal("HEAD reused a GET cache entry")
	}
	otherPartition := fragmentVariant
	otherPartition.SiteURL = mustParseURL(t, "https://other.test/page")
	if _, ok := cache.Match(&otherPartition); ok {
		t.Fatal("cross-partition request reused cache entry")
	}
}

func TestCalculateFreshnessUsesMaxAgeDateExpiresAndAge(t *testing.T) {
	storedAt := time.Date(2026, time.August, 15, 12, 0, 10, 0, time.UTC)
	tests := []struct {
		name       string
		header     http.Header
		freshAfter time.Duration
		staleAfter time.Duration
	}{
		{name: "max-age with apparent age", header: http.Header{
			"Cache-Control": []string{"max-age=60"}, "Date": []string{storedAt.Add(-10 * time.Second).Format(http.TimeFormat)},
		}, freshAfter: 49 * time.Second, staleAfter: 50 * time.Second},
		{name: "Expires with Age", header: http.Header{
			"Date": []string{storedAt.Format(http.TimeFormat)}, "Expires": []string{storedAt.Add(time.Minute).Format(http.TimeFormat)}, "Age": []string{"20"},
		}, freshAfter: 39 * time.Second, staleAfter: 40 * time.Second},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			freshness := calculateFreshness(test.header, storedAt)
			if !freshness.fresh(storedAt.Add(test.freshAfter)) {
				t.Fatal("entry became stale too early")
			}
			if freshness.fresh(storedAt.Add(test.staleAfter)) {
				t.Fatal("entry remained fresh at lifetime boundary")
			}
		})
	}
}

func TestCalculateFreshnessUsesBoundedLastModifiedHeuristic(t *testing.T) {
	storedAt := time.Date(2026, time.August, 15, 12, 0, 0, 0, time.UTC)
	header := http.Header{
		"Date":          []string{storedAt.Format(http.TimeFormat)},
		"Last-Modified": []string{storedAt.Add(-20 * 24 * time.Hour).Format(http.TimeFormat)},
	}
	value := calculateFreshness(header, storedAt)
	if value.lifetime != 24*time.Hour {
		t.Fatalf("heuristic lifetime = %v, want 24h cap", value.lifetime)
	}
}

func TestCalculateFreshnessBoundsHugeAgeAndMaxAge(t *testing.T) {
	storedAt := time.Now()
	value := calculateFreshness(http.Header{
		"Cache-Control": []string{"max-age=18446744073709551615"},
		"Age":           []string{"18446744073709551615"},
	}, storedAt)
	if value.lifetime < 0 || value.initialAge < 0 {
		t.Fatalf("overflowed freshness = %#v", value)
	}
}

func TestParseCacheControlPolicyAndNoStore(t *testing.T) {
	policy := parseCachePolicy([]string{"private, no-cache, must-revalidate", "public, immutable, future-extension=1"})
	if !policy.private || !policy.public || !policy.noCache || !policy.mustRevalidate || !policy.immutable || policy.noStore {
		t.Fatalf("policy = %#v", policy)
	}
	cache := NewHTTPCache()
	request := &Request{Method: http.MethodGet, URL: mustParseURL(t, "https://example.test/data")}
	response := &Response{StatusCode: http.StatusOK, Header: http.Header{"Cache-Control": []string{"no-store, max-age=3600"}}}
	if cache.Store(request, response) {
		t.Fatal("no-store Response was cached")
	}
	if _, ok := cache.Match(request); ok {
		t.Fatal("no-store Response was reusable")
	}
}

func TestPrivateAndPublicResponsesCanBeStoredInPrivateCache(t *testing.T) {
	for _, directive := range []string{"private, max-age=60", "public, max-age=60", "no-cache"} {
		cache := NewHTTPCache()
		request := &Request{Method: http.MethodGet, URL: mustParseURL(t, "https://example.test/"+directive[:2])}
		response := &Response{StatusCode: http.StatusOK, Header: http.Header{"Cache-Control": []string{directive}}}
		if !cache.Store(request, response) {
			t.Fatalf("Store() rejected %q in private cache", directive)
		}
	}
}

func TestMatchFreshRejectsStaleAndNoCacheEntries(t *testing.T) {
	now := time.Date(2026, time.August, 15, 12, 0, 0, 0, time.UTC)
	request := &Request{Method: http.MethodGet, URL: mustParseURL(t, "https://example.test/data")}
	cache := NewHTTPCache()
	cache.now = func() time.Time { return now }
	if !cache.Store(request, &Response{StatusCode: http.StatusOK, Header: http.Header{"Cache-Control": []string{"max-age=10"}}}) {
		t.Fatal("Store() failed")
	}
	if _, ok := cache.MatchFresh(request); !ok {
		t.Fatal("fresh entry was not reusable")
	}
	now = now.Add(10 * time.Second)
	if _, ok := cache.MatchFresh(request); ok {
		t.Fatal("stale entry was reusable")
	}
	noCache := NewHTTPCache()
	noCache.now = func() time.Time { return now }
	noCache.Store(request, &Response{StatusCode: http.StatusOK, Header: http.Header{"Cache-Control": []string{"no-cache, max-age=3600"}}})
	if _, ok := noCache.MatchFresh(request); ok {
		t.Fatal("no-cache entry was reused without validation")
	}
}

func TestHTTPCacheRejectsUnsafeOrUnusableKeys(t *testing.T) {
	cache := NewHTTPCache()
	response := &Response{StatusCode: http.StatusOK, Header: make(http.Header)}
	for _, request := range []*Request{
		{Method: http.MethodPost, URL: mustParseURL(t, "https://example.test/")},
		{Method: http.MethodGet, URL: mustParseURL(t, "https://user:secret@example.test/")},
		{Method: http.MethodGet, URL: mustParseURL(t, "file:///tmp/data")},
	} {
		if cache.Store(request, response) {
			t.Fatalf("Store(%v) accepted unsafe key", request.URL)
		}
	}
	varyStar := &Response{StatusCode: http.StatusOK, Header: http.Header{"Vary": []string{"*"}}}
	if cache.Store(&Request{Method: http.MethodGet, URL: mustParseURL(t, "https://example.test/")}, varyStar) {
		t.Fatal("Store() accepted Vary: *")
	}
}
