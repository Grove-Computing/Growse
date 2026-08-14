package network

import (
	"errors"
	"mime"
	"net/http"
	"strings"
)

var (
	ErrCORS                  = errors.New("CORS policy rejected the response")
	ErrCORSPreflightRequired = errors.New("CORS preflight is required")
)

func applyCORSRequest(request *http.Request, requestData *Request) error {
	if request == nil || requestData == nil || requestData.Kind != RequestFetch {
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

func validateCORSResponse(response *http.Response, requestData *Request) error {
	if response == nil || response.Request == nil || requestData == nil || requestData.Kind != RequestFetch ||
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
