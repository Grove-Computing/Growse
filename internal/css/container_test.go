package css

import (
	"strings"
	"testing"
)

func TestParseNamedInlineSizeContainerQueries(t *testing.T) {
	stylesheet, err := Parse(strings.NewReader(`
@container card (min-width: 300px) and (max-width: 500px) { .title { color: green } }
@container (style(--theme: dark)) { .ignored { color: red } }
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(stylesheet.Rules) != 2 || len(stylesheet.Rules[0].Containers) != 1 {
		t.Fatalf("container rules = %#v", stylesheet.Rules)
	}
	query := stylesheet.Rules[0].Containers[0]
	if !query.Valid || query.Name != "card" || len(query.Features) != 2 || query.Features[0].Name != "min-width" || query.Features[1].Name != "max-width" {
		t.Fatalf("named query = %#v", query)
	}
	if stylesheet.Rules[1].Containers[0].Valid {
		t.Fatalf("style query was accepted: %#v", stylesheet.Rules[1].Containers[0])
	}
}
