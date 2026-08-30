package conformance

import (
	"image"
	"image/color"
	"testing"
)

func TestCompareVisualMasksDynamicAreasAndGatesEveryRegionAtTwoPercent(t *testing.T) {
	reference := solidImage(100, 100, color.NRGBA{R: 245, G: 245, B: 245, A: 255})
	actual := solidImage(100, 100, color.NRGBA{R: 245, G: 245, B: 245, A: 255})
	paint(actual, image.Rect(0, 0, 10, 10), color.NRGBA{R: 220, A: 255})
	paint(actual, image.Rect(20, 20, 21, 70), color.NRGBA{B: 220, A: 255})
	regions := []VisualRegion{
		{Name: "page", Bounds: image.Rect(0, 0, 100, 100)},
		{Name: "content", Bounds: image.Rect(10, 0, 100, 100)},
	}
	report := CompareVisual(reference, actual, regions, []image.Rectangle{image.Rect(0, 0, 10, 10)})
	if !report.Passed() {
		t.Fatalf("masked 50/9000 diff failed: %+v", report)
	}
	paint(actual, image.Rect(20, 20, 23, 100), color.NRGBA{B: 220, A: 255})
	report = CompareVisual(reference, actual, regions, []image.Rectangle{image.Rect(0, 0, 10, 10)})
	if report.Passed() || report.Regions[1].Difference <= DefaultPerceptualDiffLimit {
		t.Fatalf("region above 2%% passed: %+v", report)
	}
}

func TestCompareVisualRejectsDimensionMismatchAndEmptyRegion(t *testing.T) {
	first := solidImage(10, 10, color.Black)
	if report := CompareVisual(first, solidImage(11, 10, color.Black), nil, nil); report.Passed() {
		t.Fatal("dimension mismatch passed")
	}
	if report := CompareVisual(first, first, []VisualRegion{{Name: "outside", Bounds: image.Rect(20, 20, 30, 30)}}, nil); report.Passed() {
		t.Fatal("empty region passed")
	}
}

func solidImage(width, height int, value color.Color) *image.NRGBA {
	result := image.NewNRGBA(image.Rect(0, 0, width, height))
	paint(result, result.Bounds(), value)
	return result
}

func paint(target *image.NRGBA, bounds image.Rectangle, value color.Color) {
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			target.Set(x, y, value)
		}
	}
}
