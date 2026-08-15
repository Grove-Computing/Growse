package browser

import (
	"errors"
	"net/url"
	"sync"
	"unicode/utf8"
)

var (
	ErrTabIDExhausted  = errors.New("tab id space is exhausted")
	ErrTabBrowser      = errors.New("tab browser factory returned nil")
	ErrTabBrowserReuse = errors.New("tab browser factory reused an existing browser")
	ErrTabNotFound     = errors.New("tab was not found")
	ErrTabLimit        = errors.New("tab limit was reached")
	ErrTabURLTooLong   = errors.New("tab URL is too long")
	ErrTabTitle        = errors.New("tab title is invalid")
)

const (
	defaultMaxTabs       = 64
	defaultMaxTitleBytes = 4 * 1024
	defaultMaxURLBytes   = 8 * 1024
)

// TabID identifies one tab for the lifetime of a browser session.
type TabID uint64

// TabState describes whether a tab can still receive page work.
type TabState uint8

const (
	TabActive TabState = iota
	TabBackground
	TabClosing
	TabClosed
)

// Tab is one top-level page owned by a browser session.
type Tab struct {
	id         TabID
	state      TabState
	browser    *Browser
	initialURL *url.URL
	title      string
	loading    bool
	failed     bool
	pending    bool
	status     string
}

// TabSnapshot is an immutable view of a tab suitable for browser chrome.
type TabSnapshot struct {
	ID            TabID
	Position      int
	State         TabState
	Active        bool
	URL           string
	Title         string
	Loading       bool
	Error         bool
	PendingUpdate bool
	Status        string
}

// TabCloseResult reports the active tab selected after a close operation.
type TabCloseResult struct {
	Closed    TabID
	Active    TabSnapshot
	HasActive bool
}

// BrowserFactory creates the isolated Browser owned by a new tab.
type BrowserFactory func() *Browser

// SessionPolicy bounds tab-owned state before memory or identifiers are consumed.
type SessionPolicy struct {
	MaxTabs       int
	MaxTabID      uint64
	MaxTitleBytes int
	MaxURLBytes   int
}

// DefaultSessionPolicy returns the production safety limits.
func DefaultSessionPolicy() SessionPolicy {
	return SessionPolicy{
		MaxTabs:       defaultMaxTabs,
		MaxTabID:      ^uint64(0),
		MaxTitleBytes: defaultMaxTitleBytes,
		MaxURLBytes:   defaultMaxURLBytes,
	}
}

// Session owns an ordered collection of tabs and at most one active tab.
type Session struct {
	mu               sync.RWMutex
	tabs             []*Tab
	activeID         TabID
	nextID           uint64
	factory          BrowserFactory
	policy           SessionPolicy
	onActiveMutation func()
}

// NewSession creates an empty browser session.
func NewSession(factory ...BrowserFactory) *Session {
	createBrowser := BrowserFactory(func() *Browser { return New(nil) })
	if len(factory) != 0 && factory[0] != nil {
		createBrowser = factory[0]
	}
	return NewSessionWithPolicy(createBrowser, DefaultSessionPolicy())
}

// NewSessionWithPolicy creates an empty browser session with explicit limits.
func NewSessionWithPolicy(factory BrowserFactory, policy SessionPolicy) *Session {
	if factory == nil {
		factory = func() *Browser { return New(nil) }
	}
	defaults := DefaultSessionPolicy()
	if policy.MaxTabs <= 0 {
		policy.MaxTabs = defaults.MaxTabs
	}
	if policy.MaxTabID == 0 {
		policy.MaxTabID = defaults.MaxTabID
	}
	if policy.MaxTitleBytes <= 0 {
		policy.MaxTitleBytes = defaults.MaxTitleBytes
	}
	if policy.MaxURLBytes <= 0 {
		policy.MaxURLBytes = defaults.MaxURLBytes
	}
	return &Session{factory: factory, policy: policy}
}

