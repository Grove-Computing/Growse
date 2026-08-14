package persistentapp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Grove-Computing/Growse/internal/browser"
	"github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/network"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
	"github.com/Grove-Computing/Growse/internal/runtime/yaegi"
	storagecore "github.com/Grove-Computing/Growse/internal/storage"
)

func TestPersistentAppRestoresNotesRoutesAndReusesHTTPCache(t *testing.T) {
	index := readFixture(t, "index.html")
	stylesheet := readFixture(t, "style.css")
	script := readFixture(t, "_app.go")
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		response.Header().Set("Cache-Control", "max-age=3600")
		switch request.URL.Path {
		case "/":
			response.Header().Set("Content-Type", "text/html")
			_, _ = response.Write(index)
		case "/style.css":
			response.Header().Set("Content-Type", "text/css")
			_, _ = response.Write(stylesheet)
		case "/_app.go":
			response.Header().Set("Content-Type", "text/x-go")
			_, _ = response.Write(script)
		case "/api/notes":
			response.Header().Set("Content-Type", "text/plain")
			_, _ = response.Write([]byte("Remote fixture note"))
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()
	profile := t.TempDir()
	dataRoot := filepath.Join(profile, "data")
	cacheRoot := filepath.Join(profile, "cache")

	first := newPersistentBrowser(t, server, dataRoot, cacheRoot)
	firstMutations := make(chan struct{}, 32)
	first.SetOnMutation(func() { notify(firstMutations) })
	_, err := first.Navigate(context.Background(), server.URL+"/?filter=all#note-1")
	if err != nil {
		t.Fatal(err)
	}
	waitForText(t, first, "status", "synced", firstMutations)
	waitForContains(t, first, "note", "Remote fixture note", firstMutations)
	if route := elementText(t, first, "route"); !strings.Contains(route, "filter=all") {
		t.Fatalf("initial route = %q", route)
	}
	if !first.SetInputValue(elementID(t, first, "note-input"), "Local edited note") {
		t.Fatal("SetInputValue() did not dispatch WebGo input")
	}
	waitForText(t, first, "status", "saved", firstMutations)
	if !first.HasAnimationFrameCallbacks() || !first.RunAnimationFrame(time.Date(2026, time.August, 15, 12, 0, 0, 0, time.UTC)) {
		t.Fatal("save did not request a deterministic paint frame")
	}
	if !first.DispatchClick(elementID(t, first, "all-notes"), 0, 0) || !strings.Contains(pageURL(t, first), "filter=all#notes") {
		t.Fatalf("PushState route = %s", pageURL(t, first))
	}
	if route := elementText(t, first, "route"); !strings.Contains(route, "#notes") {
		t.Fatalf("fragment route = %q", route)
	}
	if _, err := first.Back(context.Background()); err != nil {
		t.Fatal(err)
	}
	if current := pageURL(t, first); strings.Contains(current, "#notes") {
		t.Fatalf("Back route = %s", current)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	firstRequestCount := requests.Load()

	second := newPersistentBrowser(t, server, dataRoot, cacheRoot)
	defer second.Close()
	secondMutations := make(chan struct{}, 32)
	second.SetOnMutation(func() { notify(secondMutations) })
	_, err = second.Navigate(context.Background(), server.URL+"/?filter=all#note-1")
	if err != nil {
		t.Fatal(err)
	}
	waitForText(t, second, "status", "synced", secondMutations)
	waitForContains(t, second, "note", "Local edited note", secondMutations)
	if requests.Load() != firstRequestCount {
		t.Fatalf("restart Network requests = %d, want Disk Cache count %d", requests.Load(), firstRequestCount)
	}
}

func newPersistentBrowser(t *testing.T, server *httptest.Server, dataRoot, cacheRoot string) *browser.Browser {
	t.Helper()
	manager, err := storagecore.NewPersistentManager(dataRoot)
	if err != nil {
		t.Fatal(err)
	}
	client, err := network.NewClientWithCacheRoot(server.Client(), 1<<20, cacheRoot)
	if err != nil {
		t.Fatal(err)
	}
	return browser.NewWithRuntimeFactoryAndStorage(client, func() runtimemodel.Runtime { return yaegi.New() }, manager)
}

func waitForText(t *testing.T, engine *browser.Browser, id, want string, mutations <-chan struct{}) {
	t.Helper()
	waitFor(t, mutations, func() bool {
		return elementText(t, engine, id) == want
	})
}

func waitForContains(t *testing.T, engine *browser.Browser, id, want string, mutations <-chan struct{}) {
	t.Helper()
	waitFor(t, mutations, func() bool {
		return strings.Contains(elementText(t, engine, id), want)
	})
}

func elementText(t *testing.T, engine *browser.Browser, id string) string {
	t.Helper()
	var value string
	if !engine.InspectPage(func(page *browser.Page) bool {
		node, ok := page.Document.GetElementByID(id)
		if ok {
			value = node.TextContent()
		}
		return true
	}) {
		t.Fatal("InspectPage() failed")
	}
	return value
}

func elementID(t *testing.T, engine *browser.Browser, id string) dom.NodeID {
	t.Helper()
	var value dom.NodeID
	if !engine.InspectPage(func(page *browser.Page) bool {
		node, ok := page.Document.GetElementByID(id)
		if ok {
			value = node.ID
		}
		return ok
	}) || value == 0 {
		t.Fatalf("element %q was not found", id)
	}
	return value
}

func pageURL(t *testing.T, engine *browser.Browser) string {
	t.Helper()
	var value string
	if !engine.InspectPage(func(page *browser.Page) bool {
		value = page.URL.String()
		return true
	}) {
		t.Fatal("InspectPage() failed")
	}
	return value
}

func waitFor(t *testing.T, mutations <-chan struct{}, ready func() bool) {
	t.Helper()
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	for !ready() {
		select {
		case <-mutations:
		case <-deadline.C:
			t.Fatal("timed out waiting for Persistent App state")
		}
	}
}

func notify(channel chan<- struct{}) {
	select {
	case channel <- struct{}{}:
	default:
	}
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	content, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return content
}
