package browser

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image"
	"mime"
	"net/url"
	"sync"

	"github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/network"
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
	validator   string
	orientation int
}

type imageSurfaceCacheKey struct {
	URL                       string
	Validator                 string
	DeviceScaleMilli          int
	TargetWidth, TargetHeight int
	Orientation               int
	StyleRevision             uint64
}

type cachedImageSurface struct {
	ready    chan struct{}
	image    image.Image
	err      error
	bytes    int64
	lastUsed uint64
	complete bool
}

// imageResourceCache is owned by one Page generation. It coalesces duplicate
// network/decode work while retaining the validated response body for later
// consumers in the same generation.
type imageResourceCache struct {
	mu         sync.Mutex
	fetchMu    sync.Mutex
	entries    map[string]*cachedImageResource
	surfaces   map[imageSurfaceCacheKey]*cachedImageSurface
	bytes      int64
	tick       uint64
	maxBytes   int64
	maxEntries int
	hits       uint64
	misses     uint64
	evictions  uint64
}

type imageResourceCacheStats struct{ hits, misses, evictions uint64 }

func newImageResourceCache() *imageResourceCache {
	return newImageResourceCacheWithLimits(maxPageImageCacheBytes, maxPageImageResources)
}

func newImageResourceCacheWithLimits(maxBytes int64, maxEntries int) *imageResourceCache {
	return &imageResourceCache{
		entries: make(map[string]*cachedImageResource), surfaces: make(map[imageSurfaceCacheKey]*cachedImageSurface),
		maxBytes: maxBytes, maxEntries: maxEntries,
	}
}

