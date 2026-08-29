package layout

import (
	"math"
	"testing"
)

func TestSystemFontSetMeasuresMixedCJKLatinText(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	fonts := NewFontSetWithSystemFallback(nil)
	style := blockStyle{fontFamilies: []string{"system-ui", "sans-serif"}, fontSize: 16}
	width, height, ascent, ok := fonts.measure("日本語 ABC", style)
	if !ok || width <= 0 || height <= 0 || ascent <= 0 || math.IsNaN(float64(width)) || math.IsInf(float64(width), 0) {
		t.Fatalf("mixed CJK measurement = width:%v height:%v ascent:%v ok:%t", width, height, ascent, ok)
	}
}

func TestFontSetTerminatesWhenSystemDiscoveryAndGlyphCoverageAreUnavailable(t *testing.T) {
	fonts := newFontSetWithSystemFallback(nil, false)
	style := blockStyle{fontFamilies: []string{"missing-system-family", "sans-serif"}, fontSize: 16}
	value := "日本語\U0010ffff"
	width, height, ascent, ok := fonts.measure(value, style)
	if !ok || width <= 0 || height <= 0 || ascent <= 0 {
		t.Fatalf("bundled missing-glyph measurement = width:%v height:%v ascent:%v ok:%t", width, height, ascent, ok)
	}
}
