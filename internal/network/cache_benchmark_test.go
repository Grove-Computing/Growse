package network

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"
)

func BenchmarkHitAndRevalidate1000HTTPCacheEntries(b *testing.B) {
	cache := NewHTTPCache()
	now := time.Date(2026, time.August, 15, 12, 0, 0, 0, time.UTC)
	cache.now = func() time.Time { return now }
	requests := make([]*Request, 1000)
	for index := range requests {
		target, err := url.Parse(fmt.Sprintf("https://example.test/resources/%04d", index))
		if err != nil {
			b.Fatal(err)
		}
		requests[index] = &Request{Method: http.MethodGet, URL: target}
		header := http.Header{"Cache-Control": []string{"max-age=3600"}}
		if index%2 != 0 {
			header = http.Header{"Cache-Control": []string{"no-cache"}, "ETag": []string{fmt.Sprintf(`"v%d"`, index)}}
		}
		if !cache.Store(requests[index], &Response{URL: target, StatusCode: http.StatusOK, Header: header, Body: []byte("cached")}) {
			b.Fatalf("Store(%d) failed", index)
		}
	}
	b.ReportAllocs()
	b.ReportMetric(1000, "entries/op")
	b.ResetTimer()
	for b.Loop() {
		for index, request := range requests {
			if index%2 == 0 {
				if _, ok := cache.MatchFresh(request); !ok {
					b.Fatalf("fresh Match(%d) failed", index)
				}
				continue
			}
			if header, ok := cache.RevalidationHeaders(request); !ok || header.Get("If-None-Match") == "" {
				b.Fatalf("RevalidationHeaders(%d) failed", index)
			}
		}
	}
}

func BenchmarkSharedHTTPCacheHitAcross64Tabs(b *testing.B) {
	cache := NewHTTPCache()
	target, err := url.Parse("https://example.test/shared.css")
	if err != nil {
		b.Fatal(err)
	}
	request := &Request{Method: http.MethodGet, URL: target, SiteURL: target, Kind: RequestSubresource}
	if !cache.Store(request, &Response{
		URL: target, StatusCode: http.StatusOK,
		Header: http.Header{"Cache-Control": []string{"max-age=3600"}}, Body: []byte("body{}"),
	}) {
		b.Fatal("Store() failed")
	}
	b.ReportAllocs()
	b.ReportMetric(64, "hits/op")
	b.ResetTimer()
	for b.Loop() {
		for range 64 {
			if _, ok := cache.MatchFresh(request); !ok {
				b.Fatal("shared cache miss")
			}
		}
	}
}
