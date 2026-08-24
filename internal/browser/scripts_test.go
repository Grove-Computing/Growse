package browser

import (
	"context"
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/html"
	"github.com/Grove-Computing/Growse/internal/network"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
)

func TestNavigateCollectsInlineAndExternalGoScripts(t *testing.T) {
	pageURL := mustParseURL(t, "http://localhost:8080/index.html")
	externalURL := mustParseURL(t, "http://localhost:8080/app.go")
	missingURL := mustParseURL(t, "http://localhost:8080/missing.go")
	loader := &routeLoader{responses: map[string]*network.Response{
		pageURL.String(): {
			URL: pageURL, StatusCode: 200, ContentType: "text/html",
			Body: []byte(`<html><body>
<script>console.log("ignored")</script>
<script type="text/go">package main
func inline() {}</script>
<script type="text/go" src="/app.go">ignored inline fallback</script>
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

func TestLoadScriptsBlocksCrossOriginAndRedirectedSources(t *testing.T) {
	pageURL := mustParseURL(t, "http://localhost:8080/index.html")
	crossOriginURL := mustParseURL(t, "http://localhost:9090/cross.go")
	redirectURL := mustParseURL(t, "http://localhost:8080/redirect.go")
	redirectedFinalURL := mustParseURL(t, "http://127.0.0.1:8080/final.go")
	document, err := html.Parse(strings.NewReader(`
<script type="text/go" src="http://localhost:9090/cross.go"></script>
<script type="text/go" src="/redirect.go"></script>`))
	if err != nil {
		t.Fatal(err)
	}
	loader := &routeLoader{responses: map[string]*network.Response{
		crossOriginURL.String(): {URL: crossOriginURL, StatusCode: 200, ContentType: "text/go", Body: []byte("package main")},
		redirectURL.String():    {URL: redirectedFinalURL, StatusCode: 200, ContentType: "text/go", Body: []byte("package main")},
	}}
	scripts, loadErrors := loadScriptsForEngine(context.Background(), loader, pageURL, document, runtimemodel.EngineGo)
	if len(scripts) != 0 || len(loadErrors) != 2 {
		t.Fatalf("scripts=%v errors=%v, want no scripts and two policy errors", scripts, loadErrors)
	}
	if len(loader.requested) != 1 || loader.requested[0] != redirectURL.String() {
		t.Fatalf("requested scripts=%v, want only same-origin redirect source", loader.requested)
	}
	if !strings.Contains(loadErrors[1], "redirected") {
		t.Fatalf("redirect policy errors=%v", loadErrors)
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

func TestCollectJavaScriptRecognizesDefaultAndExplicitTypes(t *testing.T) {
	document, err := html.Parse(strings.NewReader(`
<script>first()</script>
<script type="">second()</script>
<script type="text/javascript" src="app.js"></script>
<script type="application/javascript">fourth()</script>
<script type="module">ignored()</script>
<script type="text/go">package main</script>
`))
	if err != nil {
		t.Fatal(err)
	}
	sources := collectScriptsForEngine(document.Root, runtimemodel.EngineJavaScript)
	if got, want := len(sources), 4; got != want {
		t.Fatalf("JavaScript source count = %d, want %d", got, want)
	}
	if strings.TrimSpace(sources[0].source) != "first()" || strings.TrimSpace(sources[1].source) != "second()" ||
		sources[2].src != "app.js" || strings.TrimSpace(sources[3].source) != "fourth()" {
		t.Fatalf("JavaScript sources = %#v", sources)
	}
}

func TestLoadScriptsForEngineAppliesCountAndTotalLimits(t *testing.T) {
	pageURL := mustParseURL(t, "http://localhost/index.html")
	tooMany := strings.Repeat(`<script type="text/javascript">x()</script>`, maxScriptsPerEngine+1)
	document, err := html.Parse(strings.NewReader(tooMany))
	if err != nil {
		t.Fatal(err)
	}
	scripts, loadErrors := loadScriptsForEngine(context.Background(), &routeLoader{}, pageURL, document, runtimemodel.EngineJavaScript)
	if len(scripts) != maxScriptsPerEngine || len(loadErrors) != 1 || !strings.Contains(loadErrors[0], "count") {
		t.Fatalf("count-limited scripts=%d errors=%v", len(scripts), loadErrors)
	}

	large := strings.Repeat("x", maxScriptBytes)
	document, err = html.Parse(strings.NewReader(strings.Repeat(`<script type="text/javascript">`+large+`</script>`, maxScriptTotalBytes/maxScriptBytes+1)))
	if err != nil {
		t.Fatal(err)
	}
	scripts, loadErrors = loadScriptsForEngine(context.Background(), &routeLoader{}, pageURL, document, runtimemodel.EngineJavaScript)
	if len(scripts) != maxScriptTotalBytes/maxScriptBytes || len(loadErrors) != 1 || !strings.Contains(loadErrors[0], "total") {
		t.Fatalf("total-limited scripts=%d errors=%v", len(scripts), loadErrors)
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

func TestIsGoContentTypeAcceptsCommonGoMIMETypes(t *testing.T) {
	for _, contentType := range []string{
		"text/go",
		"text/x-go; charset=utf-8",
		"application/x-go",
		"text/plain",
		"",
	} {
		if !isGoContentType(contentType) {
			t.Errorf("isGoContentType(%q) = false, want true", contentType)
		}
	}
	if isGoContentType("application/javascript") {
		t.Fatal("isGoContentType(application/javascript) = true, want false")
	}
}

func TestIsJavaScriptContentTypeAcceptsSupportedMIMETypes(t *testing.T) {
	for _, contentType := range []string{"text/javascript", "application/javascript; charset=utf-8", "text/plain", ""} {
		if !isJavaScriptContentType(contentType) {
			t.Errorf("isJavaScriptContentType(%q) = false, want true", contentType)
		}
	}
	if isJavaScriptContentType("text/go") {
		t.Fatal("isJavaScriptContentType(text/go) = true, want false")
	}
}
