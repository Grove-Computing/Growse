package ui

import (
	"image"
	imagedraw "image/draw"
	"math"
	"reflect"

	"gioui.org/op/paint"
	"github.com/Grove-Computing/Growse/internal/browser"
	"github.com/Grove-Computing/Growse/internal/dom"
	paintmodel "github.com/Grove-Computing/Growse/internal/paint"
	stylemodel "github.com/Grove-Computing/Growse/internal/style"
	xdraw "golang.org/x/image/draw"
)

const (
	maxPagePaintCacheBytes   = 128 << 20
	maxPagePaintCacheEntries = 256
)

type scaledImageKey struct {
	url          string
	source       uintptr
	sourceBounds image.Rectangle
	width        int
	height       int
}

type cachedScaledImage struct {
	raster   *image.NRGBA
	op       paint.ImageOp
	bytes    int64
	lastUsed uint64
}

type backgroundRasterKey struct {
	nodeID        dom.NodeID
	styleRevision uint64
	layer         int
	source        uintptr
	sourceBounds  image.Rectangle
	width         int
	height        int
	scale         uint32
}

type pageImagePaintCache struct {
	page        *browser.Page
	scaled      map[scaledImageKey]cachedScaledImage
	backgrounds map[backgroundRasterKey]cachedScaledImage
	allocations uint64
	bytes       int64
	tick        uint64
	maxBytes    int64
	maxEntries  int
}

func (cache *pageImagePaintCache) prepare(page *browser.Page) {
	if cache.page == page && cache.scaled != nil && cache.backgrounds != nil {
		return
	}
	cache.page = page
	cache.scaled = make(map[scaledImageKey]cachedScaledImage)
	cache.backgrounds = make(map[backgroundRasterKey]cachedScaledImage)
	cache.allocations = 0
	cache.bytes = 0
	cache.tick = 0
	if cache.maxBytes == 0 {
		cache.maxBytes = maxPagePaintCacheBytes
	}
	if cache.maxEntries == 0 {
		cache.maxEntries = maxPagePaintCacheEntries
	}
}

func (cache *pageImagePaintCache) background(command paintmodel.DrawBox, layer stylemodel.BackgroundLayer, layerIndex int, source image.Image, width, height int, pixelsPerDP float32, styleRevision uint64) cachedScaledImage {
	if width <= 0 || height <= 0 || pixelsPerDP <= 0 {
		return cachedScaledImage{}
	}
	key := backgroundRasterKey{
		nodeID: command.NodeID, styleRevision: styleRevision, layer: layerIndex,
		width: width, height: height, scale: math.Float32bits(pixelsPerDP),
	}
	if source != nil {
		key.source, key.sourceBounds = imagePointer(source), source.Bounds()
	}
	if cached, ok := cache.backgrounds[key]; ok {
		cache.page.RecordRenderEvent(browser.RenderImagePaintHit)
		cache.tick++
		cached.lastUsed = cache.tick
		cache.backgrounds[key] = cached
		return cached
	}
	var raster image.Image
	switch layer.Image.Kind {
	case stylemodel.BackgroundImageLinearGradient:
		raster = rasterLinearGradient(width, height, layer.Image)
	case stylemodel.BackgroundImageRadialGradient:
		raster = rasterRadialGradient(width, height, layer.Image)
	case stylemodel.BackgroundImageURL:
		if source != nil {
			layerCommand := command
			layerCommand.Image, layerCommand.Repeat, layerCommand.Position, layerCommand.Size = layer.Image, layer.Repeat, layer.Position, layer.Size
			raster = rasterBackgroundImage(width, height, source, layerCommand, pixelsPerDP)
		}
	}
	if raster == nil {
		return cachedScaledImage{}
	}
	cache.page.RecordRenderEvent(browser.RenderImagePaintMiss)
	raster = rasterFilterImage(raster, command.Filters)
	result := cachedScaledImage{op: paint.NewImageOp(raster), bytes: int64(width) * int64(height) * 4}
	if typed, ok := raster.(*image.NRGBA); ok {
		result.raster = typed
	} else {
		copy := image.NewNRGBA(raster.Bounds())
		imagedraw.Draw(copy, copy.Bounds(), raster, raster.Bounds().Min, imagedraw.Src)
		result.raster = copy
		result.op = paint.NewImageOp(copy)
	}
	cache.tick++
	result.lastUsed = cache.tick
	cache.backgrounds[key] = result
	cache.bytes += result.bytes
	cache.allocations++
	cache.evict()
	return result
}

func (cache *pageImagePaintCache) scale(url string, source image.Image, width, height int) cachedScaledImage {
	if source == nil || width <= 0 || height <= 0 {
		return cachedScaledImage{}
	}
	identity := imagePointer(source)
	key := scaledImageKey{url: url, source: identity, sourceBounds: source.Bounds(), width: width, height: height}
	if identity != 0 {
		if cached, ok := cache.scaled[key]; ok {
			cache.page.RecordRenderEvent(browser.RenderImagePaintHit)
			cache.tick++
			cached.lastUsed = cache.tick
			cache.scaled[key] = cached
			return cached
		}
	}
	cache.page.RecordRenderEvent(browser.RenderImagePaintMiss)
	raster := image.NewNRGBA(image.Rect(0, 0, width, height))
	xdraw.CatmullRom.Scale(raster, raster.Bounds(), source, source.Bounds(), imagedraw.Src, nil)
	result := cachedScaledImage{raster: raster, op: paint.NewImageOp(raster), bytes: int64(width) * int64(height) * 4}
	cache.allocations++
	if identity != 0 {
		cache.tick++
		result.lastUsed = cache.tick
		cache.scaled[key] = result
		cache.bytes += result.bytes
		cache.evict()
	}
	return result
}

func (cache *pageImagePaintCache) evict() {
	for cache.bytes > cache.maxBytes || len(cache.scaled)+len(cache.backgrounds) > cache.maxEntries {
		oldestScaled, oldestBackground := scaledImageKey{}, backgroundRasterKey{}
		oldestTick := ^uint64(0)
		kind := uint8(0)
		for key, entry := range cache.scaled {
			if entry.lastUsed < oldestTick {
				oldestScaled, oldestTick, kind = key, entry.lastUsed, 1
			}
		}
		for key, entry := range cache.backgrounds {
			if entry.lastUsed < oldestTick {
				oldestBackground, oldestTick, kind = key, entry.lastUsed, 2
			}
		}
		switch kind {
		case 1:
			cache.bytes -= cache.scaled[oldestScaled].bytes
			delete(cache.scaled, oldestScaled)
			cache.page.RecordRenderEvent(browser.RenderImagePaintEviction)
		case 2:
			cache.bytes -= cache.backgrounds[oldestBackground].bytes
			delete(cache.backgrounds, oldestBackground)
			cache.page.RecordRenderEvent(browser.RenderImagePaintEviction)
		default:
			return
		}
	}
}

func imagePointer(source image.Image) uintptr {
	value := reflect.ValueOf(source)
	if value.IsValid() && value.Kind() == reflect.Pointer && !value.IsNil() {
		return value.Pointer()
	}
	return 0
}
