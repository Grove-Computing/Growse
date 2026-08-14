package network

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestClientGetHTML(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "text/html; charset=utf-8")
		response.WriteHeader(http.StatusOK)
		_, _ = response.Write([]byte("<h1>Hello</h1>"))
	}))
	defer server.Close()

	client := NewClient()
	result, err := client.Get(context.Background(), mustParseURL(t, server.URL))
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got, want := string(result.Body), "<h1>Hello</h1>"; got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
	if got, want := result.ContentType, "text/html; charset=utf-8"; got != want {
		t.Fatalf("content type = %q, want %q", got, want)
	}
}

func TestClientReusesFreshResponseWithoutNetworkRequest(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		requests++
		response.Header().Set("Cache-Control", "max-age=60")
		response.Header().Set("Content-Type", "text/plain")
		_, _ = response.Write([]byte("cached"))
	}))
	defer server.Close()
	client := NewClientWithLimits(server.Client(), 1024)
	target := mustParseURL(t, server.URL+"/resource")
	for range 2 {
		result, err := client.Get(context.Background(), target)
		if err != nil || string(result.Body) != "cached" {
			t.Fatalf("Get() = (%v, %v)", result, err)
		}
	}
	if requests != 1 {
		t.Fatalf("Network requests = %d, want 1", requests)
	}
}

func TestClientRequestNoCacheBypassesFreshResponse(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		requests++
		response.Header().Set("Cache-Control", "max-age=60")
		_, _ = response.Write([]byte("version"))
	}))
	defer server.Close()
	client := NewClientWithLimits(server.Client(), 1024)
	target := mustParseURL(t, server.URL+"/resource")
	if _, err := client.Get(context.Background(), target); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Do(context.Background(), &Request{
		Method: http.MethodGet,
		URL:    target,
		Header: http.Header{"Cache-Control": []string{"no-cache"}},
	}); err != nil {
		t.Fatal(err)
	}
	if requests != 2 {
		t.Fatalf("Network requests = %d, want 2", requests)
	}
}

func TestSuccessfulUnsafeRequestInvalidatesRelatedSameOriginEntries(t *testing.T) {
	itemVersion := 1
	targetVersion := 1
	getRequests := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodPost {
			itemVersion++
			targetVersion++
			response.Header().Set("Content-Location", "/target")
			response.WriteHeader(http.StatusNoContent)
			return
		}
		getRequests[request.URL.Path]++
		response.Header().Set("Cache-Control", "max-age=60")
		if request.URL.Path == "/target" {
			_, _ = fmt.Fprintf(response, "target-%d", targetVersion)
			return
		}
		_, _ = fmt.Fprintf(response, "item-%d", itemVersion)
	}))
	defer server.Close()
	client := NewClientWithLimits(server.Client(), 1024)
	itemURL := mustParseURL(t, server.URL+"/item")
	targetURL := mustParseURL(t, server.URL+"/target")
	for _, target := range []*url.URL{itemURL, targetURL} {
		if _, err := client.Get(context.Background(), target); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := client.Do(context.Background(), &Request{Method: http.MethodPost, URL: itemURL}); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		target *url.URL
		body   string
	}{
		{target: itemURL, body: "item-2"},
		{target: targetURL, body: "target-2"},
	} {
		response, err := client.Get(context.Background(), test.target)
		if err != nil {
			t.Fatal(err)
		}
		if got := string(response.Body); got != test.body {
			t.Errorf("GET %s body = %q, want %q", test.target.Path, got, test.body)
		}
	}
	if getRequests["/item"] != 2 || getRequests["/target"] != 2 {
		t.Fatalf("GET requests after invalidation = %v, want item:2 target:2", getRequests)
	}
}

