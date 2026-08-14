package navigation

import (
	"errors"
	"net/url"
	"strings"
	"testing"
)

func TestCurrentReturnsDocumentURLComponentsWithoutCredentials(t *testing.T) {
	documentURL, err := url.Parse("https://alice:secret@example.test:8443/app/a%20b?mode=edit#details")
	if err != nil {
		t.Fatal(err)
	}

	got := New(documentURL).Current()
	if got.Href != "https://example.test:8443/app/a%20b?mode=edit#details" {
		t.Fatalf("Href = %q", got.Href)
	}
	if got.Origin != "https://example.test:8443" {
		t.Fatalf("Origin = %q", got.Origin)
	}
	if got.Scheme != "https" || got.Host != "example.test:8443" || got.Hostname != "example.test" || got.Port != "8443" {
		t.Fatalf("authority components = %#v", got)
	}
	if got.Path != "/app/a%20b" || got.Query != "mode=edit" || got.Fragment != "details" {
		t.Fatalf("resource components = %#v", got)
	}
}

func TestCurrentWithoutDocumentURLReturnsZeroValue(t *testing.T) {
	if got := New(nil).Current(); got != (Location{}) {
		t.Fatalf("Current() = %#v, want zero value", got)
	}
}

func TestResolveAndNavigateUseDocumentURLAsBase(t *testing.T) {
	base, err := url.Parse("https://example.test/app/pages/index.html")
	if err != nil {
		t.Fatal(err)
	}
	var navigated *url.URL
	api := NewPage(base, func(target *url.URL) error {
		navigated = target
		return nil
	})

	resolved, err := api.Resolve("../next?mode=full#result")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got, want := resolved.Href, "https://example.test/app/next?mode=full#result"; got != want {
		t.Fatalf("Resolve().Href = %q, want %q", got, want)
	}
	if got, want := resolved.Port, "443"; got != want {
		t.Fatalf("Resolve().Port = %q, want effective port %q", got, want)
	}
	if err := api.Navigate("../next?mode=full#result"); err != nil {
		t.Fatalf("Navigate() error = %v", err)
	}
	if navigated == nil || navigated.String() != resolved.Href {
		t.Fatalf("navigated URL = %v, want %q", navigated, resolved.Href)
	}
}

func TestNavigationRejectsUnsafeURLsBeforeCallback(t *testing.T) {
	base, _ := url.Parse("https://example.test/app/")
	called := false
	api := NewPage(base, func(*url.URL) error { called = true; return nil })
	for _, rawURL := range []string{
		"javascript:alert(1)",
		"data:text/plain,hello",
		"file:///tmp/secret",
		"https://alice:secret@example.test/private",
		"https://example.test:invalid/path",
		" /space",
	} {
		if err := api.Navigate(rawURL); !errors.Is(err, ErrInvalidURL) {
			t.Errorf("Navigate(%q) error = %v, want ErrInvalidURL", rawURL, err)
		}
	}
	if called {
		t.Fatal("unsafe URL reached Browser callback")
	}
}

func TestUpdateCurrentChangesLocationAndResolutionBase(t *testing.T) {
	initial, _ := url.Parse("https://example.test/notes#first")
	updated, _ := url.Parse("https://example.test/archive#second")
	api := New(initial)

	api.UpdateCurrent(updated)
	if got := api.Current().Href; got != updated.String() {
		t.Fatalf("Current().Href = %q, want %q", got, updated)
	}
	resolved, err := api.Resolve("next")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := resolved.Href, "https://example.test/next"; got != want {
		t.Fatalf("Resolve().Href = %q, want %q", got, want)
	}
}