func (cache *imageResourceCache) load(ctx context.Context, client ResourceLoader, target *url.URL, budget *imageDecodeBudget) cachedImageResource {
	if cache == nil || client == nil || target == nil {
		return cachedImageResource{failure: imageLoadRequestFailure}
	}
	key := target.String()
	cache.mu.Lock()
	if existing := cache.entries[key]; existing != nil {
		cache.hits++
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
	cache.misses++
	cache.evictLocked(0, 1)
	if cache.maxEntries <= 0 || len(cache.entries)+len(cache.surfaces) >= cache.maxEntries {
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

	cache.fetchMu.Lock()
	response, err := client.Get(ctx, target)
	cache.fetchMu.Unlock()
	result := cachedImageResource{}
	switch {
	case err != nil || response == nil:
		result.failure, result.err = imageLoadRequestFailure, err
	case len(response.Body) > maxImageBytes || !isImageContentType(response.ContentType):
		result.failure = imageLoadResponseFailure
	default:
		result.body = append([]byte(nil), response.Body...)
		result.contentType = response.ContentType
		result.validator = imageResponseValidator(response)
		result.orientation = 1
		if mediaType, _, parseErr := mime.ParseMediaType(response.ContentType); parseErr == nil && mediaType == "image/jpeg" {
			result.orientation = jpegEXIFOrientation(response.Body)
		}
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
	entry.validator = result.validator
	entry.orientation = result.orientation
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

func (cache *imageResourceCache) prepareSurface(ctx context.Context, source cachedImageResource, target *url.URL, node *dom.Node, deviceScale float32, budget *imageDecodeBudget) (image.Image, error) {
	if cache == nil || source.decoded == nil || target == nil {
		return nil, errors.New("image surface source is unavailable")
	}
	targetWidth, targetHeight, resize := imageTargetDimensions(source.decoded, node, deviceScale)
	key := imageSurfaceCacheKey{
		URL: target.String(), Validator: source.validator, DeviceScaleMilli: int(deviceScale*1000 + 0.5),
		TargetWidth: targetWidth, TargetHeight: targetHeight, Orientation: source.orientation,
		StyleRevision: imageNodeStyleRevision(node),
	}
	cache.mu.Lock()
	cache.tick++
	if sourceEntry := cache.entries[target.String()]; sourceEntry != nil {
		sourceEntry.lastUsed = cache.tick
	}
	if existing := cache.surfaces[key]; existing != nil {
		cache.hits++
		cache.tick++
		existing.lastUsed = cache.tick
		ready := existing.ready
		cache.mu.Unlock()
		select {
		case <-ready:
			return existing.image, existing.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	cache.misses++
	cache.evictLocked(0, 1)
	if cache.maxEntries <= 0 || len(cache.entries)+len(cache.surfaces) >= cache.maxEntries {
		cache.mu.Unlock()
		return nil, errors.New("image surface cache entry limit exceeded")
	}
	cache.tick++
	entry := &cachedImageSurface{ready: make(chan struct{}), lastUsed: cache.tick}
	cache.surfaces[key] = entry
	cache.mu.Unlock()

	prepared := source.decoded
	var err error
	if resize {
		prepared, err = resizeImageToTarget(ctx, source.decoded, targetWidth, targetHeight, budget)
	}
	cache.mu.Lock()
	entry.image, entry.err, entry.complete = prepared, err, true
	if err == nil && prepared != source.decoded {
		entry.bytes = int64(targetWidth) * int64(targetHeight) * 4
		cache.bytes += entry.bytes
	}
	close(entry.ready)
	if ctx.Err() != nil {
		cache.bytes -= entry.bytes
		delete(cache.surfaces, key)
	} else {
		cache.evictLocked(cache.maxBytes, 0)
	}
	cache.mu.Unlock()
	return prepared, err
}

func (cache *imageResourceCache) evictLocked(targetBytes int64, reserveEntries int) {
	for (targetBytes > 0 && cache.bytes > targetBytes) || (cache.maxEntries > 0 && len(cache.entries)+len(cache.surfaces)+reserveEntries > cache.maxEntries) {
		oldestKey := ""
		var oldestSurface imageSurfaceCacheKey
		oldestIsSurface := false
		oldestTick := ^uint64(0)
		for key, entry := range cache.entries {
			if entry == nil || !entry.complete || entry.lastUsed >= oldestTick {
				continue
			}
			oldestKey, oldestTick = key, entry.lastUsed
		}
		for key, entry := range cache.surfaces {
			if entry == nil || !entry.complete || entry.lastUsed >= oldestTick {
				continue
			}
			oldestKey, oldestSurface, oldestTick, oldestIsSurface = "", key, entry.lastUsed, true
		}
		if oldestKey == "" && !oldestIsSurface {
			return
		}
		if oldestIsSurface {
			entry := cache.surfaces[oldestSurface]
			cache.bytes -= entry.bytes
			delete(cache.surfaces, oldestSurface)
			cache.evictions++
			continue
		}
		entry := cache.entries[oldestKey]
		cache.bytes -= entry.bytes
		delete(cache.entries, oldestKey)
		for key, surface := range cache.surfaces {
			if key.URL == oldestKey {
				cache.bytes -= surface.bytes
				delete(cache.surfaces, key)
			}
		}
		cache.evictions++
	}
}

func (cache *imageResourceCache) clear() {
	if cache == nil {
		return
	}
	cache.mu.Lock()
	cache.entries = make(map[string]*cachedImageResource)
	cache.surfaces = make(map[imageSurfaceCacheKey]*cachedImageSurface)
	cache.bytes = 0
	cache.tick = 0
	cache.hits = 0
	cache.misses = 0
	cache.evictions = 0
	cache.mu.Unlock()
}

func (cache *imageResourceCache) statsSnapshot() imageResourceCacheStats {
	if cache == nil {
		return imageResourceCacheStats{}
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	return imageResourceCacheStats{hits: cache.hits, misses: cache.misses, evictions: cache.evictions}
}

func cloneCachedImageResource(entry *cachedImageResource) cachedImageResource {
	if entry == nil {
		return cachedImageResource{failure: imageLoadRequestFailure}
	}
	return cachedImageResource{
		body: entry.body, contentType: entry.contentType, decoded: entry.decoded,
		width: entry.width, height: entry.height, failure: entry.failure, err: entry.err,
		validator: entry.validator, orientation: entry.orientation,
	}
}

func imageResponseValidator(response *network.Response) string {
	if response == nil {
		return ""
	}
	if response.Header != nil {
		if value := response.Header.Get("ETag"); value != "" {
			return "etag:" + value
		}
		if value := response.Header.Get("Last-Modified"); value != "" {
			return "last-modified:" + value
		}
	}
	digest := sha256.Sum256(response.Body)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func imageNodeStyleRevision(node *dom.Node) uint64 {
	if node == nil {
		return 0
	}
	hash := uint64(1469598103934665603)
	for _, name := range []string{"width", "height", "class", "style"} {
		value, _ := node.Attribute(name)
		for _, character := range name + "=" + value + "\x00" {
			hash ^= uint64(character)
			hash *= 1099511628211
		}
	}
	return hash
}
