package css

import (
	"net/url"
	"strings"
	"testing"
)

func TestResolveResourceURLsUsesStylesheetBase(t *testing.T) {
	stylesheet, err := Parse(strings.NewReader(`.card { background-image: url('../images/card.png'); }`))
	if err != nil {
		t.Fatal(err)
	}
	base, _ := url.Parse("https://example.com/assets/css/main.css")
	ResolveResourceURLs(stylesheet, base)
	if got, want := stylesheet.Rules[0].Declarations[0].Value.Raw, `url("https://example.com/assets/images/card.png")`; got != want {
		t.Fatalf("background URL = %q, want %q", got, want)
	}
}
