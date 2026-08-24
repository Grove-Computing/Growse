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
	"strings"
	"sync"
	"time"
)

const (
	defaultTimeout      = 15 * time.Second
	defaultMaxBodyBytes = 4 << 20 // 4 MiB
	maxRedirects        = 10
	maxRequestBodyBytes = 1 << 20
	maxHeaderCount      = 100
	maxHeaderBytes      = 64 << 10
)

// ErrResponseTooLarge is returned when a response exceeds the configured
// body-size limit.
var (
	ErrResponseTooLarge  = errors.New("response body is too large")
	ErrResponseTruncated = errors.New("response body was truncated")
	ErrRedirectLoop      = errors.New("redirect loop")
	ErrRedirectLimit     = errors.New("redirect limit exceeded")
	ErrTimeout           = errors.New("request timed out")
	ErrRequestTooLarge   = errors.New("request exceeds safety limit")
	ErrHeadersTooLarge   = errors.New("HTTP headers exceed safety limit")
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
	CacheStatus string
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
	Observer    func(Observation)
}

// Observation is body-free request metadata emitted after one client operation.
type Observation struct {
	Method        string
	URL           *url.URL
	Kind          RequestKind
	StartedAt     time.Time
	Duration      time.Duration
	StatusCode    int
	Redirected    bool
	CacheStatus   string
	ResponseBytes int
	ErrorCategory string
}

// RequestKind identifies the browser operation that initiated a request.
type RequestKind uint8

const (
	RequestNavigation RequestKind = iota
	RequestSubresource
	RequestForm
	RequestFetch
	RequestStylesheet
	RequestImage
	RequestScript
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
	httpClient     *http.Client
	maxBodyBytes   int64
	preflightMu    sync.Mutex
	preflightCache map[string]time.Time
	now            func() time.Time
	cache          *HTTPCache
}

// NewClient creates a client with production defaults.
func NewClient() *Client {
	return &Client{
		httpClient: configuredHTTPClient(&http.Client{Timeout: defaultTimeout}), maxBodyBytes: defaultMaxBodyBytes,
		preflightCache: make(map[string]time.Time), now: time.Now,
		cache: NewHTTPCache(),
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
	return &Client{
		httpClient: configuredHTTPClient(httpClient), maxBodyBytes: maxBodyBytes,
		preflightCache: make(map[string]time.Time), now: time.Now,
		cache: NewHTTPCache(),
	}
}

// NewClientWithCacheRoot creates a client whose disposable HTTP Cache is
// persisted below the explicitly supplied cache root.
func NewClientWithCacheRoot(httpClient *http.Client, maxBodyBytes int64, cacheRoot string) (*Client, error) {
	client := NewClientWithLimits(httpClient, maxBodyBytes)
	cache, err := NewHTTPCacheWithDisk(cacheRoot)
	if err != nil {
		return nil, err
	}
	client.cache = cache
	return client, nil
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
func (c *Client) Do(ctx context.Context, requestData *Request) (result *Response, resultErr error) {
	if requestData == nil || requestData.URL == nil {
		return nil, errors.New("resource URL is nil")
	}
	startedAt := c.now()
	if requestData.Observer != nil {
		defer func() {
			categoryError := resultErr
			if cause := context.Cause(ctx); cause != nil {
				categoryError = errors.Join(categoryError, cause)
			}
			observation := Observation{
				Method: requestMethod(requestData), URL: cloneURL(requestData.URL), Kind: requestData.Kind,
				StartedAt: startedAt, Duration: c.now().Sub(startedAt), ErrorCategory: observationErrorCategory(categoryError),
			}
			if result != nil {
				observation.StatusCode = result.StatusCode
				observation.Redirected = result.Redirected
				observation.CacheStatus = result.CacheStatus
				observation.ResponseBytes = len(result.Body)
				if observation.ErrorCategory == "" && result.StatusCode >= http.StatusBadRequest {
					observation.ErrorCategory = "http"
				}
			}
			requestData.Observer(observation)
		}()
	}
	if len(requestData.Body) > maxRequestBodyBytes {
		return nil, ErrRequestTooLarge
	}
	if !headersWithinLimits(requestData.Header) {
		return nil, ErrHeadersTooLarge
	}
	method := requestData.Method
	if method == "" {
		method = http.MethodGet
	}
	request, err := http.NewRequestWithContext(ctx, method, requestData.URL.String(), bytes.NewReader(requestData.Body))
	if err != nil {
		return nil, &redactedError{message: "create HTTP request failed", cause: err}
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
	if err := c.prepareCORS(ctx, &operationClient, request, requestData); err != nil {
		return nil, err
	}
	cacheRequest := *requestData
	cacheRequest.Method = method
	cacheRequest.Header = request.Header.Clone()
	if cached, ok := c.cache.MatchFresh(&cacheRequest); ok {
		result, resultErr = prepareCachedResponse(cached, requestData)
		if result != nil {
			result.CacheStatus = "hit"
		}
		return result, resultErr
	}
	if validation, ok := c.cache.RevalidationHeaders(&cacheRequest); ok {
		for name, values := range validation {
			if request.Header.Get(name) == "" {
				request.Header[name] = append([]string(nil), values...)
			}
		}
	}
	redirectPolicy := operationClient.CheckRedirect
	operationClient.CheckRedirect = func(redirect *http.Request, via []*http.Request) error {
		if err := validateCORSResponse(redirect.Response, requestData); err != nil {
			return err
		}
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
		return c.prepareCORS(ctx, &operationClient, redirect, &redirectData)
	}
	response, err := operationClient.Do(request)
	if err != nil {
		return nil, classifyRequestError(err)
	}
	defer response.Body.Close()
	if !headersWithinLimits(response.Header) {
		return nil, ErrHeadersTooLarge
	}
	storeResponseCookies(jar, response, requestData)
	if err := validateCORSResponse(response, requestData); err != nil {
		return nil, err
	}

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
	cachedResult := &Response{
		URL:         cloneURL(finalURL),
		StatusCode:  response.StatusCode,
		Status:      http.StatusText(response.StatusCode),
		Header:      response.Header.Clone(),
		ContentType: response.Header.Get("Content-Type"),
		Body:        body,
		Redirected:  finalURL.String() != requestData.URL.String(),
		CacheStatus: "miss",
	}
	c.invalidateAfterStateChange(&cacheRequest, cachedResult.StatusCode, response.Header, finalURL)
	if cachedResult.StatusCode == http.StatusNotModified {
		merged, ok := c.cache.MergeNotModified(&cacheRequest, cachedResult.Header)
		if !ok {
			return nil, ErrCacheValidation
		}
		result, resultErr = prepareCachedResponse(merged, requestData)
		if result != nil {
			result.CacheStatus = "revalidated"
		}
		return result, resultErr
	}
	c.cache.Store(&cacheRequest, cachedResult)
	return prepareCachedResponse(cachedResult, requestData)
}

func observationErrorCategory(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrCORS), errors.Is(err, ErrCORSPreflightRequired):
		return "cors"
	case errors.Is(err, ErrTimeout), errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, ErrRedirectLoop):
		return "redirect_loop"
	case errors.Is(err, ErrRedirectLimit):
		return "redirect_limit"
	case errors.Is(err, ErrResponseTooLarge), errors.Is(err, ErrResponseTruncated):
		return "response_limit"
	case errors.Is(err, ErrRequestTooLarge), errors.Is(err, ErrHeadersTooLarge):
		return "request_limit"
	default:
		return "network"
	}
}

