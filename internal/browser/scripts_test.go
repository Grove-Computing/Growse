package browser

import (
	"context"
	"crypto/sha512"
	"encoding/base64"
	"net/url"
	"strings"
	"testing"
	"time"

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

func TestGoEngineDoesNotFetchTextGoFromArbitraryInternetOrigin(t *testing.T) {
	pageURL := mustParseURL(t, "https://public.example/page.html")
	scriptURL := mustParseURL(t, "https://public.example/app.go")
	document, err := html.Parse(strings.NewReader(`<script type="text/go" src="/app.go"></script>`))
	if err != nil {
		t.Fatal(err)
	}
	loader := &routeLoader{responses: map[string]*network.Response{
		scriptURL.String(): {URL: scriptURL, StatusCode: 200, ContentType: "text/go", Body: []byte(`package main; func main() {}`)},
	}}
	scripts, loadErrors := loadScriptsForEngine(context.Background(), loader, pageURL, document, runtimemodel.EngineGo)
	if len(scripts) != 0 || len(loadErrors) != 1 || !strings.Contains(loadErrors[0], "untrusted") {
		t.Fatalf("external Go scripts=%v errors=%v", scripts, loadErrors)
	}
	if len(loader.requested) != 0 {
		t.Fatalf("untrusted text/go was fetched: %v", loader.requested)
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
<script type="module">fifth()</script>
<script type="text/go">package main</script>
`))
	if err != nil {
		t.Fatal(err)
	}
	sources := collectScriptsForEngine(document.Root, runtimemodel.EngineJavaScript)
	if got, want := len(sources), 5; got != want {
		t.Fatalf("JavaScript source count = %d, want %d", got, want)
	}
	if strings.TrimSpace(sources[0].source) != "first()" || strings.TrimSpace(sources[1].source) != "second()" ||
		sources[2].src != "app.js" || strings.TrimSpace(sources[3].source) != "fourth()" ||
		sources[4].kind != runtimemodel.ScriptModule || sources[4].schedule != runtimemodel.ScriptDefer || strings.TrimSpace(sources[4].source) != "fifth()" {
		t.Fatalf("JavaScript sources = %#v", sources)
	}
}

func TestJavaScriptModuleUsesCORSAndDeferredScheduling(t *testing.T) {
	pageURL := mustParseURL(t, "https://site.example/page")
	moduleURL := mustParseURL(t, "https://cdn.example/app.js")
	document, _ := html.Parse(strings.NewReader(`<script type="module" src="https://cdn.example/app.js"></script>`))
	loader := &requestRouteLoader{routeLoader: routeLoader{responses: map[string]*network.Response{
		moduleURL.String(): {URL: moduleURL, StatusCode: 200, ContentType: "text/javascript", Body: []byte(`export const ready = true`)},
	}}}
	scripts, loadErrors := loadScriptsForEngine(context.Background(), loader, pageURL, document, runtimemodel.EngineJavaScript)
	if len(loadErrors) != 0 || len(scripts) != 1 || scripts[0].Kind != runtimemodel.ScriptModule || scripts[0].Schedule != runtimemodel.ScriptDefer {
		t.Fatalf("scripts=%#v errors=%v", scripts, loadErrors)
	}
	if loader.request == nil || loader.request.Kind != network.RequestModule || !loader.request.CORS || loader.request.Credentials != network.CredentialsSameOrigin {
		t.Fatalf("module request = %#v", loader.request)
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
	for _, contentType := range []string{"text/javascript", "application/javascript; charset=utf-8", "application/ecmascript", "text/ecmascript"} {
		if !isJavaScriptContentType(contentType) {
			t.Errorf("isJavaScriptContentType(%q) = false, want true", contentType)
		}
	}
	for _, contentType := range []string{"text/go", "text/plain", "", "application/json"} {
		if isJavaScriptContentType(contentType) {
			t.Errorf("isJavaScriptContentType(%q) = true, want false", contentType)
		}
	}
}

func TestJavaScriptClassicLoadsCrossOriginWithCORSAndIntegrity(t *testing.T) {
	pageURL := mustParseURL(t, "https://site.example/page")
	scriptURL := mustParseURL(t, "https://cdn.example/app.js")
	body := []byte(`document.title = "loaded";`)
	digest := sha512.Sum384(body)
	integrity := "sha384-" + base64.StdEncoding.EncodeToString(digest[:])
	document, err := html.Parse(strings.NewReader(`<script src="https://cdn.example/app.js" crossorigin="anonymous" integrity="` + integrity + `"></script>`))
	if err != nil {
		t.Fatal(err)
	}
	loader := &requestRouteLoader{routeLoader: routeLoader{responses: map[string]*network.Response{
		scriptURL.String(): {URL: scriptURL, StatusCode: 200, ContentType: "text/javascript", Body: body},
	}}}
	scripts, loadErrors := loadScriptsForEngine(context.Background(), loader, pageURL, document, runtimemodel.EngineJavaScript)
	if len(loadErrors) != 0 || len(scripts) != 1 || scripts[0].SourceURL.String() != scriptURL.String() || scripts[0].Integrity != integrity {
		t.Fatalf("scripts=%#v errors=%v", scripts, loadErrors)
	}
	if loader.request == nil || !loader.request.CORS || loader.request.Credentials != network.CredentialsSameOrigin || loader.request.Kind != network.RequestScript {
		t.Fatalf("script request = %#v", loader.request)
	}
}

func TestJavaScriptClassicAppliesStatusMIMEMixedContentAndIntegrityPolicy(t *testing.T) {
	pageURL := mustParseURL(t, "https://site.example/page")
	goodBody := []byte(`globalThis.loaded = true;`)
	wrongDigest := sha512.Sum384([]byte("different"))
	cases := []struct {
		name        string
		src         string
		response    *network.Response
		integrity   string
		wantRequest bool
		wantError   string
	}{
		{name: "initial mixed content", src: "http://cdn.example/app.js", wantError: "mixed-content"},
		{name: "redirected mixed content", src: "https://cdn.example/redirect.js", response: &network.Response{URL: mustParseURL(t, "http://cdn.example/app.js"), StatusCode: 200, ContentType: "text/javascript", Body: goodBody}, wantRequest: true, wantError: "redirected"},
		{name: "HTTP error", src: "https://cdn.example/error.js", response: &network.Response{URL: mustParseURL(t, "https://cdn.example/error.js"), StatusCode: 404, ContentType: "text/javascript", Body: goodBody}, wantRequest: true, wantError: "status 404"},
		{name: "invalid MIME", src: "https://cdn.example/plain.js", response: &network.Response{URL: mustParseURL(t, "https://cdn.example/plain.js"), StatusCode: 200, ContentType: "text/plain", Body: goodBody}, wantRequest: true, wantError: "Content-Type"},
		{name: "integrity mismatch", src: "https://cdn.example/hash.js", response: &network.Response{URL: mustParseURL(t, "https://cdn.example/hash.js"), StatusCode: 200, ContentType: "text/javascript", Body: goodBody}, integrity: "sha384-" + base64.StdEncoding.EncodeToString(wrongDigest[:]), wantRequest: true, wantError: "integrity"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			markup := `<script src="` + test.src + `" crossorigin="anonymous" integrity="` + test.integrity + `"></script>`
			document, err := html.Parse(strings.NewReader(markup))
			if err != nil {
				t.Fatal(err)
			}
			loader := &routeLoader{responses: make(map[string]*network.Response)}
			if test.response != nil {
				loader.responses[test.src] = test.response
			}
			scripts, loadErrors := loadScriptsForEngine(context.Background(), loader, pageURL, document, runtimemodel.EngineJavaScript)
			if len(scripts) != 0 || len(loadErrors) != 1 || !strings.Contains(loadErrors[0], test.wantError) {
				t.Fatalf("scripts=%v errors=%v", scripts, loadErrors)
			}
			if got := len(loader.requested) != 0; got != test.wantRequest {
				t.Fatalf("requested=%v, wantRequest=%t", loader.requested, test.wantRequest)
			}
		})
	}
}

func TestCrossOriginClassicWithoutCORSUsesIncludedCredentials(t *testing.T) {
	pageURL := mustParseURL(t, "https://site.example/page")
	scriptURL := mustParseURL(t, "https://cdn.example/app.js")
	document, _ := html.Parse(strings.NewReader(`<script src="https://cdn.example/app.js"></script>`))
	loader := &requestRouteLoader{routeLoader: routeLoader{responses: map[string]*network.Response{
		scriptURL.String(): {URL: scriptURL, StatusCode: 200, ContentType: "text/javascript", Body: []byte(`true`)},
	}}}
	scripts, loadErrors := loadScriptsForEngine(context.Background(), loader, pageURL, document, runtimemodel.EngineJavaScript)
	if len(scripts) != 1 || len(loadErrors) != 0 || loader.request == nil || loader.request.CORS || loader.request.Credentials != network.CredentialsInclude {
		t.Fatalf("scripts=%v errors=%v request=%#v", scripts, loadErrors, loader.request)
	}
}

type delayedScriptLoader struct {
	responses map[string]*network.Response
	delays    map[string]time.Duration
}

func (loader *delayedScriptLoader) Get(ctx context.Context, target *url.URL) (*network.Response, error) {
	if delay := loader.delays[target.String()]; delay > 0 {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return loader.responses[target.String()], nil
}

func TestAsyncClassicScriptsRecordFetchCompletionOrder(t *testing.T) {
	pageURL := mustParseURL(t, "https://site.example/page")
	firstURL := mustParseURL(t, "https://site.example/slow.js")
	secondURL := mustParseURL(t, "https://site.example/fast.js")
	deferURL := mustParseURL(t, "https://site.example/defer.js")
	document, _ := html.Parse(strings.NewReader(`
		<script async src="/slow.js"></script>
		<script defer src="/defer.js"></script>
		<script async defer src="/fast.js"></script>`))
	loader := &delayedScriptLoader{
		responses: map[string]*network.Response{
			firstURL.String():  {URL: firstURL, StatusCode: 200, ContentType: "text/javascript", Body: []byte(`slow()`)},
			secondURL.String(): {URL: secondURL, StatusCode: 200, ContentType: "text/javascript", Body: []byte(`fast()`)},
			deferURL.String():  {URL: deferURL, StatusCode: 200, ContentType: "text/javascript", Body: []byte(`deferred()`)},
		},
		delays: map[string]time.Duration{firstURL.String(): 50 * time.Millisecond, secondURL.String(): time.Millisecond},
	}
	scripts, loadErrors := loadScriptsForEngine(context.Background(), loader, pageURL, document, runtimemodel.EngineJavaScript)
	if len(loadErrors) != 0 || len(scripts) != 3 {
		t.Fatalf("scripts=%v errors=%v", scripts, loadErrors)
	}
	if scripts[0].Schedule != runtimemodel.ScriptAsync || scripts[0].FetchOrder != 2 ||
		scripts[1].Schedule != runtimemodel.ScriptDefer || scripts[1].FetchOrder != 0 ||
		scripts[2].Schedule != runtimemodel.ScriptAsync || scripts[2].FetchOrder != 1 {
		t.Fatalf("script scheduling = %#v", scripts)
	}
}
