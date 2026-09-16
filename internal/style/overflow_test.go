package style

import (
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/css"
	"github.com/Grove-Computing/Growse/internal/dom"
)

func TestOverflowAxesSupportClipTwoValueShorthandAndComputedCoupling(t *testing.T) {
	t.Parallel()
	document := dom.NewDocument()
	clip := document.CreateElement("div", map[string]string{"class": "clip"})
	mixed := document.CreateElement("div", map[string]string{"class": "mixed"})
	clippedMixed := document.CreateElement("div", map[string]string{"class": "clipped-mixed"})
	for _, node := range []*dom.Node{clip, mixed, clippedMixed} {
		if err := document.AppendChild(document.Root, node); err != nil {
			t.Fatal(err)
		}
	}
	stylesheet, err := css.Parse(strings.NewReader(`
.clip { overflow:clip }
.mixed { overflow:visible scroll }
.clipped-mixed { overflow:clip auto }
`))
	if err != nil {
		t.Fatal(err)
	}
	computed := Compute(document, stylesheet)
	clipStyle, _ := computed.For(clip)
	mixedStyle, _ := computed.For(mixed)
	clippedMixedStyle, _ := computed.For(clippedMixed)
	if clipStyle.OverflowX != OverflowClip || clipStyle.OverflowY != OverflowClip {
		t.Fatalf("clip overflow = %v/%v", clipStyle.OverflowX, clipStyle.OverflowY)
	}
	if mixedStyle.OverflowX != OverflowAuto || mixedStyle.OverflowY != OverflowScroll {
		t.Fatalf("visible/scroll computed overflow = %v/%v", mixedStyle.OverflowX, mixedStyle.OverflowY)
	}
	if clippedMixedStyle.OverflowX != OverflowHidden || clippedMixedStyle.OverflowY != OverflowAuto {
		t.Fatalf("clip/auto computed overflow = %v/%v", clippedMixedStyle.OverflowX, clippedMixedStyle.OverflowY)
	}
	for _, value := range []string{"visible", "hidden", "clip", "auto", "scroll"} {
		if !supportsDeclaration("overflow", value) {
			t.Fatalf("@supports rejected overflow:%s", value)
		}
	}
}
