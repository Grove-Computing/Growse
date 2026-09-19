package ui

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/Grove-Computing/Growse/internal/browser"
	"github.com/Grove-Computing/Growse/internal/dom"
	paintmodel "github.com/Grove-Computing/Growse/internal/paint"
	stylemodel "github.com/Grove-Computing/Growse/internal/style"
)

const v020SurfaceEvidenceHash = "4ad83c05727262cee2842ef8ab21547f5414b7e76f6a25c9ef15ed4aabf3c8fb"

// TestV020CSSSurfaceRasterEvidence exercises the same background raster cache
// used by the desktop UI. It keeps gradient, filter, and cache-key regressions
// visible as a PNG instead of accepting display-list metadata alone.
func TestV020CSSSurfaceRasterEvidence(t *testing.T) {
	cache := pageImagePaintCache{}
	cache.prepare(&browser.Page{})
	linear := surfaceRaster(t, &cache, 1, 1, stylemodel.BackgroundImage{
		Kind: stylemodel.BackgroundImageLinearGradient, GradientAngle: 90,
		GradientStops: []stylemodel.GradientStop{{Color: 0xdc2626ff, Position: 0}, {Color: 0x2563ebff, Position: 1}},
	})
	radial := surfaceRaster(t, &cache, 2, 1, stylemodel.BackgroundImage{
		Kind:           stylemodel.BackgroundImageRadialGradient,
		GradientCenter: stylemodel.BackgroundPosition{X: stylemodel.LengthPercentage{Percentage: 50}, Y: stylemodel.LengthPercentage{Percentage: 50}},
		GradientStops:  []stylemodel.GradientStop{{Color: 0xfef3c7ff, Position: 0}, {Color: 0xf59e0bff, Position: 1}},
	})
	conic := surfaceRaster(t, &cache, 3, 2, stylemodel.BackgroundImage{
		Kind: stylemodel.BackgroundImageConicGradient, GradientAngle: 30,
		GradientCenter: stylemodel.BackgroundPosition{X: stylemodel.LengthPercentage{Percentage: 50}, Y: stylemodel.LengthPercentage{Percentage: 50}},
		GradientStops:  []stylemodel.GradientStop{{Color: 0x047857ff, Position: 0}, {Color: 0x7c3aedff, Position: 1}},
	})
	if linear.NRGBAAt(0, 40).R <= linear.NRGBAAt(79, 40).R || linear.NRGBAAt(0, 40).B >= linear.NRGBAAt(79, 40).B {
		t.Fatalf("linear gradient endpoints = %v / %v", linear.NRGBAAt(0, 40), linear.NRGBAAt(79, 40))
	}
	if radial.NRGBAAt(40, 40) == radial.NRGBAAt(0, 0) || conic.NRGBAAt(40, 0) == conic.NRGBAAt(0, 40) {
		t.Fatalf("radial/conic gradient lost surface variation")
	}

	canvas := image.NewRGBA(image.Rect(0, 0, 272, 112))
	draw.Draw(canvas, canvas.Bounds(), image.NewUniform(color.RGBA{R: 248, G: 250, B: 252, A: 255}), image.Point{}, draw.Src)
	for index, raster := range []*image.NRGBA{linear, radial, conic} {
		draw.Draw(canvas, image.Rect(16+index*88, 16, 96+index*88, 96), raster, image.Point{}, draw.Over)
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, canvas); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(encoded.Bytes())
	if got := hex.EncodeToString(hash[:]); got != v020SurfaceEvidenceHash {
		t.Fatalf("CSS surface PNG hash = %s, want %s", got, v020SurfaceEvidenceHash)
	}
	writeV020SurfaceArtifact(t, canvas)
}

func surfaceRaster(t *testing.T, cache *pageImagePaintCache, nodeID dom.NodeID, revision uint64, imageValue stylemodel.BackgroundImage) *image.NRGBA {
	t.Helper()
	command := paintmodel.DrawBox{NodeID: nodeID, Filters: []stylemodel.Filter{{Kind: stylemodel.FilterBrightness, Amount: .9}}}
	cached := cache.background(command, stylemodel.BackgroundLayer{Image: imageValue}, 0, nil, 80, 80, 1, revision)
	if cached.raster == nil {
		t.Fatalf("surface raster is nil for %#v", imageValue.Kind)
	}
	return cached.raster
}

func writeV020SurfaceArtifact(t *testing.T, source image.Image) {
	t.Helper()
	directory := os.Getenv("GROWSE_VISUAL_ARTIFACT_DIR")
	if directory == "" {
		return
	}
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(filepath.Join(directory, "css-surface-evidence.png"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := png.Encode(file, source); err != nil {
		t.Fatal(err)
	}
}
