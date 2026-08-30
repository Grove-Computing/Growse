package fetch

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Grove-Computing/Growse/internal/network"
	formapi "github.com/Grove-Computing/Growse/internal/webapi/form"
	urlapi "github.com/Grove-Computing/Growse/internal/webapi/url"
)

func TestFetchResponseMetadataAndBodyHelpers(t *testing.T) {
	finalURL, err := url.Parse("https://example.test/final")
	if err != nil {
		t.Fatal(err)
	}
	api := New(finalURL, func(context.Context, *network.Request) (*network.Response, error) {
		return &network.Response{
			URL: finalURL, StatusCode: http.StatusCreated, Status: "Created", Redirected: true,
			Header: http.Header{"X-Result": []string{"one", "two"}}, Body: []byte(`{"name":"growse"}`),
		}, nil
	})

	responses := make(chan Response, 1)
	api.Fetch(Request{URL: "/data"}, func(result Response) { responses <- result }, func(message string) {
		t.Fatalf("Fetch failure = %q", message)
	})
	response := <-responses
	if response.Status != http.StatusCreated || response.StatusText != "Created" {
		t.Fatalf("status = %d %q", response.Status, response.StatusText)
	}
	if response.URL != finalURL.String() || !response.Redirected {
		t.Fatalf("URL = %q redirected = %t", response.URL, response.Redirected)
	}
	if got := response.Header["X-Result"]; !equalStrings(got, []string{"one", "two"}) {
		t.Fatalf("X-Result = %v", got)
	}
	var decoded map[string]string
	if err := response.JSON(&decoded); err != nil {
		t.Fatalf("JSON() error = %v", err)
	}
	if got, want := decoded["name"], "growse"; got != want {
		t.Fatalf("decoded name = %q, want %q", got, want)
	}
	if _, err := response.Bytes(); !errors.Is(err, ErrBodyConsumed) {
		t.Fatalf("second body consumption error = %v, want ErrBodyConsumed", err)
	}
}

func TestResponseBytesAndTextReturnBody(t *testing.T) {
	for _, test := range []struct {
		name string
		read func(Response) (string, error)
	}{
		{name: "bytes", read: func(response Response) (string, error) {
			value, err := response.Bytes()
			return string(value), err
		}},
		{name: "text", read: func(response Response) (string, error) {
			return response.Text()
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := newResponse(&network.Response{Body: []byte("hello")})
			got, err := test.read(response)
			if err != nil || got != "hello" {
				t.Fatalf("body = %q, error = %v", got, err)
			}
		})
	}
}

func TestResponseHeadersBodyUsedAndInvalidText(t *testing.T) {
	response := newResponse(&network.Response{Header: http.Header{"X-Result": {"one", "two"}}, Body: []byte{0xff}})
	if got, ok := response.Headers.Get("x-result"); !ok || got != "one" {
		t.Fatalf("Headers.Get = %q, %t", got, ok)
	}
	entries := response.Headers.Entries()
	entries[0].Value = "changed"
	if got, _ := response.Headers.Get("X-Result"); got != "one" {
		t.Fatalf("Headers leaked mutation: %q", got)
	}
	if response.BodyUsed() {
		t.Fatal("BodyUsed before consumption")
	}
	if _, err := response.Text(); !errors.Is(err, ErrInvalidText) {
		t.Fatalf("Text error = %v", err)
	}
	if !response.BodyUsed() {
		t.Fatal("BodyUsed after consumption")
	}
}

func TestResponseBodyReaderIsChunkedExclusiveAndCancelable(t *testing.T) {
	body := strings.Repeat("x", MaxStreamChunkSize+17)
	response := newResponse(&network.Response{Body: []byte(body)})
	reader, err := response.Stream()
	if err != nil {
		t.Fatal(err)
	}
	if response.BodyUsed() {
		t.Fatal("locking an undisturbed body marked it used")
	}
	if _, err := response.Stream(); !errors.Is(err, ErrBodyConsumed) {
		t.Fatalf("second Stream error = %v, want ErrBodyConsumed", err)
	}
	chunk, done, err := reader.Read()
	if err != nil || done || len(chunk) != MaxStreamChunkSize || !response.BodyUsed() {
		t.Fatalf("first stream read = %d bytes, done=%t, used=%t, err=%v", len(chunk), done, response.BodyUsed(), err)
	}
	reader.Cancel()
	chunk, done, err = reader.Read()
	if err != nil || !done || len(chunk) != 0 {
		t.Fatalf("canceled stream read = %d bytes, done=%t, err=%v", len(chunk), done, err)
	}
}

