package strconv

import "testing"

func TestItoa(t *testing.T) {
	for _, test := range []struct {
		value int
		want  string
	}{
		{value: 0, want: "0"},
		{value: 42, want: "42"},
		{value: -7, want: "-7"},
	} {
		if got := Itoa(test.value); got != test.want {
			t.Fatalf("Itoa(%d) = %q, want %q", test.value, got, test.want)
		}
	}
}
