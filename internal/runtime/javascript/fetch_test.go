package javascript

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Grove-Computing/Growse/internal/network"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
	fetchapi "github.com/Grove-Computing/Growse/internal/webapi/fetch"
	"github.com/dop251/goja"
)

func TestFetchPromiseResolvesHTTPResponseAndConsumesBodyOnce(t *testing.T) {
	baseURL, _ := url.Parse("https://example.test/page")
	requestSeen := make(chan *network.Request, 1)
	messages := make(chan string, 4)
	environment := runtimemodel.Environment{
		BaseURL:      baseURL,
		FetchLimiter: fetchapi.NewLimiter(8),
		Fetch: func(_ context.Context, request *network.Request) (*network.Response, error) {
			requestSeen <- request
			return &network.Response{
				URL: request.URL, StatusCode: http.StatusNotFound, Status: "Not Found", Redirected: true,
				Header: http.Header{"Content-Type": {"application/json"}, "X-Result": {"yes"}},
				Body:   []byte(`{"name":"Growse"}`),
			}, nil
		},
		ConsoleRecord: func(_, message string) { messages <- message },
	}
	runtime := New()
	t.Cleanup(func() { _ = runtime.Stop() })
	source := `
		fetch("/api", {
			method: "POST",
			headers: {"X-Test": "adapter"},
			body: "payload",
			credentials: "include"
		}).then(function (response) {
			console.log([response.status, response.statusText, response.ok, response.redirected, response.headers.get("X-Result")].join("|"));
			return response.json().then(function (body) {
				console.log(body.name + "|" + response.bodyUsed);
				return response.text();
			});
		}).then(function () {
			console.error("second body read unexpectedly succeeded");
		}, function (error) {
			console.log("body:" + error.message);
		});`
	startJavaScriptRuntime(t, runtime, source, environment)

	request := receiveRequest(t, requestSeen)
	if request.Method != http.MethodPost || request.URL.String() != "https://example.test/api" || string(request.Body) != "payload" ||
		request.Header.Get("X-Test") != "adapter" || request.Credentials != network.CredentialsInclude {
		t.Fatalf("Fetch request = %#v, want POST/include adapter request", request)
	}
	want := []string{
		"404|Not Found|false|true|yes",
		"Growse|true",
		"body:response body has already been consumed",
	}
	for _, expected := range want {
		if got := receiveMessage(t, messages); got != expected {
			t.Fatalf("Fetch console = %q, want %q", got, expected)
		}
	}
}

func TestFetchPromiseRejectsNetworkCORSAndAbort(t *testing.T) {
	baseURL, _ := url.Parse("https://example.test/page")
	abortStarted := make(chan struct{})
	messages := make(chan string, 4)
	environment := runtimemodel.Environment{
		BaseURL:      baseURL,
		FetchLimiter: fetchapi.NewLimiter(8),
		Fetch: func(ctx context.Context, request *network.Request) (*network.Response, error) {
			switch request.URL.Path {
			case "/network":
				return nil, errors.New("network unavailable")
			case "/cors":
				return nil, network.ErrCORS
			case "/abort":
				close(abortStarted)
				<-ctx.Done()
				return nil, ctx.Err()
			default:
				return nil, errors.New("unexpected Fetch path")
			}
		},
		ConsoleRecord: func(_, message string) { messages <- message },
	}
	runtime := New()
	t.Cleanup(func() { _ = runtime.Stop() })
	source := `
		fetch("/network").catch(function (error) { console.log("network:" + error.message); });
		fetch("/cors").catch(function (error) { console.log("cors:" + error.message); });
		var controller = new AbortController();
		fetch("/abort", {signal: controller.signal}).catch(function (error) { console.log(error.name + ":" + error.message); });`
	startJavaScriptRuntime(t, runtime, source, environment)
	select {
	case <-abortStarted:
	case <-time.After(time.Second):
		t.Fatal("abortable Fetch did not start")
	}
	if err := runtime.runSync(context.Background(), func(vm *goja.Runtime) error {
		controller := vm.Get("controller").ToObject(vm)
		abort, _ := goja.AssertFunction(controller.Get("abort"))
		_, err := abort(controller)
		return err
	}); err != nil {
		t.Fatalf("AbortController.abort(): %v", err)
	}

	got := []string{receiveMessage(t, messages), receiveMessage(t, messages), receiveMessage(t, messages)}
	joined := strings.Join(got, "\n")
	for _, expected := range []string{"network:network unavailable", "cors:CORS policy rejected the response", "AbortError:Fetch was aborted"} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("Fetch rejection records = %v, missing %q", got, expected)
		}
	}
}

