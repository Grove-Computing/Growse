package isolated

import (
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/network"
)

func TestBrokerRevalidatesForgedWorkerFetchRequests(t *testing.T) {
	pageURL := parsePolicyURL(t, "https://app.example/page")
	target := parsePolicyURL(t, "https://api.example/data")
	for _, test := range []struct {
		name    string
		request *network.Request
	}{
		{name: "cookie", request: &network.Request{Method: http.MethodGet, URL: target, Header: http.Header{"Cookie": {"session=secret"}}}},
		{name: "host", request: &network.Request{Method: http.MethodGet, URL: target, Header: http.Header{"Host": {"attacker.test"}}}},
		{name: "file", request: &network.Request{Method: http.MethodGet, URL: parsePolicyURL(t, "file:///etc/passwd")}},
		{name: "userinfo", request: &network.Request{Method: http.MethodGet, URL: parsePolicyURL(t, "https://user:secret@api.example/data")}},
		{name: "method", request: &network.Request{Method: http.MethodConnect, URL: target}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := validateBrokeredFetch(test.request, pageURL, "javascript"); err == nil {
				t.Fatal("forged worker request passed broker validation")
			}
		})
	}
}

func TestBrokerOverwritesWorkerControlledRequestAuthority(t *testing.T) {
	pageURL := parsePolicyURL(t, "https://app.example/page")
	request := &network.Request{
		Method: http.MethodPost, URL: parsePolicyURL(t, "https://api.example/data"), Header: http.Header{"X-App": {"value"}},
		SiteURL: parsePolicyURL(t, "https://attacker.test/"), Kind: network.RequestNavigation, Engine: "go",
		Credentials: network.CredentialsInclude,
	}
	if err := validateBrokeredFetch(request, pageURL, "javascript"); err != nil {
		t.Fatalf("validateBrokeredFetch() error = %v", err)
	}
	if request.SiteURL.String() != pageURL.String() || request.Kind != network.RequestFetch || request.Engine != "javascript" {
		t.Fatalf("brokered authority = site:%v kind:%v engine:%q", request.SiteURL, request.Kind, request.Engine)
	}
}

func TestBrokerForcesModuleFetchPolicy(t *testing.T) {
	pageURL := parsePolicyURL(t, "https://app.example/page")
	request := &network.Request{
		Method: http.MethodGet, URL: parsePolicyURL(t, "https://cdn.example/module.js"),
		Kind: network.RequestModule, Engine: "forged", Credentials: network.CredentialsInclude,
	}
	if err := validateBrokeredFetch(request, pageURL, "javascript"); err != nil {
		t.Fatalf("validateBrokeredFetch() error = %v", err)
	}
	if request.Kind != network.RequestModule || !request.CORS || request.Credentials != network.CredentialsInclude || request.Engine != "javascript" {
		t.Fatalf("module broker policy = %#v", request)
	}
	for _, invalid := range []*network.Request{
		{Method: http.MethodPost, URL: request.URL, Kind: network.RequestModule},
		{Method: http.MethodGet, URL: request.URL, Kind: network.RequestModule, Header: http.Header{"X-Test": {"value"}}},
	} {
		if err := validateBrokeredFetch(invalid, pageURL, "javascript"); err == nil {
			t.Fatalf("invalid module request passed: %#v", invalid)
		}
	}
}

func TestWorkerEnvironmentDoesNotInheritBrowserSecrets(t *testing.T) {
	const secretName = "GROWSE_TEST_BROWSER_SECRET"
	t.Setenv(secretName, "credential")
	for _, entry := range workerEnvironment() {
		if strings.HasPrefix(entry, secretName+"=") || strings.HasPrefix(entry, "HOME=") || strings.HasPrefix(entry, "PATH=") {
			t.Fatalf("worker inherited browser environment entry %q", entry)
		}
	}
	if os.Getenv(secretName) != "credential" {
		t.Fatal("test process secret unexpectedly changed")
	}
}

func TestPublicRuntimeURLRemovesUserinfo(t *testing.T) {
	target := parsePolicyURL(t, "https://user:secret@example.test/page?visible=yes")
	got := publicRuntimeURL(target)
	if got.User != nil || got.String() != "https://example.test/page?visible=yes" || target.User == nil {
		t.Fatalf("public runtime URL = %q, source = %q", got, target)
	}
}

func parsePolicyURL(t *testing.T, value string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(value)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}
