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

	var response Response
	api.Fetch(Request{URL: "/data"}, func(result Response) { response = result }, func(message string) {
		t.Fatalf("Fetch failure = %q", message)
	})
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
