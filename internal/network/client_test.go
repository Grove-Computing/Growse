package network

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
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
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Get() error = %v, want context deadline exceeded", err)
	}
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

func mustParseURL(t *testing.T, rawURL string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("url.Parse(%q): %v", rawURL, err)
	}
	return parsed
}
