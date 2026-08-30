package browser

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"testing"
	"time"

	"github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/layout"
	animatedwebp "github.com/gen2brain/webp"
)

func TestComposeGIFFramesHonorsDisposalPrevious(t *testing.T) {
	palette := color.Palette{color.Transparent, color.RGBA{R: 255, A: 255}, color.RGBA{B: 255, A: 255}, color.RGBA{G: 255, A: 255}}
	frame := func(bounds image.Rectangle, index uint8) *image.Paletted {
		result := image.NewPaletted(bounds, palette)
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				result.SetColorIndex(x, y, index)
			}
		}
		return result
	}
	source := &gif.GIF{
		Image: []*image.Paletted{
			frame(image.Rect(0, 0, 2, 1), 1),
			frame(image.Rect(0, 0, 1, 1), 2),
			frame(image.Rect(1, 0, 2, 1), 3),
		},
		Disposal: []byte{gif.DisposalNone, gif.DisposalPrevious, gif.DisposalNone},
		Config:   image.Config{Width: 2, Height: 1, ColorModel: palette},
	}
	frames, err := composeGIFFrames(source, newImageDecodeBudget())
	if err != nil {
		t.Fatal(err)
	}
	assertPixel := func(frameIndex, x int, expected color.RGBA) {
		t.Helper()
		actual := color.RGBAModel.Convert(frames[frameIndex].At(x, 0)).(color.RGBA)
		if actual != expected {
			t.Fatalf("frame %d pixel %d = %#v, want %#v", frameIndex, x, actual, expected)
		}
	}
	assertPixel(1, 0, color.RGBA{B: 255, A: 255})
	assertPixel(1, 1, color.RGBA{R: 255, A: 255})
	assertPixel(2, 0, color.RGBA{R: 255, A: 255})
	assertPixel(2, 1, color.RGBA{G: 255, A: 255})
}

func TestDecodeAnimatedWebPFrames(t *testing.T) {
	first := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	second := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	first.Set(0, 0, color.RGBA{R: 255, A: 255})
	second.Set(0, 0, color.RGBA{B: 255, A: 255})
	var encoded bytes.Buffer
	if err := animatedwebp.EncodeAll(&encoded, &animatedwebp.WEBP{Image: []image.Image{first, second}, Delay: []int{80, 120}}, animatedwebp.Options{Lossless: true}); err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeAnimatedImage(encoded.Bytes(), "image/webp", newImageDecodeBudget())
	if err != nil {
		t.Fatal(err)
	}
	if decoded == nil || len(decoded.frames) != 2 || decoded.delays[0] != 80*time.Millisecond || decoded.delays[1] != 120*time.Millisecond {
		t.Fatalf("unexpected animation: %#v", decoded)
	}
}

func TestAnimatedImageClockPausesOffscreenAndCloses(t *testing.T) {
	document := dom.NewDocument()
	target := document.CreateElement("img", map[string]string{"src": "animation.gif"})
	if err := document.AppendChild(document.Root, target); err != nil {
		t.Fatal(err)
	}
	red := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	blue := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	red.Set(0, 0, color.RGBA{R: 255, A: 255})
	blue.Set(0, 0, color.RGBA{B: 255, A: 255})
	player := &animatedImagePlayer{data: &animatedImageData{frames: []image.Image{red, blue}, delays: []time.Duration{100 * time.Millisecond, 100 * time.Millisecond}}}
	page := &Page{
		Document: document, ViewportWidth: 320, ViewportHeight: 240,
		ImageResources: map[dom.NodeID]layout.ImageResource{target.ID: {URL: "animation.gif"}},
		Images:         map[string]image.Image{"animation.gif": red},
		AnimatedImages: map[dom.NodeID]*animatedImagePlayer{target.ID: player},
	}
	visible := &layout.Tree{Width: 320, Height: 240, Bounds: map[dom.NodeID]layout.Rect{target.ID: {X: 10, Y: 10, Width: 20, Height: 20}}}
	start := time.Unix(100, 0)
	if dirty := page.AdvanceAnimatedImages(start, visible); len(dirty) != 0 {
		t.Fatalf("initial frame dirtied: %v", dirty)
	}
	if dirty := page.AdvanceAnimatedImages(start.Add(150*time.Millisecond), visible); len(dirty) != 1 || dirty[0] != target.ID {
		t.Fatalf("second frame dirty nodes = %v", dirty)
	}
	offscreen := &layout.Tree{Width: 320, Height: 600, Bounds: map[dom.NodeID]layout.Rect{target.ID: {X: 10, Y: 400, Width: 20, Height: 20}}}
	page.AdvanceAnimatedImages(start.Add(160*time.Millisecond), offscreen)
	if player.pausedAt.IsZero() {
		t.Fatal("offscreen animation was not paused")
	}
	if dirty := page.AdvanceAnimatedImages(start.Add(550*time.Millisecond), visible); len(dirty) != 0 || player.current != 1 {
		t.Fatalf("resume advanced paused clock: dirty=%v current=%d", dirty, player.current)
	}
	page.ReducedMotion = true
	page.AdvanceAnimatedImages(start.Add(600*time.Millisecond), visible)
	if player.pausedAt.IsZero() {
		t.Fatal("reduced motion did not pause animation")
	}
	page.releaseImageResources()
	if !player.closed || page.AnimatedImages != nil {
		t.Fatal("animation resources were not released")
	}
}
