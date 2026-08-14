// Package navigation はWebGoへ現在のDocument URLを安全な値として公開する。
package navigation

import (
	"encoding/json"
	"errors"
	"log/slog"
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

// PopStateEvent はHistory traversal後のWebGo Eventである。
type PopStateEvent struct {
	State string
}

// HashChangeEvent はfragment変更前後のcredentialを含まないURLを保持する。
type HashChangeEvent struct {
	OldURL string
	NewURL string
}

// API は1つのPageに属するNavigation APIである。
type API struct {
	mu                  sync.RWMutex
	base                *url.URL
	current             Location
	navigate            func(*url.URL) error
	pushState           func(string, *url.URL) error
	replaceState        func(string, *url.URL) error
	traverse            func(int) error
	historyInfo         func() (int, string)
	popStateListeners   []func(PopStateEvent)
	hashChangeListeners []func(HashChangeEvent)
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

// SetTraversalHandler はBrowser History traversalと現在情報の取得先を設定する。
func (api *API) SetTraversalHandler(traverse func(int) error, info func() (int, string)) {
	if api == nil {
		return
	}
	api.mu.Lock()
	api.traverse = traverse
	api.historyInfo = info
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

// Back は1つ前のHistory entryへの移動を要求する。
func (api *API) Back() error {
	return api.Go(-1)
}

// Forward は1つ後のHistory entryへの移動を要求する。
func (api *API) Forward() error {
	return api.Go(1)
}

// Go はdeltaで指定したHistory entryへの移動を要求する。
func (api *API) Go(delta int) error {
	if api == nil {
		return ErrUnavailable
	}
	if delta == 0 {
		return nil
	}
	api.mu.RLock()
	handler := api.traverse
	api.mu.RUnlock()
	if handler == nil {
		return ErrUnavailable
	}
	return handler(delta)
}

// HistoryLength は現在Sessionのentry数を返す。
func (api *API) HistoryLength() int {
	length, _ := api.historySnapshot()
	return length
}

// HistoryState は現在entryのJSON stateを返す。未設定の場合は空文字列である。
func (api *API) HistoryState() string {
	_, state := api.historySnapshot()
	return state
}

func (api *API) historySnapshot() (int, string) {
	if api == nil {
		return 0, ""
	}
	api.mu.RLock()
	info := api.historyInfo
	api.mu.RUnlock()
	if info == nil {
		return 0, ""
	}
	return info()
}

// OnPopState はHistory traversal後に呼ばれるlistenerを登録する。
func (api *API) OnPopState(listener func(PopStateEvent)) {
	if api == nil || listener == nil {
		return
	}
	api.mu.Lock()
	api.popStateListeners = append(api.popStateListeners, listener)
	api.mu.Unlock()
}

// OnHashChange はfragment Navigation後に呼ばれるlistenerを登録する。
func (api *API) OnHashChange(listener func(HashChangeEvent)) {
	if api == nil || listener == nil {
		return
	}
	api.mu.Lock()
	api.hashChangeListeners = append(api.hashChangeListeners, listener)
	api.mu.Unlock()
}

// DispatchPopState はBrowserからpopstate相当Eventを配送する。
func (api *API) DispatchPopState(state string) {
	if api == nil {
		return
	}
	api.mu.RLock()
	listeners := append([]func(PopStateEvent){}, api.popStateListeners...)
	api.mu.RUnlock()
	event := PopStateEvent{State: state}
	for _, listener := range listeners {
		invokeNavigationListener("popstate", func() { listener(event) })
	}
}

// DispatchHashChange はBrowserからhashchange相当Eventを配送する。
func (api *API) DispatchHashChange(oldURL, newURL string) {
	if api == nil {
		return
	}
	api.mu.RLock()
	listeners := append([]func(HashChangeEvent){}, api.hashChangeListeners...)
	api.mu.RUnlock()
	event := HashChangeEvent{OldURL: oldURL, NewURL: newURL}
	for _, listener := range listeners {
		invokeNavigationListener("hashchange", func() { listener(event) })
	}
}

func invokeNavigationListener(eventType string, listener func()) {
	defer func() {
		if recover() != nil {
			slog.Error("WebGo Navigation Event handlerでpanicが発生しました", "component", "navigation", "type", eventType)
		}
	}()
	listener()
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
