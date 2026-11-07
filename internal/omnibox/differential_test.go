package omnibox

import (
	"net/url"
	"strings"
	"testing"

	"golang.org/x/net/idna"
)

func TestURLDifferentialMatchesStandardAndFixedBrowserReference(t *testing.T) {
	// These identities are shared by the Chromium and Firefox reference runs
	// recorded for v0.21.0. net/url and x/net/idna independently guard the
	// parsing, escaping, and host conversion inputs used by NormalizeURL.
	for _, test := range []struct {
		name, raw, host, query, want string
	}{
		{
			name: "IDNA escaped path and fragment",
			raw:  "https://BÜCHER.example:443/a%2Fb/?q=caf%C3%A9#result",
			host: "xn--bcher-kva.example", query: "q=caf%C3%A9",
			want: "https://xn--bcher-kva.example/a%2Fb?q=caf%C3%A9",
		},
		{
			name: "Japanese IDNA and encoded query",
			raw:  "https://例え.テスト/search/?q=%E6%97%A5%E6%9C%AC%E8%AA%9E+%2B+browser",
			host: "xn--r8jz45g.xn--zckzah", query: "q=%E6%97%A5%E6%9C%AC%E8%AA%9E+%2B+browser",
			want: "https://xn--r8jz45g.xn--zckzah/search?q=%E6%97%A5%E6%9C%AC%E8%AA%9E+%2B+browser",
		},
		{
			name: "IPv6 and default port",
			raw:  "http://[0:0:0:0:0:0:0:1]:80/guide/",
			host: "0:0:0:0:0:0:0:1", query: "",
			want: "http://[::1]/guide",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			parsed, err := url.Parse(test.raw)
			if err != nil {
				t.Fatal(err)
			}
			if parsed.Hostname() != test.host {
				ascii, convertErr := idna.Lookup.ToASCII(strings.ToLower(parsed.Hostname()))
				if convertErr != nil || ascii != test.host {
					t.Fatalf("standard host = %q, IDNA = %q, %v; want %q", parsed.Hostname(), ascii, convertErr, test.host)
				}
			}
			if parsed.RawQuery != test.query {
				t.Fatalf("net/url query = %q, want %q", parsed.RawQuery, test.query)
			}
			got, err := NormalizeURL(test.raw)
			if err != nil || got != test.want {
				t.Fatalf("NormalizeURL() = %q, %v; browser reference wants %q", got, err, test.want)
			}
		})
	}
}
