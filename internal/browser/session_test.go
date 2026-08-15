package browser

import (
	"errors"
	"testing"
)

func TestSessionKeepsOrderedTabsAndOneActiveTab(t *testing.T) {
	session := NewSession()
	session.tabs = []*Tab{
		{id: 11, state: TabBackground},
		{id: 22, state: TabActive},
		{id: 33, state: TabBackground},
	}
	session.activeID = 22

	tabs := session.Tabs()
	if len(tabs) != 3 {
		t.Fatalf("len(Tabs()) = %d, want 3", len(tabs))
	}
	for index, wantID := range []TabID{11, 22, 33} {
		if tabs[index].ID != wantID || tabs[index].Position != index {
			t.Fatalf("Tabs()[%d] = %#v, want id %d at position %d", index, tabs[index], wantID, index)
		}
	}
	if tabs[0].Active || !tabs[1].Active || tabs[2].Active {
		t.Fatalf("active flags = %#v, want only the middle tab active", tabs)
	}

	active, ok := session.ActiveTab()
	if !ok || active.ID != 22 || active.State != TabActive {
		t.Fatalf("ActiveTab() = %#v, %t, want tab 22", active, ok)
	}
}

func TestEmptySessionHasNoActiveTab(t *testing.T) {
	session := NewSession()
	if tabs := session.Tabs(); len(tabs) != 0 {
		t.Fatalf("Tabs() = %#v, want empty", tabs)
	}
	if active, ok := session.ActiveTab(); ok {
		t.Fatalf("ActiveTab() = %#v, true, want no active tab", active)
	}
}

func TestSessionTabIDsAreNeverReusedForDelayedDispatch(t *testing.T) {
	session := NewSession()

	session.mu.Lock()
	firstID, err := session.allocateTabIDLocked()
	if err != nil {
		session.mu.Unlock()
		t.Fatal(err)
	}
	session.tabs = append(session.tabs, &Tab{id: firstID, state: TabActive})
	session.activeID = firstID
	session.tabs = nil // The first tab has been removed before delayed work completes.
	secondID, err := session.allocateTabIDLocked()
	if err != nil {
		session.mu.Unlock()
		t.Fatal(err)
	}
	second := &Tab{id: secondID, state: TabActive}
	session.tabs = append(session.tabs, second)
	session.activeID = secondID
	session.mu.Unlock()

	if firstID == secondID {
		t.Fatalf("reused tab id %d", firstID)
	}
	called := false
	if session.dispatchToTab(firstID, func(*Tab) { called = true }) || called {
		t.Fatal("delayed work for a removed tab was dispatched")
	}
	if !session.dispatchToTab(secondID, func(tab *Tab) { called = tab == second }) || !called {
		t.Fatal("live tab did not receive its work")
	}
}

func TestSessionRejectsTabIDOverflow(t *testing.T) {
	session := NewSession()
	session.nextID = ^uint64(0)

	session.mu.Lock()
	_, err := session.allocateTabIDLocked()
	session.mu.Unlock()
	if !errors.Is(err, ErrTabIDExhausted) {
		t.Fatalf("allocateTabIDLocked() error = %v, want %v", err, ErrTabIDExhausted)
	}
}
