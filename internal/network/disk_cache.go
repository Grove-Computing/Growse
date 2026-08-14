package network

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	diskCacheSchemaVersion = 1
	maxDiskMetadataBytes   = 256 << 10
	defaultDiskEntries     = 1024
	defaultDiskEntryBytes  = 4 << 20
	defaultDiskOriginBytes = 32 << 20
	defaultDiskTotalBytes  = 128 << 20
)

type diskCacheLimits struct {
	maxEntries     int
	maxEntryBytes  int64
	maxOriginBytes int64
	maxTotalBytes  int64
}

func defaultDiskCacheLimits() diskCacheLimits {
	return diskCacheLimits{
		maxEntries: defaultDiskEntries, maxEntryBytes: defaultDiskEntryBytes,
		maxOriginBytes: defaultDiskOriginBytes, maxTotalBytes: defaultDiskTotalBytes,
	}
}

type diskCache struct {
	mu      sync.Mutex
	root    string
	limits  diskCacheLimits
	records map[string]diskCacheRecord
	access  uint64
}

type diskCacheRecord struct {
	Version    int           `json:"version"`
	ID         string        `json:"id"`
	BaseKey    string        `json:"base_key"`
	Method     string        `json:"method"`
	URL        string        `json:"url"`
	Origin     string        `json:"origin"`
	Partition  string        `json:"partition"`
	Vary       []string      `json:"vary,omitempty"`
	VaryValues []string      `json:"vary_values,omitempty"`
	Response   diskResponse  `json:"response"`
	Freshness  diskFreshness `json:"freshness"`
	Policy     diskPolicy    `json:"policy"`
	BodyBytes  int64         `json:"body_bytes"`
	BodySHA256 string        `json:"body_sha256"`
	LastAccess uint64        `json:"last_access"`
}

type diskResponse struct {
	URL         string      `json:"url"`
	StatusCode  int         `json:"status_code"`
	Status      string      `json:"status"`
	Header      http.Header `json:"header"`
	ContentType string      `json:"content_type"`
	Redirected  bool        `json:"redirected"`
}

type diskFreshness struct {
	StoredAtUnixNano int64 `json:"stored_at_unix_nano"`
	LifetimeNanos    int64 `json:"lifetime_nanos"`
	InitialAgeNanos  int64 `json:"initial_age_nanos"`
}

type diskPolicy struct {
	NoCache        bool `json:"no_cache"`
	NoStore        bool `json:"no_store"`
	Private        bool `json:"private"`
	Public         bool `json:"public"`
	MustRevalidate bool `json:"must_revalidate"`
	Immutable      bool `json:"immutable"`
}

// DefaultCacheRoot はOS標準Cache Directory配下のHTTP Cache rootを返す。
func DefaultCacheRoot() (string, error) {
	root, err := os.UserCacheDir()
	if err != nil || root == "" || !filepath.IsAbs(root) {
		return "", errors.New("resolve Growse cache directory")
	}
	return filepath.Join(root, "growse", "http-cache"), nil
}

func newDiskCache(root string, limits diskCacheLimits) (*diskCache, error) {
	if root == "" || !filepath.IsAbs(root) {
		return nil, errors.New("invalid HTTP cache directory")
	}
	if limits.maxEntries <= 0 || limits.maxEntryBytes <= 0 || limits.maxOriginBytes <= 0 || limits.maxTotalBytes <= 0 {
		return nil, errors.New("invalid HTTP cache limits")
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, errors.New("create HTTP cache directory")
	}
	if err := os.Chmod(root, 0o700); err != nil {
		return nil, errors.New("protect HTTP cache directory")
	}
	cache := &diskCache{root: root, limits: limits, records: make(map[string]diskCacheRecord)}
	cache.scan()
	cache.evict()
	return cache, nil
}

func (cache *diskCache) scan() {
	files, err := os.ReadDir(cache.root)
	if err != nil {
		return
	}
	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(file.Name(), ".json")
		record, ok := cache.readRecord(id)
		if !ok {
			cache.removeFiles(id)
			continue
		}
		cache.records[id] = record
		cache.access = max(cache.access, record.LastAccess)
	}
}

