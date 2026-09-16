package style

import (
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/css"
	"github.com/Grove-Computing/Growse/internal/dom"
)

func TestComputeResolvesTableFormattingPropertiesAndInternalDisplays(t *testing.T) {
	document := dom.NewDocument()
	table := document.CreateElement("table", map[string]string{"class": "matrix"})
	caption := document.CreateElement("caption", nil)
	group := document.CreateElement("colgroup", nil)
	column := document.CreateElement("col", nil)
	row := document.CreateElement("tr", nil)
	cell := document.CreateElement("td", nil)
	for _, edge := range [][2]*dom.Node{
		{document.Root, table}, {table, caption}, {table, group}, {group, column}, {table, row}, {row, cell},
	} {
		if err := document.AppendChild(edge[0], edge[1]); err != nil {
			t.Fatal(err)
		}
	}
	stylesheet, err := css.Parse(strings.NewReader(`
.matrix { table-layout:fixed; border-collapse:collapse; border-spacing:5px 7px; caption-side:bottom }
`))
	if err != nil {
		t.Fatal(err)
	}
	computed := ComputeWithEnvironment(document, stylesheet, InteractionState{}, Environment{BrowserDefaults: true})
	tableStyle, _ := computed.For(table)
	captionStyle, _ := computed.For(caption)
	groupStyle, _ := computed.For(group)
	columnStyle, _ := computed.For(column)
	if tableStyle.TableLayout != TableLayoutFixed || tableStyle.BorderCollapse != BorderCollapseCollapse ||
		tableStyle.BorderSpacingX != 5 || tableStyle.BorderSpacingY != 7 || tableStyle.CaptionSide != CaptionSideBottom {
		t.Fatalf("table properties = %#v", tableStyle)
	}
	if captionStyle.Display != DisplayTableCaption || groupStyle.Display != DisplayTableColumnGroup || columnStyle.Display != DisplayTableColumn {
		t.Fatalf("internal table displays = caption:%v group:%v column:%v", captionStyle.Display, groupStyle.Display, columnStyle.Display)
	}
	if captionStyle.CaptionSide != CaptionSideBottom || captionStyle.BorderCollapse != BorderCollapseCollapse {
		t.Fatalf("inherited table properties = %#v", captionStyle)
	}
}

func TestSupportsTableSizingDeclarations(t *testing.T) {
	for _, declaration := range [][2]string{
		{"table-layout", "fixed"}, {"border-collapse", "collapse"}, {"border-spacing", "4px 8px"},
		{"caption-side", "bottom"}, {"aspect-ratio", "auto 16 / 9"},
	} {
		if !supportsDeclaration(declaration[0], declaration[1]) {
			t.Fatalf("@supports rejected %s:%s", declaration[0], declaration[1])
		}
	}
	for _, declaration := range [][2]string{{"border-spacing", "10%"}, {"aspect-ratio", "1 / 0"}, {"table-layout", "dense"}} {
		if supportsDeclaration(declaration[0], declaration[1]) {
			t.Fatalf("@supports accepted invalid %s:%s", declaration[0], declaration[1])
		}
	}
}
