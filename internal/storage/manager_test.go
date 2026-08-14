package storage

import (
	"net/url"
	"testing"
)

func parseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func TestManagerSeparatesAreasByNormalizedOriginOnly(t *testing.T) {
	manager := NewManager()
	local, session, err := manager.Areas(parseURL(t, "https://EXAMPLE.test/app?x=1#top"))
	if err != nil {
		t.Fatal("valid HTTPS origin was rejected")
	}
	sameLocal, sameSession, _ := manager.Areas(parseURL(t, "https://example.test:443/other"))
	if sameLocal != local || sameSession != session {
		t.Fatal("path or default port split the same Origin")
	}

	for _, raw := range []string{
		"http://example.test/app",
		"https://other.test/app",
		"https://example.test:8443/app",
	} {
		otherLocal, otherSession, err := manager.Areas(parseURL(t, raw))
		if err != nil || otherLocal == local || otherSession == session {
			t.Fatalf("Origin %q was not isolated", raw)
		}
	}
	for _, raw := range []string{"file:///tmp/app", "data:text/plain,hello", "about:blank"} {
		if local, session, err := manager.Areas(parseURL(t, raw)); err == nil || local != nil || session != nil {
			t.Fatalf("opaque/non-HTTP Origin %q received Storage", raw)
		}
	}
}

func TestPersistentManagerRestoresLocalButNotSessionStorage(t *testing.T) {
	root := t.TempDir()
	documentURL := parseURL(t, "https://example.test/app")
	first, err := NewPersistentManager(root)
	if err != nil {
		t.Fatal(err)
	}
	local, session, err := first.Areas(documentURL)
	if err != nil {
		t.Fatal(err)
	}
	if err := local.Set("theme", "dark"); err != nil {
		t.Fatal(err)
	}
	if err := session.Set("draft", "temporary"); err != nil {
		t.Fatal(err)
	}

	restarted, err := NewPersistentManager(root)
	if err != nil {
		t.Fatal(err)
	}
	restoredLocal, restoredSession, err := restarted.Areas(documentURL)
	if err != nil {
		t.Fatal(err)
	}
	if got, found := restoredLocal.Get("theme"); !found || got != "dark" {
		t.Fatalf("restored Local Storage = (%q, %v)", got, found)
	}
	if got, found := restoredSession.Get("draft"); found || got != "" {
		t.Fatalf("Session Storage persisted = (%q, %v)", got, found)
	}
}
