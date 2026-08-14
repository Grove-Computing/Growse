// Package browser contains the state and navigation lifecycle of a Growse
// browser window.
package browser

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	animationmodel "github.com/Grove-Computing/Growse/internal/animation"
	"github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/events"
	"github.com/Grove-Computing/Growse/internal/forms"
	htmlparser "github.com/Grove-Computing/Growse/internal/html"
	"github.com/Grove-Computing/Growse/internal/network"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
	"github.com/Grove-Computing/Growse/internal/style"
)

// ResourceLoader retrieves a resource for navigation.
type ResourceLoader interface {
	Get(ctx context.Context, resourceURL *url.URL) (*network.Response, error)
}

type requestLoader interface {
	Do(ctx context.Context, request *network.Request) (*network.Response, error)
}

type animationFrameRuntime interface {
	RunAnimationFrame(time.Time) bool
	HasAnimationFrameCallbacks() bool
}

type pageEventRuntime interface {
	DispatchPageEvent(func() bool) bool
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
	nextPageID     uint64
	history        history
	clock          animationmodel.Clock
	reducedMotion  bool
}

var (
	ErrFormValidation      = errors.New("form validation failed")
	ErrSubmissionPrevented = errors.New("form submission was prevented")
)

// New creates a browser with no page loaded.
func New(client ResourceLoader) *Browser {
	return NewWithRuntimeFactory(client, nil)
}

// NewWithRuntimeFactory は信頼済みページのスクリプトを実行するBrowserを生成する。
func NewWithRuntimeFactory(client ResourceLoader, factory runtimemodel.Factory) *Browser {
	return &Browser{client: client, runtimeFactory: factory, history: newHistory(), clock: animationmodel.SystemClock{}}
}

