package serviceworker

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/Grove-Computing/Growse/internal/network"
)

func TestCacheStoragePartitionsClonesAndStripsCredentials(t *testing.T) {
	storage := newCacheStorage()
	origin := "https://app.example"
	cache, err := storage.Open(origin, "assets")
	if err != nil {
		t.Fatal(err)
	}
	requestURL := parseServiceWorkerURL(t, "https://app.example/data?version=1#fragment")
	request := &network.Request{
		Method: http.MethodGet, URL: requestURL, Header: http.Header{"Authorization": []string{"Bearer secret"}, "Cookie": []string{"session=secret"}},
		Body: []byte("credential body"), Credentials: network.CredentialsInclude,
	}
	response := &network.Response{
		URL: requestURL, StatusCode: http.StatusOK, Status: "OK", Body: []byte("cached"),
		Header: http.Header{"Content-Type": []string{"text/plain"}, "Set-Cookie": []string{"session=secret"}, "X-Public": []string{"yes"}},
	}
	if err := cache.Put(request, response); err != nil {
		t.Fatal(err)
	}
	request.Body[0] = 'X'
	response.Body[0] = 'X'
	matched, found := cache.Match(&network.Request{Method: http.MethodGet, URL: parseServiceWorkerURL(t, "https://app.example/data?version=1")})
	if !found || string(matched.Body) != "cached" || matched.Header.Get("Set-Cookie") != "" || matched.Header.Get("X-Public") != "yes" {
		t.Fatalf("cached response = %#v, found=%t", matched, found)
	}
	matched.Body[0] = 'X'
	again, _ := storage.Match(origin, &network.Request{Method: http.MethodGet, URL: parseServiceWorkerURL(t, "https://app.example/data?version=1")})
	if string(again.Body) != "cached" {
		t.Fatalf("cached response was not cloned: %q", again.Body)
	}
	keys := cache.Keys()
	if len(keys) != 1 || len(keys[0].Header) != 0 || len(keys[0].Body) != 0 || keys[0].Credentials != network.CredentialsOmit || keys[0].URL.Fragment != "" {
		t.Fatalf("cache keys retained credential data: %#v", keys)
	}
	if _, found := storage.Match("https://other.example", request); found {
		t.Fatal("Cache Storage crossed the Origin boundary")
	}
	if !cache.Delete(request) || cache.Delete(request) {
		t.Fatal("Cache.Delete did not remove exactly one entry")
	}
	if !storage.Delete(origin, "assets") || storage.Has(origin, "assets") {
		t.Fatal("CacheStorage.Delete did not remove the named cache")
	}
}