func TestUnsafeRequestDoesNotInvalidateCrossOriginLocation(t *testing.T) {
	targetRequests := 0
	targetServer := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		targetRequests++
		response.Header().Set("Cache-Control", "max-age=60")
		_, _ = response.Write([]byte("target"))
	}))
	defer targetServer.Close()
	mutationServer := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Location", targetServer.URL+"/target")
		response.WriteHeader(http.StatusNoContent)
	}))
	defer mutationServer.Close()
	client := NewClientWithLimits(targetServer.Client(), 1024)
	targetURL := mustParseURL(t, targetServer.URL+"/target")
	if _, err := client.Get(context.Background(), targetURL); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Do(context.Background(), &Request{
		Method: http.MethodPost,
		URL:    mustParseURL(t, mutationServer.URL+"/mutation"),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Get(context.Background(), targetURL); err != nil {
		t.Fatal(err)
	}
	if targetRequests != 1 {
		t.Fatalf("cross-origin target requests = %d, want 1 cached request", targetRequests)
	}
}

func TestClientConditionallyRevalidatesStaleResponse(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		requests++
		response.Header().Set("Cache-Control", "no-cache")
		response.Header().Set("ETag", `"v1"`)
		if requests == 2 && request.Header.Get("If-None-Match") != `"v1"` {
			t.Errorf("If-None-Match = %q", request.Header.Get("If-None-Match"))
		}
		_, _ = response.Write([]byte("version"))
	}))
	defer server.Close()
	client := NewClientWithLimits(server.Client(), 1024)
	target := mustParseURL(t, server.URL+"/data")
	if _, err := client.Get(context.Background(), target); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Get(context.Background(), target); err != nil {
		t.Fatal(err)
	}
	if requests != 2 {
		t.Fatalf("Network requests = %d, want 2", requests)
	}
}

func TestClientMerges304AndReusesStoredBody(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		requests++
		if requests == 1 {
			response.Header().Set("Cache-Control", "no-cache")
			response.Header().Set("ETag", `"v1"`)
			response.Header().Set("Content-Type", "text/plain")
			_, _ = response.Write([]byte("stored-body"))
			return
		}
		if request.Header.Get("If-None-Match") != `"v1"` {
			t.Errorf("If-None-Match = %q", request.Header.Get("If-None-Match"))
		}
		response.Header().Set("Cache-Control", "max-age=60")
		response.Header().Set("X-Revalidated", "yes")
		response.WriteHeader(http.StatusNotModified)
	}))
	defer server.Close()
	client := NewClientWithLimits(server.Client(), 1024)
	target := mustParseURL(t, server.URL+"/data")
	for range 3 {
		result, err := client.Get(context.Background(), target)
		if err != nil || result.StatusCode != http.StatusOK || string(result.Body) != "stored-body" {
			t.Fatalf("Get() = (%#v, %v)", result, err)
		}
	}
	if requests != 2 {
		t.Fatalf("Network requests = %d, want 2", requests)
	}
}

func TestClientRejects304WithoutStoredEntry(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusNotModified)
	}))
	defer server.Close()
	_, err := NewClientWithLimits(server.Client(), 1024).Get(context.Background(), mustParseURL(t, server.URL))
	if !errors.Is(err, ErrCacheValidation) {
		t.Fatalf("Get() error = %v, want ErrCacheValidation", err)
	}
}

func TestClientSharesCachePolicyAcrossRequestKinds(t *testing.T) {
	counts := make(map[string]int)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		counts[request.URL.Path]++
		response.Header().Set("Cache-Control", "max-age=60")
		response.Header().Set("Access-Control-Allow-Origin", "*")
		_, _ = response.Write([]byte(request.URL.Path))
	}))
	defer server.Close()
	client := NewClientWithLimits(server.Client(), 1024)
	siteURL := mustParseURL(t, server.URL+"/page")
	requests := []*Request{
		{Method: http.MethodGet, URL: mustParseURL(t, server.URL+"/navigation"), Kind: RequestNavigation},
		{Method: http.MethodGet, URL: mustParseURL(t, server.URL+"/stylesheet"), SiteURL: siteURL, Kind: RequestSubresource},
		{Method: http.MethodGet, URL: mustParseURL(t, server.URL+"/image"), SiteURL: siteURL, Kind: RequestSubresource},
		{Method: http.MethodGet, URL: mustParseURL(t, server.URL+"/webgo"), SiteURL: siteURL, Kind: RequestSubresource},
		{Method: http.MethodGet, URL: mustParseURL(t, server.URL+"/fetch"), SiteURL: siteURL, Kind: RequestFetch, Credentials: CredentialsOmit},
	}
	for _, request := range requests {
		for range 2 {
			if _, err := client.Do(context.Background(), request); err != nil {
				t.Fatalf("Do(%s) error = %v", request.URL.Path, err)
			}
		}
		if counts[request.URL.Path] != 1 {
			t.Fatalf("%s Network requests = %d, want 1", request.URL.Path, counts[request.URL.Path])
		}
	}
}

func TestClientDoSendsMethodHeadersAndBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		if request.Method != http.MethodPost || request.Header.Get("Content-Type") != "application/x-www-form-urlencoded" || string(body) != "name=growse" {
			t.Errorf("request method=%s content-type=%q body=%q", request.Method, request.Header.Get("Content-Type"), body)
		}
		response.Header().Set("Content-Type", "text/html")
		_, _ = response.Write([]byte("<title>Saved</title>"))
	}))
	defer server.Close()

	result, err := NewClientWithLimits(server.Client(), 1024).Do(context.Background(), &Request{
		Method: http.MethodPost, URL: mustParseURL(t, server.URL), Body: []byte("name=growse"),
		Header: http.Header{"Content-Type": []string{"application/x-www-form-urlencoded"}},
	})
	if err != nil || string(result.Body) != "<title>Saved</title>" {
		t.Fatalf("Do result=%#v error=%v", result, err)
	}
}

func TestClientReturnsHTTPErrorStatusAsResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusNotFound)
		_, _ = response.Write([]byte("missing"))
	}))
	defer server.Close()

	response, err := NewClient().Get(context.Background(), mustParseURL(t, server.URL))
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if response.StatusCode != http.StatusNotFound || string(response.Body) != "missing" {
		t.Fatalf("response = status:%d body:%q", response.StatusCode, response.Body)
	}
}

func TestClientLimitsResponseBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		_, _ = response.Write([]byte(strings.Repeat("x", 9)))
	}))
	defer server.Close()

	client := NewClientWithLimits(server.Client(), 8)
	_, err := client.Get(context.Background(), mustParseURL(t, server.URL))
	if !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("Get() error = %v, want ErrResponseTooLarge", err)
	}
}

func TestClientHonorsContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		time.Sleep(100 * time.Millisecond)
		response.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := NewClient().Get(ctx, mustParseURL(t, server.URL))
	if !errors.Is(err, ErrTimeout) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Get() error = %v, want ErrTimeout and context deadline exceeded", err)
	}
}

func TestClientReportsRedirectLoopAndLimit(t *testing.T) {
	t.Run("loop", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
			response.Header().Set("Location", "/loop")
			response.WriteHeader(http.StatusFound)
		}))
		defer server.Close()
		_, err := NewClientWithLimits(server.Client(), 1024).Get(context.Background(), mustParseURL(t, server.URL+"/loop"))
		if !errors.Is(err, ErrRedirectLoop) {
			t.Fatalf("Get() error = %v, want ErrRedirectLoop", err)
		}
	})

	t.Run("limit", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
			step := 0
			_, _ = fmt.Sscanf(strings.TrimPrefix(request.URL.Path, "/"), "%d", &step)
			response.Header().Set("Location", fmt.Sprintf("/%d", step+1))
			response.WriteHeader(http.StatusFound)
		}))
		defer server.Close()
		_, err := NewClientWithLimits(server.Client(), 1024).Get(context.Background(), mustParseURL(t, server.URL+"/0"))
		if !errors.Is(err, ErrRedirectLimit) {
			t.Fatalf("Get() error = %v, want ErrRedirectLimit", err)
		}
	})
}

func TestClientReportsTruncatedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Length", "10")
		_, _ = response.Write([]byte("short"))
	}))
	defer server.Close()

	_, err := NewClientWithLimits(server.Client(), 1024).Get(context.Background(), mustParseURL(t, server.URL))
	if !errors.Is(err, ErrResponseTruncated) {
		t.Fatalf("Get() error = %v, want ErrResponseTruncated", err)
	}
}

func TestClientRejectsPathologicalRequestAndResponseHeaders(t *testing.T) {
	target := mustParseURL(t, "https://example.test")
	client := NewClientWithLimits(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK, Request: request,
			Header: http.Header{"X-Large": []string{strings.Repeat("x", maxHeaderBytes)}},
			Body:   io.NopCloser(strings.NewReader("ok")),
		}, nil
	})}, 1024)
	if _, err := client.Do(context.Background(), &Request{Method: http.MethodPost, URL: target, Body: make([]byte, maxRequestBodyBytes+1)}); !errors.Is(err, ErrRequestTooLarge) {
		t.Fatalf("request body error = %v", err)
	}
	headers := make(http.Header)
	for index := 0; index <= maxHeaderCount; index++ {
		headers[fmt.Sprintf("X-%03d", index)] = []string{"value"}
	}
	if _, err := client.Do(context.Background(), &Request{URL: target, Header: headers}); !errors.Is(err, ErrHeadersTooLarge) {
		t.Fatalf("request header error = %v", err)
	}
	if _, err := client.Get(context.Background(), target); !errors.Is(err, ErrHeadersTooLarge) {
		t.Fatalf("response header error = %v", err)
	}
}

func TestClientRedactsURLCookieAndAuthorizationFromErrors(t *testing.T) {
	sentinel := errors.New("transport sentinel")
	client := NewClientWithLimits(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("%w URL=%s Authorization=%s Cookie=%s", sentinel, request.URL.String(), request.Header.Get("Authorization"), request.Header.Get("Cookie"))
	})}, 1024)
	target := mustParseURL(t, "https://alice:password@example.test/private")
	_, err := client.Do(context.Background(), &Request{
		URL: target, Header: http.Header{"Authorization": []string{"Bearer top-secret"}, "Cookie": []string{"session=secret"}},
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("error chain lost transport sentinel: %v", err)
	}
	message := err.Error()
	for _, secret := range []string{"alice", "password", "top-secret", "session=secret"} {
		if strings.Contains(message, secret) {
			t.Fatalf("error leaked %q: %s", secret, message)
		}
	}
	if got := RedactedURL(target); got != "https://example.test/private" {
		t.Fatalf("RedactedURL() = %q", got)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func TestClientAlwaysClosesResponseBody(t *testing.T) {
	body := &trackingReadCloser{Reader: strings.NewReader("ok")}
	client := NewClientWithLimits(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Request: request, Header: make(http.Header), Body: body}, nil
	})}, 1024)
	if _, err := client.Get(context.Background(), mustParseURL(t, "https://example.test")); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !body.closed {
		t.Fatal("Response Body was not closed")
	}
}

type trackingReadCloser struct {
	io.Reader
	closed bool
}

func (body *trackingReadCloser) Close() error {
	body.closed = true
	return nil
}

