package browser

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"net/http"
	"testing"

	"github.com/Grove-Computing/Growse/internal/dom"
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
	if stats := cache.statsSnapshot(); stats.hits != 1 || stats.misses != 3 || stats.evictions != 1 || stats.decodes != 3 {
		t.Fatalf("image resource cache stats = %+v", stats)
	}
}

func TestImageGalleryWarmAccessDoesNotDecodeOrResizeAgain(t *testing.T) {
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewNRGBA(image.Rect(0, 0, 8, 8))); err != nil {
		t.Fatal(err)
	}
	loader := &routeLoader{responses: map[string]*network.Response{}}
	document := dom.NewDocument()
	cache := newImageResourceCacheWithLimits(1<<20, 16)
	budget := newImageDecodeBudget()
	type galleryItem struct {
		rawURL string
		node   *dom.Node
	}
	items := make([]galleryItem, 0, 3)
	for _, name := range []string{"a", "b", "c"} {
		rawURL := "https://example.com/" + name + ".png"
		loader.responses[rawURL] = &network.Response{URL: mustParseURL(t, rawURL), ContentType: "image/png", Body: encoded.Bytes()}
		node := document.CreateElement("img", map[string]string{"width": "4", "height": "4"})
		source := cache.load(context.Background(), loader, mustParseURL(t, rawURL), budget)
		if _, err := cache.prepareSurface(context.Background(), source, mustParseURL(t, rawURL), node, 1, budget); err != nil {
			t.Fatal(err)
		}
		items = append(items, galleryItem{rawURL: rawURL, node: node})
	}
	cold := cache.statsSnapshot()
	for _, item := range items {
		source := cache.load(context.Background(), loader, mustParseURL(t, item.rawURL), budget)
		if _, err := cache.prepareSurface(context.Background(), source, mustParseURL(t, item.rawURL), item.node, 1, budget); err != nil {
			t.Fatal(err)
		}
	}
	warm := cache.statsSnapshot()
	if cold.decodes != 3 || cold.resizes != 3 || warm.decodes != cold.decodes || warm.resizes != cold.resizes || warm.hits < cold.hits+6 {
		t.Fatalf("gallery cold/warm stats = %+v / %+v", cold, warm)
	}
}

func TestImageSurfaceCacheKeysValidatorDPRTargetOrientationAndStyleRevision(t *testing.T) {
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewNRGBA(image.Rect(0, 0, 4, 2))); err != nil {
		t.Fatal(err)
	}
	rawURL := "https://example.com/hero.png"
	loader := &routeLoader{responses: map[string]*network.Response{
		rawURL: {URL: mustParseURL(t, rawURL), ContentType: "image/png", Header: http.Header{"Etag": {`"hero-v1"`}}, Body: encoded.Bytes()},
	}}
	cache := newImageResourceCacheWithLimits(1<<20, 2)
	budget := newImageDecodeBudget()
	source := cache.load(context.Background(), loader, mustParseURL(t, rawURL), budget)
	if source.failure != imageLoadOK {
		t.Fatalf("source load = %#v", source)
	}
	document := dom.NewDocument()
	first := document.CreateElement("img", map[string]string{"width": "2", "height": "1", "class": "hero"})
	second := document.CreateElement("img", map[string]string{"width": "2", "height": "1", "class": "hero"})
	for _, node := range []*dom.Node{first, second} {
		if _, err := cache.prepareSurface(context.Background(), source, mustParseURL(t, rawURL), node, 1, budget); err != nil {
			t.Fatal(err)
		}
	}
	if len(cache.surfaces) != 1 {
		t.Fatalf("equivalent scaled surfaces were not shared: %#v", cache.surfaces)
	}
	for key := range cache.surfaces {
		if key.URL != rawURL || key.Validator != `etag:"hero-v1"` || key.DeviceScaleMilli != 1000 || key.TargetWidth != 2 || key.TargetHeight != 1 || key.Orientation != 1 || key.StyleRevision == 0 {
			t.Fatalf("surface cache key = %+v", key)
		}
	}
	second.Attributes["class"] = "hero changed"
	if _, err := cache.prepareSurface(context.Background(), source, mustParseURL(t, rawURL), second, 2, budget); err != nil {
		t.Fatal(err)
	}
	if len(cache.entries) != 1 || len(cache.surfaces) != 1 || cache.evictions == 0 {
		t.Fatalf("variant eviction/source reuse = sources:%d surfaces:%d evictions:%d", len(cache.entries), len(cache.surfaces), cache.evictions)
	}
	for key := range cache.surfaces {
		if key.DeviceScaleMilli != 2000 || key.TargetWidth != 4 || key.TargetHeight != 2 {
			t.Fatalf("replacement variant key = %+v", key)
		}
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
