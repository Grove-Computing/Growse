package searchdata

import (
	"fmt"
	"testing"
	"time"
)

func TestHistoryRecordsOnlySuccessfulTopLevelNavigation(t *testing.T) {
	store := NewMemoryStore()
	base := Navigation{URL: "https://Example.com:443/docs/#intro", Title: "Docs", VisitedAt: time.Unix(100, 0).UTC(), TopLevel: true, Success: true, Typed: true}
	if err := store.RecordNavigation(base); err != nil {
		t.Fatal(err)
	}
	for _, ignored := range []Navigation{
		{URL: "https://example.com/reload", TopLevel: true, Success: true, Reload: true},
		{URL: "https://example.com/fragment", TopLevel: true, Success: true, SameDocument: true},
		{URL: "https://example.com/redirect", TopLevel: true, Success: true, RedirectIntermediate: true},
		{URL: "https://example.com/failure", TopLevel: true},
		{URL: "https://example.com/frame", Success: true},
	} {
		if err := store.RecordNavigation(ignored); err != nil {
			t.Fatal(err)
		}
	}
	second := base
	second.URL = "https://example.com/docs"
	second.Title = "Updated"
	second.VisitedAt = time.Unix(200, 0).UTC()
	second.Typed = false
	if err := store.RecordNavigation(second); err != nil {
		t.Fatal(err)
	}
	entries := store.History()
	if len(entries) != 1 {
		t.Fatalf("history entries = %d, want 1", len(entries))
	}
	entry := entries[0]
	if entry.URL != "https://example.com/docs" || entry.Title != "Updated" || entry.VisitCount != 2 || entry.TypedCount != 1 || !entry.LastVisited.Equal(second.VisitedAt) {
		t.Fatalf("history entry = %#v", entry)
	}
}

func TestHistoryIsBoundedAndEvictsOldest(t *testing.T) {
	store := NewMemoryStore()
	store.state.data.History = make([]HistoryEntry, MaxHistoryEntries)
	for index := range store.state.data.History {
		store.state.data.History[index] = HistoryEntry{
			URL: fmt.Sprintf("https://example.com/%d", index), Title: "entry",
			LastVisited: time.Unix(int64(index+1), 0).UTC(), VisitCount: 1,
		}
	}
	if err := store.RecordNavigation(Navigation{URL: "https://new.example/", Title: "new", VisitedAt: time.Unix(MaxHistoryEntries+10, 0).UTC(), TopLevel: true, Success: true}); err != nil {
		t.Fatal(err)
	}
	entries := store.History()
	if len(entries) != MaxHistoryEntries || entries[0].URL != "https://new.example/" {
		t.Fatalf("bounded history = %d, newest = %q", len(entries), entries[0].URL)
	}
	for _, entry := range entries {
		if entry.LastVisited.Equal(time.Unix(1, 0).UTC()) {
			t.Fatal("oldest history entry was not evicted")
		}
	}
}
