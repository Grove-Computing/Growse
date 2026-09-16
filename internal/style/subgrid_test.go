package style

import (
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/css"
	"github.com/Grove-Computing/Growse/internal/dom"
)

func TestComputeRetainsSubgridAxesAndLocalLineNames(t *testing.T) {
	document := dom.NewDocument()
	grid := document.CreateElement("div", map[string]string{"class": "subgrid"})
	if err := document.AppendChild(document.Root, grid); err != nil {
		t.Fatal(err)
	}
	stylesheet, err := css.Parse(strings.NewReader(`
.subgrid {
  display:grid;
  grid-template-columns:subgrid [local-start] [local-end];
  grid-template-rows:subgrid;
}

func TestSupportsRecognizesOnlyValidSubgridTrackLists(t *testing.T) {
	if !supportsDeclaration("grid-template-columns", "subgrid [content-start] [content-end]") {
		t.Fatal("valid subgrid declaration was not supported")
	}
	if supportsDeclaration("grid-template-rows", "subgrid 10px") {
		t.Fatal("subgrid accepted an independent track size")
	}
}
`))
	if err != nil {
		t.Fatal(err)
	}
	computed, ok := Compute(document, stylesheet).For(grid)
	if !ok {
		t.Fatal("subgrid style missing")
	}
	if !computed.GridColumnsSubgrid || !computed.GridRowsSubgrid {
		t.Fatalf("subgrid axes = columns %v rows %v", computed.GridColumnsSubgrid, computed.GridRowsSubgrid)
	}
	if len(computed.GridTemplateColumns) != 0 || len(computed.GridTemplateRows) != 0 {
		t.Fatalf("subgrid created independent tracks: columns %#v rows %#v", computed.GridTemplateColumns, computed.GridTemplateRows)
	}
	if got := computed.GridColumnLines["local-start"]; len(got) != 1 || got[0] != 0 {
		t.Fatalf("local-start lines = %#v", got)
	}
	if got := computed.GridColumnLines["local-end"]; len(got) != 1 || got[0] != 1 {
		t.Fatalf("local-end lines = %#v", got)
	}
}
