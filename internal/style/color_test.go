package style

import "testing"

func TestParseColorSupportsCSSColorLevel3(t *testing.T) {
	tests := []struct {
		value string
		want  uint32
	}{
		{"aliceblue", 0xf0f8ffff},
		{"transparent", 0x00000000},
		{"#0f8", 0x00ff88ff},
		{"#0f08", 0x00ff0088},
		{"#112233", 0x112233ff},
		{"#11223344", 0x11223344},
		{"rgb(255, 128, 0)", 0xff8000ff},
		{"rgb(100%, 50%, 0%)", 0xff8000ff},
		{"rgba(255, 0, 0, 0.5)", 0xff000080},
		{"hsl(120, 100%, 50%)", 0x00ff00ff},
		{"hsla(240, 100%, 50%, 0.25)", 0x0000ff40},
		{"hsl(-120, 100%, 50%)", 0x0000ffff},
		{"rgb(300, -10, 0)", 0xff0000ff},
	}
	for _, test := range tests {
		t.Run(test.value, func(t *testing.T) {
			got, ok := parseColor(test.value, 0x12345678)
			if !ok || got != test.want {
				t.Fatalf("parseColor(%q) = (%#x, %v), want (%#x, true)", test.value, got, ok, test.want)
			}
		})
	}
	if got, ok := parseColor("currentColor", 0x12345678); !ok || got != 0x12345678 {
		t.Fatalf("currentColor = (%#x, %v)", got, ok)
	}
}

func TestParseColorRejectsInvalidValues(t *testing.T) {
	for _, value := range []string{
		"rebeccapurple", "#12", "#ggg", "rgb(1, 2)", "rgb(10%, 2, 3)",
		"rgba(1, 2, 3, 50%)", "hsl(10, 20, 30%)", "hsl(nan, 20%, 30%)",
	} {
		if _, ok := parseColor(value, 0); ok {
			t.Fatalf("parseColor(%q) was valid", value)
		}
	}
}
