// Package browser contains the state and navigation lifecycle of a Growse
// browser window.
package browser

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"mime"
	"net/url"
	"strings"
	"sync"

	"github.com/saku0512/growse/internal/dom"
	"github.com/saku0512/growse/internal/events"
	htmlparser "github.com/saku0512/growse/internal/html"
	"github.com/saku0512/growse/internal/network"
	runtimemodel "github.com/saku0512/growse/internal/runtime"
	"github.com/saku0512/growse/internal/style"
)

// ResourceLoader retrieves a resource for navigation.
type ResourceLoader interface {
	Get(ctx context.Context, resourceURL *url.URL) (*network.Response, error)
}

// Browser owns the state for one browser window.
//
// MVPでは1つのアクティブページ、線形の閲覧履歴、信頼済みページごとに
// 独立した1つのGo Runtimeを保持する。
type Browser struct {
	mu             sync.RWMutex
	page           *Page
	client         ResourceLoader
	runtimeFactory runtimemodel.Factory
	activeRuntime  runtimemodel.Runtime
	onMutation     func()
	navigationID   uint64
	history        history
}

// New creates a browser with no page loaded.
func New(client ResourceLoader) *Browser {
	return NewWithRuntimeFactory(client, nil)
}

// NewWithRuntimeFactory は信頼済みページのスクリプトを実行するBrowserを生成する。
func NewWithRuntimeFactory(client ResourceLoader, factory runtimemodel.Factory) *Browser {
	return &Browser{client: client, runtimeFactory: factory, history: newHistory()}
}

// SetOnMutation はWebGoによるDOM変更後の通知先を設定する。
func (b *Browser) SetOnMutation(callback func()) {
	b.mu.Lock()
	b.onMutation = callback
	b.mu.Unlock()
}

// Page returns the currently active page, or nil before the first successful
// navigation.
func (b *Browser) Page() *Page {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.page
}

// DispatchClick はアクティブページの対象ノードへクリックを配信する。
func (b *Browser) DispatchClick(nodeID dom.NodeID, x, y float32) bool {
	b.mu.RLock()
	page := b.page
	b.mu.RUnlock()
	if page == nil || page.Events == nil {
		return false
	}
	clickHandled := page.Events.Dispatch(events.Event{Type: events.Click, Target: nodeID, X: x, Y: y})
	submitHandled := false
	if node, ok := page.Document.NodeByID(nodeID); ok && isSubmitButton(node) {
		if form := nearestForm(node); form != nil {
			submitHandled = page.Events.Dispatch(events.Event{Type: events.Submit, Target: form.ID})
		}
	}
	return clickHandled || submitHandled
}

// SetInputValue はユーザー入力をアクティブページのテキストinputへ反映する。
func (b *Browser) SetInputValue(nodeID dom.NodeID, value string) bool {
	b.mu.Lock()
	page := b.page
	onMutation := b.onMutation
	if page == nil || page.Document == nil {
		b.mu.Unlock()
		return false
	}
	node, ok := page.Document.NodeByID(nodeID)
	if !ok || !isTextInput(node) || !page.Document.IsConnected(node) {
		b.mu.Unlock()
		return false
	}
	changed := page.Document.SetAttribute(nodeID, "value", value)
	if changed {
		page.ComputedStyles = style.Compute(page.Document, page.Stylesheet)
	}
	dispatcher := page.Events
	b.mu.Unlock()
	if changed && onMutation != nil {
		onMutation()
	}
	if changed && dispatcher != nil {
		dispatcher.Dispatch(events.Event{Type: events.Input, Target: nodeID, Value: value})
	}
	return changed
}

// CommitInputValue はテキストinputの編集確定をchangeイベントとして配信する。
func (b *Browser) CommitInputValue(nodeID dom.NodeID, value string) bool {
	b.mu.RLock()
	page := b.page
	if page == nil || page.Document == nil || page.Events == nil {
		b.mu.RUnlock()
		return false
	}
	node, ok := page.Document.NodeByID(nodeID)
	if !ok || !isTextInput(node) || !page.Document.IsConnected(node) {
		b.mu.RUnlock()
		return false
	}
	dispatcher := page.Events
	b.mu.RUnlock()
	return dispatcher.Dispatch(events.Event{Type: events.Change, Target: nodeID, Value: value})
}

