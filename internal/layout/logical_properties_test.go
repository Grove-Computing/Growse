package layout

import (
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/css"
	"github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/style"
)

func TestLogicalPropertiesDriveHorizontalLayoutGeometry(t *testing.T) {
	document := dom.NewDocument()
	box := document.CreateElement("div", map[string]string{"class": "box"})
	if err := document.AppendChild(document.Root, box); err != nil {
		t.Fatal(err)
	}
	stylesheet, err := css.Parse(strings.NewReader(`.box { inline-size:100px; block-size:40px; padding-inline:10px; border-inline:2px solid red; margin-inline-start:15px }`))
	if err != nil {
		t.Fatal(err)
	}
	tree := Build(document, style.Compute(document, stylesheet), 500)
	bounds, ok := tree.Bounds[box.ID]
	if !ok || bounds.Width != 124 || bounds.Height != 40 || bounds.X != pagePadding+15 {
		t.Fatalf("logical box bounds = %#v, found %t", bounds, ok)
	}
}
