package conformance

import (
	"image"
	"image/color"
	"math"
)

const (
	DefaultPerceptualPixelDelta = 0.05
	DefaultPerceptualDiffLimit  = 0.02
)

type VisualRegion struct {
	Name   string          `json:"name"`
	Bounds image.Rectangle `json:"bounds"`
	Limit  float64         `json:"limit,omitempty"`
}

type VisualRegionResult struct {
	Name       string  `json:"name"`
	Compared   int     `json:"compared"`
	Different  int     `json:"different"`
	Difference float64 `json:"difference"`
	Limit      float64 `json:"limit"`
	Passed     bool    `json:"passed"`
}

type VisualReport struct {
	DimensionMatch bool                 `json:"dimensionMatch"`
	Regions        []VisualRegionResult `json:"regions"`
}

func (report VisualReport) Passed() bool {
	if !report.DimensionMatch || len(report.Regions) == 0 {
		return false
	}
	for _, region := range report.Regions {
		if !region.Passed {
			return false
		}
	}
	return true
}

// CompareVisual reports the ratio of perceptually changed pixels after
// excluding explicit dynamic/font masks. Every named region is gated
// independently so a small but important panel cannot disappear unnoticed.
func CompareVisual(reference, actual image.Image, regions []VisualRegion, masks []image.Rectangle) VisualReport {
	if reference == nil || actual == nil || reference.Bounds() != actual.Bounds() {
		return VisualReport{}
	}
	bounds := reference.Bounds()
	if len(regions) == 0 {
		regions = []VisualRegion{{Name: "page", Bounds: bounds}}
	}
	report := VisualReport{DimensionMatch: true, Regions: make([]VisualRegionResult, 0, len(regions))}
	for _, region := range regions {
		limit := region.Limit
		if limit <= 0 {
			limit = DefaultPerceptualDiffLimit
		}
		result := VisualRegionResult{Name: region.Name, Limit: limit}
		clipped := region.Bounds.Intersect(bounds)
		for y := clipped.Min.Y; y < clipped.Max.Y; y++ {
			for x := clipped.Min.X; x < clipped.Max.X; x++ {
				point := image.Pt(x, y)
				if masked(point, masks) {
					continue
				}
				result.Compared++
				if perceptualPixelDelta(reference.At(x, y), actual.At(x, y)) > DefaultPerceptualPixelDelta {
					result.Different++
				}
			}
		}
		if result.Compared != 0 {
			result.Difference = float64(result.Different) / float64(result.Compared)
		}
		result.Passed = result.Compared > 0 && result.Difference <= result.Limit
		report.Regions = append(report.Regions, result)
	}
	return report
}

func masked(point image.Point, masks []image.Rectangle) bool {
	for _, mask := range masks {
		if point.In(mask) {
			return true
		}
	}
	return false
}

func perceptualPixelDelta(reference, actual color.Color) float64 {
	rr, rg, rb, ra := reference.RGBA()
	ar, ag, ab, aa := actual.RGBA()
	delta := 0.299*math.Abs(float64(rr)-float64(ar)) +
		0.587*math.Abs(float64(rg)-float64(ag)) +
		0.114*math.Abs(float64(rb)-float64(ab))
	alpha := math.Abs(float64(ra) - float64(aa))
	return math.Max(delta, alpha) / 65535
}
