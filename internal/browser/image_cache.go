package browser

import (
	"context"
	"image"
	"net/url"
	"sync"
)

const maxPageImageCacheBytes = 256 << 20

type imageLoadFailure uint8

const (
	imageLoadOK imageLoadFailure = iota
	imageLoadRequestFailure
	imageLoadResponseFailure
	imageLoadDecodeFailure
	imageLoadResourceLimit
)

type cachedImageResource struct {
	ready       chan struct{}
	body        []byte
	contentType string
	decoded     image.Image
	width       int
	height      int
	failure     imageLoadFailure
	err         error
	complete    bool
	bytes       int64
	lastUsed    uint64
}

// imageResourceCache is owned by one Page generation. It coalesces duplicate
// network/decode work while retaining the validated response body for later
// consumers in the same generation.
type imageResourceCache struct {
	mu         sync.Mutex
	entries    map[string]*cachedImageResource
	bytes      int64
	tick       uint64
	maxBytes   int64
	maxEntries int
}

func newImageResourceCache() *imageResourceCache {
	return newImageResourceCacheWithLimits(maxPageImageCacheBytes, maxPageImageResources)
}

func newImageResourceCacheWithLimits(maxBytes int64, maxEntries int) *imageResourceCache {
	return &imageResourceCache{entries: make(map[string]*cachedImageResource), maxBytes: maxBytes, maxEntries: maxEntries}
}

func (cache *imageResourceCache) load(ctx context.Context, client ResourceLoader, target *url.URL, budget *imageDecodeBudget) cachedImageResource {
	if cache == nil || client == nil || target == nil {
		return cachedImageResource{failure: imageLoadRequestFailure}
	}
	key := target.String()
	cache.mu.Lock()
	if existing := cache.entries[key]; existing != nil {
		cache.tick++
		existing.lastUsed = cache.tick
		ready := existing.ready
		cache.mu.Unlock()
		select {
		case <-ready:
			return cloneCachedImageResource(existing)
		case <-ctx.Done():
			return cachedImageResource{failure: imageLoadRequestFailure, err: ctx.Err()}
		}
	}
	cache.evictLocked(0, 1)
	if cache.maxEntries <= 0 || len(cache.entries) >= cache.maxEntries {
		cache.mu.Unlock()
		return cachedImageResource{failure: imageLoadResourceLimit}
	}
	if !budget.claim(key) {
		cache.mu.Unlock()
		return cachedImageResource{failure: imageLoadResourceLimit}
	}
	cache.tick++
	entry := &cachedImageResource{ready: make(chan struct{}), lastUsed: cache.tick}
	cache.entries[key] = entry
	cache.mu.Unlock()

	response, err := client.Get(ctx, target)
	result := cachedImageResource{}
	switch {
	case err != nil || response == nil:
		result.failure, result.err = imageLoadRequestFailure, err
	case len(response.Body) > maxImageBytes || !isImageContentType(response.ContentType):
		result.failure = imageLoadResponseFailure
	default:
		result.body = append([]byte(nil), response.Body...)
		result.contentType = response.ContentType
		result.decoded, result.width, result.height, result.err = decodeImageResponseWithBudget(result.body, result.contentType, budget)
		if result.err != nil {
			result.failure = imageLoadDecodeFailure
		}
	}

	cache.mu.Lock()
	entry.body = result.body
	entry.contentType = result.contentType
	entry.decoded = result.decoded
	entry.width = result.width
	entry.height = result.height
	entry.failure = result.failure
	entry.err = result.err
	entry.complete = true
	entry.bytes = int64(len(result.body)) + int64(result.width)*int64(result.height)*4
	cache.bytes += entry.bytes
	close(entry.ready)
	if ctx.Err() != nil {
		cache.bytes -= entry.bytes
		delete(cache.entries, key)
	} else {
		cache.evictLocked(cache.maxBytes, 0)
	}
	cache.mu.Unlock()
	return result
}

func (cache *imageResourceCache) evictLocked(targetBytes int64, reserveEntries int) {
	for (targetBytes > 0 && cache.bytes > targetBytes) || (cache.maxEntries > 0 && len(cache.entries)+reserveEntries > cache.maxEntries) {
		oldestKey := ""
		oldestTick := ^uint64(0)
		for key, entry := range cache.entries {
			if entry == nil || !entry.complete || entry.lastUsed >= oldestTick {
				continue
			}
			oldestKey, oldestTick = key, entry.lastUsed
		}
		if oldestKey == "" {
			return
		}
		entry := cache.entries[oldestKey]
		cache.bytes -= entry.bytes
		delete(cache.entries, oldestKey)
	}
}

func (cache *imageResourceCache) clear() {
	if cache == nil {
		return
	}
	cache.mu.Lock()
	cache.entries = make(map[string]*cachedImageResource)
	cache.bytes = 0
	cache.tick = 0
	cache.mu.Unlock()
}

func cloneCachedImageResource(entry *cachedImageResource) cachedImageResource {
	if entry == nil {
		return cachedImageResource{failure: imageLoadRequestFailure}
	}
	return cachedImageResource{
		body: entry.body, contentType: entry.contentType, decoded: entry.decoded,
		width: entry.width, height: entry.height, failure: entry.failure, err: entry.err,
	}
}
