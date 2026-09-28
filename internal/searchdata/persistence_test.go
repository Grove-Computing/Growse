package searchdata

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestProfileRestartRebuildsIndexAndRecoversBackup(t *testing.T) {
	root := t.TempDir()
	store, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	store.now = func() time.Time { return time.Unix(100, 0).UTC() }
	if err := store.RecordNavigation(Navigation{URL: "https://example.com/first", Title: "First snapshot", TopLevel: true, Success: true}); err != nil {
		t.Fatal(err)
	}
	store.now = func() time.Time { return time.Unix(200, 0).UTC() }
	if _, err := store.SaveBookmark("", "https://example.com/bookmark", "Latest snapshot"); err != nil {
		t.Fatal(err)
	}
	sharedProfiles.Delete(store.path)
	restarted, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := restarted.SuggestionSnapshot(context.Background(), "latest snapshot"); len(got.Bookmarks) != 1 {
		t.Fatalf("rebuilt bookmark index = %#v", got.Bookmarks)
	}
	if err := os.WriteFile(store.path, []byte("broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	sharedProfiles.Delete(store.path)
	recovered, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	if entries := recovered.History(); len(entries) != 1 || entries[0].Title != "First snapshot" {
		t.Fatalf("backup recovery history = %#v", entries)
	}
	if len(recovered.Bookmarks()) != 0 {
		t.Fatal("recovery did not use the previous valid snapshot")
	}
	if err := os.WriteFile(store.path+".bak", []byte("broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	sharedProfiles.Delete(store.path)
	empty, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(empty.History()) != 0 || len(empty.Bookmarks()) != 0 {
		t.Fatal("double corruption did not recover to an empty profile")
	}
}

func TestAtomicWriteFailurePreservesLastCommittedState(t *testing.T) {
	root := t.TempDir()
	store, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RecordNavigation(Navigation{URL: "https://example.com/kept", Title: "Kept", TopLevel: true, Success: true}); err != nil {
		t.Fatal(err)
	}
	blocked := filepath.Join(root, "blocked")
	if err := os.Mkdir(blocked, 0o700); err != nil {
		t.Fatal(err)
	}
	store.path = blocked
	if _, err := store.SaveBookmark("", "https://example.com/rejected", "Rejected"); err != ErrStorage {
		t.Fatalf("write failure = %v, want %v", err, ErrStorage)
	}
	if entries := store.History(); len(entries) != 1 || entries[0].Title != "Kept" || len(store.Bookmarks()) != 0 {
		t.Fatalf("failed commit mutated memory: history=%#v bookmarks=%#v", entries, store.Bookmarks())
	}
	files, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if strings.HasPrefix(file.Name(), ".search-data-") {
			t.Fatalf("temporary file leaked: %s", file.Name())
		}
	}
}

func TestIndependentWritersReloadUnderProfileLock(t *testing.T) {
	root := t.TempDir()
	first, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	sharedProfiles.Delete(first.path)
	second, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	var wait sync.WaitGroup
	for index, store := range []*Store{first, second} {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, saveErr := store.SaveBookmark("", "https://example.com/"+string(rune('a'+index)), "writer")
			if saveErr != nil {
				t.Errorf("SaveBookmark() error = %v", saveErr)
			}
		}()
	}
	wait.Wait()
	sharedProfiles.Delete(first.path)
	combined, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	if bookmarks := combined.Bookmarks(); len(bookmarks) != 2 {
		t.Fatalf("concurrent writer bookmarks = %#v", bookmarks)
	}
}

func TestProfileNeverPersistsIndexQueryOrPageText(t *testing.T) {
	root := t.TempDir()
	store, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RecordNavigation(Navigation{URL: "https://example.com/", Title: "Allowed title", TopLevel: true, Success: true}); err != nil {
		t.Fatal(err)
	}
	const query = "private-search-query"
	_ = store.SuggestionSnapshot(context.Background(), query)
	body, err := os.ReadFile(store.path) // #nosec G304 -- test reads the store's fixed profile path.
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), query) || strings.Contains(string(body), "page body secret") || strings.Contains(string(body), "\"index\"") {
		t.Fatalf("derived or transient data persisted: %s", body)
	}
}

func TestWriterLockReleasedWhenOwnerCloses(t *testing.T) {
	path := filepath.Join(t.TempDir(), "writer.lock")
	owner, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	contender, err := os.OpenFile(path, os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer contender.Close()
	if locked, err := tryWriterLock(owner); err != nil || !locked {
		t.Fatal(locked, err)
	}
	if locked, err := tryWriterLock(contender); err != nil || locked {
		t.Fatal("concurrent descriptor acquired writer lock", locked, err)
	}
	if err := owner.Close(); err != nil {
		t.Fatal(err)
	}
	if locked, err := tryWriterLock(contender); err != nil || !locked {
		t.Fatal("orphaned writer lock", locked, err)
	}
	releaseWriterLock(contender)
}
