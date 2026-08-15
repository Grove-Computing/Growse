package integration_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	"github.com/Grove-Computing/Growse/internal/browser"
	"github.com/Grove-Computing/Growse/internal/network"
	storagecore "github.com/Grove-Computing/Growse/internal/storage"
	"github.com/Grove-Computing/Growse/internal/webapi/scheduler"
)

type fakeClock struct {
	current time.Time
}

type fakeNavigator struct {
	session *browser.Session
}

func (navigator fakeNavigator) Open(ctx context.Context, rawURL string) (browser.TabSnapshot, error) {
	target, err := url.Parse(rawURL)
	if err != nil {
		return browser.TabSnapshot{}, err
	}
	tab, err := navigator.session.NewTab(target)
	if err != nil {
		return browser.TabSnapshot{}, err
	}
	if _, err := navigator.session.SelectTab(tab.ID); err != nil {
		return browser.TabSnapshot{}, err
	}
	_, state, ok := navigator.session.ActiveBrowserTarget()
	if !ok {
		return browser.TabSnapshot{}, browser.ErrTabNotFound
	}
	if _, err := state.Navigate(ctx, rawURL); err != nil {
		return browser.TabSnapshot{}, err
	}
	active, ok := navigator.session.ActiveTab()
	if !ok {
		return browser.TabSnapshot{}, browser.ErrTabNotFound
	}
	return active, nil
}

func TestMultipleTabsUseOnlyDeterministicInjectedDependencies(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		requests++
		response.Header().Set("Content-Type", "text/html")
		response.Header().Set("Cache-Control", "max-age=3600")
		_, _ = response.Write([]byte("<!doctype html><title>Deterministic</title>"))
	}))
	t.Cleanup(server.Close)

	profile, err := storagecore.NewPersistentManager(filepath.Join(t.TempDir(), "profile"))
	if err != nil {
		t.Fatal(err)
	}
	client := network.NewClientWithLimits(server.Client(), 4096)
	clock := &fakeClock{current: time.Date(2026, time.August, 15, 12, 0, 0, 0, time.UTC)}
	var pageProfiles []*storagecore.Manager
	session := browser.NewSession(func() *browser.Browser {
		pageProfile := profile.NewPageSession()
		pageProfiles = append(pageProfiles, pageProfile)
		state := browser.NewWithRuntimeFactoryAndStorage(client, nil, pageProfile)
		state.SetAnimationClock(clock)
		return state
	})
	t.Cleanup(func() { _ = session.Close() })
	navigator := fakeNavigator{session: session}

	first, err := navigator.Open(context.Background(), server.URL+"/workspace")
	if err != nil {
		t.Fatal(err)
	}
	clock.current = clock.current.Add(16 * time.Millisecond)
	second, err := navigator.Open(context.Background(), server.URL+"/workspace")
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == second.ID || len(session.Tabs()) != 2 || requests != 1 {
		t.Fatalf("deterministic tabs/cache = ids:%d/%d tabs:%d requests:%d", first.ID, second.ID, len(session.Tabs()), requests)
	}
	local, _, err := pageProfiles[0].Areas(parseURL(t, server.URL+"/workspace"))
	if err != nil {
		t.Fatal(err)
	}
	if err := local.Set("shared", "yes"); err != nil {
		t.Fatal(err)
	}
	peerLocal, _, err := pageProfiles[1].Areas(parseURL(t, server.URL+"/workspace"))
	if err != nil {
		t.Fatal(err)
	}
	if value, found := peerLocal.Get("shared"); !found || value != "yes" {
		t.Fatalf("temporary profile sharing = (%q, %v)", value, found)
	}
}

func (clock *fakeClock) Now() time.Time { return clock.current }

func TestSchedulerFrameProfileAndHTTPCacheAreDeterministic(t *testing.T) {
	profileRoot := t.TempDir()
	dataRoot := filepath.Join(profileRoot, "data")
	cacheRoot := filepath.Join(profileRoot, "cache")
	documentURL := parseURL(t, "https://app.example.test/notes")
	manager, err := storagecore.NewPersistentManager(dataRoot)
	if err != nil {
		t.Fatal(err)
	}
	local, _, err := manager.Areas(documentURL)
	if err != nil {
		t.Fatal(err)
	}

	start := time.Date(2026, time.August, 15, 12, 0, 0, 0, time.UTC)
	clock := &fakeClock{current: start}
	frameRequests := 0
	api := scheduler.NewPageWithClock(context.Background(), clock, func(callback func()) bool {
		callback()
		return true
	}, func() { frameRequests++ })
	t.Cleanup(api.Close)
	if _, err := api.SetTimeout(10*time.Millisecond, func() {
		if err := local.Set("note", "saved"); err != nil {
			t.Errorf("Local Storage Set() error = %v", err)
		}
	}); err != nil {
		t.Fatal(err)
	}
	frameDelivered := false
	var frameTimestamp scheduler.Timestamp
	if _, err := api.RequestAnimationFrame(func(timestamp scheduler.Timestamp) {
		frameTimestamp = timestamp
		value, ok := local.Get("note")
		frameDelivered = ok && value == "saved"
	}); err != nil {
		t.Fatal(err)
	}
	clock.current = start.Add(10 * time.Millisecond)
	api.RunDue(clock.Now())
	frameTime := start.Add(16 * time.Millisecond)
	if !api.RunAnimationFrame(frameTime) || !frameDelivered || frameRequests != 1 || frameTimestamp != scheduler.Timestamp(16*time.Millisecond) {
		t.Fatalf("deterministic frame = delivered:%t requests:%d timestamp:%v", frameDelivered, frameRequests, frameTimestamp)
	}

	restartedManager, err := storagecore.NewPersistentManager(dataRoot)
	if err != nil {
		t.Fatal(err)
	}
	restored, _, err := restartedManager.Areas(documentURL)
	if err != nil {
		t.Fatal(err)
	}
	if value, ok := restored.Get("note"); !ok || value != "saved" {
		t.Fatalf("restored Local Storage = %q, %t", value, ok)
	}

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		requests++
		response.Header().Set("Cache-Control", "max-age=3600")
		_, _ = response.Write([]byte("fixture"))
	}))
	t.Cleanup(server.Close)
	target := parseURL(t, server.URL+"/fixture")
	for range 2 {
		client, err := network.NewClientWithCacheRoot(server.Client(), 1024, cacheRoot)
		if err != nil {
			t.Fatal(err)
		}
		response, err := client.Get(context.Background(), target)
		if err != nil || string(response.Body) != "fixture" {
			t.Fatalf("HTTP fixture = (%v, %v)", response, err)
		}
	}
	if requests != 1 {
		t.Fatalf("fake HTTP requests = %d, want one plus Disk Cache hit", requests)
	}
}

func parseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}
