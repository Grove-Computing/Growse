// Package fetch provides the HTTP API exposed to WebGo programs.
package fetch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/Grove-Computing/Growse/internal/network"
	formapi "github.com/Grove-Computing/Growse/internal/webapi/form"
	urlapi "github.com/Grove-Computing/Growse/internal/webapi/url"
)

// Header preserves every value associated with an HTTP header name.
type Header map[string][]string

// CredentialsMode controls Cookie and HTTP authentication handling.
type CredentialsMode string

const (
	CredentialsOmit       CredentialsMode = "omit"
	CredentialsSameOrigin CredentialsMode = "same-origin"
	CredentialsInclude    CredentialsMode = "include"
)

// Request describes an HTTP request issued by a WebGo program.
// Body takes precedence over Text when both are set.
type Request struct {
	Method      string
	URL         string
	Header      Header
	Headers     *Headers
	Body        []byte
	Text        string
	JSON        string
	Params      *urlapi.URLSearchParams
	FormData    *formapi.FormData
	Signal      *AbortSignal
	Timeout     time.Duration
	Credentials CredentialsMode
}

// Response is delivered to a successful Fetch callback. Additional response
// metadata and body helpers are added by the HTTP lifecycle implementation.
type Response struct {
	Status     int
	StatusText string
	URL        string
	Redirected bool
	Header     Header
	Headers    *ResponseHeaders
	body       *responseBody
}

type responseBody struct {
	value    []byte
	consumed bool
}

// ErrBodyConsumed reports a second attempt to consume one response body.
var ErrBodyConsumed = errors.New("response body has already been consumed")
var ErrInvalidText = errors.New("response body is not valid UTF-8")

// Bytes consumes the response body and returns a defensive copy.
func (response Response) Bytes() ([]byte, error) {
	body, err := response.consumeBody()
	if err != nil {
		return nil, err
	}
	return append([]byte(nil), body...), nil
}

// Text consumes the response body as UTF-8 text.
func (response Response) Text() (string, error) {
	body, err := response.consumeBody()
	if err != nil {
		return "", err
	}
	if !utf8.Valid(body) {
		return "", ErrInvalidText
	}
	return string(body), nil
}

func (response Response) BodyUsed() bool { return response.body != nil && response.body.consumed }

// JSON consumes and decodes the response body into target.
func (response Response) JSON(target any) error {
	body, err := response.consumeBody()
	if err != nil {
		return err
	}
	return json.Unmarshal(body, target)
}

func (response Response) consumeBody() ([]byte, error) {
	if response.body == nil {
		return nil, nil
	}
	if response.body.consumed {
		return nil, ErrBodyConsumed
	}
	response.body.consumed = true
	return response.body.value, nil
}

// API binds WebGo Fetch calls to one page URL and network executor.
type API struct {
	ctx      context.Context
	baseURL  *url.URL
	do       func(context.Context, *network.Request) (*network.Response, error)
	enqueue  func(func()) bool
	clock    Clock
	mu       sync.Mutex
	closed   bool
	inFlight int
	active   sync.WaitGroup
}

// New creates a page-scoped Fetch API.
func New(baseURL *url.URL, do func(context.Context, *network.Request) (*network.Response, error)) *API {
	return NewPage(context.Background(), baseURL, do, func(callback func()) bool {
		callback()
		return true
	})
}

// NewPage creates an asynchronous Fetch API bound to a page callback queue.
func NewPage(ctx context.Context, baseURL *url.URL, do func(context.Context, *network.Request) (*network.Response, error), enqueue func(func()) bool) *API {
	if ctx == nil {
		ctx = context.Background()
	}
	return NewPageWithClock(ctx, baseURL, do, enqueue, systemClock{})
}

func NewPageWithClock(ctx context.Context, baseURL *url.URL, do func(context.Context, *network.Request) (*network.Response, error), enqueue func(func()) bool, clock Clock) *API {
	if ctx == nil {
		ctx = context.Background()
	}
	if clock == nil {
		clock = systemClock{}
	}
	return &API{ctx: ctx, baseURL: cloneURL(baseURL), do: do, enqueue: enqueue, clock: clock}
}

