package style

import (
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/css"
	"github.com/Grove-Computing/Growse/internal/dom"
)

func TestFontFaceSelectionAndRegisteredCustomPropertySemantics(t *testing.T) {
	document := dom.NewDocument()
	parent := document.CreateElement("section", map[string]string{"class": "parent"})
	child := document.CreateElement("div", map[string]string{"class": "child"})
	appendNode(t, document, document.Root, parent)
	appendNode(t, document, parent, child)
	stylesheet, err := css.Parse(strings.NewReader(`
@font-face { font-family: "Fixture"; src: url(normal.woff2); font-style: normal; font-weight: 400; }
@font-face { font-family: "Fixture"; src: url(bold.woff2); font-style: italic; font-weight: 700; }
@property --gap { syntax: "<length>"; initial-value: 10px; inherits: false; }
@property --tone { syntax: "<color>"; initial-value: blue; inherits: true; }
.parent { --gap: 30px; --tone: green; }
.child { font-family: "Fixture", sans-serif; font-style: italic; font-weight: 700; width: var(--gap); color: var(--tone); --gap: invalid; }
`))
	if err != nil {
		t.Fatal(err)
	}
	computed := Compute(document, stylesheet)
	parentStyle, _ := computed.For(parent)
	childStyle, _ := computed.For(child)
	if parentStyle.CustomProperties["--gap"] != "30px" || childStyle.CustomProperties["--gap"] != "10px" {
		t.Fatalf("non-inherited registered property = parent:%v child:%v", parentStyle.CustomProperties, childStyle.CustomProperties)
	}
	if childStyle.CustomProperties["--tone"] != "green" || childStyle.Color != 0x008000ff {
		t.Fatalf("inherited registered color = %#v / %#08x", childStyle.CustomProperties, childStyle.Color)
	}
	if childStyle.Width.Value.Pixels != 10 || childStyle.FontFaceIndex != 1 {
		t.Fatalf("typed width/font selection = width:%#v face:%d", childStyle.Width, childStyle.FontFaceIndex)
	}
}
