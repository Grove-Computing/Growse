package network

import (
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
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
	freshness  freshness
	policy     cachePolicy
}

type cachePolicy struct {
	noCache        bool
	noStore        bool
	private        bool
	public         bool
	mustRevalidate bool
	immutable      bool
}

type freshness struct {
	storedAt   time.Time
	lifetime   time.Duration
	initialAge time.Duration
}

func (value freshness) fresh(now time.Time) bool {
	elapsed := now.Sub(value.storedAt)
	if elapsed < 0 {
		elapsed = 0
	}
	if value.initialAge >= value.lifetime {
		return false
	}
	return elapsed < value.lifetime-value.initialAge
}

// HTTPCache はprivate HTTP Response variantをmemoryで管理する。
type HTTPCache struct {
	mu      sync.RWMutex
	entries map[string][]*cacheEntry
	now     func() time.Time
}

func NewHTTPCache() *HTTPCache {
	return &HTTPCache{entries: make(map[string][]*cacheEntry), now: time.Now}
}

// Store はCache対象Request/Responseをvariantとして保存する。
func (cache *HTTPCache) Store(request *Request, response *Response) bool {
	key, ok := baseCacheKey(request)
	if cache == nil || !ok || response == nil || response.StatusCode != http.StatusOK {
		return false
	}
	policy := parseCachePolicy(response.Header.Values("Cache-Control"))
	if policy.noStore {
		return false
	}
	vary, reusable := parseVary(response.Header.Values("Vary"))
	if !reusable {
		return false
	}
	entry := &cacheEntry{
		method: requestMethod(request), url: cacheURL(request.URL), partition: cachePartition(request),
		vary: vary, varyValues: requestHeaderValues(request.Header, vary), response: cloneResponse(response),
		freshness: calculateFreshness(response.Header, cache.now()),
		policy:    policy,
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

func parseCachePolicy(values []string) cachePolicy {
	var policy cachePolicy
	for _, value := range values {
		for _, directive := range strings.Split(value, ",") {
			name, _, _ := strings.Cut(strings.TrimSpace(directive), "=")
			switch strings.ToLower(name) {
			case "no-cache":
				policy.noCache = true
			case "no-store":
				policy.noStore = true
			case "private":
				policy.private = true
			case "public":
				policy.public = true
			case "must-revalidate":
				policy.mustRevalidate = true
			case "immutable":
				policy.immutable = true
			}
		}
	}
	return policy
}

func calculateFreshness(header http.Header, storedAt time.Time) freshness {
	date := storedAt
	if parsed, err := http.ParseTime(header.Get("Date")); err == nil {
		date = parsed
	}
	apparentAge := storedAt.Sub(date)
	if apparentAge < 0 {
		apparentAge = 0
	}
	headerAge := secondsDuration(header.Get("Age"))
	initialAge := max(apparentAge, headerAge)
	lifetime, explicit := maxAgeLifetime(header.Values("Cache-Control"))
	if !explicit {
		if expires, err := http.ParseTime(header.Get("Expires")); err == nil {
			lifetime = expires.Sub(date)
			if lifetime < 0 {
				lifetime = 0
			}
			explicit = true
		}
	}
	if !explicit {
		if modified, err := http.ParseTime(header.Get("Last-Modified")); err == nil && modified.Before(date) {
			lifetime = min(date.Sub(modified)/10, 24*time.Hour)
		}
	}
	return freshness{storedAt: storedAt, lifetime: lifetime, initialAge: initialAge}
}

func maxAgeLifetime(values []string) (time.Duration, bool) {
	for _, value := range values {
		for _, directive := range strings.Split(value, ",") {
			name, raw, found := strings.Cut(strings.TrimSpace(directive), "=")
			if !found || !strings.EqualFold(name, "max-age") {
				continue
			}
			raw = strings.Trim(strings.TrimSpace(raw), `"`)
			seconds, err := strconv.ParseUint(raw, 10, 64)
			if err != nil {
				return 0, true
			}
			return durationFromSeconds(seconds), true
		}
	}
	return 0, false
}

func secondsDuration(raw string) time.Duration {
	seconds, err := strconv.ParseUint(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		return 0
	}
	return durationFromSeconds(seconds)
}

func durationFromSeconds(seconds uint64) time.Duration {
	limit := uint64(math.MaxInt64 / int64(time.Second))
	if seconds > limit {
		return time.Duration(math.MaxInt64)
	}
	return time.Duration(seconds) * time.Second
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
	if source.URL != nil {
		copy.URL = cloneURL(source.URL)
	}
	copy.Header = source.Header.Clone()
	copy.Body = append([]byte(nil), source.Body...)
	return &copy
}
