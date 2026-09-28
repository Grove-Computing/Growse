package searchprovider

import (
	"context"
	"fmt"
	"github.com/Grove-Computing/Growse/internal/network"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestTransportBoundsPrivacyAndEncoding(t *testing.T) {
	calls := 0
	body := `["日本語 + %20",["日本語","localhost","https://secret.test"]]`
	tr := NewTransport(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Query().Get("q") != "日本語 + %20" {
			t.Fatal(r.URL)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header), Request: r}, nil
	})})
	fetch := tr.Suggestions(Builtin())
	terms, err := fetch(context.Background(), "日本語 + %20")
	if err != nil || len(terms) != 1 {
		t.Fatal(terms, err)
	}
	for _, q := range []string{"", "@search secret", "https://example.com", "user:pass@example.com", "localhost", "192.168.1.1", "look at localhost", "query https://secret.test", "x\nsecret"} {
		if _, err := fetch(context.Background(), q); err == nil {
			t.Fatal("sent", q)
		}
	}
	if calls != 1 {
		t.Fatal(calls)
	}
	body = strings.Repeat("x", MaxSuggestionBytes+1)
	if _, err := fetch(context.Background(), "日本語 + %20"); err != ErrResponse {
		t.Fatal(err)
	}
}
func TestRedirectPolicy(t *testing.T) {
	tr := NewTransport(nil)
	for _, target := range []string{"http://example.com/", "https://u:p@example.com/", "https://example.com/#x"} {
		r, _ := http.NewRequest("GET", target, nil)
		if tr.client.CheckRedirect(r, nil) == nil {
			t.Fatal(target)
		}
	}
	r, _ := http.NewRequest("GET", "https://example.com/", nil)
	if tr.client.CheckRedirect(r, make([]*http.Request, 4)) == nil {
		t.Fatal("redirect limit")
	}
	if tr.client.CheckRedirect(r, make([]*http.Request, 3)) != nil {
		t.Fatal("third redirect rejected")
	}
}

func TestSearchNavigationRedirectPolicy(t *testing.T) {
	for _, test := range []struct {
		name      string
		redirects int
		downgrade bool
		allowed   bool
	}{{"three", 3, false, true}, {"four", 4, false, false}, {"downgrade", 1, true, false}} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				status := 200
				header := make(http.Header)
				if calls <= test.redirects {
					status = 302
					target := fmt.Sprintf("https://example.com/step%d", calls)
					if test.downgrade {
						target = "http://example.com/unsafe"
					}
					header.Set("Location", target)
				}
				return &http.Response{StatusCode: status, Header: header, Body: io.NopCloser(strings.NewReader("result")), Request: r}, nil
			})}
			client := network.NewClientWithLimits(httpClient, 1024)
			target, _ := url.Parse("https://example.com/?q=secret")
			ctx := network.WithRedirectPolicy(context.Background(), target.String(), ValidateRedirect)
			_, err := client.Get(ctx, target)
			if (err == nil) != test.allowed {
				t.Fatal(err, calls)
			}
			if err != nil && strings.Contains(err.Error(), "secret") {
				t.Fatal("query leaked into error")
			}
			// A different initial resource keeps the ordinary browser policy.
			calls = 0
			other, _ := url.Parse("https://example.com/ordinary")
			if _, err := client.Get(ctx, other); err != nil {
				t.Fatal("policy escaped initial request", err)
			}
		})
	}
}
