package browser

import (
	"context"
	"encoding/binary"
	"image"
	"image/color"
	"testing"

	"github.com/Grove-Computing/Growse/internal/dom"
)

func TestImagePipelineAppliesEXIFOrientationColorConversionAndTargetResize(t *testing.T) {
	source := image.NewPaletted(image.Rect(0, 0, 2, 1), color.Palette{color.Black, color.RGBA{R: 255, A: 255}})
	source.SetColorIndex(0, 0, 1)
	exif := jpegWithOrientation(6)
	normalized := normalizeDecodedImage(source, exif, "image/jpeg")
	if normalized.Bounds().Dx() != 1 || normalized.Bounds().Dy() != 2 || normalized.NRGBAAt(0, 0).R != 255 {
		t.Fatalf("oriented color surface = %v / %#v", normalized.Bounds(), normalized.NRGBAAt(0, 0))
	}
	document := dom.NewDocument()
	node := document.CreateElement("img", map[string]string{"width": "3", "height": "4"})
	resized, err := resizeImageForNode(context.Background(), normalized, node, 2, newImageDecodeBudget())
	if err != nil || resized.Bounds().Dx() != 6 || resized.Bounds().Dy() != 8 {
		t.Fatalf("resized surface = %v / %v", resized, err)
	}
}

func TestImageResizeHonorsCancellationWithoutRetainingSurfaceBudget(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	budget := newImageDecodeBudget()
	node := dom.NewDocument().CreateElement("img", map[string]string{"width": "1024", "height": "1024"})
	_, err := resizeImageForNode(ctx, image.NewNRGBA(image.Rect(0, 0, 2, 2)), node, 1, budget)
	if err == nil || budget.surfaceBytes != 0 {
		t.Fatalf("cancelled resize error/budget = %v / %d", err, budget.surfaceBytes)
	}
}

func jpegWithOrientation(orientation uint16) []byte {
	tiff := make([]byte, 26)
	copy(tiff[:2], "II")
	binary.LittleEndian.PutUint16(tiff[2:4], 42)
	binary.LittleEndian.PutUint32(tiff[4:8], 8)
	binary.LittleEndian.PutUint16(tiff[8:10], 1)
	binary.LittleEndian.PutUint16(tiff[10:12], 0x0112)
	binary.LittleEndian.PutUint16(tiff[12:14], 3)
	binary.LittleEndian.PutUint32(tiff[14:18], 1)
	binary.LittleEndian.PutUint16(tiff[18:20], orientation)
	payload := append([]byte("Exif\x00\x00"), tiff...)
	result := []byte{0xff, 0xd8, 0xff, 0xe1, 0, byte(len(payload) + 2)}
	result = append(result, payload...)
	return append(result, 0xff, 0xd9)
}
