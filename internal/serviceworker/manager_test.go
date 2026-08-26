package serviceworker

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/network"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
)

func TestRegistrationLifecycleScopeUpdateClaimAndUnregister(t *testing.T) {
	manager := NewManager()
	clientURL := parseServiceWorkerURL(t, "https://app.example/app/page.html")
	scriptURL := parseServiceWorkerURL(t, "https://app.example/app/sw.js")
	source := []byte(`
		self.addEventListener("install", event => event.waitUntil(self.skipWaiting()));
		self.addEventListener("activate", event => event.waitUntil(clients.claim()));`)
	fetches := 0
	fetch := func(_ context.Context, request *network.Request) (*network.Response, error) {
		fetches++
		if request.Kind != network.RequestServiceWorker || request.Credentials != network.CredentialsInclude {
			t.Fatalf("Service Worker request = %#v", request)
		}
		return &network.Response{URL: scriptURL, StatusCode: http.StatusOK, ContentType: "text/javascript", Body: append([]byte(nil), source...)}, nil
	}
	registration, err := manager.Register(context.Background(), clientURL, "/app/sw.js", "", fetch)
	if err != nil {
		t.Fatal(err)
	}
	if registration.Scope != "https://app.example/app/" || registration.Active != runtimemodel.ServiceWorkerActivated || registration.ActiveScriptURL != scriptURL.String() || !registration.Claimed {
		t.Fatalf("registration = %#v", registration)
	}
	if controller := manager.Controller(clientURL); controller == nil || controller.ID != registration.ID {
		t.Fatalf("controller = %#v", controller)
	}
	outside, err := manager.GetRegistration(clientURL, "/outside/page.html")
	if err != nil || outside != nil {
		t.Fatalf("outside registration = %#v, %v", outside, err)
	}
	identical, err := manager.Update(context.Background(), clientURL, registration.Scope, fetch)
	if err != nil || identical.ID != registration.ID || identical.Waiting != "" || fetches != 2 {
		t.Fatalf("identical update = %#v, %v, fetches=%d", identical, err, fetches)
	}

	source = []byte(`self.addEventListener("install", event => event.waitUntil(Promise.resolve()));`)
	waiting, err := manager.Update(context.Background(), clientURL, registration.Scope, fetch)
	if err != nil || waiting.Active != runtimemodel.ServiceWorkerActivated || waiting.Waiting != runtimemodel.ServiceWorkerInstalled || waiting.WaitingScriptURL != scriptURL.String() {
		t.Fatalf("waiting update = %#v, %v", waiting, err)
	}
	source = []byte(`self.addEventListener("install", () => self.skipWaiting()); self.addEventListener("activate", () => clients.claim());`)
	activated, err := manager.Update(context.Background(), clientURL, registration.Scope, fetch)
	if err != nil || activated.Waiting != "" || activated.Active != runtimemodel.ServiceWorkerActivated || !activated.Claimed {
		t.Fatalf("skipWaiting update = %#v, %v", activated, err)
	}
	removed, err := manager.Unregister(clientURL, registration.Scope)
	if err != nil || !removed || manager.Controller(clientURL) != nil {
		t.Fatalf("Unregister() = %t, %v", removed, err)
	}
	removed, err = manager.Unregister(clientURL, registration.Scope)
	if err != nil || removed {
		t.Fatalf("second Unregister() = %t, %v", removed, err)
	}
}

func TestRegistrationRejectsInsecureCrossOriginAndWideScope(t *testing.T) {
	manager := NewManager()
	fetch := func(context.Context, *network.Request) (*network.Response, error) {
		return nil, errors.New("must not fetch")
	}
	if _, err := manager.Register(context.Background(), parseServiceWorkerURL(t, "http://public.example/app"), "/sw.js", "", fetch); !errors.Is(err, ErrInsecureContext) {
		t.Fatalf("insecure Register() error = %v", err)
	}
	secure := parseServiceWorkerURL(t, "https://app.example/app/page")
	if _, err := manager.Register(context.Background(), secure, "https://other.example/sw.js", "", fetch); !errors.Is(err, ErrCrossOrigin) {
		t.Fatalf("cross-origin Register() error = %v", err)
	}
	if _, err := manager.Register(context.Background(), secure, "/app/sw.js", "/", fetch); !errors.Is(err, ErrScope) {
		t.Fatalf("wide-scope Register() error = %v", err)
	}
	if !IsSecureContext(parseServiceWorkerURL(t, "http://localhost:8080/app")) || !IsSecureContext(parseServiceWorkerURL(t, "http://127.0.0.1/app")) {
		t.Fatal("loopback development origin was not secure")
	}
}

