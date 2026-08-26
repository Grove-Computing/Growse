package serviceworker

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/Grove-Computing/Growse/internal/network"
)

func TestPersistentManagerRestoresRegistrationCacheUpdateAndUnregister(t *testing.T) {
	root := t.TempDir()
	manager, err := NewPersistentManager(root)
	if err != nil {
		t.Fatal(err)
	}
	clientURL := parseServiceWorkerURL(t, "https://persist.example/app/page.html")
	scriptURL := parseServiceWorkerURL(t, "https://persist.example/app/sw.js")
	source := []byte(`
		self.addEventListener("install", event => {
			event.waitUntil(caches.open("offline").then(cache => cache.put("/app/cached", new Response("persisted-cache"))));
			self.skipWaiting();
		});
		self.addEventListener("activate", () => clients.claim());
		self.addEventListener("fetch", event => event.respondWith(caches.match(event.request)));`)
	fetch := func(context.Context, *network.Request) (*network.Response, error) {
		return &network.Response{URL: scriptURL, StatusCode: http.StatusOK, ContentType: "text/javascript", Body: append([]byte(nil), source...)}, nil
	}
	registration, err := manager.Register(context.Background(), clientURL, scriptURL.String(), "", fetch)
	if err != nil {
		t.Fatal(err)
	}
	profilePath := filepath.Join(root, "service-workers", persistentOriginFilename("https://persist.example"))
	info, err := os.Stat(profilePath)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("profile mode = %o", info.Mode().Perm())
	}

	restored, err := NewPersistentManager(root)
	if err != nil {
		t.Fatal(err)
	}
	controller := restored.Controller(clientURL)
	if controller == nil || controller.ID != registration.ID || !controller.Claimed {
		t.Fatalf("restored controller = %#v", controller)
	}
	cachedURL := parseServiceWorkerURL(t, "https://persist.example/app/cached")
	response, err := restored.DispatchFetch(context.Background(), &network.Request{Method: http.MethodGet, URL: cachedURL, Kind: network.RequestNavigation}, func(context.Context, *network.Request) (*network.Response, error) {
		return nil, errors.New("restored Cache must avoid network fallback")
	})
	if err != nil || string(response.Body) != "persisted-cache" {
		t.Fatalf("restored cache response = %#v, %v", response, err)
	}

	source = []byte(`
		self.addEventListener("install", () => self.skipWaiting());
		self.addEventListener("activate", () => clients.claim());
		self.addEventListener("fetch", event => event.respondWith(new Response("updated-worker")));`)
	if _, err := restored.Update(context.Background(), clientURL, registration.Scope, fetch); err != nil {
		t.Fatal(err)
	}
	restarted, err := NewPersistentManager(root)
	if err != nil {
		t.Fatal(err)
	}
	response, err = restarted.DispatchFetch(context.Background(), &network.Request{Method: http.MethodGet, URL: cachedURL, Kind: network.RequestNavigation}, func(context.Context, *network.Request) (*network.Response, error) {
		return nil, errors.New("updated worker must avoid network fallback")
	})
	if err != nil || string(response.Body) != "updated-worker" {
		t.Fatalf("updated restored response = %#v, %v", response, err)
	}
	removed, err := restarted.Unregister(clientURL, registration.Scope)
	if err != nil || !removed {
		t.Fatalf("persistent unregister = %t, %v", removed, err)
	}
	afterUnregister, err := NewPersistentManager(root)
	if err != nil {
		t.Fatal(err)
	}
	registrations, err := afterUnregister.GetRegistrations(clientURL)
	if err != nil || len(registrations) != 0 || !afterUnregister.Caches().Has("https://persist.example", "offline") {
		t.Fatalf("state after unregister = registrations:%#v cache:%t error:%v", registrations, afterUnregister.Caches().Has("https://persist.example", "offline"), err)
	}
}

func TestPersistentManagerQuarantinesOnlyCorruptOrigin(t *testing.T) {
	root := t.TempDir()
	manager, err := NewPersistentManager(root)
	if err != nil {
		t.Fatal(err)
	}
	register := func(origin string) *url.URL {
		clientURL := parseServiceWorkerURL(t, origin+"/app/page")
		scriptURL := parseServiceWorkerURL(t, origin+"/app/sw.js")
		_, err := manager.Register(context.Background(), clientURL, scriptURL.String(), "", func(context.Context, *network.Request) (*network.Response, error) {
			return &network.Response{
				URL: scriptURL, StatusCode: http.StatusOK, ContentType: "text/javascript",
				Body: []byte(`self.addEventListener("install", () => self.skipWaiting()); self.addEventListener("activate", () => clients.claim());`),
			}, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		return clientURL
	}
	healthyURL := register("https://healthy.example")
	corruptURL := register("https://corrupt.example")
	directory := filepath.Join(root, "service-workers")
	corruptPath := filepath.Join(directory, persistentOriginFilename("https://corrupt.example"))
	if err := os.WriteFile(corruptPath, []byte(`{"version":1,"origin":"https://corrupt.example","registrations":[`), 0o600); err != nil {
		t.Fatal(err)
	}
	restored, err := NewPersistentManager(root)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Controller(healthyURL) == nil {
		t.Fatal("healthy Origin was lost while quarantining another Origin")
	}
	if restored.Controller(corruptURL) != nil {
		t.Fatal("corrupt Origin was restored")
	}
	quarantine, err := os.ReadDir(filepath.Join(directory, "quarantine"))
	if err != nil || len(quarantine) != 1 {
		t.Fatalf("quarantine entries = %#v, %v", quarantine, err)
	}
}
