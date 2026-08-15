package network

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestDiskCacheRestoresFreshResponseAcrossClientRestart(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		requests++
		response.Header().Set("Cache-Control", "max-age=3600")
		response.Header().Set("Content-Type", "text/plain")
		_, _ = response.Write([]byte("persistent"))
	}))
	defer server.Close()
	root := t.TempDir()
	target := mustParseURL(t, server.URL+"/data")
	first, err := NewClientWithCacheRoot(server.Client(), 1024, root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.Get(context.Background(), target); err != nil {
		t.Fatal(err)
	}
	second, err := NewClientWithCacheRoot(server.Client(), 1024, root)
	if err != nil {
		t.Fatal(err)
	}
	response, err := second.Get(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	if requests != 1 || string(response.Body) != "persistent" {
		t.Fatalf("restored response = requests:%d body:%q", requests, response.Body)
	}
}

func TestDiskCacheAppliesEntryOriginTotalAndCountLimitsWithLRUEviction(t *testing.T) {
	limits := diskCacheLimits{maxEntries: 2, maxEntryBytes: 6, maxOriginBytes: 12, maxTotalBytes: 12}
	root := t.TempDir()
	cache := newTestPersistentCache(t, root, limits)
	requests := []*Request{
		{Method: http.MethodGet, URL: mustParseURL(t, "https://example.test/first")},
		{Method: http.MethodGet, URL: mustParseURL(t, "https://example.test/second")},
		{Method: http.MethodGet, URL: mustParseURL(t, "https://example.test/third")},
	}
	for _, request := range requests {
		cache.Store(request, diskTestResponse(request, []byte("123456")))
	}
	restarted := newTestPersistentCache(t, root, limits)
	if _, ok := restarted.Match(requests[0]); ok {
		t.Fatal("least-recently-used entry survived deterministic eviction")
	}
	for _, request := range requests[1:] {
		if _, ok := restarted.Match(request); !ok {
			t.Fatalf("recent entry %s was evicted", request.URL)
		}
	}

	oversized := &Request{Method: http.MethodGet, URL: mustParseURL(t, "https://example.test/oversized")}
	cache.Store(oversized, diskTestResponse(oversized, []byte("1234567")))
	if _, ok := newTestPersistentCache(t, root, limits).Match(oversized); ok {
		t.Fatal("oversized entry was persisted")
	}
}

func TestDiskCacheChecksumFailureRemovesOnlyCorruptEntry(t *testing.T) {
	root := t.TempDir()
	limits := diskCacheLimits{maxEntries: 4, maxEntryBytes: 1024, maxOriginBytes: 4096, maxTotalBytes: 4096}
	cache := newTestPersistentCache(t, root, limits)
	corrupt := &Request{Method: http.MethodGet, URL: mustParseURL(t, "https://example.test/corrupt")}
	healthy := &Request{Method: http.MethodGet, URL: mustParseURL(t, "https://example.test/healthy")}
	cache.Store(corrupt, diskTestResponse(corrupt, []byte("corrupt-body")))
	cache.Store(healthy, diskTestResponse(healthy, []byte("healthy-body")))
	key, _ := baseCacheKey(corrupt)
	id := diskEntryID(key, nil, nil)
	if err := os.WriteFile(filepath.Join(root, id+".body"), []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	restarted := newTestPersistentCache(t, root, limits)
	if _, ok := restarted.Match(corrupt); ok {
		t.Fatal("checksum-mismatched entry was returned")
	}
	if _, err := os.Stat(filepath.Join(root, id+".json")); !os.IsNotExist(err) {
		t.Fatalf("corrupt metadata still exists: %v", err)
	}
	if response, ok := restarted.Match(healthy); !ok || string(response.Body) != "healthy-body" {
		t.Fatalf("healthy sibling entry = (%v, %v)", response, ok)
	}
}

func TestDiskCacheSchemaMismatchRemovesOnlyIncompatibleEntry(t *testing.T) {
	root := t.TempDir()
	limits := diskCacheLimits{maxEntries: 4, maxEntryBytes: 1024, maxOriginBytes: 4096, maxTotalBytes: 4096}
	cache := newTestPersistentCache(t, root, limits)
	incompatible := &Request{Method: http.MethodGet, URL: mustParseURL(t, "https://example.test/old")}
	healthy := &Request{Method: http.MethodGet, URL: mustParseURL(t, "https://example.test/current")}
	cache.Store(incompatible, diskTestResponse(incompatible, []byte("old")))
	cache.Store(healthy, diskTestResponse(healthy, []byte("current")))
	key, _ := baseCacheKey(incompatible)
	id := diskEntryID(key, nil, nil)
	metadataPath := filepath.Join(root, id+".json")
	content, err := os.ReadFile(metadataPath)
	if err != nil {
		t.Fatal(err)
	}
	var record diskCacheRecord
	if err := json.Unmarshal(content, &record); err != nil {
		t.Fatal(err)
	}
	record.Version++
	content, err = json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(metadataPath, content, 0o600); err != nil {
		t.Fatalf("rewrite incompatible metadata: %v", err)
	}
	restarted := newTestPersistentCache(t, root, limits)
	if _, ok := restarted.Match(incompatible); ok {
		t.Fatal("schema-mismatched entry was restored")
	}
	if response, ok := restarted.Match(healthy); !ok || string(response.Body) != "current" {
		t.Fatalf("healthy sibling entry = (%v, %v)", response, ok)
	}
}

func TestConcurrentDiskCacheWritesKeepMetadataAndBodyTogether(t *testing.T) {
	root := t.TempDir()
	limits := diskCacheLimits{maxEntries: 8, maxEntryBytes: 1024, maxOriginBytes: 8192, maxTotalBytes: 8192}
	cache := newTestPersistentCache(t, root, limits)
	request := &Request{Method: http.MethodGet, URL: mustParseURL(t, "https://example.test/shared")}

	var wait sync.WaitGroup
	for index := 0; index < 32; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			value := fmt.Sprintf("response-%02d", index)
			cache.Store(request, &Response{
				URL: request.URL, StatusCode: http.StatusOK, ContentType: "text/plain",
				Header: http.Header{"Cache-Control": []string{"max-age=3600"}, "X-Body-ID": []string{value}},
				Body:   []byte(value),
			})
		}(index)
	}
	wait.Wait()

	restarted := newTestPersistentCache(t, root, limits)
	response, found := restarted.Match(request)
	if !found {
		t.Fatal("concurrently written entry was not restored")
	}
	if metadata, body := headerValue(response.Header, "X-Body-ID"), string(response.Body); metadata != body {
		t.Fatalf("cache metadata/body mismatch = %q/%q", metadata, body)
	}
}

func newTestPersistentCache(t *testing.T, root string, limits diskCacheLimits) *HTTPCache {
	t.Helper()
	disk, err := newDiskCache(root, limits)
	if err != nil {
		t.Fatal(err)
	}
	return &HTTPCache{entries: make(map[string][]*cacheEntry), now: time.Now, disk: disk}
}

func diskTestResponse(request *Request, body []byte) *Response {
	return &Response{
		URL: request.URL, StatusCode: http.StatusOK,
		Header: http.Header{"Cache-Control": []string{"max-age=3600"}}, Body: body,
	}
}
