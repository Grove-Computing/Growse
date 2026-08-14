// Package navigation はWebGoへ現在のDocument URLを安全な値として公開する。
package navigation

import (
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"sync"

	"github.com/Grove-Computing/Growse/internal/network"
)

const (
	maxURLBytes         = 8192
	MaxHistoryStateSize = 64 * 1024
)

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
	mu           sync.RWMutex
	base         *url.URL
	current      Location
	navigate     func(*url.URL) error
	pushState    func(string, *url.URL) error
	replaceState func(string, *url.URL) error
}

// SetPushStateHandler はsame-document History entryの追加先を設定する。
func (api *API) SetPushStateHandler(handler func(string, *url.URL) error) {
	if api == nil {
		return
	}
	api.mu.Lock()
	api.pushState = handler
	api.mu.Unlock()
}

// SetReplaceStateHandler は現在History entryの置換先を設定する。
func (api *API) SetReplaceStateHandler(handler func(string, *url.URL) error) {
	if api == nil {
		return
	}
	api.mu.Lock()
	api.replaceState = handler
	api.mu.Unlock()
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

// PushState はJSON stateとsame-origin URLを新しいHistory entryへ追加する。
func (api *API) PushState(stateJSON, rawURL string) error {
	if err := validateHistoryState(stateJSON); err != nil {
		return err
	}
	target, err := api.resolveHistoryURL(rawURL)
	if err != nil {
		return err
	}
	api.mu.RLock()
	handler := api.pushState
	api.mu.RUnlock()
	if handler == nil {
		return ErrUnavailable
	}
	if err := handler(stateJSON, target); err != nil {
		return err
	}
	api.UpdateCurrent(target)
	return nil
}

// ReplaceState は現在History entryのJSON stateとsame-origin URLを置換する。
func (api *API) ReplaceState(stateJSON, rawURL string) error {
	if err := validateHistoryState(stateJSON); err != nil {
		return err
	}
	target, err := api.resolveHistoryURL(rawURL)
	if err != nil {
		return err
	}
	api.mu.RLock()
	handler := api.replaceState
	api.mu.RUnlock()
	if handler == nil {
		return ErrUnavailable
	}
	if err := handler(stateJSON, target); err != nil {
		return err
	}
	api.UpdateCurrent(target)
	return nil
}

func validateHistoryState(stateJSON string) error {
	if len(stateJSON) == 0 || len(stateJSON) > MaxHistoryStateSize || !json.Valid([]byte(stateJSON)) {
		return errors.New("invalid history state")
	}
	return nil
}

func (api *API) resolveHistoryURL(rawURL string) (*url.URL, error) {
	api.mu.RLock()
	base := cloneURL(api.base)
	api.mu.RUnlock()
	if base == nil {
		return nil, ErrInvalidURL
	}
	if rawURL == "" {
		return base, nil
	}
	target, err := api.resolve(rawURL)
	if err != nil || !network.SameOrigin(base, target) {
		return nil, ErrInvalidURL
	}
	return target, nil
}

func (api *API) resolve(rawURL string) (*url.URL, error) {
	if api == nil || len(rawURL) == 0 || len(rawURL) > maxURLBytes || strings.TrimSpace(rawURL) != rawURL {
		return nil, ErrInvalidURL
	}
	api.mu.RLock()
	base := cloneURL(api.base)
	api.mu.RUnlock()
	if base == nil {
		return nil, ErrInvalidURL
	}
	reference, err := url.Parse(rawURL)
	if err != nil {
		return nil, ErrInvalidURL
	}
	target := base.ResolveReference(reference)
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
	api.mu.RLock()
	defer api.mu.RUnlock()
	return api.current
}

// UpdateCurrent はsame-document Navigation後のDocument URLを反映する。
func (api *API) UpdateCurrent(documentURL *url.URL) {
	if api == nil {
		return
	}
	api.mu.Lock()
	api.base = cloneURL(documentURL)
	api.current = locationFromURL(documentURL)
	api.mu.Unlock()
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
