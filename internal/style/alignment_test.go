package style

import (
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/css"
	"github.com/Grove-Computing/Growse/internal/dom"
)

func TestComputePreservesLogicalAndOverflowAlignment(t *testing.T) {
	document := dom.NewDocument()
	container := document.CreateElement("div", map[string]string{"class": "container"})
	item := document.CreateElement("div", map[string]string{"class": "item"})
	if err := document.AppendChild(document.Root, container); err != nil {
		t.Fatal(err)
	}
	if err := document.AppendChild(container, item); err != nil {
		t.Fatal(err)
	}
	stylesheet, err := css.Parse(strings.NewReader(`
.container {
  writing-mode:vertical-rl;
  direction:rtl;
  display:flex;
  justify-content:safe end;
  align-items:unsafe start;
  align-content:space-evenly;
}
.item { align-self:safe self-end; justify-self:unsafe center }
`))
	if err != nil {
		t.Fatal(err)
	}
	styles := Compute(document, stylesheet)
	containerStyle, _ := styles.For(container)
	itemStyle, _ := styles.For(item)
	if containerStyle.WritingMode != WritingModeVerticalRL || containerStyle.Direction != DirectionRTL {
		t.Fatalf("writing mode/direction = %v/%v", containerStyle.WritingMode, containerStyle.Direction)
	}
	if containerStyle.JustifyContent != JustifyEnd || containerStyle.JustifyContentSafety != OverflowAlignmentSafe {
		t.Fatalf("justify-content = %v safety %v", containerStyle.JustifyContent, containerStyle.JustifyContentSafety)
	}
	if containerStyle.AlignItems != AlignStart || containerStyle.AlignItemsSafety != OverflowAlignmentUnsafe || containerStyle.AlignContent != AlignSpaceEvenly {
		t.Fatalf("container alignment = items %v/%v content %v", containerStyle.AlignItems, containerStyle.AlignItemsSafety, containerStyle.AlignContent)
	}
	if itemStyle.AlignSelf != AlignSelfEnd || itemStyle.AlignSelfSafety != OverflowAlignmentSafe || itemStyle.JustifySelf != AlignCenter || itemStyle.JustifySelfSafety != OverflowAlignmentUnsafe {
		t.Fatalf("self alignment = align %v/%v justify %v/%v", itemStyle.AlignSelf, itemStyle.AlignSelfSafety, itemStyle.JustifySelf, itemStyle.JustifySelfSafety)
	}
}