func TestPushStateValidatesJSONAndSameOriginBeforeAddingEntry(t *testing.T) {
	base, _ := url.Parse("https://example.test/app/index.html")
	api := New(base)
	var gotState string
	var gotURL *url.URL
	api.SetPushStateHandler(func(state string, target *url.URL) error {
		gotState, gotURL = state, target
		return nil
	})

	if err := api.PushState(`{"note":7}`, "../notes/7?mode=edit"); err != nil {
		t.Fatalf("PushState() error = %v", err)
	}
	if gotState != `{"note":7}` || gotURL == nil || gotURL.String() != "https://example.test/notes/7?mode=edit" {
		t.Fatalf("history entry = (%q, %v)", gotState, gotURL)
	}
	if got := api.Current().Href; got != gotURL.String() {
		t.Fatalf("Current().Href = %q, want %q", got, gotURL)
	}

	for _, test := range []struct{ state, rawURL string }{
		{state: `{`, rawURL: "/bad-json"},
		{state: `null`, rawURL: "https://other.test/cross-origin"},
		{state: `null`, rawURL: "https://user:secret@example.test/private"},
		{state: `"` + strings.Repeat("x", MaxHistoryStateSize) + `"`, rawURL: "/huge"},
	} {
		if err := api.PushState(test.state, test.rawURL); err == nil {
			t.Errorf("PushState(%q, %q) error = nil", test.state, test.rawURL)
		} else if strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), test.state) {
			t.Errorf("PushState error exposed sensitive input: %q", err)
		}
	}
}

func TestHistoryRejectsInvalidUTF8CredentialAndLongURLWithoutCallingBrowser(t *testing.T) {
	base, _ := url.Parse("https://example.test/app")
	api := New(base)
	called := false
	api.SetPushStateHandler(func(string, *url.URL) error { called = true; return nil })
	tests := []struct{ state, rawURL string }{
		{state: string([]byte{'"', 0xff, '"'}), rawURL: "/invalid-utf8"},
		{state: `null`, rawURL: "https://alice:super-secret@example.test/private"},
		{state: `null`, rawURL: "/" + strings.Repeat("x", MaxURLSize)},
	}
	for _, test := range tests {
		err := api.PushState(test.state, test.rawURL)
		if err == nil {
			t.Fatalf("PushState() error = nil for rejected input")
		}
		if strings.Contains(err.Error(), "super-secret") || strings.Contains(err.Error(), "alice") || strings.Contains(err.Error(), test.state) {
			t.Fatalf("error exposed rejected input: %q", err)
		}
	}
	if called {
		t.Fatal("rejected History input reached Browser handler")
	}
}

func TestReplaceStateUsesCurrentURLWhenURLIsEmpty(t *testing.T) {
	base, _ := url.Parse("https://example.test/app?old=1")
	api := New(base)
	var state string
	var target *url.URL
	api.SetReplaceStateHandler(func(gotState string, gotTarget *url.URL) error {
		state, target = gotState, gotTarget
		return nil
	})

	if err := api.ReplaceState(`["replaced"]`, ""); err != nil {
		t.Fatalf("ReplaceState() error = %v", err)
	}
	if state != `["replaced"]` || target == nil || target.String() != base.String() {
		t.Fatalf("replacement = (%q, %v)", state, target)
	}
}

func TestHistoryTraversalAndSnapshotUseBrowserHandlers(t *testing.T) {
	api := New(nil)
	var deltas []int
	api.SetTraversalHandler(func(delta int) error {
		deltas = append(deltas, delta)
		return nil
	}, func() (int, string) { return 4, `{"page":2}` })

	if err := api.Back(); err != nil {
		t.Fatal(err)
	}
	if err := api.Forward(); err != nil {
		t.Fatal(err)
	}
	if err := api.Go(-2); err != nil {
		t.Fatal(err)
	}
	if len(deltas) != 3 || deltas[0] != -1 || deltas[1] != 1 || deltas[2] != -2 {
		t.Fatalf("traversal deltas = %v", deltas)
	}
	if api.HistoryLength() != 4 || api.HistoryState() != `{"page":2}` {
		t.Fatalf("history snapshot = (%d, %q)", api.HistoryLength(), api.HistoryState())
	}
}

func TestNavigationEventListenersReceivePayloadsInOrder(t *testing.T) {
	api := New(nil)
	var events []string
	api.OnPopState(func(event PopStateEvent) { events = append(events, "pop:"+event.State) })
	api.OnHashChange(func(event HashChangeEvent) { events = append(events, "hash:"+event.OldURL+"->"+event.NewURL) })

	api.DispatchPopState(`{"page":1}`)
	api.DispatchHashChange("https://example.test/#one", "https://example.test/#two")
	want := []string{`pop:{"page":1}`, "hash:https://example.test/#one->https://example.test/#two"}
	if len(events) != len(want) || events[0] != want[0] || events[1] != want[1] {
		t.Fatalf("events = %v, want %v", events, want)
	}
}