// Fetch starts request asynchronously and delivers exactly one callback.
func (api *API) Fetch(request Request, success func(Response), failure func(string)) {
	api.mu.Lock()
	closed := api.closed
	api.mu.Unlock()
	if closed {
		return
	}
	if request.Signal != nil && request.Signal.Aborted() {
		api.deliver(func() {
			if failure != nil {
				failure("AbortError: Fetch was aborted")
			}
		})
		return
	}
	networkRequest, err := api.prepare(request)
	if err != nil {
		api.deliver(func() {
			if failure != nil {
				failure(err.Error())
			}
		})
		return
	}
	api.mu.Lock()
	if api.closed {
		api.mu.Unlock()
		return
	}
	if api.inFlight >= 16 {
		api.mu.Unlock()
		api.deliver(func() {
			if failure != nil {
				failure("QuotaError: Fetch concurrency limit reached")
			}
		})
		return
	}
	api.inFlight++
	api.active.Add(1)
	api.mu.Unlock()
	go func() {
		defer func() { api.mu.Lock(); api.inFlight--; api.mu.Unlock(); api.active.Done() }()
		ctx, cancel := context.WithCancel(api.ctx)
		unsubscribe := request.Signal.subscribe(cancel)
		timedOut := make(chan struct{})
		var timer Timer
		if request.Timeout > 0 {
			timer = api.clock.AfterFunc(request.Timeout, func() { close(timedOut); cancel() })
		}
		response, fetchError := api.do(ctx, networkRequest)
		if timer != nil {
			timer.Stop()
		}
		unsubscribe()
		cancel()
		api.deliver(func() {
			select {
			case <-timedOut:
				if failure != nil {
					failure("TimeoutError: Fetch timed out")
				}
				return
			default:
			}
			if request.Signal != nil && request.Signal.Aborted() {
				if failure != nil {
					failure("AbortError: Fetch was aborted")
				}
				return
			}
			if fetchError != nil {
				if failure != nil {
					failure(fetchError.Error())
				}
				return
			}
			if success != nil {
				success(newResponse(response))
			}
		})
	}()
}

// Close rejects new Fetches and waits for all request goroutines to release references.
func (api *API) Close() {
	if api == nil {
		return
	}
	api.mu.Lock()
	api.closed = true
	api.mu.Unlock()
	api.active.Wait()
	api.mu.Lock()
	api.ctx = nil
	api.baseURL = nil
	api.do = nil
	api.enqueue = nil
	api.mu.Unlock()
}

func (api *API) deliver(callback func()) {
	if api == nil || callback == nil || api.enqueue == nil {
		return
	}
	api.enqueue(callback)
}

func newResponse(response *network.Response) Response {
	if response == nil {
		return Response{}
	}
	header := make(Header, len(response.Header))
	entries := make([]HeaderEntry, 0)
	for name, values := range response.Header {
		header[name] = append([]string(nil), values...)
		for _, value := range values {
			entries = append(entries, HeaderEntry{Name: name, Value: value})
		}
	}
	finalURL := ""
	if response.URL != nil {
		finalURL = response.URL.String()
	}
	return Response{
		Status:     response.StatusCode,
		StatusText: response.Status,
		URL:        finalURL,
		Redirected: response.Redirected,
		Header:     header,
		Headers:    &ResponseHeaders{entries: entries},
		body:       &responseBody{value: append([]byte(nil), response.Body...)},
	}
}

func (api *API) fetch(ctx context.Context, request Request) (*network.Response, error) {
	networkRequest, err := api.prepare(request)
	if err != nil {
		return nil, err
	}
	return api.do(ctx, networkRequest)
}

