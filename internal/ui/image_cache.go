package ui

import (
	"image"
	imagedraw "image/draw"
	"reflect"

	"gioui.org/op/paint"
	"github.com/Grove-Computing/Growse/internal/browser"
	xdraw "golang.org/x/image/draw"
)

type scaledImageKey struct {
	url          string
	source       uintptr
	sourceBounds image.Rectangle
	width        int
	height       int
}

type cachedScaledImage struct {
	raster *image.NRGBA
	op     paint.ImageOp
}

type pageImagePaintCache struct {
	page        *browser.Page
	scaled      map[scaledImageKey]cachedScaledImage
	allocations uint64
}

func (cache *pageImagePaintCache) prepare(page *browser.Page) {
	if cache.page == page && cache.scaled != nil {
		return
	}
	cache.page = page
	cache.scaled = make(map[scaledImageKey]cachedScaledImage)
	cache.allocations = 0
}

func (cache *pageImagePaintCache) scale(url string, source image.Image, width, height int) cachedScaledImage {
	if source == nil || width <= 0 || height <= 0 {
		return cachedScaledImage{}
	}
	identity := imagePointer(source)
	key := scaledImageKey{url: url, source: identity, sourceBounds: source.Bounds(), width: width, height: height}
	if identity != 0 {
		if cached, ok := cache.scaled[key]; ok {
			return cached
		}
	}
	raster := image.NewNRGBA(image.Rect(0, 0, width, height))
	xdraw.CatmullRom.Scale(raster, raster.Bounds(), source, source.Bounds(), imagedraw.Src, nil)
	result := cachedScaledImage{raster: raster, op: paint.NewImageOp(raster)}
	cache.allocations++
	if identity != 0 {
		cache.scaled[key] = result
	}
	return result
}

func imagePointer(source image.Image) uintptr {
	value := reflect.ValueOf(source)
	if value.IsValid() && value.Kind() == reflect.Pointer && !value.IsNil() {
		return value.Pointer()
	}
	return 0
}
