// Package network retrieves resources for the Growse browser.
package network

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"
)

const (
	defaultTimeout      = 15 * time.Second
	defaultMaxBodyBytes = 4 << 20 // 4 MiB
	maxRedirects        = 10
)

// ErrResponseTooLarge is returned when a response exceeds the configured
// body-size limit.
var (
	ErrResponseTooLarge  = errors.New("response body is too large")
	ErrResponseTruncated = errors.New("response body was truncated")
	ErrRedirectLoop      = errors.New("redirect loop")
	ErrRedirectLimit     = errors.New("redirect limit exceeded")
	ErrTimeout           = errors.New("request timed out")
)

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
	Method      string
	URL         *url.URL
	Header      http.Header
	Body        []byte
	SiteURL     *url.URL
	Kind        RequestKind
	Credentials CredentialsMode
}

// RequestKind identifies the browser operation that initiated a request.
type RequestKind uint8

const (
	RequestNavigation RequestKind = iota
	RequestSubresource
	RequestForm
	RequestFetch
)

// CredentialsMode controls whether Fetch sends and stores credentials.
type CredentialsMode string

const (
	CredentialsOmit       CredentialsMode = "omit"
	CredentialsSameOrigin CredentialsMode = "same-origin"
	CredentialsInclude    CredentialsMode = "include"
)

// Client is a size-limited HTTP resource loader.
type Client struct {
	httpClient   *http.Client
	maxBodyBytes int64
}

// NewClient creates a client with production defaults.
func NewClient() *Client {
	return &Client{
		httpClient:   configuredHTTPClient(&http.Client{Timeout: defaultTimeout}),
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
	return &Client{httpClient: configuredHTTPClient(httpClient), maxBodyBytes: maxBodyBytes}
}

func configuredHTTPClient(source *http.Client) *http.Client {
	copy := *source
	if copy.Jar == nil {
		copy.Jar = newPolicyCookieJar()
	}
	if copy.CheckRedirect == nil {
		copy.CheckRedirect = applyRedirectPolicy
	}
	return &copy
}

func applyRedirectPolicy(request *http.Request, via []*http.Request) error {
	if request == nil || request.Response == nil || len(via) == 0 {
		return nil
	}
	for _, previous := range via {
		if previous.URL != nil && request.URL != nil && previous.URL.String() == request.URL.String() {
			return ErrRedirectLoop
		}
	}
	if len(via) >= maxRedirects {
		return ErrRedirectLimit
	}
	previous := via[len(via)-1]
	switch request.Response.StatusCode {
	case http.StatusMovedPermanently, http.StatusFound:
		if previous.Method == http.MethodPost {
			makeRedirectGET(request)
		}
	case http.StatusSeeOther:
		if previous.Method != http.MethodHead {
			makeRedirectGET(request)
		}
	case http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
		// net/http reconstructs the request body through GetBody for 307/308.
	}
	return nil
}

func makeRedirectGET(request *http.Request) {
	request.Method = http.MethodGet
	request.Body = nil
	request.GetBody = nil
	request.ContentLength = 0
	request.Header.Del("Content-Length")
	request.Header.Del("Content-Type")
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

	operationClient := *c.httpClient
	jar := operationClient.Jar
	operationClient.Jar = nil
	addRequestCookies(request, jar, requestData)
	redirectPolicy := operationClient.CheckRedirect
	operationClient.CheckRedirect = func(redirect *http.Request, via []*http.Request) error {
		storeResponseCookies(jar, redirect.Response, requestData)
		if redirectPolicy != nil {
			if err := redirectPolicy(redirect, via); err != nil {
				return err
			}
		}
		redirect.Header.Del("Cookie")
		redirectData := *requestData
		redirectData.URL = redirect.URL
		redirectData.Method = redirect.Method
		addRequestCookies(redirect, jar, &redirectData)
		return nil
	}
	response, err := operationClient.Do(request)
	if err != nil {
		return nil, classifyRequestError(err)
	}
	defer response.Body.Close()
	storeResponseCookies(jar, response, requestData)

	if response.ContentLength > c.maxBodyBytes {
		return nil, fmt.Errorf("%w: limit is %d bytes", ErrResponseTooLarge, c.maxBodyBytes)
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, c.maxBodyBytes+1))
	if err != nil {
		if errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, fmt.Errorf("%w: %v", ErrResponseTruncated, err)
		}
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

func addRequestCookies(request *http.Request, jar http.CookieJar, requestData *Request) {
	if request == nil || jar == nil || requestData == nil {
		return
	}
	if !credentialsAllowed(request.URL, requestData) {
		return
	}
	cookies := jar.Cookies(request.URL)
	if policyJar, ok := jar.(*policyCookieJar); ok {
		cookies = policyJar.cookiesForRequest(request.URL, requestData.SiteURL, requestData.Kind, request.Method)
	}
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
}

func storeResponseCookies(jar http.CookieJar, response *http.Response, requestData *Request) {
	if jar == nil || response == nil || response.Request == nil || response.Request.URL == nil {
		return
	}
	if !credentialsAllowed(response.Request.URL, requestData) {
		return
	}
	jar.SetCookies(response.Request.URL, response.Cookies())
}

func credentialsAllowed(target *url.URL, requestData *Request) bool {
	if requestData == nil || requestData.Kind != RequestFetch {
		return true
	}
	switch requestData.Credentials {
	case CredentialsOmit:
		return false
	case CredentialsInclude:
		return true
	case "", CredentialsSameOrigin:
		return SameOrigin(requestData.SiteURL, target)
	default:
		return false
	}
}

func classifyRequestError(err error) error {
	switch {
	case errors.Is(err, ErrRedirectLoop):
		return fmt.Errorf("send request: %w", ErrRedirectLoop)
	case errors.Is(err, ErrRedirectLimit):
		return fmt.Errorf("send request: %w", ErrRedirectLimit)
	case errors.Is(err, context.Canceled):
		return fmt.Errorf("send request: %w", context.Canceled)
	case errors.Is(err, context.DeadlineExceeded):
		return fmt.Errorf("%w: %w", ErrTimeout, context.DeadlineExceeded)
	}
	var networkError net.Error
	if errors.As(err, &networkError) && networkError.Timeout() {
		return fmt.Errorf("%w: %v", ErrTimeout, err)
	}
	return fmt.Errorf("send request: %w", err)
}

func cloneURL(source *url.URL) *url.URL {
	copy := *source
	return &copy
}
