package network

import (
	"context"
	"errors"
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

func TestClientRejectsHTTPError(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()

	_, err := NewClient().Get(context.Background(), mustParseURL(t, server.URL))
	var statusError *StatusError
	if !errors.As(err, &statusError) || statusError.Code != http.StatusNotFound {
		t.Fatalf("Get() error = %v, want 404 StatusError", err)
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

func mustParseURL(t *testing.T, rawURL string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("url.Parse(%q): %v", rawURL, err)
	}
	return parsed
}
