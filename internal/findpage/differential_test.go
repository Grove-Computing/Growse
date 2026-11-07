package findpage

import (
	"strings"
	"testing"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"

	"github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/style"
)

func TestUnicodeDifferentialMatchesStandardAndFixedBrowserReference(t *testing.T) {
	for _, test := range []struct {
		name, source, query, matched string
	}{
		{name: "combining sequence", source: "Start CAFE\u0301 end", query: "café", matched: "CAFE\u0301"},
		{name: "case fold expansion", source: "Die Straße endet", query: "STRASSE", matched: "Straße"},
		{name: "CJK substring", source: "日本語の検索対象です", query: "検索対象", matched: "検索対象"},
	} {
		t.Run(test.name, func(t *testing.T) {
			standardSource := cases.Fold().String(norm.NFC.String(test.source))
			standardQuery := cases.Fold().String(norm.NFC.String(test.query))
			if !strings.Contains(standardSource, standardQuery) {
				t.Fatalf("standard Unicode normalization did not match %q in %q", test.query, test.source)
			}

			document := dom.NewDocument()
			body := document.CreateElement("body", nil)
			text := document.CreateText(test.source)
			if err := document.AppendChild(document.Root, body); err != nil {
				t.Fatal(err)
			}
			if err := document.AppendChild(body, text); err != nil {
				t.Fatal(err)
			}
			result := Search(document, style.Compute(document, nil), test.query, Options{}, Limits{})
			if len(result.Matches) != 1 {
				t.Fatalf("matches = %+v, fixed browser reference wants one match", result.Matches)
			}
			match := result.Matches[0]
			if got := test.source[match.Start:match.End]; got != test.matched {
				t.Fatalf("matched source = %q, fixed browser reference wants %q", got, test.matched)
			}
		})
	}
}