type javascriptFetchClock struct {
	mu         sync.Mutex
	callback   func()
	registered chan struct{}
}

func (clock *javascriptFetchClock) AfterFunc(_ time.Duration, callback func()) fetchapi.Timer {
	clock.mu.Lock()
	clock.callback = callback
	clock.mu.Unlock()
	select {
	case clock.registered <- struct{}{}:
	default:
	}
	return javascriptFetchTimer{}
}

func (clock *javascriptFetchClock) Fire() {
	clock.mu.Lock()
	callback := clock.callback
	clock.mu.Unlock()
	if callback != nil {
		callback()
	}
}

type javascriptFetchTimer struct{}

func (javascriptFetchTimer) Stop() bool { return true }

func TestFetchPromiseRejectsTimeoutDeterministically(t *testing.T) {
	baseURL, _ := url.Parse("https://example.test/page")
	clock := &javascriptFetchClock{registered: make(chan struct{}, 1)}
	started := make(chan struct{})
	messages := make(chan string, 1)
	runtime := New()
	runtime.fetchClock = clock
	t.Cleanup(func() { _ = runtime.Stop() })
	environment := runtimemodel.Environment{
		BaseURL:      baseURL,
		FetchLimiter: fetchapi.NewLimiter(8),
		Fetch: func(ctx context.Context, _ *network.Request) (*network.Response, error) {
			close(started)
			<-ctx.Done()
			return nil, ctx.Err()
		},
		ConsoleRecord: func(_, message string) { messages <- message },
	}
	startJavaScriptRuntime(t, runtime, `fetch("/timeout", {timeout: 25}).catch(function (error) { console.log(error.name + ":" + error.message); });`, environment)
	select {
	case <-clock.registered:
	case <-time.After(time.Second):
		t.Fatal("Fetch timeout was not registered")
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("timed Fetch did not start")
	}
	clock.Fire()
	if got, want := receiveMessage(t, messages), "TimeoutError:Fetch timed out"; got != want {
		t.Fatalf("timeout rejection = %q, want %q", got, want)
	}
}

func TestFetchPageCloseCancelsAndDropsLateCompletion(t *testing.T) {
	baseURL, _ := url.Parse("https://example.test/page")
	started := make(chan struct{})
	finished := make(chan struct{})
	var messages []string
	environment := runtimemodel.Environment{
		BaseURL:      baseURL,
		FetchLimiter: fetchapi.NewLimiter(8),
		Fetch: func(ctx context.Context, _ *network.Request) (*network.Response, error) {
			close(started)
			<-ctx.Done()
			close(finished)
			return nil, ctx.Err()
		},
		ConsoleRecord: func(_, message string) { messages = append(messages, message) },
	}
	runtime := New()
	startJavaScriptRuntime(t, runtime, `fetch("/slow").then(function () { console.log("late success"); }, function () { console.log("late failure"); });`, environment)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("slow Fetch did not start")
	}
	if err := runtime.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	select {
	case <-finished:
	default:
		t.Fatal("Stop() returned before Fetch released Page references")
	}
	if len(messages) != 0 {
		t.Fatalf("Page close delivered Fetch completion: %v", messages)
	}
}

func receiveRequest(t *testing.T, requests <-chan *network.Request) *network.Request {
	t.Helper()
	select {
	case request := <-requests:
		return request
	case <-time.After(time.Second):
		t.Fatal("Fetch request was not received")
		return nil
	}
}

func receiveMessage(t *testing.T, messages <-chan string) string {
	t.Helper()
	select {
	case message := <-messages:
		return message
	case <-time.After(time.Second):
		t.Fatal("JavaScript callback message was not received")
		return ""
	}
}
