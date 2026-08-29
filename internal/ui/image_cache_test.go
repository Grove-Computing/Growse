package ui

import (
	"image"
	"testing"

	"github.com/Grove-Computing/Growse/internal/browser"
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
