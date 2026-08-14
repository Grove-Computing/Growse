package network

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchAllowsSameOriginAndSimpleCORS(t *testing.T) {
	var seenOrigin string
	allowedOrigin := ""
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		seenOrigin = request.Header.Get("Origin")
		if allowedOrigin != "" {
			response.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
		}
		_, _ = response.Write([]byte("ok"))
	}))
	defer server.Close()
	target := parseOriginURL(t, server.URL+"/data")
	client := NewClientWithLimits(server.Client(), 1024)

	if _, err := client.Do(context.Background(), &Request{
		Method: http.MethodGet, URL: target, SiteURL: parseOriginURL(t, server.URL+"/page"), Kind: RequestFetch,
	}); err != nil {
		t.Fatalf("same-origin Fetch error = %v", err)
	}
	if seenOrigin != "" {
		t.Fatalf("same-origin Origin header = %q", seenOrigin)
	}

	crossSite := parseOriginURL(t, "https://app.example.test/page")
	allowedOrigin = "https://app.example.test"
	if _, err := client.Do(context.Background(), &Request{
		Method: http.MethodPost, URL: target, SiteURL: crossSite, Kind: RequestFetch,
		Header: http.Header{"Content-Type": []string{"text/plain"}}, Body: []byte("hello"),
	}); err != nil {
		t.Fatalf("simple CORS Fetch error = %v", err)
	}
	if seenOrigin != allowedOrigin {
		t.Fatalf("CORS Origin header = %q, want %q", seenOrigin, allowedOrigin)
	}
}

func TestFetchRejectsInvalidSimpleCORSResponseAndNonSimpleRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Access-Control-Allow-Origin", "https://wrong.example")
		_, _ = response.Write([]byte("secret"))
	}))
	defer server.Close()
	target := parseOriginURL(t, server.URL)
	site := parseOriginURL(t, "https://app.example.test/page")
	client := NewClientWithLimits(server.Client(), 1024)

	_, err := client.Do(context.Background(), &Request{Method: http.MethodGet, URL: target, SiteURL: site, Kind: RequestFetch})
	if !errors.Is(err, ErrCORS) {
		t.Fatalf("simple CORS error = %v, want ErrCORS", err)
	}
	_, err = client.Do(context.Background(), &Request{
		Method: http.MethodPut, URL: target, SiteURL: site, Kind: RequestFetch,
		Header: http.Header{"X-Custom": []string{"value"}},
	})
	if !errors.Is(err, ErrCORS) {
		t.Fatalf("rejected preflight error = %v, want ErrCORS", err)
	}
}

func TestCredentialedCORSRejectsWildcardOrigin(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Access-Control-Allow-Origin", "*")
	}))
	defer server.Close()
	_, err := NewClientWithLimits(server.Client(), 1024).Do(context.Background(), &Request{
		Method: http.MethodGet, URL: parseOriginURL(t, server.URL), SiteURL: parseOriginURL(t, "https://app.example.test"),
		Kind: RequestFetch, Credentials: CredentialsInclude,
	})
	if !errors.Is(err, ErrCORS) {
		t.Fatalf("credentialed wildcard CORS error = %v, want ErrCORS", err)
	}
}

