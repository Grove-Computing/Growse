package browser

import (
	"context"
	"strings"
	"testing"

	"github.com/saku0512/growse/internal/html"
	"github.com/saku0512/growse/internal/network"
)

func TestNavigateCollectsInlineAndExternalGoScripts(t *testing.T) {
	pageURL := mustParseURL(t, "https://example.com/index.html")
	externalURL := mustParseURL(t, "https://cdn.example.org/app.go")
	missingURL := mustParseURL(t, "https://example.com/missing.go")
	loader := &routeLoader{responses: map[string]*network.Response{
		pageURL.String(): {
			URL: pageURL, StatusCode: 200, ContentType: "text/html",
			Body: []byte(`<html><body>
<script>console.log("ignored")</script>
<script type="text/go">package main
func inline() {}</script>
<script type="text/go" src="https://cdn.example.org/app.go">ignored inline fallback</script>
<script type="text/go" src="/missing.go"></script>
</body></html>`),
		},
		externalURL.String(): {
			URL: externalURL, StatusCode: 200, ContentType: "text/go; charset=utf-8",
			Body: []byte("package main\nfunc external() {}"),
		},
	}}

	page, err := New(loader).Navigate(context.Background(), pageURL.String())
	if err != nil {
		t.Fatalf("Navigate() error = %v", err)
	}
	if got, want := len(page.Scripts), 2; got != want {
		t.Fatalf("script count = %d, want %d", got, want)
	}
	if !page.Scripts[0].Inline || !strings.Contains(page.Scripts[0].Source, "func inline") {
		t.Fatalf("first script = %#v, want inline Go source", page.Scripts[0])
	}
	if page.Scripts[1].Inline || page.Scripts[1].SourceURL.String() != externalURL.String() ||
		!strings.Contains(page.Scripts[1].Source, "func external") {
		t.Fatalf("second script = %#v, want external Go source", page.Scripts[1])
	}
	if got, want := len(page.ScriptErrors), 1; got != want {
		t.Fatalf("script error count = %d, want %d (%v)", got, want, page.ScriptErrors)
	}
	if !strings.Contains(page.ScriptErrors[0], missingURL.String()) {
		t.Fatalf("script error = %q, want missing URL", page.ScriptErrors[0])
	}
}

func TestCollectScriptsPreservesDocumentOrderAndExternalPriority(t *testing.T) {
	document, err := html.Parse(strings.NewReader(`
<script type="text/go">first</script>
<script type="text/javascript" src="ignored.js"></script>
<script type="TEXT/GO" src="app.go">fallback</script>
`))
	if err != nil {
		t.Fatal(err)
	}

	sources := collectScripts(document.Root)
	if got, want := len(sources), 2; got != want {
		t.Fatalf("source count = %d, want %d", got, want)
	}
	if !sources[0].inline || strings.TrimSpace(sources[0].source) != "first" {
		t.Fatalf("first source = %#v, want inline", sources[0])
	}
	if sources[1].inline || sources[1].src != "app.go" || sources[1].source != "" {
		t.Fatalf("second source = %#v, want external source only", sources[1])
	}
}

func TestIsTrustedOriginAllowsOnlyLoopbackHosts(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{url: "http://localhost:8080", want: true},
		{url: "https://127.0.0.1/app", want: true},
		{url: "http://[::1]:8080", want: true},
		{url: "https://localhost.example.com", want: false},
		{url: "file:///tmp/app.go", want: false},
	}
	for _, test := range tests {
		t.Run(test.url, func(t *testing.T) {
			if got := IsTrustedOrigin(mustParseURL(t, test.url)); got != test.want {
				t.Fatalf("IsTrustedOrigin(%q) = %v, want %v", test.url, got, test.want)
			}
		})
	}
}
