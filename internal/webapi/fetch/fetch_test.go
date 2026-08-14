package fetch

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"testing"

	"github.com/Grove-Computing/Growse/internal/network"
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
