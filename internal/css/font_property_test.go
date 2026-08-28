package css

import (
	"net/url"
	"strings"
	"testing"
)

func TestParseFontFaceAndRegisteredPropertyDescriptors(t *testing.T) {
	stylesheet, err := Parse(strings.NewReader(`
@font-face {
  font-family: "Inter Fixture";
  src: url("inter.woff2") format("woff2");
  font-style: italic;
  font-weight: 700;
  font-stretch: condensed;
  unicode-range: U+0000-00FF;
  font-display: swap;
}
@property --card-gap {
  syntax: "<length>";
  initial-value: 12px;
  inherits: false;
}
@property --invalid { syntax: "<future>"; inherits: true; }
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(stylesheet.FontFaces) != 1 {
		t.Fatalf("font faces = %#v", stylesheet.FontFaces)
	}
	face := stylesheet.FontFaces[0]
	if face.Family != "Inter Fixture" || face.Style != "italic" || face.Weight != "700" || face.Stretch != "condensed" || face.Display != "swap" {
		t.Fatalf("font face = %#v", face)
	}
	if len(stylesheet.Properties) != 2 || !stylesheet.Properties[0].Valid || stylesheet.Properties[0].Name != "--card-gap" || stylesheet.Properties[0].InitialValue != "12px" || stylesheet.Properties[0].Inherits {
		t.Fatalf("registered properties = %#v", stylesheet.Properties)
	}
	if stylesheet.Properties[1].Valid {
		t.Fatalf("unknown syntax was accepted: %#v", stylesheet.Properties[1])
	}
}

func TestResolveResourceURLsResolvesFontFaceCandidates(t *testing.T) {
	stylesheet, err := Parse(strings.NewReader(`@font-face {
		font-family: Fixture;
		src: local("Fixture"), url("../fonts/a.woff2") format("woff2"), url(a.woff) format("woff");
	}`))
	if err != nil {
		t.Fatal(err)
	}
	baseURL, err := url.Parse("https://example.com/css/app.css")
	if err != nil {
		t.Fatal(err)
	}
	ResolveResourceURLs(stylesheet, baseURL)
	want := `local("Fixture"),url("https://example.com/fonts/a.woff2") format("woff2"),url("https://example.com/css/a.woff") format("woff")`
	if got := stylesheet.FontFaces[0].Source; got != want {
		t.Fatalf("font src = %q, want %q", got, want)
	}
}
