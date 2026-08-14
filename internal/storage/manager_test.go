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
	local, session, ok := manager.Areas(parseURL(t, "https://EXAMPLE.test/app?x=1#top"))
	if !ok {
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
		otherLocal, otherSession, ok := manager.Areas(parseURL(t, raw))
		if !ok || otherLocal == local || otherSession == session {
			t.Fatalf("Origin %q was not isolated", raw)
		}
	}
	for _, raw := range []string{"file:///tmp/app", "data:text/plain,hello", "about:blank"} {
		if local, session, ok := manager.Areas(parseURL(t, raw)); ok || local != nil || session != nil {
			t.Fatalf("opaque/non-HTTP Origin %q received Storage", raw)
		}
	}
}
