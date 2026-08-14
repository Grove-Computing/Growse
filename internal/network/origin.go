package network

import (
	"errors"
	"net/url"
	"strings"
)

// ErrInvalidOrigin reports a URL that cannot identify an HTTP(S) origin.
var ErrInvalidOrigin = errors.New("invalid HTTP origin")

// Origin is the scheme, host, and effective port security boundary of a URL.
type Origin struct {
	Scheme string
	Host   string
	Port   string
}

// OriginFromURL returns the normalized HTTP(S) origin of target.
func OriginFromURL(target *url.URL) (Origin, error) {
	if target == nil {
		return Origin{}, ErrInvalidOrigin
	}
	scheme := strings.ToLower(target.Scheme)
	host := strings.ToLower(strings.TrimSuffix(target.Hostname(), "."))
	if host == "" || scheme != "http" && scheme != "https" {
		return Origin{}, ErrInvalidOrigin
	}
	port := target.Port()
	if port == "" {
		if scheme == "http" {
			port = "80"
		} else {
			port = "443"
		}
	}
	return Origin{Scheme: scheme, Host: host, Port: port}, nil
}

// SameOrigin reports whether two URLs share scheme, host, and effective port.
func SameOrigin(left, right *url.URL) bool {
	leftOrigin, leftError := OriginFromURL(left)
	rightOrigin, rightError := OriginFromURL(right)
	return leftError == nil && rightError == nil && leftOrigin == rightOrigin
}

// String serializes an origin for the HTTP Origin header.
func (origin Origin) String() string {
	defaultPort := origin.Scheme == "http" && origin.Port == "80" || origin.Scheme == "https" && origin.Port == "443"
	host := origin.Host
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	if defaultPort {
		return origin.Scheme + "://" + host
	}
	return origin.Scheme + "://" + host + ":" + origin.Port
}