// SubmitForm はinputを含む最も近いformへsubmitイベントを配信する。
func (b *Browser) SubmitForm(nodeID dom.NodeID) bool {
	b.mu.RLock()
	page := b.page
	if page == nil || page.Document == nil || page.Events == nil {
		b.mu.RUnlock()
		return false
	}
	node, ok := page.Document.NodeByID(nodeID)
	if !ok || !isTextInput(node) || !page.Document.IsConnected(node) {
		b.mu.RUnlock()
		return false
	}
	form := nearestForm(node)
	dispatcher := page.Events
	b.mu.RUnlock()
	if form == nil {
		return false
	}
	return dispatcher.Dispatch(events.Event{Type: events.Submit, Target: form.ID})
}

// SetPage replaces the active page. Passing nil clears the active page.
func (b *Browser) SetPage(page *Page) {
	b.mu.Lock()
	b.navigationID++
	activeRuntime := b.activeRuntime
	b.activeRuntime = nil
	b.page = page
	if page == nil {
		b.history = newHistory()
	} else {
		b.history.reset(page.URL)
	}
	b.mu.Unlock()
	if activeRuntime != nil {
		_ = activeRuntime.Stop()
	}
}

// Close はアクティブページに属するRuntimeを停止する。
func (b *Browser) Close() error {
	b.mu.Lock()
	b.navigationID++
	activeRuntime := b.activeRuntime
	b.activeRuntime = nil
	b.mu.Unlock()
	if activeRuntime != nil {
		return activeRuntime.Stop()
	}
	return nil
}

// Navigate retrieves an HTML document and makes it the active page. The
// current page is preserved if validation or loading fails.
func (b *Browser) Navigate(ctx context.Context, rawURL string) (*Page, error) {
	pageURL, err := normalizeURL(rawURL)
	if err != nil {
		return nil, err
	}
	return b.load(ctx, pageURL, historyPush, -1)
}

// Back loads the previous successful navigation entry.
func (b *Browser) Back(ctx context.Context) (*Page, error) {
	return b.traverse(ctx, -1)
}

// Forward loads the next navigation entry after a successful Back.
func (b *Browser) Forward(ctx context.Context) (*Page, error) {
	return b.traverse(ctx, 1)
}

// Reload refreshes the active page without adding a history entry.
func (b *Browser) Reload(ctx context.Context) (*Page, error) {
	b.mu.RLock()
	if b.page == nil || b.page.URL == nil {
		b.mu.RUnlock()
		return nil, errors.New("no active page to reload")
	}
	pageURL := cloneURL(b.page.URL)
	index := b.history.index
	b.mu.RUnlock()
	return b.load(ctx, pageURL, historyReplace, index)
}

// CanBack reports whether Back has a target entry.
func (b *Browser) CanBack() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.history.canBack()
}

// CanForward reports whether Forward has a target entry.
func (b *Browser) CanForward() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.history.canForward()
}

func (b *Browser) traverse(ctx context.Context, delta int) (*Page, error) {
	b.mu.RLock()
	target, index, ok := b.history.target(delta)
	b.mu.RUnlock()
	if !ok {
		return nil, errors.New("no history entry in requested direction")
	}
	return b.load(ctx, target, historyTraverse, index)
}

type historyCommit uint8

const (
	historyPush historyCommit = iota
	historyTraverse
	historyReplace
)

func (b *Browser) load(ctx context.Context, pageURL *url.URL, commit historyCommit, historyIndex int) (*Page, error) {
	b.mu.Lock()
	b.navigationID++
	navigationID := b.navigationID
	client := b.client
	runtimeFactory := b.runtimeFactory
	onMutation := b.onMutation
	b.mu.Unlock()

	if client == nil {
		return nil, errors.New("network client is not configured")
	}

	response, err := client.Get(ctx, pageURL)
	if err != nil {
		return nil, fmt.Errorf("navigate to %s: %w", pageURL.Redacted(), err)
	}

	mediaType, _, err := mime.ParseMediaType(response.ContentType)
	if err != nil {
		return nil, fmt.Errorf("invalid Content-Type %q: %w", response.ContentType, err)
	}
	if mediaType != "text/html" && mediaType != "application/xhtml+xml" {
		return nil, fmt.Errorf("unsupported Content-Type %q", mediaType)
	}
	document, err := htmlparser.Parse(bytes.NewReader(response.Body))
	if err != nil {
		return nil, fmt.Errorf("build DOM for %s: %w", pageURL.Redacted(), err)
	}
	stylesheet, err := b.loadStyles(ctx, client, response.URL, document)
	if err != nil {
		return nil, fmt.Errorf("load styles for %s: %w", pageURL.Redacted(), err)
	}
	computedStyles := style.Compute(document, stylesheet)
	scripts, scriptErrors := loadScripts(ctx, client, response.URL, document)

	page := &Page{
		URL:            cloneURL(response.URL),
		StatusCode:     response.StatusCode,
		ContentType:    response.ContentType,
		Source:         append([]byte(nil), response.Body...),
		Document:       document,
		Events:         events.NewDispatcher(),
		Stylesheet:     stylesheet,
		ComputedStyles: computedStyles,
		Scripts:        scripts,
		ScriptErrors:   scriptErrors,
	}
	pageRuntime := startRuntime(ctx, runtimeFactory, page, onMutation)
	if err := ctx.Err(); err != nil {
		if pageRuntime != nil {
			_ = pageRuntime.Stop()
		}
		return nil, err
	}
	b.mu.Lock()
	if navigationID != b.navigationID {
		b.mu.Unlock()
		if pageRuntime != nil {
			_ = pageRuntime.Stop()
		}
		return nil, context.Canceled
	}
	previousRuntime := b.activeRuntime
	b.activeRuntime = pageRuntime
	b.page = page
	switch commit {
	case historyPush:
		b.history.push(page.URL)
	case historyTraverse:
		b.history.index = historyIndex
		b.history.replace(page.URL)
	case historyReplace:
		b.history.index = historyIndex
		b.history.replace(page.URL)
	}
	b.mu.Unlock()
	if previousRuntime != nil {
		_ = previousRuntime.Stop()
	}
	return page, nil
}

