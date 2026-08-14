package persistentapp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
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
	"github.com/Grove-Computing/Growse/internal/layout"
	"github.com/Grove-Computing/Growse/internal/network"
	paintmodel "github.com/Grove-Computing/Growse/internal/paint"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
	"github.com/Grove-Computing/Growse/internal/runtime/yaegi"
	storagecore "github.com/Grove-Computing/Growse/internal/storage"
)

var visualTime = time.Date(2026, time.August, 15, 12, 0, 0, 0, time.UTC)

type fixedClock struct{ current time.Time }

func (clock fixedClock) Now() time.Time { return clock.current }

type lifecycleGolden struct {
	FramePaintSHA   string `json:"frame_paint_sha256"`
	BackPaintSHA    string `json:"back_paint_sha256"`
	ForwardPaintSHA string `json:"forward_paint_sha256"`
	BackScroll      string `json:"back_scroll"`
	ForwardScroll   string `json:"forward_scroll"`
}

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
	if !first.HasAnimationFrameCallbacks() || !first.RunAnimationFrame(visualTime) {
		t.Fatal("save did not request a deterministic paint frame")
	}
	framePaint := paintSHA(t, first)
	first.UpdateHistoryScroll(2, -10)
	if !first.DispatchClick(elementID(t, first, "all-notes"), 0, 0) || !strings.Contains(pageURL(t, first), "filter=all#notes") {
		t.Fatalf("PushState route = %s", pageURL(t, first))
	}
	if route := elementText(t, first, "route"); !strings.Contains(route, "#notes") {
		t.Fatalf("fragment route = %q", route)
	}
	first.UpdateHistoryScroll(5, -24)
	forwardPaint := paintSHA(t, first)
	if _, err := first.Back(context.Background()); err != nil {
		t.Fatal(err)
	}
	if current := pageURL(t, first); strings.Contains(current, "#notes") {
		t.Fatalf("Back route = %s", current)
	}
	backScroll := scrollSnapshot(t, first)
	if backScroll != "first=2 offset=-10" {
		t.Fatalf("Back scroll = %s", backScroll)
	}
	backPaint := paintSHA(t, first)
	if _, err := first.Forward(context.Background()); err != nil {
		t.Fatal(err)
	}
	forwardScroll := scrollSnapshot(t, first)
	if forwardScroll != "first=5 offset=-24" {
		t.Fatalf("Forward scroll = %s", forwardScroll)
	}
	if got := paintSHA(t, first); got != forwardPaint {
		t.Fatalf("Forward paint = %s, want restored %s", got, forwardPaint)
	}
	assertLifecycleGolden(t, lifecycleGolden{
		FramePaintSHA: framePaint, BackPaintSHA: backPaint, ForwardPaintSHA: forwardPaint,
		BackScroll: backScroll, ForwardScroll: forwardScroll,
	})
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
	engine := browser.NewWithRuntimeFactoryAndStorage(client, func() runtimemodel.Runtime { return yaegi.New() }, manager)
	engine.SetAnimationClock(fixedClock{current: visualTime})
	return engine
}

func paintSHA(t *testing.T, engine *browser.Browser) string {
	t.Helper()
	var lines []string
	if !engine.InspectPage(func(page *browser.Page) bool {
		tree := layout.BuildWithViewport(page.Document, page.AnimatedStyles(visualTime), 480, 600)
		list := paintmodel.Build(tree)
		for _, command := range list.Commands {
			switch value := command.(type) {
			case paintmodel.DrawBox:
				lines = append(lines, fmt.Sprintf("box:%d:%.1f:%.1f:%.1f:%.1f:%08x:%.2f", value.NodeID, value.X, value.Y, value.Width, value.Height, value.Color, value.Opacity))
			case paintmodel.DrawText:
				lines = append(lines, fmt.Sprintf("text:%d:%.1f:%.1f:%.1f:%.1f:%08x:%s", value.NodeID, value.X, value.Y, value.Width, value.Height, value.Color, value.Text))
			case paintmodel.DrawInput:
				lines = append(lines, fmt.Sprintf("input:%d:%.1f:%.1f:%.1f:%.1f:%s", value.NodeID, value.X, value.Y, value.Width, value.Height, value.Value))
			case paintmodel.DrawButton:
				lines = append(lines, fmt.Sprintf("button:%d:%.1f:%.1f:%.1f:%.1f:%s", value.NodeID, value.X, value.Y, value.Width, value.Height, value.Label))
			}
		}
		return true
	}) {
		t.Fatal("InspectPage() failed while painting")
	}
	digest := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(digest[:])
}

func scrollSnapshot(t *testing.T, engine *browser.Browser) string {
	t.Helper()
	var result string
	if !engine.InspectPage(func(page *browser.Page) bool {
		result = fmt.Sprintf("first=%d offset=%d", page.ScrollFirst, page.ScrollOffset)
		return true
	}) {
		t.Fatal("InspectPage() failed while reading scroll")
	}
	return result
}

func assertLifecycleGolden(t *testing.T, actual lifecycleGolden) {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("testdata", "lifecycle.golden.json"))
	if err != nil {
		encoded, _ := json.MarshalIndent(actual, "", "  ")
		t.Fatalf("read lifecycle golden: %v\n--- actual ---\n%s", err, encoded)
	}
	var expected lifecycleGolden
	if err := json.Unmarshal(content, &expected); err != nil {
		t.Fatal(err)
	}
	if actual != expected {
		encoded, _ := json.MarshalIndent(actual, "", "  ")
		t.Fatalf("Persistent App lifecycle visual changed\n--- actual ---\n%s", encoded)
	}
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
