package browser

import (
	"context"
	"image"
	"net/url"
	"sync"
)

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
}

// imageResourceCache is owned by one Page generation. It coalesces duplicate
// network/decode work while retaining the validated response body for later
// consumers in the same generation.
type imageResourceCache struct {
	mu      sync.Mutex
	entries map[string]*cachedImageResource
}

func newImageResourceCache() *imageResourceCache {
	return &imageResourceCache{entries: make(map[string]*cachedImageResource)}
}

func (cache *imageResourceCache) load(ctx context.Context, client ResourceLoader, target *url.URL, budget *imageDecodeBudget) cachedImageResource {
	if cache == nil || client == nil || target == nil {
		return cachedImageResource{failure: imageLoadRequestFailure}
	}
	key := target.String()
	cache.mu.Lock()
	if existing := cache.entries[key]; existing != nil {
		ready := existing.ready
		cache.mu.Unlock()
		select {
		case <-ready:
			return cloneCachedImageResource(existing)
		case <-ctx.Done():
			return cachedImageResource{failure: imageLoadRequestFailure, err: ctx.Err()}
		}
	}
	if !budget.claim(key) {
		cache.mu.Unlock()
		return cachedImageResource{failure: imageLoadResourceLimit}
	}
	entry := &cachedImageResource{ready: make(chan struct{})}
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
	close(entry.ready)
	if ctx.Err() != nil {
		delete(cache.entries, key)
	}
	cache.mu.Unlock()
	return result
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