func (api *API) prepare(request Request) (*network.Request, error) {
	if api == nil || api.do == nil {
		return nil, errors.New("Fetch is not configured")
	}
	reference, err := url.Parse(strings.TrimSpace(request.URL))
	if err != nil {
		return nil, err
	}
	if api.baseURL == nil && !reference.IsAbs() {
		return nil, errors.New("relative Fetch URL has no page URL")
	}
	resolved := reference
	if api.baseURL != nil {
		resolved = api.baseURL.ResolveReference(reference)
	}
	if resolved.Scheme != "http" && resolved.Scheme != "https" || resolved.Host == "" {
		return nil, errors.New("Fetch URL must use HTTP or HTTPS")
	}

	body, contentType, hasBody, err := requestBody(request)
	if err != nil {
		return nil, err
	}
	method := strings.ToUpper(strings.TrimSpace(request.Method))
	if method == "" {
		method = http.MethodGet
	}
	if !allowedMethod(method) || !validToken(method) {
		return nil, errors.New("invalid or unsupported Fetch method")
	}
	if (method == http.MethodGet || method == http.MethodHead) && hasBody {
		return nil, errors.New("GET and HEAD Fetch requests cannot have a body")
	}
	credentials := network.CredentialsMode(request.Credentials)
	if credentials == "" {
		credentials = network.CredentialsSameOrigin
	}
	if credentials != network.CredentialsOmit && credentials != network.CredentialsSameOrigin && credentials != network.CredentialsInclude {
		return nil, errors.New("invalid Fetch credentials mode")
	}
	if request.Header != nil && request.Headers != nil {
		return nil, errors.New("Fetch request cannot use both Header and Headers")
	}
	headers := request.Headers
	if headers == nil {
		headers, err = legacyHeaders(request.Header)
		if err != nil {
			return nil, err
		}
	}
	header, err := headers.httpHeader()
	if err != nil {
		return nil, err
	}
	if contentType != "" && header.Get("Content-Type") == "" {
		header.Set("Content-Type", contentType)
	}
	return &network.Request{
		Method:      method,
		URL:         resolved,
		Header:      header,
		Body:        body,
		SiteURL:     cloneURL(api.baseURL),
		Kind:        network.RequestFetch,
		Credentials: credentials,
	}, nil
}

const maxRequestBodySize = 1024 * 1024

func requestBody(request Request) ([]byte, string, bool, error) {
	count := 0
	if request.Body != nil {
		count++
	}
	if request.Text != "" {
		count++
	}
	if request.JSON != "" {
		count++
	}
	if request.Params != nil {
		count++
	}
	if request.FormData != nil {
		count++
	}
	if count > 1 {
		return nil, "", false, errors.New("Fetch request body must use exactly one body type")
	}
	if count == 0 {
		return nil, "", false, nil
	}
	var body []byte
	contentType := ""
	switch {
	case request.Body != nil:
		body = append([]byte(nil), request.Body...)
	case request.Text != "":
		if !utf8.ValidString(request.Text) {
			return nil, "", false, errors.New("invalid Fetch text body")
		}
		body, contentType = []byte(request.Text), "text/plain;charset=UTF-8"
	case request.JSON != "":
		if !utf8.ValidString(request.JSON) || !json.Valid([]byte(request.JSON)) {
			return nil, "", false, errors.New("invalid Fetch JSON body")
		}
		body, contentType = []byte(request.JSON), "application/json"
	case request.Params != nil:
		encoded, err := request.Params.Encode()
		if err != nil {
			return nil, "", false, fmt.Errorf("encode Fetch URLSearchParams body: %w", err)
		}
		body, contentType = []byte(encoded), "application/x-www-form-urlencoded;charset=UTF-8"
	case request.FormData != nil:
		encoded, err := request.FormData.Encode()
		if err != nil {
			return nil, "", false, fmt.Errorf("encode Fetch FormData body: %w", err)
		}
		body, contentType = []byte(encoded), formapi.ContentTypeURLEncoded
	}
	if len(body) > maxRequestBodySize {
		return nil, "", false, errors.New("Fetch request body exceeds size limit")
	}
	return body, contentType, true, nil
}

func allowedMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func forbiddenHeader(name string) bool {
	lower := strings.ToLower(name)
	if strings.HasPrefix(lower, "proxy-") || strings.HasPrefix(lower, "sec-") {
		return true
	}
	switch lower {
	case "accept-charset", "accept-encoding", "access-control-request-headers", "access-control-request-method",
		"connection", "content-length", "cookie", "cookie2", "date", "dnt", "expect", "host",
		"keep-alive", "origin", "permissions-policy", "referer", "te", "trailer", "transfer-encoding", "upgrade", "via":
		return true
	default:
		return false
	}
}

func validToken(value string) bool {
	if value == "" {
		return false
	}
	for index := 0; index < len(value); index++ {
		character := value[index]
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' ||
			character >= '0' && character <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", rune(character)) {
			continue
		}
		return false
	}
	return true
}

func cloneURL(source *url.URL) *url.URL {
	if source == nil {
		return nil
	}
	copy := *source
	return &copy
}
