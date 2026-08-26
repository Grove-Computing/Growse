package serviceworker

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Grove-Computing/Growse/internal/network"
)

func TestServiceWorkerProcessPersistsCrashesIdlesAndRestarts(t *testing.T) {
	manager := NewManager()
	manager.idleTimeout = 30 * time.Millisecond
	t.Cleanup(func() { _ = manager.Close() })
	clientURL := parseServiceWorkerURL(t, "https://worker.example/app/page")
	scriptURL := parseServiceWorkerURL(t, "https://worker.example/app/sw.js")
	source := []byte(`
		let requests = 0;
		self.addEventListener("install", () => self.skipWaiting());
		self.addEventListener("activate", () => clients.claim());
		self.addEventListener("fetch", event => event.respondWith(new Response(String(++requests))));`)
	registration, err := manager.Register(context.Background(), clientURL, scriptURL.String(), "", scriptResponse(scriptURL, &source))
	if err != nil {
		t.Fatal(err)
	}
	key := registrationKey("https://worker.example", parseServiceWorkerURL(t, registration.Scope))
	manager.workerMu.Lock()
	process := manager.workers[key]
	starts := manager.workerStarts
	manager.workerMu.Unlock()
	if process == nil || process.command.Process.Pid == os.Getpid() || len(process.constraints) == 0 || starts != 1 {
		t.Fatalf("isolated process = %#v, starts=%d", process, starts)
	}
	request := &network.Request{Method: http.MethodGet, URL: parseServiceWorkerURL(t, "https://worker.example/app/data"), Kind: network.RequestNavigation}
	fallback := func(context.Context, *network.Request) (*network.Response, error) {
		return nil, errors.New("fallback must not run")
	}
	first, err := manager.DispatchFetch(context.Background(), request, fallback)
	if err != nil {
		t.Fatal(err)
	}
	second, err := manager.DispatchFetch(context.Background(), request, fallback)
	if err != nil || string(first.Body) != "1" || string(second.Body) != "2" {
		t.Fatalf("persistent worker responses = %q, %q, %v", first.Body, second.Body, err)
	}
	if err := manager.crashWorkerForTest(key); err != nil {
		t.Fatal(err)
	}
	restarted, err := manager.DispatchFetch(context.Background(), request, fallback)
	if err != nil || string(restarted.Body) != "1" {
		t.Fatalf("crash restart response = %q, %v", restarted.Body, err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		manager.workerMu.Lock()
		count := len(manager.workers)
		manager.workerMu.Unlock()
		if count == 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	manager.workerMu.Lock()
	idleCount := len(manager.workers)
	startsBeforeIdleRestart := manager.workerStarts
	manager.workerMu.Unlock()
	if idleCount != 0 {
		t.Fatal("idle Service Worker process did not stop")
	}
	afterIdle, err := manager.DispatchFetch(context.Background(), request, fallback)
	manager.workerMu.Lock()
	startsAfterIdleRestart := manager.workerStarts
	manager.workerMu.Unlock()
	if err != nil || string(afterIdle.Body) != "1" || startsAfterIdleRestart != startsBeforeIdleRestart+1 {
		t.Fatalf("idle restart = response:%q error:%v starts:%d->%d", afterIdle.Body, err, startsBeforeIdleRestart, startsAfterIdleRestart)
	}
}

func TestServiceWorkerInternalFetchDoesNotRecurseAndSurvivesPageCloseCancellation(t *testing.T) {
	manager := NewManager()
	t.Cleanup(func() { _ = manager.Close() })
	clientURL := parseServiceWorkerURL(t, "https://cancel.example/app/page")
	scriptURL := parseServiceWorkerURL(t, "https://cancel.example/app/sw.js")
	source := []byte(`
		self.addEventListener("install", () => self.skipWaiting());
		self.addEventListener("activate", () => clients.claim());
		self.addEventListener("fetch", event => event.respondWith(fetch(event.request)));`)
	if _, err := manager.Register(context.Background(), clientURL, scriptURL.String(), "", scriptResponse(scriptURL, &source)); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	fallbackCalls := 0
	fallback := func(ctx context.Context, request *network.Request) (*network.Response, error) {
		fallbackCalls++
		if request.Kind != network.RequestServiceWorkerFetch {
			return nil, errors.New("recursive service worker fetch kind was not isolated")
		}
		close(started)
		<-release
		if ctx.Err() != nil {
			return nil, errors.New("pending service worker event inherited caller cancellation")
		}
		return &network.Response{URL: request.URL, StatusCode: http.StatusOK, Body: []byte("survived")}, nil
	}
	requestContext, cancel := context.WithCancel(context.Background())
	result := make(chan struct {
		response *network.Response
		err      error
	}, 1)
	go func() {
		response, err := manager.DispatchFetch(requestContext, &network.Request{
			Method: http.MethodGet, URL: parseServiceWorkerURL(t, "https://cancel.example/app/data"), Kind: network.RequestFetch, SiteURL: clientURL,
		}, fallback)
		result <- struct {
			response *network.Response
			err      error
		}{response: response, err: err}
	}()
	<-started
	cancel()
	select {
	case value := <-result:
		t.Fatalf("Service Worker event stopped with caller: %#v", value)
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	value := <-result
	if value.err != nil || string(value.response.Body) != "survived" || fallbackCalls != 1 {
		t.Fatalf("detached event = response:%#v error:%v fallbacks:%d", value.response, value.err, fallbackCalls)
	}
}

func TestServiceWorkerTimeoutIsContainedAndStopsOnlyTargetProcess(t *testing.T) {
	manager := NewManager()
	t.Cleanup(func() { _ = manager.Close() })
	hangingClient := parseServiceWorkerURL(t, "https://hang.example/app/page")
	hangingScript := parseServiceWorkerURL(t, "https://hang.example/app/sw.js")
	hangingSource := []byte(`
		self.addEventListener("install", () => self.skipWaiting());
		self.addEventListener("activate", () => clients.claim());
		self.addEventListener("fetch", () => { while (true) {} });`)
	if _, err := manager.Register(context.Background(), hangingClient, hangingScript.String(), "", scriptResponse(hangingScript, &hangingSource)); err != nil {
		t.Fatal(err)
	}
	manager.taskTimeout = 50 * time.Millisecond
	_, err := manager.DispatchFetch(context.Background(), &network.Request{
		Method: http.MethodGet, URL: parseServiceWorkerURL(t, "https://hang.example/app/data"), Kind: network.RequestNavigation,
	}, func(context.Context, *network.Request) (*network.Response, error) {
		return nil, errors.New("fallback must not run")
	})
	if !errors.Is(err, ErrServiceWorkerProcess) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout error = %v", err)
	}
	manager.workerMu.Lock()
	remaining := len(manager.workers)
	manager.workerMu.Unlock()
	if remaining != 0 {
		t.Fatalf("timed out worker count = %d", remaining)
	}
	manager.taskTimeout = defaultServiceWorkerTaskTimeout

	healthyClient := parseServiceWorkerURL(t, "https://healthy-worker.example/app/page")
	healthyScript := parseServiceWorkerURL(t, "https://healthy-worker.example/app/sw.js")
	healthySource := []byte(`
		self.addEventListener("install", () => self.skipWaiting());
		self.addEventListener("activate", () => clients.claim());
		self.addEventListener("fetch", event => event.respondWith(new Response("healthy")));`)
	if _, err := manager.Register(context.Background(), healthyClient, healthyScript.String(), "", scriptResponse(healthyScript, &healthySource)); err != nil {
		t.Fatal(err)
	}
	response, err := manager.DispatchFetch(context.Background(), &network.Request{
		Method: http.MethodGet, URL: parseServiceWorkerURL(t, "https://healthy-worker.example/app/data"), Kind: network.RequestNavigation,
	}, func(context.Context, *network.Request) (*network.Response, error) {
		return nil, errors.New("fallback must not run")
	})
	if err != nil || string(response.Body) != "healthy" || strings.Contains(errString(err), "hang.example") {
		t.Fatalf("healthy Origin after timeout = %#v, %v", response, err)
	}
}

func TestServiceWorkerProcessChunksPayloadsWithinIPCMessageLimit(t *testing.T) {
	manager := NewManager()
	t.Cleanup(func() { _ = manager.Close() })
	clientURL := parseServiceWorkerURL(t, "https://chunks.example/app/page")
	scriptURL := parseServiceWorkerURL(t, "https://chunks.example/app/sw.js")
	source := []byte(`
		self.addEventListener("install", () => self.skipWaiting());
		self.addEventListener("activate", () => clients.claim());
		self.addEventListener("fetch", event => event.respondWith(new Response("x".repeat(2 * 1024 * 1024))));
		/*` + strings.Repeat("padding", 170_000) + `*/`)
	if len(source) <= 1<<20 || len(source) > MaxWorkerScriptBytes {
		t.Fatalf("test worker source size = %d", len(source))
	}
	if _, err := manager.Register(context.Background(), clientURL, scriptURL.String(), "", scriptResponse(scriptURL, &source)); err != nil {
		t.Fatal(err)
	}
	response, err := manager.DispatchFetch(context.Background(), &network.Request{
		Method: http.MethodGet, URL: parseServiceWorkerURL(t, "https://chunks.example/app/large"), Kind: network.RequestNavigation,
	}, func(context.Context, *network.Request) (*network.Response, error) {
		return nil, errors.New("fallback must not run")
	})
	if err != nil || len(response.Body) != 2<<20 {
		t.Fatalf("chunked response bytes = %d, error = %v", len(response.Body), err)
	}
}

func scriptResponse(scriptURL *url.URL, source *[]byte) FetchScript {
	return func(context.Context, *network.Request) (*network.Response, error) {
		return &network.Response{URL: scriptURL, StatusCode: http.StatusOK, ContentType: "text/javascript", Body: append([]byte(nil), (*source)...)}, nil
	}
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