func TestFetchRejectsInvalidRequestBeforeSending(t *testing.T) {
	baseURL, err := url.Parse("https://example.test/page")
	if err != nil {
		t.Fatal(err)
	}
	requests := 0
	api := New(baseURL, func(context.Context, *network.Request) (*network.Response, error) {
		requests++
		return &network.Response{}, nil
	})
	tests := []struct {
		name    string
		request Request
	}{
		{name: "unsupported method", request: Request{Method: "TRACE", URL: "/data"}},
		{name: "invalid method token", request: Request{Method: "PO ST", URL: "/data"}},
		{name: "unsupported URL scheme", request: Request{URL: "file:///tmp/data"}},
		{name: "forbidden host", request: Request{URL: "/data", Header: Header{"Host": []string{"other.test"}}}},
		{name: "forbidden cookie case insensitive", request: Request{URL: "/data", Header: Header{"cOoKiE": []string{"session=secret"}}}},
		{name: "forbidden sec prefix", request: Request{URL: "/data", Header: Header{"Sec-Fetch-Site": []string{"same-origin"}}}},
		{name: "invalid header name", request: Request{URL: "/data", Header: Header{"Bad Header": []string{"value"}}}},
		{name: "invalid header value", request: Request{URL: "/data", Header: Header{"X-Test": []string{"safe\r\ninjected"}}}},
		{name: "GET body", request: Request{Method: http.MethodGet, URL: "/data", Body: []byte{}}},
		{name: "HEAD text body", request: Request{Method: http.MethodHead, URL: "/data", Text: "body"}},
		{name: "competing body types", request: Request{URL: "/data", Text: "body", JSON: `{}`}},
		{name: "invalid JSON", request: Request{URL: "/data", JSON: `{`}},
		{name: "invalid text UTF-8", request: Request{URL: "/data", Text: string([]byte{0xff})}},
		{name: "large body", request: Request{URL: "/data", Body: []byte(strings.Repeat("x", maxRequestBodySize+1))}},
		{name: "invalid credentials", request: Request{URL: "/data", Credentials: "always"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			before := requests
			if _, err := api.fetch(context.Background(), test.request); err == nil {
				t.Fatal("fetch() error = nil")
			}
			if requests != before {
				t.Fatalf("network requests = %d, want %d", requests, before)
			}
		})
	}
}

func TestFetchEncodesEachStructuredBodyType(t *testing.T) {
	baseURL, _ := url.Parse("https://example.test/page")
	params, err := urlapi.Parse("tag=go&tag=web+api")
	if err != nil {
		t.Fatal(err)
	}
	formData := formapi.New()
	if err := formData.Append("name", "Growse"); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name               string
		request            Request
		wantBody, wantType string
	}{
		{"bytes", Request{Method: http.MethodPost, URL: "/data", Body: []byte{0, 1}}, string([]byte{0, 1}), ""},
		{"text", Request{Method: http.MethodPost, URL: "/data", Text: "hello"}, "hello", "text/plain;charset=UTF-8"},
		{"JSON", Request{Method: http.MethodPost, URL: "/data", JSON: `{"name":"growse"}`}, `{"name":"growse"}`, "application/json"},
		{"params", Request{Method: http.MethodPost, URL: "/data", Params: params}, "tag=go&tag=web+api", "application/x-www-form-urlencoded;charset=UTF-8"},
		{"form data", Request{Method: http.MethodPost, URL: "/data", FormData: formData}, "name=Growse", formapi.ContentTypeURLEncoded},
		{"explicit content type", Request{Method: http.MethodPost, URL: "/data", Text: "hello", Headers: mustHeaders(t, "Content-Type", "text/custom")}, "hello", "text/custom"},
	} {
		t.Run(test.name, func(t *testing.T) {
			api := New(baseURL, func(_ context.Context, request *network.Request) (*network.Response, error) {
				if got := string(request.Body); got != test.wantBody {
					t.Errorf("body = %q, want %q", got, test.wantBody)
				}
				if got := request.Header.Get("Content-Type"); got != test.wantType {
					t.Errorf("Content-Type = %q, want %q", got, test.wantType)
				}
				return &network.Response{}, nil
			})
			if _, err := api.fetch(context.Background(), test.request); err != nil {
				t.Fatalf("fetch() error = %v", err)
			}
		})
	}
}

