package storage

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
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

func TestPersistentAreaRollsBackWhenAtomicRenameFails(t *testing.T) {
	root := t.TempDir()
	documentURL := parseURL(t, "https://example.test/app")
	manager, err := NewPersistentManager(root)
	if err != nil {
		t.Fatal(err)
	}
	local, _, err := manager.Areas(documentURL)
	if err != nil {
		t.Fatal(err)
	}
	if err := local.Set("draft", "stable"); err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(root, "local-storage")
	path := persistentFilePath(directory, "https://example.test")
	backup := path + ".backup"
	if err := os.Rename(path, backup); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}

	if err := local.Set("draft", "uncommitted"); !errors.Is(err, ErrStorageIO) {
		t.Fatalf("Set() error = %v, want ErrStorageIO", err)
	}
	if got, found := local.Get("draft"); !found || got != "stable" {
		t.Fatalf("in-memory rollback = (%q, %v)", got, found)
	}
	files, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if strings.HasPrefix(file.Name(), ".storage-") {
			t.Fatalf("abandoned transaction file = %q", file.Name())
		}
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(backup, path); err != nil {
		t.Fatal(err)
	}

	restarted, err := NewPersistentManager(root)
	if err != nil {
		t.Fatal(err)
	}
	restored, _, err := restarted.Areas(documentURL)
	if err != nil {
		t.Fatal(err)
	}
	if got, found := restored.Get("draft"); !found || got != "stable" {
		t.Fatalf("persisted rollback = (%q, %v)", got, found)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("storage file permissions = %o, want 600", info.Mode().Perm())
	}
}

func TestManagerCapsOriginNamespaces(t *testing.T) {
	manager := NewManager()
	for index := 0; index < MaxStorageOrigins; index++ {
		if _, _, err := manager.Areas(parseURL(t, fmt.Sprintf("https://origin-%d.test/", index))); err != nil {
			t.Fatalf("Areas(%d) error = %v", index, err)
		}
	}
	if _, _, err := manager.Areas(parseURL(t, "https://overflow.test/")); !errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("Origin namespace quota error = %v", err)
	}
}

func TestPersistentManagerAppliesProfileQuotaBeforeWrite(t *testing.T) {
	root := t.TempDir()
	manager, err := NewPersistentManager(root)
	if err != nil {
		t.Fatal(err)
	}
	local, _, err := manager.Areas(parseURL(t, "https://example.test/"))
	if err != nil {
		t.Fatal(err)
	}
	filler := filepath.Join(root, "local-storage", "existing-profile-data")
	file, err := os.Create(filler)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(MaxProfileStorageBytes); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := local.Set("key", "value"); !errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("profile quota error = %v", err)
	}
	if _, found := local.Get("key"); found {
		t.Fatal("profile quota failure mutated Local Storage")
	}
}

func TestPersistentManagerLocalizesSchemaAndCorruptDataErrors(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    error
	}{
		{name: "schema", content: `{"version":99,"origin":"https://broken.test","entries":[]}`, want: ErrSchemaMismatch},
		{name: "json", content: `{broken`, want: ErrCorruptData},
		{name: "duplicate", content: `{"version":1,"origin":"https://broken.test","entries":[{"key":"x","value":"1"},{"key":"x","value":"2"}]}`, want: ErrCorruptData},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			manager, err := NewPersistentManager(root)
			if err != nil {
				t.Fatal(err)
			}
			path := persistentFilePath(filepath.Join(root, "local-storage"), "https://broken.test")
			if err := os.WriteFile(path, []byte(test.content), 0o600); err != nil {
				t.Fatal(err)
			}
			local, session, err := manager.Areas(parseURL(t, "https://broken.test/"))
			if !errors.Is(err, test.want) || local == nil || !errors.Is(local.Error(), test.want) {
				t.Fatalf("broken Origin = local:%v error:%v", local, err)
			}
			if session == nil || session.Set("available", "yes") != nil {
				t.Fatal("broken Local Storage disabled Session Storage")
			}
			other, _, err := manager.Areas(parseURL(t, "https://healthy.test/"))
			if err != nil || other.Set("healthy", "yes") != nil {
				t.Fatalf("broken Origin affected healthy Origin: %v", err)
			}
		})
	}
}

func TestPersistentManagerReportsProfilePathIOError(t *testing.T) {
	root := filepath.Join(t.TempDir(), "profile-file")
	if err := os.WriteFile(root, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewPersistentManager(root); !errors.Is(err, ErrStorageIO) {
		t.Fatalf("NewPersistentManager() error = %v, want ErrStorageIO", err)
	}
}
