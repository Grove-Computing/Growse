package network

import (
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
)

const (
	maxCacheKeyBytes = 8 * 1024
	maxVaryHeaders   = 16
)

type cacheEntry struct {
	method     string
	url        string
	partition  string
	vary       []string
	varyValues []string
	response   *Response
}

// HTTPCache はprivate HTTP Response variantをmemoryで管理する。
type HTTPCache struct {
	mu      sync.RWMutex
	entries map[string][]*cacheEntry
}

func NewHTTPCache() *HTTPCache {
	return &HTTPCache{entries: make(map[string][]*cacheEntry)}
}

// Store はCache対象Request/Responseをvariantとして保存する。
func (cache *HTTPCache) Store(request *Request, response *Response) bool {
	key, ok := baseCacheKey(request)
	if cache == nil || !ok || response == nil || response.StatusCode != http.StatusOK {
		return false
	}
	vary, reusable := parseVary(response.Header.Values("Vary"))
	if !reusable {
		return false
	}
	entry := &cacheEntry{
		method: requestMethod(request), url: cacheURL(request.URL), partition: cachePartition(request),
		vary: vary, varyValues: requestHeaderValues(request.Header, vary), response: cloneResponse(response),
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	variants := cache.entries[key]
	for index, current := range variants {
		if sameVariant(current, entry) {
			variants[index] = entry
			cache.entries[key] = variants
			return true
		}
	}
	cache.entries[key] = append(variants, entry)
	return true
}

// Match はRequest Headerを含めて一致する保存済みvariantを返す。
func (cache *HTTPCache) Match(request *Request) (*Response, bool) {
	key, ok := baseCacheKey(request)
	if cache == nil || !ok {
		return nil, false
	}
	cache.mu.RLock()
	defer cache.mu.RUnlock()
	for _, entry := range cache.entries[key] {
		if equalStrings(entry.varyValues, requestHeaderValues(request.Header, entry.vary)) {
			return cloneResponse(entry.response), true
		}
	}
	return nil, false
}

func baseCacheKey(request *Request) (string, bool) {
	if request == nil || request.URL == nil || request.URL.User != nil {
		return "", false
	}
	method := requestMethod(request)
	if method != http.MethodGet && method != http.MethodHead {
		return "", false
	}
	if _, err := OriginFromURL(request.URL); err != nil {
		return "", false
	}
	key := method + "\n" + cachePartition(request) + "\n" + cacheURL(request.URL)
	return key, len(key) <= maxCacheKeyBytes
}

func requestMethod(request *Request) string {
	if request == nil || request.Method == "" {
		return http.MethodGet
	}
	return strings.ToUpper(request.Method)
}

func cacheURL(source *url.URL) string {
	if source == nil {
		return ""
	}
	copy := *source
	copy.User = nil
	copy.Fragment, copy.RawFragment = "", ""
	return copy.String()
}

func cachePartition(request *Request) string {
	partitionURL := request.URL
	if request != nil && request.Kind != RequestNavigation && request.SiteURL != nil {
		partitionURL = request.SiteURL
	}
	origin, err := OriginFromURL(partitionURL)
	if err != nil {
		return ""
	}
	return origin.String()
}

func parseVary(values []string) ([]string, bool) {
	set := make(map[string]struct{})
	for _, value := range values {
		for _, name := range strings.Split(value, ",") {
			name = strings.ToLower(strings.TrimSpace(name))
			if name == "*" {
				return nil, false
			}
			if name != "" {
				set[name] = struct{}{}
			}
		}
	}
	if len(set) > maxVaryHeaders {
		return nil, false
	}
	result := make([]string, 0, len(set))
	for name := range set {
		result = append(result, name)
	}
	sort.Strings(result)
	return result, true
}

func requestHeaderValues(header http.Header, names []string) []string {
	values := make([]string, len(names))
	for index, name := range names {
		values[index] = strings.Join(header.Values(name), "\x00")
	}
	return values
}

func sameVariant(left, right *cacheEntry) bool {
	return left != nil && right != nil && left.method == right.method && left.url == right.url && left.partition == right.partition && equalStrings(left.vary, right.vary) && equalStrings(left.varyValues, right.varyValues)
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func cloneResponse(source *Response) *Response {
	if source == nil {
		return nil
	}
	copy := *source
	copy.URL = cloneURL(source.URL)
	copy.Header = source.Header.Clone()
	copy.Body = append([]byte(nil), source.Body...)
	return &copy
}
