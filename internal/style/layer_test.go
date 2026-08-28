package style

import (
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/css"
	"github.com/Grove-Computing/Growse/internal/dom"
)

func TestCascadeLayerOrderImportantReversalAndRevertLayer(t *testing.T) {
	document := dom.NewDocument()
	target := document.CreateElement("div", map[string]string{"class": "target"})
	appendNode(t, document, document.Root, target)
	stylesheet, err := css.Parse(strings.NewReader(`
@layer reset, theme;
@layer reset {
  .target { color: red !important; background-color: red; font-size: 12px }
}
@layer theme {
  .target { color: green !important; background-color: green; font-size: 20px }
  .target { font-size: revert-layer }
}
.target { color: blue !important; background-color: blue }
`))
	if err != nil {
		t.Fatal(err)
	}
	computed, ok := Compute(document, stylesheet).For(target)
	if !ok {
		t.Fatal("target has no computed style")
	}
	if computed.Color != 0xff0000ff {
		t.Fatalf("important layer reversal color = %#08x, want red", computed.Color)
	}
	if computed.BackgroundColor != 0x0000ffff {
		t.Fatalf("unlayered normal background = %#08x, want blue", computed.BackgroundColor)
	}
	if computed.FontSize != 12 {
		t.Fatalf("revert-layer font size = %v, want 12", computed.FontSize)
	}
}

func TestInlineStyleOutranksLayeredAuthorDeclarations(t *testing.T) {
	document := dom.NewDocument()
	target := document.CreateElement("div", map[string]string{
		"class": "target", "style": "color: blue; background-color: blue !important",
	})
	appendNode(t, document, document.Root, target)
	stylesheet, err := css.Parse(strings.NewReader(`@layer theme { .target { color: red; background-color: red !important } }`))
	if err != nil {
		t.Fatal(err)
	}
	computed, _ := Compute(document, stylesheet).For(target)
	if computed.Color != 0x0000ffff || computed.BackgroundColor != 0x0000ffff {
		t.Fatalf("inline cascade = color:%#08x background:%#08x", computed.Color, computed.BackgroundColor)
	}
}
