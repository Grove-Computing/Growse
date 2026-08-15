package browser

import (
	"errors"
	"net/url"
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

func TestSessionCreatesEmptyAndURLTabs(t *testing.T) {
	created := 0
	session := NewSession(func() *Browser {
		created++
		return New(nil)
	})

	empty, err := session.NewTab(nil)
	if err != nil {
		t.Fatal(err)
	}
	if !empty.Active || empty.State != TabActive || empty.URL != "" {
		t.Fatalf("empty tab = %#v", empty)
	}

	target, err := url.Parse("https://example.test/notes?q=go#today")
	if err != nil {
		t.Fatal(err)
	}
	withURL, err := session.NewTab(target)
	if err != nil {
		t.Fatal(err)
	}
	target.Host = "mutated.test"
	if withURL.Active || withURL.State != TabBackground || withURL.URL != "https://example.test/notes?q=go#today" {
		t.Fatalf("URL tab = %#v", withURL)
	}
	if created != 2 {
		t.Fatalf("browser factory calls = %d, want 2", created)
	}
	if tabs := session.Tabs(); len(tabs) != 2 || tabs[1].URL != withURL.URL {
		t.Fatalf("Tabs() = %#v", tabs)
	}
}

func TestSessionRejectsNilBrowserFromFactory(t *testing.T) {
	session := NewSession(func() *Browser { return nil })
	if _, err := session.NewTab(nil); !errors.Is(err, ErrTabBrowser) {
		t.Fatalf("NewTab() error = %v, want %v", err, ErrTabBrowser)
	}
	if len(session.Tabs()) != 0 {
		t.Fatal("failed tab creation changed the collection")
	}
}

func TestSessionSelectsTabByIDAndPosition(t *testing.T) {
	session := NewSession()
	first, err := session.NewTab(nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := session.NewTab(nil)
	if err != nil {
		t.Fatal(err)
	}
	third, err := session.NewTab(nil)
	if err != nil {
		t.Fatal(err)
	}

	selected, err := session.SelectTab(second.ID)
	if err != nil || selected.ID != second.ID || !selected.Active || selected.Position != 1 {
		t.Fatalf("SelectTab() = %#v, %v", selected, err)
	}
	selected, err = session.SelectTabAt(2)
	if err != nil || selected.ID != third.ID || !selected.Active {
		t.Fatalf("SelectTabAt() = %#v, %v", selected, err)
	}
	active, ok := session.ActiveTab()
	if !ok || active.ID != third.ID {
		t.Fatalf("ActiveTab() = %#v, %t", active, ok)
	}
	tabs := session.Tabs()
	if tabs[0].ID != first.ID || tabs[0].State != TabBackground || tabs[1].State != TabBackground || tabs[2].State != TabActive {
		t.Fatalf("Tabs() = %#v", tabs)
	}
}

func TestSessionRejectsInvalidTabSelectionWithoutChangingActiveTab(t *testing.T) {
	session := NewSession()
	first, err := session.NewTab(nil)
	if err != nil {
		t.Fatal(err)
	}

	for _, selectInvalid := range []func() error{
		func() error { _, err := session.SelectTab(999); return err },
		func() error { _, err := session.SelectTabAt(-1); return err },
		func() error { _, err := session.SelectTabAt(1); return err },
	} {
		if err := selectInvalid(); !errors.Is(err, ErrTabNotFound) {
			t.Fatalf("selection error = %v, want %v", err, ErrTabNotFound)
		}
	}
	active, ok := session.ActiveTab()
	if !ok || active.ID != first.ID {
		t.Fatalf("ActiveTab() = %#v, %t", active, ok)
	}
}

func TestSessionSelectsNextAndPreviousTabsCyclically(t *testing.T) {
	session := NewSession()
	first, _ := session.NewTab(nil)
	second, _ := session.NewTab(nil)
	third, _ := session.NewTab(nil)

	for _, want := range []TabID{second.ID, third.ID, first.ID} {
		selected, err := session.SelectNext()
		if err != nil || selected.ID != want {
			t.Fatalf("SelectNext() = %#v, %v, want %d", selected, err, want)
		}
	}
	for _, want := range []TabID{third.ID, second.ID, first.ID} {
		selected, err := session.SelectPrevious()
		if err != nil || selected.ID != want {
			t.Fatalf("SelectPrevious() = %#v, %v, want %d", selected, err, want)
		}
	}
}

func TestEmptySessionCannotSelectRelativeTab(t *testing.T) {
	session := NewSession()
	if _, err := session.SelectNext(); !errors.Is(err, ErrTabNotFound) {
		t.Fatalf("SelectNext() error = %v, want %v", err, ErrTabNotFound)
	}
	if _, err := session.SelectPrevious(); !errors.Is(err, ErrTabNotFound) {
		t.Fatalf("SelectPrevious() error = %v, want %v", err, ErrTabNotFound)
	}
}
