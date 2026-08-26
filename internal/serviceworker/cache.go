package serviceworker

import (
	"errors"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"

	"github.com/Grove-Computing/Growse/internal/network"
)

const (
	MaxCachesPerOrigin    = 32
	MaxCacheEntries       = 4096
	MaxCacheResponseBytes = 4 << 20
	MaxCacheOriginBytes   = 128 << 20
	maxCacheNameBytes     = 1024
)

var (
	ErrCacheQuota   = errors.New("service worker Cache Storage quota exceeded")
	ErrCacheRequest = errors.New("service worker Cache only stores GET requests")
)

type cacheEntry struct {
	request  *network.Request
	response *network.Response
}

type cacheBucket struct {
	entries map[string]cacheEntry
}

type originCaches struct {
	buckets map[string]*cacheBucket
	bytes   int
	entries int
}

// CacheStorage is a concurrency-safe, origin-partitioned response store.
type CacheStorage struct {
	mu      sync.RWMutex
	origins map[string]*originCaches
}

// Cache is a handle to one named origin cache.
type Cache struct {
	storage *CacheStorage
	origin  string
	name    string
}

func (storage *CacheStorage) cache(origin, name string) (*Cache, bool) {
	if storage == nil {
		return nil, false
	}
	storage.mu.RLock()
	partition := storage.origins[origin]
	found := partition != nil && partition.buckets[name] != nil
	storage.mu.RUnlock()
	if !found {
		return nil, false
	}
	return &Cache{storage: storage, origin: origin, name: name}, true
}

func newCacheStorage() *CacheStorage {
	return &CacheStorage{origins: make(map[string]*originCaches)}
}

// Open creates or returns one named cache without crossing an Origin boundary.
func (storage *CacheStorage) Open(origin, name string) (*Cache, error) {
	if storage == nil || origin == "" || len(name) > maxCacheNameBytes {
		return nil, errors.New("invalid Service Worker cache name")
	}
	storage.mu.Lock()
	partition := storage.origins[origin]
	if partition == nil {
		partition = &originCaches{buckets: make(map[string]*cacheBucket)}
		storage.origins[origin] = partition
	}
	if partition.buckets[name] == nil {
		if len(partition.buckets) >= MaxCachesPerOrigin {
			storage.mu.Unlock()
			return nil, ErrCacheQuota
		}
		partition.buckets[name] = &cacheBucket{entries: make(map[string]cacheEntry)}
	}
	storage.mu.Unlock()
	return &Cache{storage: storage, origin: origin, name: name}, nil
}

// Match searches named caches in stable lexical order.
func (storage *CacheStorage) Match(origin string, request *network.Request) (*network.Response, bool) {
	key, ok := cacheRequestKey(request)
	if storage == nil || !ok {
		return nil, false
	}
	storage.mu.RLock()
	partition := storage.origins[origin]
	if partition == nil {
		storage.mu.RUnlock()
		return nil, false
	}
	names := make([]string, 0, len(partition.buckets))
	for name := range partition.buckets {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if entry, found := partition.buckets[name].entries[key]; found {
			response := cloneResponse(entry.response)
			storage.mu.RUnlock()
			return response, true
		}
	}
	storage.mu.RUnlock()
	return nil, false
}

// Has reports whether a named cache exists for origin.
func (storage *CacheStorage) Has(origin, name string) bool {
	if storage == nil {
		return false
	}
	storage.mu.RLock()
	partition := storage.origins[origin]
	found := partition != nil && partition.buckets[name] != nil
	storage.mu.RUnlock()
	return found
}

// Delete removes a named cache and releases its quota atomically.
func (storage *CacheStorage) Delete(origin, name string) bool {
	if storage == nil {
		return false
	}
	storage.mu.Lock()
	partition := storage.origins[origin]
	cache := (*cacheBucket)(nil)
	if partition != nil {
		cache = partition.buckets[name]
	}
	if cache == nil {
		storage.mu.Unlock()
		return false
	}
	for _, entry := range cache.entries {
		partition.bytes -= len(entry.response.Body)
		partition.entries--
	}
	delete(partition.buckets, name)
	if len(partition.buckets) == 0 {
		delete(storage.origins, origin)
	}
	storage.mu.Unlock()
	return true
}

// Keys returns cache names in deterministic order.
func (storage *CacheStorage) Keys(origin string) []string {
	if storage == nil {
		return nil
	}
	storage.mu.RLock()
	partition := storage.origins[origin]
	result := make([]string, 0)
	if partition != nil {
		result = make([]string, 0, len(partition.buckets))
		for name := range partition.buckets {
			result = append(result, name)
		}
	}
	storage.mu.RUnlock()
	sort.Strings(result)
	return result
}