func prepareCachedResponse(cached *Response, requestData *Request) (*Response, error) {
	if cached == nil {
		return nil, errors.New("cached response is nil")
	}
	finalURL := cached.URL
	if finalURL == nil && requestData != nil {
		finalURL = requestData.URL
	}
	probe := &http.Response{Header: cached.Header.Clone(), Request: &http.Request{URL: finalURL}}
	if err := validateCORSResponse(probe, requestData); err != nil {
		return nil, err
	}
	result := cloneResponse(cached)
	result.Header = filterFetchResponseHeaders(cached.Header, requestData, finalURL)
	result.ContentType = cached.Header.Get("Content-Type")
	return result, nil
}

func (c *Client) invalidateAfterStateChange(request *Request, statusCode int, header http.Header, finalURL *url.URL) {
	if c == nil || c.cache == nil || request == nil || !isUnsafeMethod(requestMethod(request)) || statusCode < 200 || statusCode >= 400 {
		return
	}
	c.cache.InvalidateURL(request, request.URL)
	if finalURL != nil && SameOrigin(request.URL, finalURL) {
		c.cache.InvalidateURL(request, finalURL)
	}
	for _, name := range []string{"Location", "Content-Location"} {
		raw := strings.TrimSpace(header.Get(name))
		if raw == "" || finalURL == nil {
			continue
		}
		reference, err := url.Parse(raw)
		if err != nil {
			continue
		}
		target := finalURL.ResolveReference(reference)
		if SameOrigin(finalURL, target) {
			c.cache.InvalidateURL(request, target)
		}
	}
}

func isUnsafeMethod(method string) bool {
	switch strings.ToUpper(method) {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
		return false
	default:
		return true
	}
}

func headersWithinLimits(header http.Header) bool {
	count := 0
	size := 0
	for name, values := range header {
		count += len(values)
		for _, value := range values {
			size += len(name) + len(value)
		}
		if count > maxHeaderCount || size > maxHeaderBytes {
			return false
		}
	}
	return true
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
	case errors.Is(err, ErrCORS):
		return fmt.Errorf("send request: %w", ErrCORS)
	case errors.Is(err, ErrCORSPreflightRequired):
		return fmt.Errorf("send request: %w", ErrCORSPreflightRequired)
	case errors.Is(err, context.Canceled):
		return fmt.Errorf("send request: %w", context.Canceled)
	case errors.Is(err, context.DeadlineExceeded):
		return &redactedError{message: ErrTimeout.Error(), cause: errors.Join(ErrTimeout, context.DeadlineExceeded)}
	}
	var networkError net.Error
	if errors.As(err, &networkError) && networkError.Timeout() {
		return &redactedError{message: ErrTimeout.Error(), cause: errors.Join(ErrTimeout, err)}
	}
	return &redactedError{message: "network request failed", cause: err}
}

type redactedError struct {
	message string
	cause   error
}

func (err *redactedError) Error() string { return err.message }
func (err *redactedError) Unwrap() error { return err.cause }

// RedactedURL removes userinfo before a URL is included in UI or errors.
func RedactedURL(target *url.URL) string {
	if target == nil {
		return "unknown"
	}
	copy := *target
	copy.User = nil
	return copy.String()
}

func cloneURL(source *url.URL) *url.URL {
	copy := *source
	return &copy
}