func TestCacheHitReevaluatesCORSCredentialsAndHeaderExposure(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		requests++
		response.Header().Set("Cache-Control", "max-age=60")
		response.Header().Set("Access-Control-Allow-Origin", "*")
		response.Header().Set("Access-Control-Expose-Headers", "X-Visible")
		response.Header().Set("X-Visible", "shown")
		response.Header().Set("X-Secret", "hidden")
		_, _ = response.Write([]byte("cached"))
	}))
	defer server.Close()
	client := NewClientWithLimits(server.Client(), 1024)
	request := &Request{
		Method: http.MethodGet, URL: parseOriginURL(t, server.URL+"/data"),
		SiteURL: parseOriginURL(t, "https://app.example.test/page"), Kind: RequestFetch, Credentials: CredentialsOmit,
	}
	for range 2 {
		response, err := client.Do(context.Background(), request)
		if err != nil {
			t.Fatal(err)
		}
		if response.Header.Get("X-Visible") != "shown" || response.Header.Get("X-Secret") != "" || response.Header.Get("Access-Control-Allow-Origin") != "" {
			t.Fatalf("filtered Fetch headers = %v", response.Header)
		}
	}
	credentialed := *request
	credentialed.Credentials = CredentialsInclude
	if _, err := client.Do(context.Background(), &credentialed); !errors.Is(err, ErrCORS) {
		t.Fatalf("credentialed cache hit error = %v, want ErrCORS", err)
	}
	if requests != 1 {
		t.Fatalf("Network requests = %d, want 1 before cached policy checks", requests)
	}
}

func TestCORSPreflightValidatesAndCachesPermission(t *testing.T) {
	options := 0
	actual := 0
	allowedOrigin := "https://app.example.test"
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
		if request.Method == http.MethodOptions {
			options++
			response.Header().Set("Access-Control-Allow-Methods", "PUT")
			response.Header().Set("Access-Control-Allow-Headers", "X-Custom")
			response.Header().Set("Access-Control-Max-Age", "600")
			return
		}
		actual++
		_, _ = response.Write([]byte("updated"))
	}))
	defer server.Close()
	client := NewClientWithLimits(server.Client(), 1024)
	request := &Request{
		Method: http.MethodPut, URL: parseOriginURL(t, server.URL+"/data"),
		SiteURL: parseOriginURL(t, allowedOrigin+"/page"), Kind: RequestFetch,
		Header: http.Header{"X-Custom": []string{"value"}}, Body: []byte("body"),
	}
	for index := 0; index < 2; index++ {
		if _, err := client.Do(context.Background(), request); err != nil {
			t.Fatalf("Do(%d) error = %v", index, err)
		}
	}
	if options != 1 || actual != 2 {
		t.Fatalf("request counts = OPTIONS:%d actual:%d, want 1 and 2", options, actual)
	}
}

func TestCORSFiltersResponseHeadersAndHidesRejectedResponse(t *testing.T) {
	allowedOrigin := "https://app.example.test"
	allow := true
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		if allow {
			response.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
		}
		response.Header().Set("Content-Type", "application/json")
		response.Header().Set("X-Public", "visible")
		response.Header().Set("X-Secret", "hidden")
		response.Header().Set("Access-Control-Expose-Headers", "X-Public")
		http.SetCookie(response, &http.Cookie{Name: "session", Value: "secret", Path: "/"})
		_, _ = response.Write([]byte(`{"secret":true}`))
	}))
	defer server.Close()
	client := NewClientWithLimits(server.Client(), 1024)
	request := &Request{
		Method: http.MethodGet, URL: parseOriginURL(t, server.URL), SiteURL: parseOriginURL(t, allowedOrigin+"/page"), Kind: RequestFetch,
	}
	response, err := client.Do(context.Background(), request)
	if err != nil {
		t.Fatalf("allowed CORS Do() error = %v", err)
	}
	if response.Header.Get("Content-Type") != "application/json" || response.Header.Get("X-Public") != "visible" {
		t.Fatalf("public Response Header = %v", response.Header)
	}
	for _, hidden := range []string{"X-Secret", "Set-Cookie", "Access-Control-Allow-Origin", "Access-Control-Expose-Headers"} {
		if value := response.Header.Get(hidden); value != "" {
			t.Fatalf("hidden Response Header %s = %q", hidden, value)
		}
	}

	allow = false
	response, err = client.Do(context.Background(), request)
	if !errors.Is(err, ErrCORS) || response != nil {
		t.Fatalf("rejected CORS response = %#v error = %v", response, err)
	}
}
