package browser

import (
	"context"
	"errors"
	"net/url"
	"testing"

	"github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/events"
	"github.com/Grove-Computing/Growse/internal/network"
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

func TestSessionClosingActiveTabSelectsRightThenLeftNeighbor(t *testing.T) {
	session := NewSession()
	first, _ := session.NewTab(nil)
	second, _ := session.NewTab(nil)
	third, _ := session.NewTab(nil)

	closed, err := session.CloseTab(first.ID)
	if err != nil || !closed.HasActive || closed.Active.ID != second.ID {
		t.Fatalf("CloseTab(first) = %#v, %v", closed, err)
	}
	if tabs := session.Tabs(); len(tabs) != 2 || tabs[0].ID != second.ID || tabs[1].ID != third.ID {
		t.Fatalf("Tabs() after closing first = %#v", tabs)
	}

	if _, err := session.SelectTab(third.ID); err != nil {
		t.Fatal(err)
	}
	closed, err = session.CloseTab(third.ID)
	if err != nil || !closed.HasActive || closed.Active.ID != second.ID {
		t.Fatalf("CloseTab(third) = %#v, %v", closed, err)
	}
	if tabs := session.Tabs(); len(tabs) != 1 || tabs[0].ID != second.ID || !tabs[0].Active {
		t.Fatalf("Tabs() after closing third = %#v", tabs)
	}
}

func TestSessionClosingBackgroundTabKeepsCurrentActiveTab(t *testing.T) {
	session := NewSession()
	first, _ := session.NewTab(nil)
	second, _ := session.NewTab(nil)

	closed, err := session.CloseTab(second.ID)
	if err != nil || !closed.HasActive || closed.Active.ID != first.ID {
		t.Fatalf("CloseTab(background) = %#v, %v", closed, err)
	}
	if _, err := session.CloseTab(second.ID); !errors.Is(err, ErrTabNotFound) {
		t.Fatalf("second CloseTab() error = %v, want %v", err, ErrTabNotFound)
	}
}

func TestSessionClosingLastTabCreatesActiveBlankTab(t *testing.T) {
	created := 0
	session := NewSession(func() *Browser {
		created++
		return New(nil)
	})
	last, err := session.NewTab(mustURL(t, "https://example.test/current"))
	if err != nil {
		t.Fatal(err)
	}

	closed, err := session.CloseTab(last.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !closed.HasActive || closed.Active.ID == last.ID || closed.Active.URL != "" || !closed.Active.Active {
		t.Fatalf("CloseTab(last) = %#v", closed)
	}
	tabs := session.Tabs()
	if len(tabs) != 1 || tabs[0].ID != closed.Active.ID || tabs[0].State != TabActive {
		t.Fatalf("Tabs() = %#v", tabs)
	}
	if created != 2 {
		t.Fatalf("browser factory calls = %d, want 2", created)
	}
	if session.dispatchToTab(last.ID, func(*Tab) { t.Fatal("closed tab received delayed work") }) {
		t.Fatal("closed tab remains live")
	}
}

func TestSessionKeepsLastTabWhenBlankReplacementCannotBeCreated(t *testing.T) {
	created := 0
	session := NewSession(func() *Browser {
		created++
		if created > 1 {
			return nil
		}
		return New(nil)
	})
	last, err := session.NewTab(nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := session.CloseTab(last.ID); !errors.Is(err, ErrTabBrowser) {
		t.Fatalf("CloseTab(last) error = %v, want %v", err, ErrTabBrowser)
	}
	active, ok := session.ActiveTab()
	if !ok || active.ID != last.ID || active.State != TabActive {
		t.Fatalf("ActiveTab() = %#v, %t", active, ok)
	}
}

func TestSessionAppliesTabCountAndIDLimitsWithoutChangingExistingTabs(t *testing.T) {
	policy := DefaultSessionPolicy()
	policy.MaxTabs = 1
	policy.MaxTabID = 1
	session := NewSessionWithPolicy(func() *Browser { return New(nil) }, policy)
	first, err := session.NewTab(nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := session.NewTab(nil); !errors.Is(err, ErrTabLimit) {
		t.Fatalf("NewTab() error = %v, want %v", err, ErrTabLimit)
	}
	if tabs := session.Tabs(); len(tabs) != 1 || tabs[0].ID != first.ID || !tabs[0].Active {
		t.Fatalf("Tabs() = %#v", tabs)
	}

	policy.MaxTabs = 2
	idLimited := NewSessionWithPolicy(func() *Browser { return New(nil) }, policy)
	if _, err := idLimited.NewTab(nil); err != nil {
		t.Fatal(err)
	}
	if _, err := idLimited.NewTab(nil); !errors.Is(err, ErrTabIDExhausted) {
		t.Fatalf("ID-limited NewTab() error = %v, want %v", err, ErrTabIDExhausted)
	}
}

func TestSessionAppliesURLAndTitleLimits(t *testing.T) {
	policy := DefaultSessionPolicy()
	policy.MaxURLBytes = 32
	policy.MaxTitleBytes = len("Goタブ")
	created := 0
	session := NewSessionWithPolicy(func() *Browser {
		created++
		return New(nil)
	}, policy)

	if _, err := session.NewTab(mustURL(t, "https://example.test/this-path-is-too-long")); !errors.Is(err, ErrTabURLTooLong) {
		t.Fatalf("long URL error = %v, want %v", err, ErrTabURLTooLong)
	}
	if created != 0 {
		t.Fatalf("browser factory called %d times for rejected URL", created)
	}
	tab, err := session.NewTab(mustURL(t, "https://u:p@e.test/"))
	if err != nil {
		t.Fatal(err)
	}
	if tab.URL != "https://e.test/" {
		t.Fatalf("display URL = %q, want credentials removed", tab.URL)
	}
	updated, err := session.SetTabTitle(tab.ID, "Goタブ")
	if err != nil || updated.Title != "Goタブ" {
		t.Fatalf("SetTabTitle() = %#v, %v", updated, err)
	}
	if _, err := session.SetTabTitle(tab.ID, "Goタブ!"); !errors.Is(err, ErrTabTitle) {
		t.Fatalf("long title error = %v, want %v", err, ErrTabTitle)
	}
	if _, err := session.SetTabTitle(tab.ID, string([]byte{0xff})); !errors.Is(err, ErrTabTitle) {
		t.Fatalf("invalid title error = %v, want %v", err, ErrTabTitle)
	}
	if current := session.Tabs()[0].Title; current != "Goタブ" {
		t.Fatalf("title after rejected updates = %q", current)
	}
}

func TestDefaultSessionPolicyMatchesReleaseLimits(t *testing.T) {
	policy := DefaultSessionPolicy()
	if policy.MaxTabs != 64 || policy.MaxTitleBytes != 4*1024 || policy.MaxURLBytes != 8*1024 || policy.MaxTabID != ^uint64(0) {
		t.Fatalf("DefaultSessionPolicy() = %#v", policy)
	}
}

func TestSessionPublishesBackgroundNavigationStateWithoutSelectingTab(t *testing.T) {
	session := NewSession()
	first, err := session.NewTab(nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := session.NewTab(nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := session.BeginTabNavigation(first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := session.SelectTab(second.ID); err != nil {
		t.Fatal(err)
	}
	finished, err := session.FinishTabNavigation(first.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if finished.Active || finished.Loading || !finished.Error || !finished.PendingUpdate {
		t.Fatalf("background navigation state = %+v", finished)
	}
	if active, ok := session.ActiveTab(); !ok || active.ID != second.ID {
		t.Fatalf("active tab = (%+v, %v), want %d", active, ok, second.ID)
	}
	selected, err := session.SelectTab(first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if selected.PendingUpdate {
		t.Fatalf("pending update remained after selection: %+v", selected)
	}
}

func TestSessionSubmitsFormAndFormtargetBlankToNewTab(t *testing.T) {
	tests := []struct {
		name            string
		formTarget      string
		submitterTarget string
		useSubmitter    bool
	}{
		{name: "form target", formTarget: "_blank"},
		{name: "submitter formtarget", formTarget: "_self", submitterTarget: "_blank", useSubmitter: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			document := dom.NewDocument()
			form := document.CreateElement("form", map[string]string{"action": "/result", "target": test.formTarget})
			button := document.CreateElement("button", map[string]string{"type": "submit", "formtarget": test.submitterTarget})
			if err := document.AppendChild(document.Root, form); err != nil {
				t.Fatal(err)
			}
			if err := document.AppendChild(form, button); err != nil {
				t.Fatal(err)
			}
			sourceURL := mustURL(t, "https://example.test/source")
			targetURL := mustURL(t, "https://example.test/result")
			source := New(nil)
			source.SetPage(&Page{URL: sourceURL, Document: document, Events: events.NewDispatcher()})
			loader := &routeLoader{responses: map[string]*network.Response{
				targetURL.String(): {URL: targetURL, StatusCode: 200, ContentType: "text/html", Body: []byte("<title>Result</title>")},
			}}
			created := 0
			session := NewSession(func() *Browser {
				created++
				if created == 1 {
					return source
				}
				return New(loader)
			})
			sourceTab, err := session.NewTab(nil)
			if err != nil {
				t.Fatal(err)
			}
			submitterID := dom.NodeID(0)
			if test.useSubmitter {
				submitterID = button.ID
			}
			opened, page, err := session.SubmitFormToNewTab(context.Background(), sourceTab.ID, form.ID, submitterID)
			if err != nil {
				t.Fatal(err)
			}
			if page == nil || page.URL.String() != targetURL.String() {
				t.Fatalf("submitted page = %+v, want %s", page, targetURL)
			}
			if active, ok := session.ActiveTab(); !ok || active.ID != opened.ID || opened.ID == sourceTab.ID {
				t.Fatalf("active tab after submission = (%+v, %v), opened=%+v", active, ok, opened)
			}
			if source.Page().URL.String() != sourceURL.String() {
				t.Fatalf("source tab navigated to %s", source.Page().URL)
			}
		})
	}
}

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}
