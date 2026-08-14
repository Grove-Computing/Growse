package network

import (
	"net/http"
	"testing"
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