func (cache *diskCache) readRecord(id string) (diskCacheRecord, bool) {
	file, err := os.Open(cache.metadataPath(id))
	if err != nil {
		return diskCacheRecord{}, false
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, maxDiskMetadataBytes+1))
	if err != nil || len(content) > maxDiskMetadataBytes {
		return diskCacheRecord{}, false
	}
	var record diskCacheRecord
	if json.Unmarshal(content, &record) != nil || !cache.validRecord(id, record) {
		return diskCacheRecord{}, false
	}
	return record, true
}

func (cache *diskCache) validRecord(id string, record diskCacheRecord) bool {
	if record.Version != diskCacheSchemaVersion || record.ID != id || record.BaseKey == "" || record.BodyBytes < 0 || record.BodyBytes > cache.limits.maxEntryBytes {
		return false
	}
	if len(record.Vary) != len(record.VaryValues) || len(record.Vary) > maxVaryHeaders || len(record.BodySHA256) != sha256.Size*2 {
		return false
	}
	parsed, err := url.Parse(record.URL)
	if err != nil || parsed.User != nil || cacheURL(parsed) != record.URL {
		return false
	}
	if record.Response.Header.Get("Set-Cookie") != "" || record.Response.Header.Get("Set-Cookie2") != "" {
		return false
	}
	origin, err := OriginFromURL(parsed)
	return err == nil && origin.String() == record.Origin && headersWithinLimits(record.Response.Header)
}

