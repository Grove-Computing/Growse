package style

import "testing"

func TestParseColorLevel4FunctionsToSRGB(t *testing.T) {
	tests := []struct {
		value string
		want  uint32
	}{
		{"rgb(255 0 128 / 50%)", 0xff008080},
		{"hsl(120 100% 50% / .25)", 0x00ff0040},
		{"hwb(0 0% 0%)", 0xff0000ff},
		{"lab(100 0 0)", 0xffffffff},
		{"lch(100 0 40deg)", 0xffffffff},
		{"oklab(100% 0 0)", 0xffffffff},
		{"oklch(100% 0 20deg)", 0xffffffff},
		{"color-mix(in srgb, red, blue)", 0x800080ff},
		{"color-mix(in srgb, rgb(255 0 0) 25%, blue 75%)", 0x4000bfff},
	}
	for _, test := range tests {
		got, ok := parseColor(test.value, 0x12345678)
		if !ok || got != test.want {
			t.Errorf("parseColor(%q) = %#08x, %t; want %#08x", test.value, got, ok, test.want)
		}
	}
}

func TestParseColorLevel4RejectsNonFiniteAndUnknownSpaces(t *testing.T) {
	for _, value := range []string{
		"lab(nan 0 0)", "oklch(50% infinity 0)", "color(display-p3 1 0 0)",
		"color-mix(in future, red, blue)", "color-mix(in srgb, unknown(1), blue)",
	} {
		if _, ok := parseColor(value, 0); ok {
			t.Errorf("parseColor(%q) was valid", value)
		}
	}
}
