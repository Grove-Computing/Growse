package browser

import (
	"testing"

	stylemodel "github.com/Grove-Computing/Growse/internal/style"
)

func TestCSSOMExposesWritingModeAndOverflowAlignment(t *testing.T) {
	computed := stylemodel.ComputedStyle{
		WritingMode:          stylemodel.WritingModeVerticalRL,
		Direction:            stylemodel.DirectionRTL,
		JustifyContent:       stylemodel.JustifyEnd,
		JustifyContentSafety: stylemodel.OverflowAlignmentSafe,
		AlignItems:           stylemodel.AlignSelfStart,
		AlignItemsSafety:     stylemodel.OverflowAlignmentUnsafe,
		AlignSelf:            stylemodel.AlignAuto,
	}
	properties := cssomProperties(computed, 100, 50)
	want := map[string]string{
		"writing-mode": "vertical-rl", "direction": "rtl",
		"justify-content": "safe end", "align-items": "unsafe self-start", "align-self": "auto",
	}
	for property, expected := range want {
		if properties[property] != expected {
			t.Fatalf("%s = %q, want %q", property, properties[property], expected)
		}
	}
}
