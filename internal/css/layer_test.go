package css

import (
	"strings"
	"testing"
)

func TestParseCascadeLayersAndLayeredImports(t *testing.T) {
	stylesheet, err := Parse(strings.NewReader(`
@layer reset, components;
@import "theme.css" layer(framework) screen;
@import "anonymous.css" layer;
@layer components {
  @layer controls { button { color: green } }
}
@layer { p { color: blue } }
div { color: black }
`))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(stylesheet.Imports), 2; got != want {
		t.Fatalf("imports = %d, want %d", got, want)
	}
	if imported := stylesheet.Imports[0]; !imported.Layered || imported.Layer != "framework" || len(imported.Media) != 1 {
		t.Fatalf("named layered import = %#v", imported)
	}
	if imported := stylesheet.Imports[1]; !imported.Layered || imported.Layer == "" {
		t.Fatalf("anonymous layered import = %#v", imported)
	}
	if got, want := stylesheet.Rules[0].Layer, "components.controls"; got != want {
		t.Fatalf("nested layer = %q, want %q", got, want)
	}
	if stylesheet.Rules[1].Layer == "" || stylesheet.Rules[2].Layer != "" {
		t.Fatalf("anonymous/unlayered rules = %q / %q", stylesheet.Rules[1].Layer, stylesheet.Rules[2].Layer)
	}
	for _, want := range []string{"reset", "components", "framework", "components.controls"} {
		found := false
		for _, layer := range stylesheet.LayerOrder {
			found = found || layer == want
		}
		if !found {
			t.Fatalf("layer order %v does not contain %q", stylesheet.LayerOrder, want)
		}
	}
}

func TestNestUnderLayerPreservesImportedLayerHierarchy(t *testing.T) {
	stylesheet, err := Parse(strings.NewReader(`@layer tokens { p { color: red } } div { color: blue }`))
	if err != nil {
		t.Fatal(err)
	}
	stylesheet.NestUnderLayer("framework")
	if got, want := stylesheet.LayerOrder, []string{"framework", "framework.tokens"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("layer order = %v, want %v", got, want)
	}
	if stylesheet.Rules[0].Layer != "framework.tokens" || stylesheet.Rules[1].Layer != "framework" {
		t.Fatalf("nested imported rules = %q / %q", stylesheet.Rules[0].Layer, stylesheet.Rules[1].Layer)
	}
}
