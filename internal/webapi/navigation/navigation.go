// Package navigation はWebGoへ現在のDocument URLを安全な値として公開する。
package navigation

import (
	"errors"
	"net/url"
	"strings"

	"github.com/Grove-Computing/Growse/internal/network"
)

const maxURLBytes = 8192

var (
	// ErrInvalidURL はNavigation対象として扱えないURLを表す。
	ErrInvalidURL = errors.New("invalid navigation URL")
	// ErrUnavailable はBrowserにNavigation処理が接続されていないことを表す。
	ErrUnavailable = errors.New("navigation is unavailable")
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
	base     *url.URL
	current  Location
	navigate func(*url.URL) error
}

// New はDocument URLを読み取り専用のWebGo APIへ変換する。
func New(documentURL *url.URL) *API {
	return NewPage(documentURL, nil)
}

// NewPage はDocument URLとBrowserのNavigation処理を結び付ける。
func NewPage(documentURL *url.URL, navigate func(*url.URL) error) *API {
	return &API{base: cloneURL(documentURL), current: locationFromURL(documentURL), navigate: navigate}
}

// Resolve は現在のDocument URLを基準にrawURLを解決する。
func (api *API) Resolve(rawURL string) (Location, error) {
	target, err := api.resolve(rawURL)
	if err != nil {
		return Location{}, err
	}
	return locationFromURL(target), nil
}

// Navigate は検証・解決済みURLへのBrowser Navigationを要求する。
// Network完了を待たず、commit前に検出できるErrorだけを返す。
func (api *API) Navigate(rawURL string) error {
	target, err := api.resolve(rawURL)
	if err != nil {
		return err
	}
	if api.navigate == nil {
		return ErrUnavailable
	}
	return api.navigate(target)
}

func (api *API) resolve(rawURL string) (*url.URL, error) {
	if api == nil || api.base == nil || len(rawURL) == 0 || len(rawURL) > maxURLBytes || strings.TrimSpace(rawURL) != rawURL {
		return nil, ErrInvalidURL
	}
	reference, err := url.Parse(rawURL)
	if err != nil {
		return nil, ErrInvalidURL
	}
	target := api.base.ResolveReference(reference)
	if target.Scheme != "http" && target.Scheme != "https" || target.Hostname() == "" || target.User != nil {
		return nil, ErrInvalidURL
	}
	if _, err := network.OriginFromURL(target); err != nil {
		return nil, ErrInvalidURL
	}
	return target, nil
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
		location.Port = origin.Port
	}
	return location
}

func cloneURL(source *url.URL) *url.URL {
	if source == nil {
		return nil
	}
	copy := *source
	return &copy
}