func (cache *diskCache) store(baseKey string, entry *cacheEntry) bool {
	if cache == nil || entry == nil || entry.response == nil || int64(len(entry.response.Body)) > cache.limits.maxEntryBytes {
		return false
	}
	originURL, err := url.Parse(entry.url)
	if err != nil {
		return false
	}
	origin, err := OriginFromURL(originURL)
	if err != nil {
		return false
	}
	id := diskEntryID(baseKey, entry.vary, entry.varyValues)
	digest := sha256.Sum256(entry.response.Body)
	responseURL := entry.response.URL
	if responseURL == nil {
		responseURL = originURL
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	cache.access++
	record := diskCacheRecord{
		Version: diskCacheSchemaVersion, ID: id, BaseKey: baseKey, Method: entry.method,
		URL: entry.url, Origin: origin.String(), Partition: entry.partition,
		Vary: append([]string(nil), entry.vary...), VaryValues: append([]string(nil), entry.varyValues...),
		Response: diskResponse{
			URL: responseURL.String(), StatusCode: entry.response.StatusCode, Status: entry.response.Status,
			Header: entry.response.Header.Clone(), ContentType: entry.response.ContentType, Redirected: entry.response.Redirected,
		},
		Freshness: diskFreshness{
			StoredAtUnixNano: entry.freshness.storedAt.UnixNano(), LifetimeNanos: int64(entry.freshness.lifetime), InitialAgeNanos: int64(entry.freshness.initialAge),
		},
		Policy: diskPolicy{
			NoCache: entry.policy.noCache, NoStore: entry.policy.noStore, Private: entry.policy.private,
			Public: entry.policy.public, MustRevalidate: entry.policy.mustRevalidate, Immutable: entry.policy.immutable,
		},
		BodyBytes: int64(len(entry.response.Body)), BodySHA256: hex.EncodeToString(digest[:]), LastAccess: cache.access,
	}
	if !cache.writeBody(id, entry.response.Body) || !cache.writeRecord(record) {
		cache.removeFiles(id)
		delete(cache.records, id)
		return false
	}
	cache.records[id] = record
	cache.evict()
	_, retained := cache.records[id]
	return retained
}

func (cache *diskCache) match(request *Request) *cacheEntry {
	baseKey, ok := baseCacheKey(request)
	if cache == nil || !ok {
		return nil
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	ids := make([]string, 0)
	for id, record := range cache.records {
		if record.BaseKey == baseKey && equalStrings(record.VaryValues, requestHeaderValues(request.Header, record.Vary)) {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	for _, id := range ids {
		record := cache.records[id]
		body, err := os.ReadFile(cache.bodyPath(id))
		digest := sha256.Sum256(body)
		if err != nil || int64(len(body)) != record.BodyBytes || hex.EncodeToString(digest[:]) != record.BodySHA256 {
			cache.remove(id)
			continue
		}
		responseURL, err := url.Parse(record.Response.URL)
		if err != nil {
			cache.remove(id)
			continue
		}
		cache.access++
		record.LastAccess = cache.access
		cache.records[id] = record
		_ = cache.writeRecord(record)
		return &cacheEntry{
			method: record.Method, url: record.URL, partition: record.Partition,
			vary: append([]string(nil), record.Vary...), varyValues: append([]string(nil), record.VaryValues...),
			response: &Response{
				URL: responseURL, StatusCode: record.Response.StatusCode, Status: record.Response.Status,
				Header: record.Response.Header.Clone(), ContentType: record.Response.ContentType, Body: body, Redirected: record.Response.Redirected,
			},
			freshness: freshness{
				storedAt: time.Unix(0, record.Freshness.StoredAtUnixNano), lifetime: time.Duration(record.Freshness.LifetimeNanos), initialAge: time.Duration(record.Freshness.InitialAgeNanos),
			},
			policy: cachePolicy{
				noCache: record.Policy.NoCache, noStore: record.Policy.NoStore, private: record.Policy.Private,
				public: record.Policy.Public, mustRevalidate: record.Policy.MustRevalidate, immutable: record.Policy.Immutable,
			},
		}
	}
	return nil
}

func (cache *diskCache) invalidate(baseKeys []string) {
	if cache == nil {
		return
	}
	set := make(map[string]struct{}, len(baseKeys))
	for _, key := range baseKeys {
		set[key] = struct{}{}
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	for id, record := range cache.records {
		if _, ok := set[record.BaseKey]; ok {
			cache.remove(id)
		}
	}
}

func (cache *diskCache) evict() {
	for cache.exceedsLimits() {
		ids := make([]string, 0, len(cache.records))
		for id := range cache.records {
			ids = append(ids, id)
		}
		sort.Slice(ids, func(left, right int) bool {
			leftRecord, rightRecord := cache.records[ids[left]], cache.records[ids[right]]
			if leftRecord.LastAccess == rightRecord.LastAccess {
				return ids[left] < ids[right]
			}
			return leftRecord.LastAccess < rightRecord.LastAccess
		})
		if len(ids) == 0 {
			return
		}
		cache.remove(ids[0])
	}
}

func (cache *diskCache) exceedsLimits() bool {
	if len(cache.records) > cache.limits.maxEntries {
		return true
	}
	var total int64
	byOrigin := make(map[string]int64)
	for _, record := range cache.records {
		total += record.BodyBytes
		byOrigin[record.Origin] += record.BodyBytes
	}
	if total > cache.limits.maxTotalBytes {
		return true
	}
	for _, size := range byOrigin {
		if size > cache.limits.maxOriginBytes {
			return true
		}
	}
	return false
}

func (cache *diskCache) remove(id string) {
	delete(cache.records, id)
	cache.removeFiles(id)
}

func (cache *diskCache) removeFiles(id string) {
	_ = os.Remove(cache.metadataPath(id))
	_ = os.Remove(cache.bodyPath(id))
}

func (cache *diskCache) writeBody(id string, body []byte) bool {
	return writeAtomicCacheFile(cache.root, cache.bodyPath(id), body)
}

func (cache *diskCache) writeRecord(record diskCacheRecord) bool {
	content, err := json.Marshal(record)
	return err == nil && len(content) <= maxDiskMetadataBytes && writeAtomicCacheFile(cache.root, cache.metadataPath(record.ID), content)
}

func writeAtomicCacheFile(root, target string, content []byte) bool {
	temporary, err := os.CreateTemp(root, ".cache-*.tmp")
	if err != nil {
		return false
	}
	temporaryPath := temporary.Name()
	committed := false
	defer func() {
		_ = temporary.Close()
		if !committed {
			_ = os.Remove(temporaryPath)
		}
	}()
	if temporary.Chmod(0o600) != nil {
		return false
	}
	if _, err := temporary.Write(content); err != nil || temporary.Sync() != nil || temporary.Close() != nil {
		return false
	}
	if os.Rename(temporaryPath, target) != nil {
		return false
	}
	committed = true
	return true
}

func diskEntryID(baseKey string, vary, values []string) string {
	content := baseKey + "\n" + strings.Join(vary, "\x00") + "\n" + strings.Join(values, "\x00")
	digest := sha256.Sum256([]byte(content))
	return hex.EncodeToString(digest[:])
}

func (cache *diskCache) metadataPath(id string) string { return filepath.Join(cache.root, id+".json") }
func (cache *diskCache) bodyPath(id string) string     { return filepath.Join(cache.root, id+".body") }
