package ui

import (
	"image"
	"testing"

	"github.com/Grove-Computing/Growse/internal/browser"
	"github.com/Grove-Computing/Growse/internal/dom"
	paintmodel "github.com/Grove-Computing/Growse/internal/paint"
	stylemodel "github.com/Grove-Computing/Growse/internal/style"
)

func TestPageImagePaintCacheReusesWarmResizeAndImageOp(t *testing.T) {
	page := &browser.Page{}
	cache := pageImagePaintCache{}
	cache.prepare(page)
	source := image.NewNRGBA(image.Rect(0, 0, 16, 8))
	first := cache.scale("https://example.com/photo.png", source, 320, 160)
	second := cache.scale("https://example.com/photo.png", source, 320, 160)
	if first.raster == nil || second.raster != first.raster || cache.allocations != 1 || len(cache.scaled) != 1 {
		t.Fatalf("warm resize cache = first:%p second:%p allocations:%d entries:%d", first.raster, second.raster, cache.allocations, len(cache.scaled))
	}

	resized := cache.scale("https://example.com/photo.png", source, 160, 80)
	if resized.raster == first.raster || cache.allocations != 2 {
		t.Fatalf("target-specific resize = raster:%p allocations:%d", resized.raster, cache.allocations)
	}
	if metrics := page.RenderMetricsSnapshot(); metrics.ImagePaintHits != 1 || metrics.ImagePaintMisses != 2 {
		t.Fatalf("warm image metrics = %+v", metrics)
	}
}

func TestPageImagePaintCacheReusesBackgroundRasterByStyleAndGeometryRevision(t *testing.T) {
	cache := pageImagePaintCache{}
	cache.prepare(&browser.Page{})
	command := paintmodel.DrawBox{NodeID: dom.NodeID(7), Filters: []stylemodel.Filter{{Kind: stylemodel.FilterBrightness, Amount: .8}}}
	layer := stylemodel.BackgroundLayer{Image: stylemodel.BackgroundImage{
		Kind: stylemodel.BackgroundImageLinearGradient, GradientAngle: 90,
		GradientStops: []stylemodel.GradientStop{{Color: 0xff0000ff, Position: 0}, {Color: 0x0000ffff, Position: 1}},
	}}
	first := cache.background(command, layer, 0, nil, 320, 180, 1, 9)
	warm := cache.background(command, layer, 0, nil, 320, 180, 1, 9)
	if first.raster == nil || warm.raster != first.raster || cache.allocations != 1 {
		t.Fatalf("warm background cache = first:%p warm:%p allocations:%d", first.raster, warm.raster, cache.allocations)
	}
	restyled := cache.background(command, layer, 0, nil, 320, 180, 1, 10)
	resized := cache.background(command, layer, 0, nil, 321, 180, 1, 10)
	if restyled.raster == first.raster || resized.raster == restyled.raster || cache.allocations != 3 {
		t.Fatalf("background revision cache = first:%p restyled:%p resized:%p allocations:%d", first.raster, restyled.raster, resized.raster, cache.allocations)
	}
}

func TestPageImagePaintCacheDropsPreviousGeneration(t *testing.T) {
	cache := pageImagePaintCache{}
	firstPage, secondPage := &browser.Page{}, &browser.Page{}
	source := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	cache.prepare(firstPage)
	first := cache.scale("https://example.com/photo.png", source, 20, 20)
	cache.prepare(secondPage)
	second := cache.scale("https://example.com/photo.png", source, 20, 20)
	if first.raster == second.raster || cache.page != secondPage || cache.allocations != 1 || len(cache.scaled) != 1 {
		t.Fatalf("page generation reset = first:%p second:%p allocations:%d entries:%d", first.raster, second.raster, cache.allocations, len(cache.scaled))
	}
}

func TestPageImagePaintCacheUsesBoundedLRUAndClearsOnClose(t *testing.T) {
	page := &browser.Page{}
	cache := pageImagePaintCache{maxBytes: 801, maxEntries: 2}
	cache.prepare(page)
	sources := []*image.NRGBA{
		image.NewNRGBA(image.Rect(0, 0, 2, 2)),
		image.NewNRGBA(image.Rect(0, 0, 3, 3)),
		image.NewNRGBA(image.Rect(0, 0, 4, 4)),
	}
	cache.scale("a", sources[0], 10, 10)
	cache.scale("b", sources[1], 10, 10)
	cache.scale("a", sources[0], 10, 10)
	cache.scale("c", sources[2], 10, 10)
	bKey := scaledImageKey{url: "b", source: imagePointer(sources[1]), sourceBounds: sources[1].Bounds(), width: 10, height: 10}
	if _, exists := cache.scaled[bKey]; exists || len(cache.scaled) != 2 || cache.bytes > cache.maxBytes {
		t.Fatalf("paint LRU entries/bytes = %#v / %d", cache.scaled, cache.bytes)
	}
	cache.prepare(nil)
	if cache.page != nil || len(cache.scaled) != 0 || len(cache.backgrounds) != 0 || cache.bytes != 0 {
		t.Fatalf("closed page paint cache = page:%p scaled:%d backgrounds:%d bytes:%d", cache.page, len(cache.scaled), len(cache.backgrounds), cache.bytes)
	}
}