func TestRequestDataDoesNotMutatePageURLOrInjectCookies(t *testing.T) {
	baseURL, _ := url.Parse("https://example.test/page?current=1")
	params, err := urlapi.Parse("tag=web+api")
	if err != nil {
		t.Fatal(err)
	}
	formData := formapi.New()
	if err := formData.Append("name", "Growse"); err != nil {
		t.Fatal(err)
	}
	api := New(baseURL, func(_ context.Context, request *network.Request) (*network.Response, error) {
		if got := request.Header.Get("Cookie"); got != "" {
			t.Errorf("request injected Cookie header %q", got)
		}
		if got, want := baseURL.String(), "https://example.test/page?current=1"; got != want {
			t.Errorf("page URL mutated to %q", got)
		}
		return &network.Response{}, nil
	})
	if _, err := api.fetch(context.Background(), Request{Method: http.MethodPost, URL: "/submit", Params: params}); err != nil {
		t.Fatalf("params fetch error = %v", err)
	}
	if _, err := api.fetch(context.Background(), Request{Method: http.MethodPost, URL: "/submit", FormData: formData}); err != nil {
		t.Fatalf("FormData fetch error = %v", err)
	}
	if got, err := params.Encode(); err != nil || got != "tag=web+api" {
		t.Fatalf("params mutated: %q, %v", got, err)
	}
	if got, err := formData.Encode(); err != nil || got != "name=Growse" {
		t.Fatalf("FormData mutated: %q, %v", got, err)
	}
}

func mustHeaders(t *testing.T, name, value string) *Headers {
	t.Helper()
	headers := NewHeaders()
	if err := headers.Append(name, value); err != nil {
		t.Fatal(err)
	}
	return headers
}

func TestFetchDistinguishesHTTPErrorStatusFromNetworkError(t *testing.T) {
	baseURL, err := url.Parse("https://example.test/page")
	if err != nil {
		t.Fatal(err)
	}
	t.Run("HTTP status uses success callback", func(t *testing.T) {
		api := New(baseURL, func(context.Context, *network.Request) (*network.Response, error) {
			return &network.Response{StatusCode: http.StatusServiceUnavailable, Status: "Service Unavailable"}, nil
		})
		success := make(chan Response, 1)
		failure := make(chan string, 1)
		api.Fetch(Request{URL: "/status"}, func(response Response) { success <- response }, func(message string) { failure <- message })
		select {
		case response := <-success:
			if response.Status != http.StatusServiceUnavailable {
				t.Fatalf("status = %d", response.Status)
			}
		case message := <-failure:
			t.Fatalf("HTTP status used failure callback: %s", message)
		}
	})
	t.Run("network failure uses error callback", func(t *testing.T) {
		api := New(baseURL, func(context.Context, *network.Request) (*network.Response, error) {
			return nil, errors.New("connection reset")
		})
		success := make(chan Response, 1)
		failure := make(chan string, 1)
		api.Fetch(Request{URL: "/data"}, func(response Response) { success <- response }, func(message string) { failure <- message })
		select {
		case response := <-success:
			t.Fatalf("network failure used success callback: %#v", response)
		case message := <-failure:
			if message != "connection reset" {
				t.Fatalf("failure = %q", message)
			}
		}
	})
}