// Match returns a cloned cached response.
func (cache *Cache) Match(request *network.Request) (*network.Response, bool) {
	key, ok := cacheRequestKey(request)
	if cache == nil || cache.storage == nil || !ok {
		return nil, false
	}
	cache.storage.mu.RLock()
	partition := cache.storage.origins[cache.origin]
	if partition == nil || partition.buckets[cache.name] == nil {
		cache.storage.mu.RUnlock()
		return nil, false
	}
	entry, found := partition.buckets[cache.name].entries[key]
	response := cloneResponse(entry.response)
	cache.storage.mu.RUnlock()
	return response, found
}

// Put atomically stores a buffered response after stripping credential headers.
func (cache *Cache) Put(request *network.Request, response *network.Response) error {
	key, ok := cacheRequestKey(request)
	if cache == nil || cache.storage == nil || !ok {
		return ErrCacheRequest
	}
	if response == nil || len(response.Body) > MaxCacheResponseBytes {
		return ErrCacheQuota
	}
	requestCopy := cacheSafeRequest(request)
	responseCopy := cacheSafeResponse(response)
	storage := cache.storage
	storage.mu.Lock()
	defer storage.mu.Unlock()
	partition := storage.origins[cache.origin]
	if partition == nil || partition.buckets[cache.name] == nil {
		return errors.New("service worker Cache was deleted")
	}
	bucket := partition.buckets[cache.name]
	previous, replacing := bucket.entries[key]
	nextEntries := partition.entries
	nextBytes := partition.bytes + len(responseCopy.Body)
	if replacing {
		nextBytes -= len(previous.response.Body)
	} else {
		nextEntries++
	}
	if nextEntries > MaxCacheEntries || nextBytes > MaxCacheOriginBytes {
		return ErrCacheQuota
	}
	bucket.entries[key] = cacheEntry{request: requestCopy, response: responseCopy}
	partition.entries, partition.bytes = nextEntries, nextBytes
	return nil
}

// Delete removes one request entry.
func (cache *Cache) Delete(request *network.Request) bool {
	key, ok := cacheRequestKey(request)
	if cache == nil || cache.storage == nil || !ok {
		return false
	}
	cache.storage.mu.Lock()
	partition := cache.storage.origins[cache.origin]
	if partition == nil || partition.buckets[cache.name] == nil {
		cache.storage.mu.Unlock()
		return false
	}
	entry, found := partition.buckets[cache.name].entries[key]
	if found {
		delete(partition.buckets[cache.name].entries, key)
		partition.bytes -= len(entry.response.Body)
		partition.entries--
	}
	cache.storage.mu.Unlock()
	return found
}

// Keys returns cloned requests in deterministic URL order.
func (cache *Cache) Keys() []*network.Request {
	if cache == nil || cache.storage == nil {
		return nil
	}
	cache.storage.mu.RLock()
	partition := cache.storage.origins[cache.origin]
	if partition == nil || partition.buckets[cache.name] == nil {
		cache.storage.mu.RUnlock()
		return nil
	}
	entries := partition.buckets[cache.name].entries
	keys := make([]string, 0, len(entries))
	for key := range entries {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]*network.Request, 0, len(keys))
	for _, key := range keys {
		result = append(result, cloneRequest(entries[key].request))
	}
	cache.storage.mu.RUnlock()
	return result
}

func cacheRequestKey(request *network.Request) (string, bool) {
	if request == nil || request.URL == nil || request.URL.User != nil {
		return "", false
	}
	method := strings.ToUpper(strings.TrimSpace(request.Method))
	if method == "" {
		method = http.MethodGet
	}
	if method != http.MethodGet {
		return "", false
	}
	value := *request.URL
	value.Fragment = ""
	if value.Scheme != "http" && value.Scheme != "https" || value.Host == "" {
		return "", false
	}
	return method + " " + value.String(), true
}

func cacheSafeRequest(request *network.Request) *network.Request {
	result := cloneRequest(request)
	result.URL.Fragment = ""
	result.Header = nil
	result.Body = nil
	result.SiteURL = nil
	result.Observer = nil
	result.Credentials = network.CredentialsOmit
	return result
}

func cacheSafeResponse(response *network.Response) *network.Response {
	result := cloneResponse(response)
	if result.URL != nil {
		result.URL.User = nil
	}
	if result.Header == nil {
		result.Header = make(http.Header)
	}
	if result.Header.Get("Content-Type") == "" && result.ContentType != "" {
		result.Header.Set("Content-Type", result.ContentType)
	}
	for _, name := range []string{"Authorization", "Proxy-Authorization", "Cookie", "Set-Cookie", "Set-Cookie2", "WWW-Authenticate", "Proxy-Authenticate"} {
		result.Header.Del(name)
	}
	result.ContentType = result.Header.Get("Content-Type")
	return result
}

func originString(value *url.URL) string {
	origin, err := network.OriginFromURL(value)
	if err != nil {
		return ""
	}
	return origin.String()
}
