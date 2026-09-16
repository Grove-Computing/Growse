package browser

import (
	"testing"

	stylemodel "github.com/Grove-Computing/Growse/internal/style"
)

func TestCSSOMExposesTableAndAspectRatioComputedValues(t *testing.T) {
	computed := stylemodel.ComputedStyle{
		Display:        stylemodel.DisplayTableCaption,
		TableLayout:    stylemodel.TableLayoutFixed,
		BorderCollapse: stylemodel.BorderCollapseCollapse,
		BorderSpacingX: 4,
		BorderSpacingY: 8,
		CaptionSide:    stylemodel.CaptionSideBottom,
		AspectRatio:    1.5,
		BoxSizing:      stylemodel.BoxSizingBorderBox,
	}
	properties := cssomProperties(computed, 120, 80)
	want := map[string]string{
		"display": "table-caption", "table-layout": "fixed", "border-collapse": "collapse",
		"border-spacing": "4px 8px", "caption-side": "bottom", "aspect-ratio": "1.5 / 1",
	}
	for property, expected := range want {
		if properties[property] != expected {
			t.Fatalf("%s = %q, want %q", property, properties[property], expected)
		}
	}
}