func TestCacheStorageEnforcesCacheAndResponseQuotas(t *testing.T) {
	storage := newCacheStorage()
	origin := "https://app.example"
	for index := 0; index < MaxCachesPerOrigin; index++ {
		if _, err := storage.Open(origin, string(rune('a'+index))); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := storage.Open(origin, "overflow"); !errors.Is(err, ErrCacheQuota) {
		t.Fatalf("cache count quota error = %v", err)
	}
	cache, err := storage.Open("https://quota.example", "responses")
	if err != nil {
		t.Fatal(err)
	}
	request := &network.Request{Method: http.MethodGet, URL: parseServiceWorkerURL(t, "https://quota.example/large")}
	if err := cache.Put(request, &network.Response{StatusCode: http.StatusOK, Body: make([]byte, MaxCacheResponseBytes+1)}); !errors.Is(err, ErrCacheQuota) {
		t.Fatalf("response quota error = %v", err)
	}
	if err := cache.Put(&network.Request{Method: http.MethodPost, URL: request.URL}, &network.Response{StatusCode: http.StatusOK}); !errors.Is(err, ErrCacheRequest) {
		t.Fatalf("method restriction error = %v", err)
	}
	credentialURL := parseServiceWorkerURL(t, "https://user:password@quota.example/secret")
	if err := cache.Put(&network.Request{Method: http.MethodGet, URL: credentialURL}, &network.Response{StatusCode: http.StatusOK}); !errors.Is(err, ErrCacheRequest) {
		t.Fatalf("URL credential restriction error = %v", err)
	}

	entryCache, err := storage.Open("https://entries.example", "entries")
	if err != nil {
		t.Fatal(err)
	}
	storage.mu.Lock()
	storage.origins["https://entries.example"].entries = MaxCacheEntries
	storage.mu.Unlock()
	if err := entryCache.Put(&network.Request{Method: http.MethodGet, URL: parseServiceWorkerURL(t, "https://entries.example/overflow")}, &network.Response{StatusCode: http.StatusOK}); !errors.Is(err, ErrCacheQuota) {
		t.Fatalf("entry quota error = %v", err)
	}
	byteCache, err := storage.Open("https://bytes.example", "bytes")
	if err != nil {
		t.Fatal(err)
	}
	storage.mu.Lock()
	storage.origins["https://bytes.example"].bytes = MaxCacheOriginBytes
	storage.mu.Unlock()
	byteRequest := &network.Request{Method: http.MethodGet, URL: parseServiceWorkerURL(t, "https://bytes.example/overflow")}
	if err := byteCache.Put(byteRequest, &network.Response{StatusCode: http.StatusOK, Body: []byte("x")}); !errors.Is(err, ErrCacheQuota) {
		t.Fatalf("Origin byte quota error = %v", err)
	}
	if _, found := byteCache.Match(byteRequest); found {
		t.Fatal("quota failure partially stored a response")
	}
}

func TestServiceWorkerUsesCacheAPIsAcrossLifecycleAndFetch(t *testing.T) {
	manager := NewManager()
	clientURL := parseServiceWorkerURL(t, "https://app.example/app/page.html")
	scriptURL := parseServiceWorkerURL(t, "https://app.example/app/sw.js")
	source := []byte(`
		self.addEventListener("install", event => {
			event.waitUntil(caches.open("assets").then(cache => {
				return cache.put("/app/offline", new Response("from-cache", {
					headers: {"Content-Type": "text/plain", "Set-Cookie": "secret=true"}
				})).then(() => cache.match("/app/offline")).then(response => response.text()).then(text => {
					if (text !== "from-cache") throw new Error("Cache.match failed");
					return cache.keys();
				}).then(requests => {
					if (requests.length !== 1) throw new Error("Cache.keys failed");
				});
			}));
			self.skipWaiting();
		});
		self.addEventListener("activate", event => event.waitUntil(
			caches.open("temporary").then(() => caches.has("temporary")).then(found => {
				if (!found) throw new Error("CacheStorage.has failed");
				return caches.keys();
			}).then(names => {
				if (names.length !== 2) throw new Error("CacheStorage.keys failed");
				return caches.delete("temporary");
			}).then(removed => {
				if (!removed) throw new Error("CacheStorage.delete failed");
				return clients.claim();
			})
		));
		self.addEventListener("fetch", event => {
			if (event.request.url.endsWith("/offline")) event.respondWith(caches.match(event.request));
		});`)
	_, err := manager.Register(context.Background(), clientURL, scriptURL.String(), "", func(context.Context, *network.Request) (*network.Response, error) {
		return &network.Response{URL: scriptURL, StatusCode: http.StatusOK, ContentType: "text/javascript", Body: source}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if names := manager.Caches().Keys("https://app.example"); len(names) != 1 || names[0] != "assets" {
		t.Fatalf("Cache Storage names = %#v", names)
	}
	offlineURL := parseServiceWorkerURL(t, "https://app.example/app/offline")
	response, err := manager.DispatchFetch(context.Background(), &network.Request{Method: http.MethodGet, URL: offlineURL, Kind: network.RequestNavigation}, func(context.Context, *network.Request) (*network.Response, error) {
		return nil, errors.New("network fallback must not run")
	})
	if err != nil || string(response.Body) != "from-cache" || response.Header.Get("Set-Cookie") != "" {
		t.Fatalf("cached fetch response = %#v, %v", response, err)
	}
}
