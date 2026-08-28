package isolated

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/Grove-Computing/Growse/internal/network"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
)

const (
	maxBrokerRequestBodyBytes = 1 << 20
	maxBrokerHeaderCount      = 100
	maxBrokerHeaderBytes      = 64 << 10
	maxBrokerHeaderNameBytes  = 256
	maxBrokerHeaderValueBytes = 8 << 10
)

func validateBrokeredFetch(request *network.Request, pageURL *url.URL, engine string) error {
	if request == nil || request.URL == nil || pageURL == nil {
		return errors.New("sandbox fetch requires request and page URLs")
	}
	if !httpURL(request.URL) || request.URL.User != nil {
		return errors.New("sandbox fetch URL must be HTTP(S) without userinfo")
	}
	requestedKind := request.Kind
	method := strings.ToUpper(strings.TrimSpace(request.Method))
	if method == "" {
		method = http.MethodGet
	}
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
	default:
		return fmt.Errorf("sandbox fetch method %q is not allowed", method)
	}
	resourceRequest := requestedKind == network.RequestModule || requestedKind == network.RequestScript ||
		requestedKind == network.RequestStylesheet || requestedKind == network.RequestImage || requestedKind == network.RequestSubresource
	if resourceRequest {
		if engine != string(runtimemodel.EngineJavaScript) || method != http.MethodGet || len(request.Body) != 0 || len(request.Header) != 0 {
			return errors.New("sandbox script fetch must be a header-free JavaScript GET")
		}
	}
	if requestedKind == network.RequestModule {
		if request.Credentials != network.CredentialsInclude {
			request.Credentials = network.CredentialsSameOrigin
		}
		request.CORS = true
	} else if resourceRequest {
		if request.CORS && request.Credentials != network.CredentialsInclude {
			request.Credentials = network.CredentialsSameOrigin
		} else if !request.CORS && request.Credentials == "" {
			request.Credentials = network.CredentialsInclude
		}
	}
	if len(request.Body) > maxBrokerRequestBodyBytes {
		return errors.New("sandbox fetch body exceeds size limit")
	}
	if err := validateBrokeredHeaders(request.Header); err != nil {
		return err
	}
	switch request.Credentials {
	case "", network.CredentialsOmit, network.CredentialsSameOrigin, network.CredentialsInclude:
	default:
		return errors.New("sandbox fetch credentials mode is invalid")
	}
	request.Method = method
	request.SiteURL = publicRuntimeURL(pageURL)
	if requestedKind == network.RequestModule {
		request.Kind = network.RequestModule
	} else if resourceRequest {
		request.Kind = requestedKind
	} else {
		request.Kind = network.RequestFetch
	}
	request.Engine = engine
	return nil
}

func validateBrokeredHeaders(header http.Header) error {
	count, size := 0, 0
	for name, values := range header {
		if len(name) == 0 || len(name) > maxBrokerHeaderNameBytes || !validHeaderName(name) || forbiddenBrokerHeader(name) {
			return fmt.Errorf("sandbox fetch header %q is forbidden or invalid", name)
		}
		for _, value := range values {
			count++
			if !utf8.ValidString(value) || len(value) > maxBrokerHeaderValueBytes || strings.ContainsAny(value, "\r\n\x00") {
				return fmt.Errorf("sandbox fetch header %q has an invalid value", name)
			}
			size += len(name) + len(value)
			if count > maxBrokerHeaderCount || size > maxBrokerHeaderBytes {
				return errors.New("sandbox fetch headers exceed size limit")
			}
		}
	}
	return nil
}

func forbiddenBrokerHeader(name string) bool {
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

func validHeaderName(value string) bool {
	for index := 0; index < len(value); index++ {
		character := value[index]
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' ||
			character >= '0' && character <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", rune(character)) {
			continue
		}
		return false
	}
	return value != ""
}

func publicRuntimeURL(source *url.URL) *url.URL {
	if source == nil {
		return nil
	}
	copy := *source
	copy.User = nil
	return &copy
}

func httpURL(target *url.URL) bool {
	return target != nil && (strings.EqualFold(target.Scheme, "http") || strings.EqualFold(target.Scheme, "https")) && target.Host != ""
}