func TestFetchAbortDeliversOneFailureAndCancelsRequest(t *testing.T) {
	baseURL, _ := url.Parse("https://example.test/page")
	t.Run("before start", func(t *testing.T) {
		controller := NewAbortController()
		controller.Abort()
		called := 0
		New(baseURL, func(context.Context, *network.Request) (*network.Response, error) {
			t.Fatal("request started")
			return nil, nil
		}).Fetch(Request{URL: "/data", Signal: controller.Signal()}, nil, func(message string) {
			called++
			if message != "AbortError: Fetch was aborted" {
				t.Errorf("failure = %q", message)
			}
		})
		if called != 1 {
			t.Fatalf("callbacks = %d", called)
		}
	})
	t.Run("in flight", func(t *testing.T) {
		controller := NewAbortController()
		started := make(chan struct{})
		failure := make(chan string, 1)
		api := New(baseURL, func(ctx context.Context, _ *network.Request) (*network.Response, error) {
			close(started)
			<-ctx.Done()
			return nil, ctx.Err()
		})
		api.Fetch(Request{URL: "/data", Signal: controller.Signal()}, func(Response) { t.Error("success called") }, func(message string) { failure <- message })
		<-started
		controller.Abort()
		if got := <-failure; got != "AbortError: Fetch was aborted" {
			t.Fatalf("failure = %q", got)
		}
	})
}

func TestFetchTimeoutUsesInjectableClockAndDeliversOneFailure(t *testing.T) {
	baseURL, _ := url.Parse("https://example.test/page")
	clock := &fetchFakeClock{}
	started := make(chan struct{})
	failure := make(chan string, 2)
	api := NewPageWithClock(context.Background(), baseURL, func(ctx context.Context, _ *network.Request) (*network.Response, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	}, func(callback func()) bool { callback(); return true }, clock)
	api.Fetch(Request{URL: "/slow", Timeout: time.Second}, func(Response) { t.Error("success called") }, func(message string) { failure <- message })
	<-started
	clock.Fire()
	if got := <-failure; got != "TimeoutError: Fetch timed out" {
		t.Fatalf("failure = %q", got)
	}
	select {
	case extra := <-failure:
		t.Fatalf("extra callback = %q", extra)
	default:
	}
}

func TestFetchDiscardsLateResponseAfterAbort(t *testing.T) {
	baseURL, _ := url.Parse("https://example.test/page")
	controller := NewAbortController()
	started := make(chan struct{})
	release := make(chan struct{})
	success := make(chan struct{}, 1)
	failure := make(chan string, 1)
	api := New(baseURL, func(context.Context, *network.Request) (*network.Response, error) {
		close(started)
		<-release
		return &network.Response{StatusCode: http.StatusOK}, nil
	})
	api.Fetch(Request{URL: "/late", Signal: controller.Signal()}, func(Response) { success <- struct{}{} }, func(message string) { failure <- message })
	<-started
	controller.Abort()
	close(release)
	if got := <-failure; got != "AbortError: Fetch was aborted" {
		t.Fatalf("failure = %q", got)
	}
	select {
	case <-success:
		t.Fatal("late response delivered success")
	default:
	}
}

type fetchFakeClock struct {
	callback func()
	stopped  bool
}

func (clock *fetchFakeClock) AfterFunc(_ time.Duration, callback func()) Timer {
	clock.callback = callback
	return fetchFakeTimer{clock}
}
func (clock *fetchFakeClock) Fire() {
	if clock.callback != nil && !clock.stopped {
		clock.callback()
	}
}

type fetchFakeTimer struct{ clock *fetchFakeClock }

func (timer fetchFakeTimer) Stop() bool { timer.clock.stopped = true; return true }