func TestClientAppliesRedirectMethodAndBodyRules(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/final" {
			body, _ := io.ReadAll(request.Body)
			response.Header().Set("Content-Type", "text/plain")
			_, _ = response.Write([]byte(request.Method + ":" + string(body)))
			return
		}
		status := http.StatusFound
		_, _ = fmt.Sscanf(request.URL.Query().Get("status"), "%d", &status)
		response.Header().Set("Location", "./final")
		response.WriteHeader(status)
	}))
	defer server.Close()

	tests := []struct {
		status int
		method string
		body   string
		want   string
	}{
		{status: http.StatusMovedPermanently, method: http.MethodPost, body: "payload", want: "GET:"},
		{status: http.StatusFound, method: http.MethodPost, body: "payload", want: "GET:"},
		{status: http.StatusSeeOther, method: http.MethodPut, body: "payload", want: "GET:"},
		{status: http.StatusSeeOther, method: http.MethodHead, body: "", want: ""},
		{status: http.StatusTemporaryRedirect, method: http.MethodPatch, body: "payload", want: "PATCH:payload"},
		{status: http.StatusPermanentRedirect, method: http.MethodPatch, body: "payload", want: "PATCH:payload"},
	}
	for _, test := range tests {
		t.Run(fmt.Sprintf("%d_%s", test.status, test.method), func(t *testing.T) {
			startURL := mustParseURL(t, fmt.Sprintf("%s/start?status=%d", server.URL, test.status))
			response, err := NewClientWithLimits(server.Client(), 1024).Do(context.Background(), &Request{
				Method: test.method, URL: startURL, Body: []byte(test.body),
				Header: http.Header{"Content-Type": []string{"text/plain"}},
			})
			if err != nil {
				t.Fatalf("Do() error = %v", err)
			}
			if got := string(response.Body); got != test.want {
				t.Fatalf("final request = %q, want %q", got, test.want)
			}
			if response.URL.Path != "/final" || !response.Redirected {
				t.Fatalf("final URL = %s redirected = %t", response.URL, response.Redirected)
			}
		})
	}
}

func TestClientSharesInMemoryCookieJarAcrossNavigationFormAndFetch(t *testing.T) {
	var failures []string
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/navigation":
			http.SetCookie(response, &http.Cookie{Name: "session", Value: "navigation", Path: "/"})
		case "/form":
			if cookie, err := request.Cookie("session"); err != nil || cookie.Value != "navigation" {
				failures = append(failures, "Form did not receive Navigation Cookie")
			}
			http.SetCookie(response, &http.Cookie{Name: "form", Value: "submission", Path: "/"})
		case "/fetch":
			for name, value := range map[string]string{"session": "navigation", "form": "submission"} {
				if cookie, err := request.Cookie(name); err != nil || cookie.Value != value {
					failures = append(failures, "Fetch did not receive "+name+" Cookie")
				}
			}
		}
		response.Header().Set("Content-Type", "text/html")
		_, _ = response.Write([]byte("ok"))
	}))
	defer server.Close()

	client := NewClientWithLimits(server.Client(), 1024)
	if _, err := client.Get(context.Background(), mustParseURL(t, server.URL+"/navigation")); err != nil {
		t.Fatalf("Navigation Get() error = %v", err)
	}
	if _, err := client.Do(context.Background(), &Request{
		Method: http.MethodPost, URL: mustParseURL(t, server.URL+"/form"), Body: []byte("name=growse"),
	}); err != nil {
		t.Fatalf("Form Do() error = %v", err)
	}
	if _, err := client.Do(context.Background(), &Request{
		Method: http.MethodGet, URL: mustParseURL(t, server.URL+"/fetch"),
	}); err != nil {
		t.Fatalf("Fetch Do() error = %v", err)
	}
	if len(failures) != 0 {
		t.Fatal(strings.Join(failures, "; "))
	}

	isolated := NewClientWithLimits(server.Client(), 1024)
	if cookies := isolated.httpClient.Jar.Cookies(mustParseURL(t, server.URL)); len(cookies) != 0 {
		t.Fatalf("new Browser client inherited Cookies: %v", cookies)
	}
}

