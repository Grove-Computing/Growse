package style

import (
	"testing"

	"github.com/Grove-Computing/Growse/internal/dom"
)

func TestColumnPropertiesResolveShorthandsAndFragmentationControls(t *testing.T) {
	document := dom.NewDocument()
	container := document.CreateElement("section", map[string]string{"style": `
		columns: 180px 3;
		column-gap: 24px;
		column-rule: 2px dashed #123456;
		column-fill: auto;
		widows: 3;
		orphans: 4;
	`})
	spanner := document.CreateElement("h2", map[string]string{"style": "column-span:all; break-before:column; break-after:avoid-column; break-inside:avoid"})
	appendNode(t, document, document.Root, container)
	appendNode(t, document, container, spanner)

	computed := Compute(document, nil)
	containerStyle, ok := computed.For(container)
	if !ok {
		t.Fatal("container has no computed style")
	}
	if containerStyle.ColumnCount != 3 || containerStyle.ColumnWidth.Kind != SizeLength || containerStyle.ColumnWidth.Value.Pixels != 180 {
		t.Fatalf("column shorthand = count:%d width:%#v", containerStyle.ColumnCount, containerStyle.ColumnWidth)
	}
	if containerStyle.ColumnGapNormal || containerStyle.ColumnGap.Pixels != 24 {
		t.Fatalf("column gap = normal:%v value:%#v", containerStyle.ColumnGapNormal, containerStyle.ColumnGap)
	}
	if containerStyle.ColumnRule.Width != 2 || containerStyle.ColumnRule.Style != BorderDashed || containerStyle.ColumnRule.Color != 0x123456ff {
		t.Fatalf("column rule = %#v", containerStyle.ColumnRule)
	}
	if containerStyle.ColumnFill != ColumnFillAuto || containerStyle.Widows != 3 || containerStyle.Orphans != 4 {
		t.Fatalf("fragment settings = fill:%v widows:%d orphans:%d", containerStyle.ColumnFill, containerStyle.Widows, containerStyle.Orphans)
	}

	spanStyle, ok := computed.For(spanner)
	if !ok || spanStyle.ColumnSpan != ColumnSpanAll || spanStyle.BreakBefore != FragmentBreakColumn ||
		spanStyle.BreakAfter != FragmentBreakAvoidColumn || spanStyle.BreakInside != FragmentBreakAvoid {
		t.Fatalf("spanner fragmentation style = %#v", spanStyle)
	}
}

func TestColumnDefaultsAndSupportsDeclarations(t *testing.T) {
	document := dom.NewDocument()
	container := document.CreateElement("div", nil)
	appendNode(t, document, document.Root, container)
	computed, _ := Compute(document, nil).For(container)
	if computed.ColumnCount != 0 || computed.ColumnWidth.Kind != SizeAuto || !computed.ColumnGapNormal ||
		computed.ColumnFill != ColumnFillBalance || computed.Widows != 2 || computed.Orphans != 2 {
		t.Fatalf("column defaults = %#v", computed)
	}

	for _, declaration := range [][2]string{
		{"columns", "12rem 3"}, {"column-rule", "thin solid red"}, {"column-span", "all"},
		{"column-fill", "balance"}, {"break-before", "column"}, {"break-inside", "avoid-column"}, {"widows", "3"},
	} {
		if !supportsDeclaration(declaration[0], declaration[1]) {
			t.Errorf("supportsDeclaration(%q, %q) = false", declaration[0], declaration[1])
		}
	}
	for _, declaration := range [][2]string{{"column-count", "0"}, {"column-width", "-1px"}, {"column-span", "page"}, {"break-inside", "column"}} {
		if supportsDeclaration(declaration[0], declaration[1]) {
			t.Errorf("supportsDeclaration(%q, %q) = true", declaration[0], declaration[1])
		}
	}
}
