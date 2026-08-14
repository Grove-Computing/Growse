// Package network retrieves resources for the Growse browser.
package network

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const (
	defaultTimeout      = 15 * time.Second
	defaultMaxBodyBytes = 4 << 20 // 4 MiB
)

// ErrResponseTooLarge is returned when a response exceeds the configured
// body-size limit.
var ErrResponseTooLarge = errors.New("response body is too large")

// Response contains the network metadata needed to construct a browser page.
type Response struct {
	URL         *url.URL
	StatusCode  int
	Status      string
	Header      http.Header
	ContentType string
	Body        []byte
	Redirected  bool
}

// Request contains the HTTP request data accepted by the network client.
type Request struct {
	Method string
	URL    *url.URL
	Header http.Header
	Body   []byte
}

// Client is a size-limited HTTP resource loader.
type Client struct {
	httpClient   *http.Client
	maxBodyBytes int64
}

// NewClient creates a client with production defaults.
func NewClient() *Client {
	return &Client{
		httpClient:   &http.Client{Timeout: defaultTimeout},
		maxBodyBytes: defaultMaxBodyBytes,
	}
}

// NewClientWithLimits creates a client with explicit dependencies and limits.
// It is primarily useful for tests and alternative transports.
func NewClientWithLimits(httpClient *http.Client, maxBodyBytes int64) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultTimeout}
	}
	if maxBodyBytes <= 0 {
		maxBodyBytes = defaultMaxBodyBytes
	}
	return &Client{httpClient: httpClient, maxBodyBytes: maxBodyBytes}
}

// Get retrieves one HTTP(S) resource.
func (c *Client) Get(ctx context.Context, resourceURL *url.URL) (*Response, error) {
	return c.Do(ctx, &Request{Method: http.MethodGet, URL: resourceURL})
}

// Do sends a size-limited HTTP request.
func (c *Client) Do(ctx context.Context, requestData *Request) (*Response, error) {
	if requestData == nil || requestData.URL == nil {
		return nil, errors.New("resource URL is nil")
	}
	method := requestData.Method
	if method == "" {
		method = http.MethodGet
	}
	request, err := http.NewRequestWithContext(ctx, method, requestData.URL.String(), bytes.NewReader(requestData.Body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	request.Header = requestData.Header.Clone()
	if request.Header == nil {
		request.Header = make(http.Header)
	}
	if request.Header.Get("Accept") == "" {
		request.Header.Set("Accept", "text/html,application/xhtml+xml")
	}
	if request.Header.Get("User-Agent") == "" {
		request.Header.Set("User-Agent", "Growse/0.1")
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer response.Body.Close()

	if response.ContentLength > c.maxBodyBytes {
		return nil, fmt.Errorf("%w: limit is %d bytes", ErrResponseTooLarge, c.maxBodyBytes)
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, c.maxBodyBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}
	if int64(len(body)) > c.maxBodyBytes {
		return nil, fmt.Errorf("%w: limit is %d bytes", ErrResponseTooLarge, c.maxBodyBytes)
	}

	finalURL := requestData.URL
	if response.Request != nil && response.Request.URL != nil {
		finalURL = response.Request.URL
	}

	return &Response{
		URL:         cloneURL(finalURL),
		StatusCode:  response.StatusCode,
		Status:      http.StatusText(response.StatusCode),
		Header:      response.Header.Clone(),
		ContentType: response.Header.Get("Content-Type"),
		Body:        body,
		Redirected:  finalURL.String() != requestData.URL.String(),
	}, nil
}

func cloneURL(source *url.URL) *url.URL {
	copy := *source
	return &copy
}