func TestCookieJarMatchesHostDomainPathAndLifecycle(t *testing.T) {
	client := NewClient()
	jar := client.httpClient.Jar
	origin := mustParseURL(t, "https://www.example.com/app/index")
	jar.SetCookies(origin, []*http.Cookie{
		{Name: "host", Value: "only", Path: "/"},
		{Name: "domain", Value: "shared", Domain: ".example.com", Path: "/"},
		{Name: "path", Value: "app", Path: "/app"},
		{Name: "expired", Value: "old", Path: "/", Expires: time.Unix(1, 0)},
		{Name: "foreign", Value: "blocked", Domain: "unrelated.test", Path: "/"},
	})

	if got := cookieValues(jar.Cookies(mustParseURL(t, "https://www.example.com/app/data"))); !reflect.DeepEqual(got, map[string]string{
		"host": "only", "domain": "shared", "path": "app",
	}) {
		t.Fatalf("same host /app Cookies = %v", got)
	}
	if got := cookieValues(jar.Cookies(mustParseURL(t, "https://sub.example.com/app/data"))); !reflect.DeepEqual(got, map[string]string{
		"domain": "shared",
	}) {
		t.Fatalf("subdomain Cookies = %v", got)
	}
	if got := cookieValues(jar.Cookies(mustParseURL(t, "https://www.example.com/other"))); !reflect.DeepEqual(got, map[string]string{
		"host": "only", "domain": "shared",
	}) {
		t.Fatalf("other path Cookies = %v", got)
	}

	jar.SetCookies(origin, []*http.Cookie{{Name: "host", Value: "updated", Path: "/"}})
	if got := cookieValues(jar.Cookies(origin))["host"]; got != "updated" {
		t.Fatalf("overwritten host Cookie = %q", got)
	}
	jar.SetCookies(origin, []*http.Cookie{{Name: "host", Value: "", Path: "/", MaxAge: -1}})
	if _, exists := cookieValues(jar.Cookies(origin))["host"]; exists {
		t.Fatal("deleted host Cookie is still present")
	}
}

func TestFetchCredentialsModesControlCookieSendAndStore(t *testing.T) {
	var receivedCookie string
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		receivedCookie = request.Header.Get("Cookie")
		if origin := request.Header.Get("Origin"); origin != "" {
			response.Header().Set("Access-Control-Allow-Origin", origin)
			response.Header().Set("Access-Control-Allow-Credentials", "true")
		}
		http.SetCookie(response, &http.Cookie{Name: "received", Value: "yes", Path: "/"})
		_, _ = response.Write([]byte("ok"))
	}))
	defer server.Close()
	target := mustParseURL(t, server.URL+"/data")
	crossOriginSite := mustParseURL(t, "http://127.0.0.1:1/page")
	sameOriginSite := mustParseURL(t, server.URL+"/page")

	tests := []struct {
		name      string
		mode      CredentialsMode
		siteURL   *url.URL
		wantSend  bool
		wantStore bool
	}{
		{name: "omit", mode: CredentialsOmit, siteURL: sameOriginSite},
		{name: "default cross origin", siteURL: crossOriginSite},
		{name: "same-origin cross origin", mode: CredentialsSameOrigin, siteURL: crossOriginSite},
		{name: "same-origin matching", mode: CredentialsSameOrigin, siteURL: sameOriginSite, wantSend: true, wantStore: true},
		{name: "include cross origin", mode: CredentialsInclude, siteURL: crossOriginSite, wantSend: true, wantStore: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			receivedCookie = ""
			client := NewClientWithLimits(server.Client(), 1024)
			client.httpClient.Jar.SetCookies(target, []*http.Cookie{{Name: "session", Value: "stored", Path: "/"}})
			_, err := client.Do(context.Background(), &Request{
				Method: http.MethodGet, URL: target, SiteURL: test.siteURL, Kind: RequestFetch, Credentials: test.mode,
			})
			if err != nil {
				t.Fatalf("Do() error = %v", err)
			}
			if sent := strings.Contains(receivedCookie, "session=stored"); sent != test.wantSend {
				t.Fatalf("Cookie sent = %t (%q), want %t", sent, receivedCookie, test.wantSend)
			}
			_, stored := cookieValues(client.httpClient.Jar.Cookies(target))["received"]
			if stored != test.wantStore {
				t.Fatalf("Set-Cookie stored = %t, want %t", stored, test.wantStore)
			}
		})
	}
}

func cookieValues(cookies []*http.Cookie) map[string]string {
	values := make(map[string]string, len(cookies))
	for _, cookie := range cookies {
		values[cookie.Name] = cookie.Value
	}
	return values
}

func mustParseURL(t *testing.T, rawURL string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("url.Parse(%q): %v", rawURL, err)
	}
	return parsed
}