// NewTab adds an empty tab or a tab with a requested initial URL. Navigation
// is performed separately so callers can control its context and lifecycle.
func (s *Session) NewTab(initialURL *url.URL) (TabSnapshot, error) {
	if s == nil {
		return TabSnapshot{}, ErrTabBrowser
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.tabs) >= s.policy.MaxTabs {
		return TabSnapshot{}, ErrTabLimit
	}
	if initialURL != nil && len(initialURL.String()) > s.policy.MaxURLBytes {
		return TabSnapshot{}, ErrTabURLTooLong
	}

	state := TabBackground
	if len(s.tabs) == 0 {
		state = TabActive
	}
	tab, err := s.newTabLocked(initialURL, state)
	if err != nil {
		return TabSnapshot{}, err
	}
	if state == TabActive {
		s.activeID = tab.id
	}
	tab.browser.SetTabActive(state == TabActive)
	s.tabs = append(s.tabs, tab)
	return snapshotTab(tab, len(s.tabs)-1, state == TabActive), nil
}

// SetTabTitle stores a validated document title for browser chrome.
func (s *Session) SetTabTitle(id TabID, title string) (TabSnapshot, error) {
	if s == nil {
		return TabSnapshot{}, ErrTabNotFound
	}
	if !utf8.ValidString(title) || len(title) > s.policy.MaxTitleBytes {
		return TabSnapshot{}, ErrTabTitle
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for index, tab := range s.tabs {
		if tab != nil && tab.id == id && tab.state != TabClosing && tab.state != TabClosed {
			tab.title = title
			return snapshotTab(tab, index, tab.id == s.activeID), nil
		}
	}
	return TabSnapshot{}, ErrTabNotFound
}

// BeginTabNavigation marks one tab as loading without changing selection.
func (s *Session) BeginTabNavigation(id TabID) (TabSnapshot, error) {
	if s == nil {
		return TabSnapshot{}, ErrTabNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for index, tab := range s.tabs {
		if tab != nil && tab.id == id && tab.state != TabClosing && tab.state != TabClosed {
			tab.loading = true
			tab.failed = false
			tab.pending = false
			tab.status = "読み込み中"
			return snapshotTab(tab, index, tab.id == s.activeID), nil
		}
	}
	return TabSnapshot{}, ErrTabNotFound
}

// FinishTabNavigation publishes a result to its target tab without selecting it.
func (s *Session) FinishTabNavigation(id TabID, failed bool) (TabSnapshot, error) {
	if s == nil {
		return TabSnapshot{}, ErrTabNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for index, tab := range s.tabs {
		if tab == nil || tab.id != id || tab.state == TabClosing || tab.state == TabClosed {
			continue
		}
		tab.loading = false
		runtimeFailed := false
		if tab.browser != nil {
			if page := tab.browser.Page(); page != nil {
				runtimeFailed = page.RuntimeError != ""
			}
		}
		tab.failed = failed || runtimeFailed
		tab.pending = tab.id != s.activeID
		if runtimeFailed {
			tab.status = "Runtimeエラー"
		} else if failed {
			tab.status = "読み込みエラー"
		} else {
			tab.status = "取得完了"
		}
		if tab.browser != nil {
			if page := tab.browser.Page(); page != nil && page.Document != nil {
				if title := page.Document.Title(); utf8.ValidString(title) && len(title) <= s.policy.MaxTitleBytes {
					tab.title = title
				}
			}
		}
		return snapshotTab(tab, index, tab.id == s.activeID), nil
	}
	return TabSnapshot{}, ErrTabNotFound
}

func (s *Session) newTabLocked(initialURL *url.URL, state TabState) (*Tab, error) {
	if s.factory == nil {
		return nil, ErrTabBrowser
	}
	browser := s.factory()
	if browser == nil {
		return nil, ErrTabBrowser
	}
	for _, existing := range s.tabs {
		if existing != nil && existing.browser == browser && existing.state != TabClosed {
			return nil, ErrTabBrowserReuse
		}
	}
	id, err := s.allocateTabIDLocked()
	if err != nil {
		_ = browser.Close()
		return nil, err
	}
	browser.SetOnMutation(func() { s.handleTabMutation(id) })
	return &Tab{id: id, state: state, browser: browser, initialURL: cloneURL(initialURL)}, nil
}

// SetOnActiveMutation registers the Browser Window invalidation callback.
func (s *Session) SetOnActiveMutation(callback func()) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.onActiveMutation = callback
	s.mu.Unlock()
}

func (s *Session) handleTabMutation(id TabID) {
	s.mu.Lock()
	tab, ok := s.tabByIDLocked(id)
	if !ok || tab.state == TabClosing || tab.state == TabClosed {
		s.mu.Unlock()
		return
	}
	if id != s.activeID {
		tab.pending = true
		s.mu.Unlock()
		return
	}
	callback := s.onActiveMutation
	s.mu.Unlock()
	if callback != nil {
		callback()
	}
}

// SelectTab makes the identified live tab active.
func (s *Session) SelectTab(id TabID) (TabSnapshot, error) {
	if s == nil {
		return TabSnapshot{}, ErrTabNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for index, tab := range s.tabs {
		if tab != nil && tab.id == id && tab.state != TabClosing && tab.state != TabClosed {
			return s.selectTabLocked(index), nil
		}
	}
	return TabSnapshot{}, ErrTabNotFound
}

// SelectTabAt makes the live tab at position active.
func (s *Session) SelectTabAt(position int) (TabSnapshot, error) {
	if s == nil {
		return TabSnapshot{}, ErrTabNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if position < 0 || position >= len(s.tabs) || s.tabs[position] == nil || s.tabs[position].state == TabClosing || s.tabs[position].state == TabClosed {
		return TabSnapshot{}, ErrTabNotFound
	}
	return s.selectTabLocked(position), nil
}

// SelectNext selects the next live tab, wrapping at the end.
func (s *Session) SelectNext() (TabSnapshot, error) {
	return s.selectRelative(1)
}

// SelectPrevious selects the previous live tab, wrapping at the beginning.
func (s *Session) SelectPrevious() (TabSnapshot, error) {
	return s.selectRelative(-1)
}

func (s *Session) selectRelative(step int) (TabSnapshot, error) {
	if s == nil {
		return TabSnapshot{}, ErrTabNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.tabs) == 0 {
		return TabSnapshot{}, ErrTabNotFound
	}
	activeIndex := -1
	for index, tab := range s.tabs {
		if tab != nil && tab.id == s.activeID && tab.state == TabActive {
			activeIndex = index
			break
		}
	}
	if activeIndex < 0 {
		return TabSnapshot{}, ErrTabNotFound
	}
	for offset := 1; offset <= len(s.tabs); offset++ {
		position := (activeIndex + step*offset) % len(s.tabs)
		if position < 0 {
			position += len(s.tabs)
		}
		tab := s.tabs[position]
		if tab != nil && tab.state != TabClosing && tab.state != TabClosed {
			return s.selectTabLocked(position), nil
		}
	}
	return TabSnapshot{}, ErrTabNotFound
}

func (s *Session) selectTabLocked(position int) TabSnapshot {
	target := s.tabs[position]
	for _, tab := range s.tabs {
		if tab != nil && tab.state == TabActive {
			tab.state = TabBackground
			if tab.browser != nil {
				tab.browser.SetTabActive(false)
			}
		}
	}
	target.state = TabActive
	if target.browser != nil {
		target.browser.SetTabActive(true)
	}
	target.pending = false
	s.activeID = target.id
	return snapshotTab(target, position, true)
}

// CloseTab removes a tab and deterministically selects its right neighbor or,
// when there is no right neighbor, its left neighbor.
func (s *Session) CloseTab(id TabID) (TabCloseResult, error) {
	if s == nil {
		return TabCloseResult{}, ErrTabNotFound
	}
	s.mu.Lock()
	position := -1
	var closing *Tab
	for index, tab := range s.tabs {
		if tab != nil && tab.id == id && tab.state != TabClosing && tab.state != TabClosed {
			position = index
			closing = tab
			break
		}
	}
	if closing == nil {
		s.mu.Unlock()
		return TabCloseResult{}, ErrTabNotFound
	}
	wasActive := closing.id == s.activeID && closing.state == TabActive
	var replacement *Tab
	if wasActive && len(s.tabs) == 1 {
		var err error
		replacement, err = s.newTabLocked(nil, TabActive)
		if err != nil {
			s.mu.Unlock()
			return TabCloseResult{}, err
		}
	}
	closing.state = TabClosing
	s.tabs = append(s.tabs[:position], s.tabs[position+1:]...)
	closing.state = TabClosed
	result := TabCloseResult{Closed: id}
	if wasActive {
		s.activeID = 0
		if replacement != nil {
			s.tabs = append(s.tabs, replacement)
			s.activeID = replacement.id
			result.Active = snapshotTab(replacement, 0, true)
			result.HasActive = true
		} else if len(s.tabs) != 0 {
			selection := position
			if selection >= len(s.tabs) {
				selection = len(s.tabs) - 1
			}
			result.Active = s.selectTabLocked(selection)
			result.HasActive = true
		}
	} else if active, activePosition, ok := s.activeTabLocked(); ok {
		result.Active = snapshotTab(active, activePosition, true)
		result.HasActive = true
	}
	s.mu.Unlock()

	if closing.browser != nil {
		return result, closing.browser.Close()
	}
	return result, nil
}

func (s *Session) activeTabLocked() (*Tab, int, bool) {
	for index, tab := range s.tabs {
		if tab != nil && tab.id == s.activeID && tab.state == TabActive {
			return tab, index, true
		}
	}
	return nil, -1, false
}

// Tabs returns the tabs in their display order.
func (s *Session) Tabs() []TabSnapshot {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]TabSnapshot, len(s.tabs))
	for index, tab := range s.tabs {
		result[index] = snapshotTab(tab, index, tab.id == s.activeID)
	}
	return result
}

// ActiveTab returns the active tab without exposing mutable session state.
func (s *Session) ActiveTab() (TabSnapshot, bool) {
	if s == nil {
		return TabSnapshot{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for index, tab := range s.tabs {
		if tab.id == s.activeID && tab.state == TabActive {
			return snapshotTab(tab, index, true), true
		}
	}
	return TabSnapshot{}, false
}

// ActiveBrowser returns the Browser currently selected by browser chrome.
// Callers keep the returned pointer to pin an operation to this tab even if
// another tab becomes active before the operation completes.
func (s *Session) ActiveBrowser() (*Browser, bool) {
	_, active, ok := s.ActiveBrowserTarget()
	return active, ok
}

// ActiveBrowserTarget atomically returns the ID and Browser selected at the
// start of a chrome operation.
func (s *Session) ActiveBrowserTarget() (TabID, *Browser, bool) {
	if s == nil {
		return 0, nil, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	tab, _, ok := s.activeTabLocked()
	if !ok || tab.browser == nil {
		return 0, nil, false
	}
	return tab.id, tab.browser, true
}

// Close transitions every tab to closed and releases each owned Browser.
func (s *Session) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	tabs := s.tabs
	s.tabs = nil
	s.activeID = 0
	for _, tab := range tabs {
		if tab != nil {
			tab.state = TabClosing
		}
	}
	s.mu.Unlock()
	var closeErr error
	for _, tab := range tabs {
		if tab == nil {
			continue
		}
		if tab.browser != nil {
			closeErr = errors.Join(closeErr, tab.browser.Close())
		}
		tab.state = TabClosed
	}
	return closeErr
}

func snapshotTab(tab *Tab, position int, active bool) TabSnapshot {
	if tab == nil {
		return TabSnapshot{Position: position}
	}
	snapshot := TabSnapshot{
		ID: tab.id, Position: position, State: tab.state, Active: active, Title: tab.title,
		Loading: tab.loading, Error: tab.failed, PendingUpdate: tab.pending, Status: tab.status,
	}
	if tab.initialURL != nil {
		snapshot.URL = displayTabURL(tab.initialURL)
	} else if tab.browser != nil {
		page := tab.browser.Page()
		if page != nil && page.URL != nil {
			snapshot.URL = displayTabURL(page.URL)
		}
	}
	return snapshot
}

func (s *Session) allocateTabIDLocked() (TabID, error) {
	if s.nextID >= s.policy.MaxTabID {
		return 0, ErrTabIDExhausted
	}
	s.nextID++
	return TabID(s.nextID), nil
}

func displayTabURL(source *url.URL) string {
	if source == nil {
		return ""
	}
	copy := *source
	copy.User = nil
	return copy.String()
}

func (s *Session) tabByIDLocked(id TabID) (*Tab, bool) {
	if id == 0 {
		return nil, false
	}
	for _, tab := range s.tabs {
		if tab != nil && tab.id == id && tab.state != TabClosing && tab.state != TabClosed {
			return tab, true
		}
	}
	return nil, false
}

// dispatchToTab validates a delayed operation's destination while holding the
// session lock. Removing a tab makes every outstanding operation for its ID stale.
func (s *Session) dispatchToTab(id TabID, dispatch func(*Tab)) bool {
	if s == nil || dispatch == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tab, ok := s.tabByIDLocked(id)
	if !ok {
		return false
	}
	dispatch(tab)
	return true
}
