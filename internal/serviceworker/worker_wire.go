package serviceworker

import (
	"context"
	"errors"
	"net/http"
	"net/url"

	"github.com/Grove-Computing/Growse/internal/network"
)

type workerLoadRequest struct {
	ScriptURL string `json:"scriptUrl"`
	Source    uint64 `json:"source"`
}
type workerLoadResponse struct {
	Constraints []string `json:"constraints,omitempty"`
}

type workerLifecycleRequest struct {
	Activate bool `json:"activate"`
}

type workerLifecycleResponse struct {
	SkipWaiting bool `json:"skipWaiting"`
	Claim       bool `json:"claim"`
}

type workerFetchRequest struct{ Request wireNetworkRequest }
type workerFetchResponse struct{ Response wireNetworkResponse }

type wireNetworkRequest struct {
	Method      string                  `json:"method"`
	URL         string                  `json:"url"`
	Header      http.Header             `json:"header,omitempty"`
	Body        uint64                  `json:"body,omitempty"`
	SiteURL     string                  `json:"siteUrl,omitempty"`
	Kind        network.RequestKind     `json:"kind"`
	Credentials network.CredentialsMode `json:"credentials,omitempty"`
	CORS        bool                    `json:"cors,omitempty"`
}

type wireNetworkResponse struct {
	Found       bool        `json:"found"`
	URL         string      `json:"url,omitempty"`
	StatusCode  int         `json:"statusCode,omitempty"`
	Status      string      `json:"status,omitempty"`
	Header      http.Header `json:"header,omitempty"`
	ContentType string      `json:"contentType,omitempty"`
	Body        uint64      `json:"body,omitempty"`
	Redirected  bool        `json:"redirected,omitempty"`
	CacheStatus string      `json:"cacheStatus,omitempty"`
}

type workerCacheNameRequest struct {
	Name string `json:"name"`
}
type workerCacheNamesResponse struct {
	Names []string `json:"names,omitempty"`
}
type workerCacheRequest struct {
	Name    string             `json:"name,omitempty"`
	Request wireNetworkRequest `json:"request"`
}
type workerCachePutRequest struct {
	Name     string              `json:"name"`
	Request  wireNetworkRequest  `json:"request"`
	Response wireNetworkResponse `json:"response"`
}
type workerCacheMatchResponse struct {
	Response wireNetworkResponse `json:"response"`
}
type workerCacheKeysResponse struct {
	Requests []wireNetworkRequest `json:"requests,omitempty"`
}
type workerBoolResponse struct {
	Value bool `json:"value"`
}

func requestToWire(ctx context.Context, blobs *workerBlobStore, request *network.Request) (wireNetworkRequest, error) {
	if request == nil || request.URL == nil {
		return wireNetworkRequest{}, errors.New("service worker request is invalid")
	}
	body, err := blobs.send(ctx, request.Body)
	if err != nil {
		return wireNetworkRequest{}, err
	}
	return wireNetworkRequest{
		Method: request.Method, URL: request.URL.String(), Header: request.Header.Clone(), Body: body,
		SiteURL: urlString(request.SiteURL), Kind: request.Kind, Credentials: request.Credentials, CORS: request.CORS,
	}, nil
}

func requestFromWire(blobs *workerBlobStore, request wireNetworkRequest) (*network.Request, error) {
	requestURL, err := url.Parse(request.URL)
	if err != nil || requestURL == nil || requestURL.Host == "" {
		return nil, errors.New("service worker request URL is invalid")
	}
	siteURL, err := parseOptionalURL(request.SiteURL)
	if err != nil {
		return nil, errors.New("service worker site URL is invalid")
	}
	body, err := blobs.take(request.Body)
	if err != nil {
		return nil, err
	}
	return &network.Request{
		Method: request.Method, URL: requestURL, Header: request.Header.Clone(), Body: body, SiteURL: siteURL,
		Kind: request.Kind, Credentials: request.Credentials, CORS: request.CORS,
	}, nil
}

func responseToWire(ctx context.Context, blobs *workerBlobStore, response *network.Response) (wireNetworkResponse, error) {
	if response == nil {
		return wireNetworkResponse{}, nil
	}
	body, err := blobs.send(ctx, response.Body)
	if err != nil {
		return wireNetworkResponse{}, err
	}
	return wireNetworkResponse{
		Found: true, URL: urlString(response.URL), StatusCode: response.StatusCode, Status: response.Status,
		Header: response.Header.Clone(), ContentType: response.ContentType, Body: body,
		Redirected: response.Redirected, CacheStatus: response.CacheStatus,
	}, nil
}

func responseFromWire(blobs *workerBlobStore, response wireNetworkResponse) (*network.Response, error) {
	if !response.Found {
		return nil, nil
	}
	responseURL, err := parseOptionalURL(response.URL)
	if err != nil {
		return nil, errors.New("service worker response URL is invalid")
	}
	body, err := blobs.take(response.Body)
	if err != nil {
		return nil, err
	}
	return &network.Response{
		URL: responseURL, StatusCode: response.StatusCode, Status: response.Status, Header: response.Header.Clone(),
		ContentType: response.ContentType, Body: body, Redirected: response.Redirected, CacheStatus: response.CacheStatus,
	}, nil
}
