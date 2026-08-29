package browser

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"testing"

	"github.com/Grove-Computing/Growse/internal/network"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
)

func TestImageResourceCacheEvictsLeastRecentlyUsedEntryWithinLimits(t *testing.T) {
	encode := func(value uint8) []byte {
		var output bytes.Buffer
		pixels := image.NewNRGBA(image.Rect(0, 0, 10, 10))
		pixels.Pix[0] = value
		if err := png.Encode(&output, pixels); err != nil {
			t.Fatal(err)
		}
		return output.Bytes()
	}
	urls := []string{"https://example.com/a.png", "https://example.com/b.png", "https://example.com/c.png"}
	loader := &routeLoader{responses: map[string]*network.Response{}}
	for index, rawURL := range urls {
		loader.responses[rawURL] = &network.Response{URL: mustParseURL(t, rawURL), ContentType: "image/png", Body: encode(uint8(index + 1))}
	}
	entryBytes := int64(len(loader.responses[urls[0]].Body) + 10*10*4)
	cache := newImageResourceCacheWithLimits(entryBytes*2+1, 2)
	budget := newImageDecodeBudget()
	for _, rawURL := range urls[:2] {
		if result := cache.load(context.Background(), loader, mustParseURL(t, rawURL), budget); result.failure != imageLoadOK {
			t.Fatalf("load %s failed: %#v", rawURL, result)
		}
	}
	_ = cache.load(context.Background(), loader, mustParseURL(t, urls[0]), budget)
	if result := cache.load(context.Background(), loader, mustParseURL(t, urls[2]), budget); result.failure != imageLoadOK {
		t.Fatalf("load %s failed: %#v", urls[2], result)
	}
	if cache.entries[urls[0]] == nil || cache.entries[urls[1]] != nil || cache.entries[urls[2]] == nil || len(cache.entries) != 2 || cache.bytes > cache.maxBytes {
		t.Fatalf("LRU entries/bytes = %#v / %d", cache.entries, cache.bytes)
	}
}

func TestImageResourceCacheIsReleasedOnEngineSwitch(t *testing.T) {
	pageURL := "https://example.com/"
	loader := &routeLoader{responses: map[string]*network.Response{
		pageURL: {URL: mustParseURL(t, pageURL), StatusCode: 200, ContentType: "text/html", Body: []byte("<p>next generation</p>")},
	}}
	cache := newImageResourceCache()
	cache.entries["https://example.com/old.png"] = &cachedImageResource{ready: make(chan struct{}), complete: true, body: []byte("old"), bytes: 3}
	cache.bytes = 3
	oldPage := &Page{URL: mustParseURL(t, pageURL), imageCache: cache, Images: map[string]image.Image{"old": image.NewNRGBA(image.Rect(0, 0, 1, 1))}}
	browserState := NewWithEngineFactory(loader, func(runtimemodel.Engine) runtimemodel.Runtime { return &runtimeStub{} })
	defer browserState.Close()
	browserState.SetPage(oldPage)
	newPage, err := browserState.SetEngine(context.Background(), runtimemodel.EngineJavaScript)
	if err != nil {
		t.Fatal(err)
	}
	if newPage == nil || newPage == oldPage || oldPage.imageCache != nil || oldPage.Images != nil || len(cache.entries) != 0 || cache.bytes != 0 {
		t.Fatalf("engine switch cache release = new:%p oldCache:%p oldImages:%v entries:%d bytes:%d", newPage, oldPage.imageCache, oldPage.Images, len(cache.entries), cache.bytes)
	}
}
