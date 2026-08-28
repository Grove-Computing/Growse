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

func TestResolveLengthEvaluatesDimensionSafeCalc(t *testing.T) {
	context := LengthContext{
		FontSize: 20, RootFontSize: 16, ViewportWidth: 1000, ViewportHeight: 600, PercentageBase: 400,
	}
	tests := []struct {
		value string
		want  float32
	}{
		{"calc(100% - 2rem)", 368},
		{"calc((2em + 1in) / 2)", 68},
		{"calc(2 * 3px)", 6},
		{"calc(3px * 2 + 4px / 2)", 8},
		{"calc(-(1em - 5px))", -15},
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

func TestResolveLengthRejectsInvalidCalcDimensions(t *testing.T) {
	context := LengthContext{FontSize: 16, RootFontSize: 16, ViewportWidth: 800, ViewportHeight: 600}
	for _, value := range []string{
		"calc(1px + 1)", "calc(1px * 1px)", "calc(1px / 0)",
		"calc(1 / 1px)", "calc(1px +)", "calc(1e38px * 1e38)",
	} {
		if _, ok := ResolveLength(value, context); ok {
			t.Fatalf("ResolveLength(%q) was valid", value)
		}
	}
}

func TestResolveLengthSupportsMathFunctionsAndDynamicViewportUnits(t *testing.T) {
	context := LengthContext{
		FontSize: 16, RootFontSize: 16, ViewportWidth: 1000, ViewportHeight: 600,
		PercentageBase: 400, ContainerWidth: 300, ContainerHeight: 200,
	}
	tests := []struct {
		value string
		want  float32
	}{
		{"min(50%, 250px)", 200},
		{"max(10cqw, calc(20px + 5cqw))", 35},
		{"clamp(100px, calc(50% + 10px), 180px)", 180},
		{"calc(min(20dvw, 250px) - max(2svw, 10px))", 180},
		{"10lvh", 60},
	}
	for _, test := range tests {
		resolved, ok := ResolveLength(test.value, context)
		if !ok || math.Abs(float64(resolved.Resolve(context.PercentageBase)-test.want)) > 0.001 {
			t.Errorf("ResolveLength(%q) = %#v, %t; want %v", test.value, resolved, ok, test.want)
		}
	}
	for _, invalid := range []string{"min(1px, 2)", "clamp(1px, 2px)", "max(1px, nanpx)", "clamp(1px, calc(1px / 0), 2px)"} {
		if _, ok := ResolveLength(invalid, context); ok {
			t.Errorf("ResolveLength(%q) was valid", invalid)
		}
	}
}
