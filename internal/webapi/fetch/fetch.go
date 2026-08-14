// Package fetch provides the HTTP API exposed to WebGo programs.
package fetch

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/Grove-Computing/Growse/internal/network"
)

// Header preserves every value associated with an HTTP header name.
type Header map[string][]string

// Request describes an HTTP request issued by a WebGo program.
// Body takes precedence over Text when both are set.
type Request struct {
	Method string
	URL    string
	Header Header
	Body   []byte
	Text   string
}

// Response is delivered to a successful Fetch callback. Additional response
// metadata and body helpers are added by the HTTP lifecycle implementation.
type Response struct {
	Status     int
	StatusText string
	URL        string
	Redirected bool
	Header     Header
	body       *responseBody
}

type responseBody struct {
	value    []byte
	consumed bool
}

// ErrBodyConsumed reports a second attempt to consume one response body.
var ErrBodyConsumed = errors.New("response body has already been consumed")

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
	return string(body), nil
}

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
	ctx     context.Context
	baseURL *url.URL
	do      func(context.Context, *network.Request) (*network.Response, error)
	enqueue func(func()) bool
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
	return &API{ctx: ctx, baseURL: cloneURL(baseURL), do: do, enqueue: enqueue}
}

// Fetch starts request asynchronously and delivers exactly one callback.
func (api *API) Fetch(request Request, success func(Response), failure func(string)) {
	networkRequest, err := api.prepare(request)
	if err != nil {
		api.deliver(func() {
			if failure != nil {
				failure(err.Error())
			}
		})
		return
	}
	go func() {
		response, fetchError := api.do(api.ctx, networkRequest)
		api.deliver(func() {
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
	for name, values := range response.Header {
		header[name] = append([]string(nil), values...)
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

	hasBody := request.Body != nil || request.Text != ""
	body := append([]byte(nil), request.Body...)
	if request.Body == nil && request.Text != "" {
		body = []byte(request.Text)
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
	header := make(http.Header, len(request.Header))
	for name, values := range request.Header {
		if !validToken(name) {
			return nil, errors.New("invalid Fetch header name")
		}
		if forbiddenHeader(name) {
			return nil, errors.New("forbidden Fetch request header: " + name)
		}
		for _, value := range values {
			if strings.ContainsAny(value, "\r\n\x00") {
				return nil, errors.New("invalid Fetch header value")
			}
		}
		header[name] = append([]string(nil), values...)
	}
	return &network.Request{
		Method:  method,
		URL:     resolved,
		Header:  header,
		Body:    body,
		SiteURL: cloneURL(api.baseURL),
		Kind:    network.RequestFetch,
	}, nil
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
