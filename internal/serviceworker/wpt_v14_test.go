package serviceworker

import "testing"

// WPT source: service-workers/service-worker/register-default-scope.https.html.
func TestWPTServiceWorkerDefaultScopeIsScriptDirectory(t *testing.T) {
	clientURL := parseServiceWorkerURL(t, "https://worker.example/app/page.html")
	scriptURL, scopeURL, origin, err := resolveRegistration(clientURL, "workers/sw.js", "")
	if err != nil {
		t.Fatal(err)
	}
	if scriptURL.String() != "https://worker.example/app/workers/sw.js" || scopeURL.String() != "https://worker.example/app/workers/" || origin != "https://worker.example" {
		t.Fatalf("default registration = script:%s scope:%s origin:%s", scriptURL, scopeURL, origin)
	}
}
