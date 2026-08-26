package network

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

const maxPreflightAge = 2 * time.Hour

var (
	ErrCORS                  = errors.New("CORS policy rejected the response")
	ErrCORSPreflightRequired = errors.New("CORS preflight is required")
)

func applyCORSRequest(request *http.Request, requestData *Request) error {
	if request == nil || requestData == nil || !requiresCORS(requestData) {
		return nil
	}
	request.Header.Del("Origin")
	if SameOrigin(requestData.SiteURL, request.URL) {
		return nil
	}
	origin, err := OriginFromURL(requestData.SiteURL)
	if err != nil {
		return ErrCORS
	}
	if !isSimpleCORSRequest(request) {
		return ErrCORSPreflightRequired
	}
	request.Header.Set("Origin", origin.String())
	return nil
}

func (client *Client) prepareCORS(ctx context.Context, httpClient *http.Client, request *http.Request, requestData *Request) error {
	err := applyCORSRequest(request, requestData)
	if !errors.Is(err, ErrCORSPreflightRequired) {
		return err
	}
	if err := client.ensurePreflight(ctx, httpClient, request, requestData); err != nil {
		return err
	}
	origin, err := OriginFromURL(requestData.SiteURL)
	if err != nil {
		return ErrCORS
	}
	request.Header.Set("Origin", origin.String())
	return nil
}

func (client *Client) ensurePreflight(ctx context.Context, httpClient *http.Client, request *http.Request, requestData *Request) error {
	key, err := preflightKey(request, requestData)
	if err != nil {
		return err
	}
	now := client.now()
	client.preflightMu.Lock()
	expires, cached := client.preflightCache[key]
	client.preflightMu.Unlock()
	if cached && now.Before(expires) {
		return nil
	}
	origin, err := OriginFromURL(requestData.SiteURL)
	if err != nil {
		return ErrCORS
	}
	headerNames := corsUnsafeHeaderNames(request.Header)
	preflight, err := http.NewRequestWithContext(ctx, http.MethodOptions, request.URL.String(), nil) // #nosec G704 -- browser fetch URLs are intentionally remote; CORS validation and the caller's network policy govern the request.
	if err != nil {
		return fmt.Errorf("create CORS preflight: %w", err)
	}
	preflight.Header.Set("Origin", origin.String())
	preflight.Header.Set("Access-Control-Request-Method", request.Method)
	if len(headerNames) != 0 {
		preflight.Header.Set("Access-Control-Request-Headers", strings.Join(headerNames, ", "))
	}
	preflightClient := *httpClient
	preflightClient.Jar = nil
	preflightClient.CheckRedirect = func(*http.Request, []*http.Request) error { return ErrCORS }
	response, err := preflightClient.Do(preflight) // #nosec G704 -- this is the validated CORS preflight for the browser request above.
	if err != nil {
		return classifyRequestError(err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices ||
		validateCORSResponse(response, requestData) != nil || !headerListContains(response.Header.Get("Access-Control-Allow-Methods"), request.Method) {
		return ErrCORS
	}
	for _, name := range headerNames {
		if !headerListContains(response.Header.Get("Access-Control-Allow-Headers"), name) {
			return ErrCORS
		}
	}
	seconds, _ := strconv.ParseInt(strings.TrimSpace(response.Header.Get("Access-Control-Max-Age")), 10, 64)
	if seconds > 0 {
		age := time.Duration(seconds) * time.Second
		if age > maxPreflightAge {
			age = maxPreflightAge
		}
		client.preflightMu.Lock()
		client.preflightCache[key] = now.Add(age)
		client.preflightMu.Unlock()
	}
	return nil
}

func preflightKey(request *http.Request, requestData *Request) (string, error) {
	requestOrigin, err := OriginFromURL(request.URL)
	if err != nil {
		return "", ErrCORS
	}
	siteOrigin, err := OriginFromURL(requestData.SiteURL)
	if err != nil {
		return "", ErrCORS
	}
	return siteOrigin.String() + "|" + requestOrigin.String() + "|" + request.Method + "|" +
		strings.Join(corsUnsafeHeaderNames(request.Header), ",") + "|" + string(requestData.Credentials), nil
}

func corsUnsafeHeaderNames(header http.Header) []string {
	result := make([]string, 0)
	for name, values := range header {
		probe := &http.Request{Method: http.MethodGet, Header: http.Header{name: values}}
		if isSimpleCORSRequest(probe) {
			continue
		}
		lower := strings.ToLower(name)
		if lower != "cookie" && lower != "origin" && lower != "user-agent" {
			result = append(result, lower)
		}
	}
	sort.Strings(result)
	return result
}

func headerListContains(value, wanted string) bool {
	for _, item := range strings.Split(value, ",") {
		if strings.EqualFold(strings.TrimSpace(item), wanted) || strings.TrimSpace(item) == "*" {
			return true
		}
	}
	return false
}

func filterFetchResponseHeaders(header http.Header, requestData *Request, finalURL *url.URL) http.Header {
	result := header.Clone()
	result.Del("Set-Cookie")
	result.Del("Set-Cookie2")
	if requestData == nil || requestData.Kind != RequestFetch || SameOrigin(requestData.SiteURL, finalURL) {
		return result
	}
	exposed := make(map[string]bool)
	for _, name := range []string{"cache-control", "content-language", "content-length", "content-type", "expires", "last-modified", "pragma"} {
		exposed[name] = true
	}
	wildcard := false
	for _, item := range strings.Split(header.Get("Access-Control-Expose-Headers"), ",") {
		name := strings.ToLower(strings.TrimSpace(item))
		if name == "*" && requestData.Credentials != CredentialsInclude {
			wildcard = true
		} else if name != "" {
			exposed[name] = true
		}
	}
	filtered := make(http.Header)
	for name, values := range result {
		lower := strings.ToLower(name)
		if lower != "set-cookie" && lower != "set-cookie2" && (wildcard || exposed[lower]) {
			filtered[name] = append([]string(nil), values...)
		}
	}
	return filtered
}

func validateCORSResponse(response *http.Response, requestData *Request) error {
	if response == nil || response.Request == nil || requestData == nil || !requiresCORS(requestData) ||
		SameOrigin(requestData.SiteURL, response.Request.URL) {
		return nil
	}
	origin, err := OriginFromURL(requestData.SiteURL)
	if err != nil {
		return ErrCORS
	}
	allowedOrigin := strings.TrimSpace(response.Header.Get("Access-Control-Allow-Origin"))
	if requestData.Credentials == CredentialsInclude && !strings.EqualFold(strings.TrimSpace(response.Header.Get("Access-Control-Allow-Credentials")), "true") {
		return ErrCORS
	}
	if allowedOrigin == origin.String() {
		return nil
	}
	if allowedOrigin == "*" && requestData.Credentials != CredentialsInclude {
		return nil
	}
	return ErrCORS
}

func requiresCORS(request *Request) bool {
	return request != nil && (request.Kind == RequestFetch || request.CORS)
}

func isSimpleCORSRequest(request *http.Request) bool {
	if request == nil || request.Method != http.MethodGet && request.Method != http.MethodHead && request.Method != http.MethodPost {
		return false
	}
	for name, values := range request.Header {
		switch strings.ToLower(name) {
		case "accept", "accept-language", "content-language", "origin", "user-agent", "cookie":
			continue
		case "content-type":
			for _, value := range values {
				mediaType, _, err := mime.ParseMediaType(value)
				if err != nil || mediaType != "application/x-www-form-urlencoded" && mediaType != "multipart/form-data" && mediaType != "text/plain" {
					return false
				}
			}
		default:
			return false
		}
	}
	return true
}