// SetAnimationClock replaces the page animation clock. Tests can inject a
// controllable Clock before navigation; nil restores the production clock.
func (b *Browser) SetAnimationClock(clock animationmodel.Clock) {
	b.mu.Lock()
	if clock == nil {
		clock = animationmodel.SystemClock{}
	}
	b.clock = clock
	b.mu.Unlock()
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

// RunAnimationFrame delivers one Gio frame timestamp to the active WebGo runtime.
func (b *Browser) RunAnimationFrame(current time.Time) bool {
	b.mu.RLock()
	activeRuntime := b.activeRuntime
	b.mu.RUnlock()
	runtime, ok := activeRuntime.(animationFrameRuntime)
	return ok && runtime.RunAnimationFrame(current)
}

// HasAnimationFrameCallbacks reports whether WebGo requested another frame.
func (b *Browser) HasAnimationFrameCallbacks() bool {
	b.mu.RLock()
	activeRuntime := b.activeRuntime
	b.mu.RUnlock()
	runtime, ok := activeRuntime.(animationFrameRuntime)
	return ok && runtime.HasAnimationFrameCallbacks()
}

func (b *Browser) dispatchPageEvent(page *Page, event events.Event) bool {
	if page == nil || page.Events == nil {
		return false
	}
	b.mu.RLock()
	activeRuntime := b.activeRuntime
	active := b.page == page
	b.mu.RUnlock()
	if runtime, ok := activeRuntime.(pageEventRuntime); ok && active {
		return runtime.DispatchPageEvent(func() bool {
			return page.Events.Dispatch(event)
		})
	}
	return page.Events.Dispatch(event)
}

// DispatchClick はアクティブページの対象ノードへクリックを配信する。
func (b *Browser) DispatchClick(nodeID dom.NodeID, x, y float32) bool {
	b.mu.RLock()
	page := b.page
	b.mu.RUnlock()
	if page == nil || page.Events == nil {
		return false
	}
	if page.Document != nil {
		if clickedNode, exists := page.Document.NodeByID(nodeID); exists && forms.Disabled(clickedNode) {
			return false
		}
	}
	clickHandled := b.dispatchPageEvent(page, events.Event{Type: events.Click, Target: nodeID, X: x, Y: y})
	labelHandled := false
	if page.Document != nil {
		if node, ok := page.Document.NodeByID(nodeID); ok {
			if control := forms.LabeledControl(page.Document, node); control != nil && !forms.Disabled(control) {
				b.UpdateFocus(control.ID)
				if _, checkable := forms.CheckableState(control); checkable {
					labelHandled = b.ActivateCheckable(control.ID)
				}
			}
		}
	}
	submitHandled := false
	if page.Document != nil {
		if node, ok := page.Document.NodeByID(nodeID); ok && isSubmitButton(node) {
			if form := forms.FormOwner(page.Document, node); form != nil {
				b.mu.Lock()
				if b.page == page {
					page.Submitter = node.ID
				}
				b.mu.Unlock()
				submitHandled = b.dispatchPageEvent(page, events.Event{Type: events.Submit, Target: form.ID})
			}
		}
	}
	return clickHandled || submitHandled || labelHandled
}

// SetInputValue はユーザー入力をアクティブページの編集可能なText Controlへ反映する。
func (b *Browser) SetInputValue(nodeID dom.NodeID, value string) bool {
	b.mu.Lock()
	page := b.page
	onMutation := b.onMutation
	if page == nil || page.Document == nil {
		b.mu.Unlock()
		return false
	}
	node, ok := page.Document.NodeByID(nodeID)
	if !ok || !isEditableTextControl(node) || !page.Document.IsConnected(node) || forms.Disabled(node) || forms.ReadOnly(node) {
		b.mu.Unlock()
		return false
	}
	changed := forms.SetCurrentValue(node, value)
	if changed {
		recomputePageStyles(page, b.currentTime())
	}
	dispatcher := page.Events
	b.mu.Unlock()
	if changed && onMutation != nil {
		onMutation()
	}
	if changed && dispatcher != nil {
		b.dispatchPageEvent(page, events.Event{Type: events.Input, Target: nodeID, Value: value})
	}
	return changed
}

// SetSelectValue changes a select to an enabled option and dispatches the
// input/change pair produced by a committed user selection.
func (b *Browser) SetSelectValue(nodeID dom.NodeID, value string) bool {
	b.mu.Lock()
	page := b.page
	onMutation := b.onMutation
	if page == nil || page.Document == nil || !forms.SetSelectedValue(page.Document, nodeID, value) {
		b.mu.Unlock()
		return false
	}
	recomputePageStyles(page, b.currentTime())
	dispatcher := page.Events
	b.mu.Unlock()
	if onMutation != nil {
		onMutation()
	}
	if dispatcher != nil {
		b.dispatchPageEvent(page, events.Event{Type: events.Input, Target: nodeID, Value: value})
		b.dispatchPageEvent(page, events.Event{Type: events.Change, Target: nodeID, Value: value})
	}
	return true
}

// ActivateCheckable toggles a checkbox or selects one radio in its group.
func (b *Browser) ActivateCheckable(nodeID dom.NodeID) bool {
	b.mu.Lock()
	page := b.page
	onMutation := b.onMutation
	if page == nil || page.Document == nil {
		b.mu.Unlock()
		return false
	}
	checked, changed := forms.ActivateCheckable(page.Document, nodeID)
	if !changed {
		b.mu.Unlock()
		return false
	}
	recomputePageStyles(page, b.currentTime())
	dispatcher := page.Events
	b.mu.Unlock()
	if onMutation != nil {
		onMutation()
	}
	value := "false"
	if checked {
		value = "true"
	}
	if dispatcher != nil {
		b.dispatchPageEvent(page, events.Event{Type: events.Input, Target: nodeID, Value: value})
		b.dispatchPageEvent(page, events.Event{Type: events.Change, Target: nodeID, Value: value})
	}
	return true
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
	if !ok || !isEditableTextControl(node) || !page.Document.IsConnected(node) {
		b.mu.RUnlock()
		return false
	}
	b.mu.RUnlock()
	return b.dispatchPageEvent(page, events.Event{Type: events.Change, Target: nodeID, Value: value})
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
	form := forms.FormOwner(page.Document, node)
	b.mu.RUnlock()
	if form == nil {
		return false
	}
	b.mu.Lock()
	if b.page == page {
		page.Submitter = 0
	}
	b.mu.Unlock()
	return b.dispatchPageEvent(page, events.Event{Type: events.Submit, Target: form.ID})
}

// ResetForm restores a form's controls to their HTML attribute defaults.
func (b *Browser) ResetForm(nodeID dom.NodeID) bool {
	b.mu.Lock()
	page := b.page
	onMutation := b.onMutation
	if page == nil || page.Document == nil {
		b.mu.Unlock()
		return false
	}
	node, ok := page.Document.NodeByID(nodeID)
	if !ok || !page.Document.IsConnected(node) {
		b.mu.Unlock()
		return false
	}
	form := node
	if form.TagName != "form" {
		form = forms.FormOwner(page.Document, node)
	}
	if form == nil {
		b.mu.Unlock()
		return false
	}
	changed := forms.Reset(form)
	if changed {
		recomputePageStyles(page, b.currentTime())
	}
	b.mu.Unlock()
	if changed && onMutation != nil {
		onMutation()
	}
	handled := b.dispatchPageEvent(page, events.Event{Type: events.Reset, Target: form.ID})
	return changed || handled
}

// ValidateForm focuses the first invalid control and reports whether the form is valid.
func (b *Browser) ValidateForm(nodeID dom.NodeID) bool {
	b.mu.RLock()
	page := b.page
	if page == nil || page.Document == nil {
		b.mu.RUnlock()
		return false
	}
	node, ok := page.Document.NodeByID(nodeID)
	if !ok || !page.Document.IsConnected(node) {
		b.mu.RUnlock()
		return false
	}
	form := node
	if form.TagName != "form" {
		form = forms.FormOwner(page.Document, node)
	}
	first, invalid := forms.FirstInvalidControl(page.Document, form)
	b.mu.RUnlock()
	if !invalid {
		return form != nil
	}
	b.UpdateFocus(first.ID)
	return false
}

// UpdateHover updates the active page's hovered element path.
func (b *Browser) UpdateHover(nodeID dom.NodeID, x, y float32) bool {
	b.mu.Lock()
	page := b.page
	onMutation := b.onMutation
	if page == nil || page.Document == nil {
		b.mu.Unlock()
		return false
	}
	path := hoverPath(page.Document, nodeID)
	if equalNodeIDs(page.HoverPath, path) {
		b.mu.Unlock()
		return false
	}
	previousPath := append([]dom.NodeID(nil), page.HoverPath...)
	page.HoverTarget = nodeID
	if len(path) == 0 {
		page.HoverTarget = 0
	}
	page.HoverPath = path
	recomputePageStyles(page, b.currentTime())
	b.mu.Unlock()
	if onMutation != nil {
		onMutation()
	}
	b.dispatchHoverEvents(page, previousPath, path, x, y)
	return true
}

// ClearHover clears transient hover state from the active page.
func (b *Browser) ClearHover() bool {
	return b.UpdateHover(0, 0, 0)
}

// UpdateFocus updates the element which matches the :focus pseudo-class.
func (b *Browser) UpdateFocus(nodeID dom.NodeID) bool {
	b.mu.Lock()
	page := b.page
	onMutation := b.onMutation
	if page == nil || page.Document == nil {
		b.mu.Unlock()
		return false
	}
	target := validFocusTarget(page.Document, nodeID)
	if page.FocusTarget == target {
		b.mu.Unlock()
		return false
	}
	previous := page.FocusTarget
	page.FocusTarget = target
	recomputePageStyles(page, b.currentTime())
	dispatcher := page.Events
	b.mu.Unlock()
	if onMutation != nil {
		onMutation()
	}
	if dispatcher != nil {
		if previous != 0 {
			b.dispatchPageEvent(page, events.Event{Type: events.Blur, Target: previous})
		}
		if target != 0 {
			b.dispatchPageEvent(page, events.Event{Type: events.Focus, Target: target})
		}
	}
	return true
}

// MoveFormFocus advances focus through enabled controls in DOM order.
func (b *Browser) MoveFormFocus(reverse bool) bool {
	b.mu.RLock()
	page := b.page
	if page == nil || page.Document == nil {
		b.mu.RUnlock()
		return false
	}
	target := forms.NextFocusable(page.Document, page.FocusTarget, reverse)
	b.mu.RUnlock()
	return b.UpdateFocus(target)
}

// UpdateViewport recomputes viewport-relative values when the content area changes.
func (b *Browser) UpdateViewport(width, height float32) bool {
	b.mu.Lock()
	page := b.page
	if page == nil || page.Document == nil || width <= 0 || height <= 0 {
		b.mu.Unlock()
		return false
	}
	if page.ViewportWidth == width && page.ViewportHeight == height {
		b.mu.Unlock()
		return false
	}
	page.ViewportWidth, page.ViewportHeight = width, height
	recomputePageStyles(page, b.currentTime())
	b.mu.Unlock()
	return true
}

// SetReducedMotion updates the browser preference exposed through the
// prefers-reduced-motion media feature.
func (b *Browser) SetReducedMotion(reduce bool) bool {
	b.mu.Lock()
	if b.reducedMotion == reduce {
		b.mu.Unlock()
		return false
	}
	b.reducedMotion = reduce
	page := b.page
	onMutation := b.onMutation
	if page != nil {
		page.ReducedMotion = reduce
		recomputePageStyles(page, b.currentTime())
	}
	b.mu.Unlock()
	if page != nil && onMutation != nil {
		onMutation()
	}
	return true
}

// SetPage replaces the active page. Passing nil clears the active page.
func (b *Browser) SetPage(page *Page) {
	b.mu.Lock()
	b.navigationID++
	activeRuntime := b.activeRuntime
	previousPage := b.page
	b.activeRuntime = nil
	b.page = page
	if previousPage != nil && previousPage != page && previousPage.Animations != nil {
		previousPage.Animations.Clear()
	}
	if previousPage != nil && previousPage != page && previousPage.Transitions != nil {
		previousPage.Transitions.Clear()
	}
	if page != nil {
		if page.ComputedStyles == nil {
			page.ComputedStyles = computePageStyles(page)
		}
		if page.Animations == nil {
			page.Animations = style.NewAnimationRegistry()
		}
		if page.Transitions == nil {
			page.Transitions = style.NewTransitionRegistry()
		}
		if page.StyleRevision == 0 {
			page.StyleRevision = 1
		}
		page.Animations.Reconcile(page.ComputedStyles, b.currentTime())
	}
	if page == nil {
		b.history = newHistory()
	} else {
		b.nextPageID++
		page.HistoryID = b.nextPageID
		b.history = newHistory()
		b.history.pushEntry(&historyEntry{URL: page.URL, PageID: page.HistoryID})
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
	page := b.page
	b.activeRuntime = nil
	if page != nil && page.Animations != nil {
		page.Animations.Clear()
	}
	if page != nil && page.Transitions != nil {
		page.Transitions.Clear()
	}
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
	fragmentTarget := cloneURL(pageURL)
	if reference, parseErr := url.Parse(strings.TrimSpace(rawURL)); parseErr == nil {
		fragmentTarget.Fragment = reference.Fragment
		fragmentTarget.RawFragment = reference.RawFragment
	}
	if page, ok := b.commitFragmentNavigation(fragmentTarget); ok {
		return page, nil
	}
	return b.load(ctx, pageURL, historyPush, -1)
}

// SubmitGET serializes a form into the action query and navigates with history.
func (b *Browser) SubmitGET(ctx context.Context, formID, submitterID dom.NodeID) (*Page, error) {
	b.mu.RLock()
	page := b.page
	if page == nil || page.Document == nil || page.URL == nil {
		b.mu.RUnlock()
		return nil, errors.New("no active page for form submission")
	}
	form, ok := page.Document.NodeByID(formID)
	if !ok {
		b.mu.RUnlock()
		return nil, errors.New("form was not found")
	}
	var submitter *dom.Node
	if submitterID != 0 {
		submitter, ok = page.Document.NodeByID(submitterID)
		if !ok {
			b.mu.RUnlock()
			return nil, errors.New("submitter was not found")
		}
	}
	config, ok := forms.ResolveFormSubmission(page.Document, form, submitter)
	if !ok || config.Method != "get" {
		b.mu.RUnlock()
		return nil, errors.New("form is not a GET submission")
	}
	entries := forms.CollectEntries(page.Document, form, submitter)
	baseURL := cloneURL(page.URL)
	b.mu.RUnlock()

	target, err := resolveFormAction(baseURL, config.Action)
	if err != nil {
		return nil, err
	}
	encoded, err := forms.EncodeURLEncodedLimited(entries)
	if err != nil {
		return nil, err
	}
	target.RawQuery = encoded
	target.Fragment = ""
	return b.load(ctx, target, historyPush, -1)
}

// SubmitPOST sends URL-encoded entries and navigates to the response document.
func (b *Browser) SubmitPOST(ctx context.Context, formID, submitterID dom.NodeID) (*Page, error) {
	b.mu.Lock()
	page := b.page
	if page == nil || page.Document == nil || page.URL == nil {
		b.mu.Unlock()
		return nil, errors.New("no active page for form submission")
	}
	form, ok := page.Document.NodeByID(formID)
	if !ok {
		b.mu.Unlock()
		return nil, errors.New("form was not found")
	}
	var submitter *dom.Node
	if submitterID != 0 {
		submitter, ok = page.Document.NodeByID(submitterID)
		if !ok {
			b.mu.Unlock()
			return nil, errors.New("submitter was not found")
		}
	}
	config, ok := forms.ResolveFormSubmission(page.Document, form, submitter)
	if !ok || config.Method != "post" || config.Enctype != forms.URLEncoded || config.Target != "_self" {
		b.mu.Unlock()
		return nil, errors.New("unsupported POST form configuration")
	}
	entries := forms.CollectEntries(page.Document, form, submitter)
	target, err := resolveFormAction(page.URL, config.Action)
	if err != nil {
		b.mu.Unlock()
		return nil, err
	}
	target.Fragment = ""
	client := b.client
	loader, ok := client.(requestLoader)
	if !ok {
		b.mu.Unlock()
		return nil, errors.New("network client does not support POST")
	}
	b.navigationID++
	navigationID := b.navigationID
	runtimeFactory := b.runtimeFactory
	onMutation := b.onMutation
	reducedMotion := b.reducedMotion
	b.mu.Unlock()

	encoded, err := forms.EncodeURLEncodedLimited(entries)
	if err != nil {
		return nil, err
	}
	body := []byte(encoded)
	response, err := loader.Do(ctx, &network.Request{
		Method: http.MethodPost, URL: target, Body: body,
		Header:  http.Header{"Content-Type": []string{forms.URLEncoded}},
		SiteURL: cloneURL(page.URL), Kind: network.RequestForm,
	})
	if err != nil {
		return nil, fmt.Errorf("submit form to %s: %w", network.RedactedURL(target), err)
	}
	return b.finishLoad(ctx, target, response, historyPush, -1, navigationID, client, client, runtimeFactory, onMutation, reducedMotion)
}

// Submit validates and dispatches a cancelable submit event before navigation.
func (b *Browser) Submit(ctx context.Context, formID, submitterID dom.NodeID) (*Page, error) {
	b.mu.RLock()
	page := b.page
	if page == nil || page.Document == nil {
		b.mu.RUnlock()
		return nil, errors.New("no active page for form submission")
	}
	form, ok := page.Document.NodeByID(formID)
	if !ok || form.TagName != "form" {
		b.mu.RUnlock()
		return nil, errors.New("form was not found")
	}
	var submitter *dom.Node
	if submitterID != 0 {
		submitter, ok = page.Document.NodeByID(submitterID)
		if !ok {
			b.mu.RUnlock()
			return nil, errors.New("submitter was not found")
		}
	}
	config, ok := forms.ResolveFormSubmission(page.Document, form, submitter)
	if !ok {
		b.mu.RUnlock()
		return nil, errors.New("invalid form submission configuration")
	}
	firstInvalid, invalid := forms.FirstInvalidControl(page.Document, form)
	dispatcher := page.Events
	b.mu.RUnlock()
	b.mu.Lock()
	if b.page == page {
		page.Submitter = submitterID
	}
	b.mu.Unlock()

	if invalid && !config.NoValidate {
		b.UpdateFocus(firstInvalid.ID)
		return nil, ErrFormValidation
	}
	submitEvent := events.Cancelable(events.Submit, form.ID)
	if dispatcher != nil {
		b.dispatchPageEvent(page, submitEvent)
	}
	if submitEvent.DefaultPrevented() {
		return nil, ErrSubmissionPrevented
	}
	if config.Target != "_self" {
		return nil, fmt.Errorf("unsupported form target %q", config.Target)
	}
	switch config.Method {
	case "get":
		return b.SubmitGET(ctx, formID, submitterID)
	case "post":
		return b.SubmitPOST(ctx, formID, submitterID)
	default:
		return nil, fmt.Errorf("unsupported form method %q", config.Method)
	}
}

func resolveFormAction(baseURL *url.URL, action string) (*url.URL, error) {
	if baseURL == nil {
		return nil, errors.New("form action has no base URL")
	}
	reference, err := url.Parse(strings.TrimSpace(action))
	if err != nil {
		return nil, fmt.Errorf("parse form action: %w", err)
	}
	target := baseURL.ResolveReference(reference)
	if target.Scheme != "http" && target.Scheme != "https" {
		return nil, fmt.Errorf("unsupported form action scheme %q", target.Scheme)
	}
	return target, nil
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
	return b.reload(ctx, false)
}

// ReloadIgnoringCache refreshes the active page and its subresources while
// asking HTTP caches to revalidate them. It does not add a history entry.
func (b *Browser) ReloadIgnoringCache(ctx context.Context) (*Page, error) {
	return b.reload(ctx, true)
}

func (b *Browser) reload(ctx context.Context, ignoreCache bool) (*Page, error) {
	b.mu.RLock()
	if b.page == nil || b.page.URL == nil {
		b.mu.RUnlock()
		return nil, errors.New("no active page to reload")
	}
	pageURL := cloneURL(b.page.URL)
	index := b.history.index
	b.mu.RUnlock()
	if ignoreCache {
		return b.loadIgnoringCache(ctx, pageURL, historyReplace, index)
	}
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
	return b.loadWithClient(ctx, pageURL, commit, historyIndex, false)
}

func (b *Browser) loadIgnoringCache(ctx context.Context, pageURL *url.URL, commit historyCommit, historyIndex int) (*Page, error) {
	return b.loadWithClient(ctx, pageURL, commit, historyIndex, true)
}

func (b *Browser) loadWithClient(ctx context.Context, pageURL *url.URL, commit historyCommit, historyIndex int, ignoreCache bool) (*Page, error) {
	b.mu.Lock()
	b.navigationID++
	navigationID := b.navigationID
	client := b.client
	runtimeFactory := b.runtimeFactory
	onMutation := b.onMutation
	reducedMotion := b.reducedMotion
	b.mu.Unlock()

	if client == nil {
		return nil, errors.New("network client is not configured")
	}
	resourceClient := client
	if ignoreCache {
		resourceClient = cacheRevalidatingLoader{ResourceLoader: client}
	}

	response, err := resourceClient.Get(ctx, pageURL)
	if err != nil {
		return nil, fmt.Errorf("navigate to %s: %w", network.RedactedURL(pageURL), err)
	}
	return b.finishLoad(ctx, pageURL, response, commit, historyIndex, navigationID, resourceClient, client, runtimeFactory, onMutation, reducedMotion)
}

type cacheRevalidatingLoader struct {
	ResourceLoader
}

func (loader cacheRevalidatingLoader) Get(ctx context.Context, resourceURL *url.URL) (*network.Response, error) {
	if requestClient, ok := loader.ResourceLoader.(requestLoader); ok {
		return requestClient.Do(ctx, &network.Request{
			Method: http.MethodGet,
			URL:    resourceURL,
			Header: http.Header{
				"Cache-Control": []string{"no-cache"},
				"Pragma":        []string{"no-cache"},
			},
		})
	}
	return loader.ResourceLoader.Get(ctx, resourceURL)
}

func (b *Browser) finishLoad(ctx context.Context, pageURL *url.URL, response *network.Response, commit historyCommit, historyIndex int, navigationID uint64, resourceClient ResourceLoader, runtimeClient ResourceLoader, runtimeFactory runtimemodel.Factory, onMutation func(), reducedMotion bool) (*Page, error) {
	mediaType, _, err := mime.ParseMediaType(response.ContentType)
	if err != nil {
		return nil, fmt.Errorf("invalid Content-Type %q: %w", response.ContentType, err)
	}
	if mediaType != "text/html" && mediaType != "application/xhtml+xml" {
		return nil, fmt.Errorf("unsupported Content-Type %q", mediaType)
	}
	document, err := htmlparser.Parse(bytes.NewReader(response.Body))
	if err != nil {
		return nil, fmt.Errorf("build DOM for %s: %w", network.RedactedURL(pageURL), err)
	}
	stylesheet, err := b.loadStyles(ctx, resourceClient, response.URL, document)
	if err != nil {
		return nil, fmt.Errorf("load styles for %s: %w", network.RedactedURL(pageURL), err)
	}
	computedStyles := style.ComputeWithEnvironment(document, stylesheet, style.InteractionState{}, style.Environment{
		ViewportWidth: 1280, ViewportHeight: 720, RootFontSize: 16, ResolutionDPI: 96,
		ColorScheme: "light", Hover: true, Pointer: "fine", ReducedMotion: reducedMotion,
	})
	backgroundImages, backgroundErrors := loadBackgroundImages(ctx, resourceClient, computedStyles)
	scripts, scriptErrors := loadScripts(ctx, resourceClient, response.URL, document)

	page := &Page{
		URL:              cloneURL(response.URL),
		StatusCode:       response.StatusCode,
		ContentType:      response.ContentType,
		Source:           append([]byte(nil), response.Body...),
		Document:         document,
		Events:           events.NewDispatcher(),
		Stylesheet:       stylesheet,
		ComputedStyles:   computedStyles,
		Animations:       style.NewAnimationRegistry(),
		Transitions:      style.NewTransitionRegistry(),
		StyleRevision:    1,
		ReducedMotion:    reducedMotion,
		BackgroundImages: backgroundImages,
		BackgroundErrors: backgroundErrors,
		Scripts:          scripts,
		ScriptErrors:     scriptErrors,
	}
	page.Animations.Reconcile(computedStyles, b.currentTime())
	navigationReady := make(chan struct{})
	pageRuntime := startRuntime(ctx, runtimeFactory, page, runtimeClient, onMutation, b.currentTime, func(target *url.URL) error {
		resolved := cloneURL(target)
		go func() {
			<-navigationReady
			b.navigateFromRuntime(page, resolved)
		}()
		return nil
	}, func(state string, target *url.URL) error {
		return b.pushHistoryState(page, target, state)
	})
	if err := ctx.Err(); err != nil {
		close(navigationReady)
		page.Animations.Clear()
		page.Transitions.Clear()
		if pageRuntime != nil {
			_ = pageRuntime.Stop()
		}
		return nil, err
	}
	b.mu.Lock()
	if navigationID != b.navigationID {
		b.mu.Unlock()
		close(navigationReady)
		page.Animations.Clear()
		page.Transitions.Clear()
		if pageRuntime != nil {
			_ = pageRuntime.Stop()
		}
		return nil, context.Canceled
	}
	previousRuntime := b.activeRuntime
	previousPage := b.page
	b.nextPageID++
	page.HistoryID = b.nextPageID
	b.activeRuntime = pageRuntime
	b.page = page
	if previousPage != nil && previousPage != page && previousPage.Animations != nil {
		previousPage.Animations.Clear()
	}
	if previousPage != nil && previousPage != page && previousPage.Transitions != nil {
		previousPage.Transitions.Clear()
	}
	switch commit {
	case historyPush:
		b.history.pushEntry(&historyEntry{URL: page.URL, PageID: page.HistoryID})
	case historyTraverse:
		b.history.index = historyIndex
		b.history.replaceEntry(&historyEntry{URL: page.URL, PageID: page.HistoryID})
	case historyReplace:
		b.history.index = historyIndex
		current, _ := b.history.current()
		state := ""
		if current != nil {
			state = current.State
		}
		b.history.replaceEntry(&historyEntry{URL: page.URL, State: state, PageID: page.HistoryID})
	}
	b.mu.Unlock()
	close(navigationReady)
	if previousRuntime != nil {
		_ = previousRuntime.Stop()
	}
	return page, nil
}

func (b *Browser) navigateFromRuntime(source *Page, target *url.URL) {
	b.mu.RLock()
	active := b.page == source
	b.mu.RUnlock()
	if !active {
		return
	}
	if _, ok := b.commitFragmentNavigation(target); ok {
		return
	}
	_, err := b.load(context.Background(), target, historyPush, -1)
	if err != nil {
		b.mu.Lock()
		if b.page == source {
			source.RuntimeError = fmt.Sprintf("navigation failed: %v", err)
		}
		b.mu.Unlock()
	}
	b.mu.RLock()
	onMutation := b.onMutation
	b.mu.RUnlock()
	if onMutation != nil {
		onMutation()
	}
}

func (b *Browser) commitFragmentNavigation(target *url.URL) (*Page, bool) {
	b.mu.Lock()
	page := b.page
	if page == nil || page.URL == nil || !fragmentOnlyChange(page.URL, target) {
		b.mu.Unlock()
		return nil, false
	}
	page.URL = cloneURL(target)
	b.history.pushEntry(&historyEntry{URL: target, SameDocument: true, PageID: page.HistoryID})
	activeRuntime := b.activeRuntime
	onMutation := b.onMutation
	b.mu.Unlock()
	if updater, ok := activeRuntime.(runtimemodel.LocationUpdater); ok {
		updater.UpdateLocation(target)
	}
	if onMutation != nil {
		onMutation()
	}
	return page, true
}

func fragmentOnlyChange(current, target *url.URL) bool {
	if current == nil || target == nil || current.Fragment == target.Fragment {
		return false
	}
	left := cloneURL(current)
	right := cloneURL(target)
	left.Fragment, left.RawFragment = "", ""
	right.Fragment, right.RawFragment = "", ""
	return left.String() == right.String()
}

func (b *Browser) pushHistoryState(source *Page, target *url.URL, state string) error {
	b.mu.Lock()
	if source == nil || b.page != source || source.URL == nil || target == nil {
		b.mu.Unlock()
		return errors.New("history is unavailable")
	}
	if target.User != nil || !network.SameOrigin(source.URL, target) {
		b.mu.Unlock()
		return errors.New("history URL is not same-origin")
	}
	source.URL = cloneURL(target)
	b.history.pushEntry(&historyEntry{
		URL: target, State: state, SameDocument: true, PageID: source.HistoryID,
	})
	activeRuntime := b.activeRuntime
	onMutation := b.onMutation
	b.mu.Unlock()
	if updater, ok := activeRuntime.(runtimemodel.LocationUpdater); ok {
		updater.UpdateLocation(target)
	}
	if onMutation != nil {
		onMutation()
	}
	return nil
}

func startRuntime(ctx context.Context, factory runtimemodel.Factory, page *Page, client ResourceLoader, onMutation func(), now func() time.Time, navigate func(*url.URL) error, historyPush func(string, *url.URL) error) runtimemodel.Runtime {
	if factory == nil || page == nil || len(page.Scripts) == 0 {
		return nil
	}
	if !IsTrustedOrigin(page.URL) {
		page.RuntimeError = fmt.Sprintf("blocked Go script execution from untrusted origin: %s", network.RedactedURL(page.URL))
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
	var frameMu sync.RWMutex
	var frameTime time.Time
	frameActive := false
	runtimeNow := func() time.Time {
		frameMu.RLock()
		active, current := frameActive, frameTime
		frameMu.RUnlock()
		if active {
			return current
		}
		return now()
	}
	environment := runtimemodel.Environment{
		Document:    page.Document,
		Events:      page.Events,
		BaseURL:     cloneURL(page.URL),
		Navigate:    navigate,
		HistoryPush: historyPush,
		OnMutation: func() {
			page.HoverPath = hoverPath(page.Document, page.HoverTarget)
			if len(page.HoverPath) == 0 {
				page.HoverTarget = 0
			}
			page.FocusTarget = validFocusTarget(page.Document, page.FocusTarget)
			recomputePageStyles(page, runtimeNow())
			if onMutation != nil {
				onMutation()
			}
		},
		RequestFrame: onMutation,
		FrameScope: func(current time.Time, callback func()) {
			frameMu.Lock()
			frameTime = current
			frameActive = true
			frameMu.Unlock()
			defer func() {
				frameMu.Lock()
				frameActive = false
				frameTime = time.Time{}
				frameMu.Unlock()
			}()
			callback()
		},
	}
	if loader, ok := client.(requestLoader); ok {
		environment.Fetch = loader.Do
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

func hoverPath(document *dom.Document, nodeID dom.NodeID) []dom.NodeID {
	if document == nil || nodeID == 0 {
		return nil
	}
	node, ok := document.NodeByID(nodeID)
	if !ok || !document.IsConnected(node) {
		return nil
	}
	for node != nil && node.Type != dom.NodeElement {
		node = node.Parent
	}
	var reversed []dom.NodeID
	for current := node; current != nil; current = current.Parent {
		if current.Type == dom.NodeElement {
			reversed = append(reversed, current.ID)
		}
	}
	path := make([]dom.NodeID, len(reversed))
	for index := range reversed {
		path[len(reversed)-1-index] = reversed[index]
	}
	return path
}

func interactionState(page *Page) style.InteractionState {
	state := style.InteractionState{
		Hovered: make(map[dom.NodeID]bool, len(page.HoverPath)), Focused: page.FocusTarget,
	}
	for _, nodeID := range page.HoverPath {
		state.Hovered[nodeID] = true
	}
	return state
}

func computePageStyles(page *Page) style.Map {
	if page == nil {
		return nil
	}
	return style.ComputeWithEnvironment(page.Document, page.Stylesheet, interactionState(page), style.Environment{
		ViewportWidth: page.ViewportWidth, ViewportHeight: page.ViewportHeight, RootFontSize: 16,
		ResolutionDPI: 96, ColorScheme: "light", Hover: true, Pointer: "fine",
		ReducedMotion: page.ReducedMotion,
	})
}

func recomputePageStyles(page *Page, current time.Time) {
	if page == nil {
		return
	}
	previous := page.ComputedStyles
	page.ComputedStyles = computePageStyles(page)
	page.StyleRevision++
	if page.Transitions == nil {
		page.Transitions = style.NewTransitionRegistry()
	}
	page.Transitions.Reconcile(previous, page.ComputedStyles, current)
	if page.Animations == nil {
		page.Animations = style.NewAnimationRegistry()
	}
	page.Animations.Reconcile(page.ComputedStyles, current)
}

func (b *Browser) currentTime() time.Time {
	if b.clock == nil {
		return time.Now()
	}
	return b.clock.Now()
}

func validFocusTarget(document *dom.Document, nodeID dom.NodeID) dom.NodeID {
	if document == nil || nodeID == 0 {
		return 0
	}
	node, ok := document.NodeByID(nodeID)
	if !ok || node.Type != dom.NodeElement || !document.IsConnected(node) {
		return 0
	}
	return node.ID
}

func equalNodeIDs(left, right []dom.NodeID) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func (b *Browser) dispatchHoverEvents(page *Page, previous, current []dom.NodeID, x, y float32) {
	if page == nil || page.Events == nil {
		return
	}
	common := 0
	for common < len(previous) && common < len(current) && previous[common] == current[common] {
		common++
	}
	for index := len(previous) - 1; index >= common; index-- {
		b.dispatchPageEvent(page, events.Event{Type: events.MouseLeave, Target: previous[index], X: x, Y: y})
	}
	for index := common; index < len(current); index++ {
		b.dispatchPageEvent(page, events.Event{Type: events.MouseEnter, Target: current[index], X: x, Y: y})
	}
}

func runtimeOrigin(sourceURL *url.URL) string {
	if sourceURL == nil {
		return "unknown"
	}
	return network.RedactedURL(sourceURL)
}

func isEditableTextControl(node *dom.Node) bool {
	return forms.IsEditableTextControl(node)
}

func isTextInput(node *dom.Node) bool {
	return node != nil && node.TagName == "input" && isEditableTextControl(node)
}

func isSubmitButton(node *dom.Node) bool {
	return forms.IsSubmitButton(node)
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
