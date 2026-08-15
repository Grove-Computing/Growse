package browser

import (
	"errors"
	"net/url"
	"sync"
)

var (
	ErrTabIDExhausted = errors.New("tab id space is exhausted")
	ErrTabBrowser     = errors.New("tab browser factory returned nil")
	ErrTabNotFound    = errors.New("tab was not found")
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
}

// TabSnapshot is an immutable view of a tab suitable for browser chrome.
type TabSnapshot struct {
	ID       TabID
	Position int
	State    TabState
	Active   bool
	URL      string
}

// TabCloseResult reports the active tab selected after a close operation.
type TabCloseResult struct {
	Closed    TabID
	Active    TabSnapshot
	HasActive bool
}

// BrowserFactory creates the isolated Browser owned by a new tab.
type BrowserFactory func() *Browser

// Session owns an ordered collection of tabs and at most one active tab.
type Session struct {
	mu       sync.RWMutex
	tabs     []*Tab
	activeID TabID
	nextID   uint64
	factory  BrowserFactory
}

// NewSession creates an empty browser session.
func NewSession(factory ...BrowserFactory) *Session {
	createBrowser := BrowserFactory(func() *Browser { return New(nil) })
	if len(factory) != 0 && factory[0] != nil {
		createBrowser = factory[0]
	}
	return &Session{factory: createBrowser}
}

// NewTab adds an empty tab or a tab with a requested initial URL. Navigation
// is performed separately so callers can control its context and lifecycle.
func (s *Session) NewTab(initialURL *url.URL) (TabSnapshot, error) {
	if s == nil {
		return TabSnapshot{}, ErrTabBrowser
	}
	s.mu.Lock()
	defer s.mu.Unlock()

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
	s.tabs = append(s.tabs, tab)
	return snapshotTab(tab, len(s.tabs)-1, state == TabActive), nil
}

func (s *Session) newTabLocked(initialURL *url.URL, state TabState) (*Tab, error) {
	if s.factory == nil {
		return nil, ErrTabBrowser
	}
	browser := s.factory()
	if browser == nil {
		return nil, ErrTabBrowser
	}
	id, err := s.allocateTabIDLocked()
	if err != nil {
		_ = browser.Close()
		return nil, err
	}
	return &Tab{id: id, state: state, browser: browser, initialURL: cloneURL(initialURL)}, nil
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
		}
	}
	target.state = TabActive
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

func snapshotTab(tab *Tab, position int, active bool) TabSnapshot {
	if tab == nil {
		return TabSnapshot{Position: position}
	}
	snapshot := TabSnapshot{ID: tab.id, Position: position, State: tab.state, Active: active}
	if tab.initialURL != nil {
		snapshot.URL = tab.initialURL.String()
	} else if tab.browser != nil && tab.browser.Page() != nil && tab.browser.Page().URL != nil {
		snapshot.URL = tab.browser.Page().URL.String()
	}
	return snapshot
}

func (s *Session) allocateTabIDLocked() (TabID, error) {
	if s.nextID == ^uint64(0) {
		return 0, ErrTabIDExhausted
	}
	s.nextID++
	return TabID(s.nextID), nil
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