func startRuntime(ctx context.Context, factory runtimemodel.Factory, page *Page, onMutation func()) runtimemodel.Runtime {
	if factory == nil || page == nil || len(page.Scripts) == 0 {
		return nil
	}
	if !IsTrustedOrigin(page.URL) {
		page.RuntimeError = fmt.Sprintf("blocked Go script execution from untrusted origin: %s", page.URL.Redacted())
		return nil
	}
	for _, script := range page.Scripts {
		if !IsTrustedOrigin(script.SourceURL) {
			page.RuntimeError = fmt.Sprintf("blocked Go script execution from untrusted origin: %s", runtimeOrigin(script.SourceURL))
			return nil
		}
	}
	if len(page.ScriptErrors) != 0 {
		page.RuntimeError = "Go script loading failed; runtime was not started"
		return nil
	}

	pageRuntime := factory()
	if pageRuntime == nil {
		page.RuntimeError = "Go runtime factory returned nil"
		return nil
	}
	environment := runtimemodel.Environment{
		Document: page.Document,
		Events:   page.Events,
		BaseURL:  cloneURL(page.URL),
		OnMutation: func() {
			page.ComputedStyles = style.Compute(page.Document, page.Stylesheet)
			if onMutation != nil {
				onMutation()
			}
		},
	}
	if err := pageRuntime.Load(ctx, page.Scripts, environment); err != nil {
		page.RuntimeError = fmt.Sprintf("load Go runtime: %v", err)
		_ = pageRuntime.Stop()
		return nil
	}
	if err := pageRuntime.Start(ctx); err != nil {
		page.RuntimeError = fmt.Sprintf("start Go runtime: %v", err)
		_ = pageRuntime.Stop()
		return nil
	}
	page.RuntimeStarted = true
	return pageRuntime
}

func runtimeOrigin(sourceURL *url.URL) string {
	if sourceURL == nil {
		return "unknown"
	}
	return sourceURL.Redacted()
}

func isTextInput(node *dom.Node) bool {
	if node == nil || node.Type != dom.NodeElement || node.TagName != "input" {
		return false
	}
	typeValue, ok := node.Attribute("type")
	return !ok || strings.EqualFold(strings.TrimSpace(typeValue), "text")
}

func isSubmitButton(node *dom.Node) bool {
	if node == nil || node.Type != dom.NodeElement || node.TagName != "button" {
		return false
	}
	typeValue, ok := node.Attribute("type")
	return !ok || strings.EqualFold(strings.TrimSpace(typeValue), "submit")
}

func nearestForm(node *dom.Node) *dom.Node {
	for current := node; current != nil; current = current.Parent {
		if current.Type == dom.NodeElement && current.TagName == "form" {
			return current
		}
	}
	return nil
}

func normalizeURL(rawURL string) (*url.URL, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil, errors.New("URL is empty")
	}
	if !strings.Contains(rawURL, "://") {
		if strings.HasPrefix(rawURL, "localhost") || strings.HasPrefix(rawURL, "127.0.0.1") || strings.HasPrefix(rawURL, "[::1]") {
			rawURL = "http://" + rawURL
		} else {
			rawURL = "https://" + rawURL
		}
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parse URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("unsupported URL scheme %q", parsed.Scheme)
	}
	if parsed.Host == "" {
		return nil, errors.New("URL host is empty")
	}
	parsed.Fragment = ""
	return parsed, nil
}
