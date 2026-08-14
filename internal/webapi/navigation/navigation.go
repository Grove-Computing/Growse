// Package navigation はWebGoへ現在のDocument URLを安全な値として公開する。
package navigation

import (
	"net/url"

	"github.com/Grove-Computing/Growse/internal/network"
)

// Location は現在のDocument URLをWebGoから参照するための不変な値である。
// QueryとFragmentには先頭の?と#を含めない。
type Location struct {
	Href     string
	Origin   string
	Scheme   string
	Host     string
	Hostname string
	Port     string
	Path     string
	Query    string
	Fragment string
}

// API は1つのPageに属するNavigation APIである。
type API struct {
	current Location
}

// New はDocument URLを読み取り専用のWebGo APIへ変換する。
func New(documentURL *url.URL) *API {
	return &API{current: locationFromURL(documentURL)}
}

// Current は現在のDocument URLの構成要素を返す。
func (api *API) Current() Location {
	if api == nil {
		return Location{}
	}
	return api.current
}

func locationFromURL(documentURL *url.URL) Location {
	if documentURL == nil {
		return Location{}
	}
	publicURL := *documentURL
	publicURL.User = nil
	location := Location{
		Href:     publicURL.String(),
		Scheme:   publicURL.Scheme,
		Host:     publicURL.Host,
		Hostname: publicURL.Hostname(),
		Port:     publicURL.Port(),
		Path:     publicURL.EscapedPath(),
		Query:    publicURL.RawQuery,
		Fragment: publicURL.Fragment,
	}
	if origin, err := network.OriginFromURL(&publicURL); err == nil {
		location.Origin = origin.String()
	}
	return location
}
