package style

import (
	"math"
	"testing"
)

func TestResolveLengthSupportsCSS3Units(t *testing.T) {
	context := LengthContext{
		FontSize: 20, RootFontSize: 16, ViewportWidth: 1000, ViewportHeight: 600, PercentageBase: 400,
	}
	tests := []struct {
		value string
		want  float32
	}{
		{"1px", 1}, {"1in", 96}, {"2.54cm", 96}, {"25.4mm", 96},
		{"101.6q", 96}, {"72pt", 96}, {"6pc", 96}, {"2em", 40},
		{"2rem", 32}, {"2ex", 20}, {"2ch", 20}, {"10vw", 100},
		{"10vh", 60}, {"10vmin", 60}, {"10vmax", 100}, {"25%", 100}, {"0", 0},
	}
	for _, test := range tests {
		t.Run(test.value, func(t *testing.T) {
			value, ok := ResolveLength(test.value, context)
			if !ok {
				t.Fatalf("ResolveLength(%q) was invalid", test.value)
			}
			if got := value.Resolve(context.PercentageBase); math.Abs(float64(got-test.want)) > 0.001 {
				t.Fatalf("ResolveLength(%q) = %v, want %v", test.value, got, test.want)
			}
		})
	}
}

func TestResolveLengthRejectsInvalidValues(t *testing.T) {
	for _, value := range []string{"", "1", "1unknown", "nanpx", "infinitypx"} {
		if _, ok := ResolveLength(value, LengthContext{}); ok {
			t.Fatalf("ResolveLength(%q) was valid", value)
		}
	}
}