func TestConcurrentFetchesDeliverCallbacksInCompletionOrder(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	queue := make(chan func(), 2)
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		for {
			select {
			case callback := <-queue:
				callback()
			case <-ctx.Done():
				return
			}
		}
	}()
	baseURL, err := url.Parse("https://example.test/page")
	if err != nil {
		t.Fatal(err)
	}
	firstStarted := make(chan struct{})
	secondStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	releaseSecond := make(chan struct{})
	api := NewPage(ctx, baseURL, func(_ context.Context, request *network.Request) (*network.Response, error) {
		switch request.URL.Path {
		case "/first":
			close(firstStarted)
			<-releaseFirst
		case "/second":
			close(secondStarted)
			<-releaseSecond
		}
		return &network.Response{URL: request.URL, StatusCode: http.StatusOK}, nil
	}, func(callback func()) bool {
		select {
		case queue <- callback:
			return true
		case <-ctx.Done():
			return false
		}
	})

	completed := make(chan struct{}, 2)
	order := make([]string, 0, 2)
	failure := func(message string) { t.Errorf("Fetch failure = %q", message) }
	api.Fetch(Request{URL: "/first"}, func(Response) {
		order = append(order, "first")
		completed <- struct{}{}
	}, failure)
	api.Fetch(Request{URL: "/second"}, func(Response) {
		order = append(order, "second")
		completed <- struct{}{}
	}, failure)
	<-firstStarted
	<-secondStarted
	close(releaseSecond)
	<-completed
	close(releaseFirst)
	<-completed
	if got, want := strings.Join(order, ","), "second,first"; got != want {
		t.Fatalf("callback order = %q, want %q", got, want)
	}
	cancel()
	<-workerDone
}

func TestFetchRejectsRequestsAbovePerPageConcurrencyLimit(t *testing.T) {
	baseURL, _ := url.Parse("https://example.test/page")
	release := make(chan struct{})
	started := make(chan struct{}, 16)
	failures := make(chan string, 1)
	api := New(baseURL, func(context.Context, *network.Request) (*network.Response, error) {
		started <- struct{}{}
		<-release
		return &network.Response{}, nil
	})
	for index := 0; index < 16; index++ {
		api.Fetch(Request{URL: "/held"}, nil, nil)
	}
	for index := 0; index < 16; index++ {
		<-started
	}
	api.Fetch(Request{URL: "/rejected"}, nil, func(message string) { failures <- message })
	if got := <-failures; got != "QuotaError: Fetch concurrency limit reached" {
		t.Fatalf("failure = %q", got)
	}
	close(release)
	api.Close()
}

func TestFetchRejectsRequestsAboveSharedSessionLimit(t *testing.T) {
	baseURL, _ := url.Parse("https://example.test/page")
	limiter := NewLimiter(1)
	release := make(chan struct{})
	started := make(chan struct{})
	failure := make(chan string, 1)
	first := New(baseURL, func(context.Context, *network.Request) (*network.Response, error) {
		close(started)
		<-release
		return &network.Response{}, nil
	})
	second := New(baseURL, func(context.Context, *network.Request) (*network.Response, error) {
		t.Fatal("second request started")
		return nil, nil
	})
	first.SetLimiter(limiter)
	second.SetLimiter(limiter)
	first.Fetch(Request{URL: "/first"}, nil, nil)
	<-started
	second.Fetch(Request{URL: "/second"}, nil, func(message string) { failure <- message })
	if got := <-failure; got != "QuotaError: Session Fetch concurrency limit reached" {
		t.Fatalf("failure = %q", got)
	}
	close(release)
	first.Close()
}

func TestCloseWaitsForCanceledFetchOperation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	finished := make(chan struct{})
	baseURL, _ := url.Parse("https://example.test/page")
	api := NewPage(ctx, baseURL, func(ctx context.Context, _ *network.Request) (*network.Response, error) {
		close(started)
		<-ctx.Done()
		close(finished)
		return nil, ctx.Err()
	}, func(func()) bool { return false })
	api.Fetch(Request{URL: "/slow"}, nil, nil)
	<-started
	cancel()
	api.Close()
	select {
	case <-finished:
	default:
		t.Fatal("Close returned before Fetch goroutine released its references")
	}
	if api.ctx != nil || api.baseURL != nil || api.do != nil || api.enqueue != nil {
		t.Fatal("Close retained Fetch executor or Page references")
	}
	api.Fetch(Request{URL: "/ignored"}, nil, nil)
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