func TestActiveWorkerDispatchesFetchResponseAndNetworkFallback(t *testing.T) {
	manager := NewManager()
	clientURL := parseServiceWorkerURL(t, "https://app.example/app/page.html")
	scriptURL := parseServiceWorkerURL(t, "https://app.example/app/sw.js")
	source := []byte(`
		self.addEventListener("install", () => self.skipWaiting());
		self.addEventListener("activate", () => clients.claim());
		self.addEventListener("fetch", event => {
			if (event.request.url.endsWith("/offline")) {
				event.respondWith(Promise.resolve(new Response("offline", {
					status: 203, headers: {"Content-Type": "text/html", "X-Service-Worker": "yes"}
				})));
			} else if (event.request.url.endsWith("/passthrough")) {
				event.respondWith(fetch(event.request));
			}
		});`)
	_, err := manager.Register(context.Background(), clientURL, scriptURL.String(), "", func(context.Context, *network.Request) (*network.Response, error) {
		return &network.Response{URL: scriptURL, StatusCode: http.StatusOK, ContentType: "text/javascript", Body: source}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	fallbackCalls := 0
	fallback := func(_ context.Context, request *network.Request) (*network.Response, error) {
		fallbackCalls++
		if strings.HasSuffix(request.URL.Path, "/passthrough") && request.Kind != network.RequestServiceWorkerFetch {
			t.Fatalf("internal fetch kind = %v", request.Kind)
		}
		return &network.Response{
			URL: request.URL, StatusCode: http.StatusOK, Status: "OK", ContentType: "text/plain",
			Header: http.Header{"Content-Type": []string{"text/plain"}}, Body: []byte("network"),
		}, nil
	}
	offlineURL := parseServiceWorkerURL(t, "https://app.example/app/offline")
	response, err := manager.DispatchFetch(context.Background(), &network.Request{Method: http.MethodGet, URL: offlineURL, Kind: network.RequestNavigation}, fallback)
	if err != nil {
		t.Fatal(err)
	}
	if string(response.Body) != "offline" || response.StatusCode != 203 || response.Header.Get("X-Service-Worker") != "yes" || fallbackCalls != 0 {
		t.Fatalf("synthetic response = %#v, fallback calls = %d", response, fallbackCalls)
	}
	passURL := parseServiceWorkerURL(t, "https://app.example/app/passthrough")
	response, err = manager.DispatchFetch(context.Background(), &network.Request{Method: http.MethodGet, URL: passURL, Kind: network.RequestFetch, SiteURL: clientURL}, fallback)
	if err != nil || string(response.Body) != "network" || fallbackCalls != 1 {
		t.Fatalf("respondWith(fetch()) = %#v, %v, fallback calls = %d", response, err, fallbackCalls)
	}
	networkURL := parseServiceWorkerURL(t, "https://app.example/app/network")
	response, err = manager.DispatchFetch(context.Background(), &network.Request{Method: http.MethodGet, URL: networkURL, Kind: network.RequestSubresource, SiteURL: clientURL}, fallback)
	if err != nil || string(response.Body) != "network" || fallbackCalls != 2 {
		t.Fatalf("implicit fallback = %#v, %v, fallback calls = %d", response, err, fallbackCalls)
	}
	outsideURL := parseServiceWorkerURL(t, "https://app.example/outside/page")
	_, err = manager.DispatchFetch(context.Background(), &network.Request{Method: http.MethodGet, URL: outsideURL, Kind: network.RequestNavigation}, fallback)
	if err != nil || fallbackCalls != 3 {
		t.Fatalf("outside fallback = %v, calls = %d", err, fallbackCalls)
	}
}

func parseServiceWorkerURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	value, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
