package browser

import "sync"

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
	id      TabID
	state   TabState
	browser *Browser
}

// TabSnapshot is an immutable view of a tab suitable for browser chrome.
type TabSnapshot struct {
	ID       TabID
	Position int
	State    TabState
	Active   bool
}

// Session owns an ordered collection of tabs and at most one active tab.
type Session struct {
	mu       sync.RWMutex
	tabs     []*Tab
	activeID TabID
}

// NewSession creates an empty browser session.
func NewSession() *Session {
	return &Session{}
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
	return TabSnapshot{ID: tab.id, Position: position, State: tab.state, Active: active}
}
