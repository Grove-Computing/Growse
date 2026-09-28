package searchdata

import (
	"context"
	"testing"
	"time"
)

func TestHistoryDeletionUpdatesDataAndIndexTogether(t *testing.T) {
	store := NewMemoryStore()
	for index, rawURL := range []string{"https://example.com/old", "https://example.com/middle", "https://example.com/new"} {
		if err := store.RecordNavigation(Navigation{
			URL: rawURL, Title: rawURL, VisitedAt: time.Unix(int64(index+1)*100, 0).UTC(), TopLevel: true, Success: true,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.DeleteHistory("https://example.com/middle#fragment"); err != nil {
		t.Fatal(err)
	}
	if got := store.SuggestionSnapshot(context.Background(), "middle"); len(got.History) != 0 {
		t.Fatalf("deleted URL remained in index: %#v", got.History)
	}
	if err := store.DeleteHistoryPeriod(time.Unix(0, 0).UTC(), time.Unix(150, 0).UTC()); err != nil {
		t.Fatal(err)
	}
	if got := store.SuggestionSnapshot(context.Background(), "old"); len(got.History) != 0 {
		t.Fatalf("period-deleted URL remained in index: %#v", got.History)
	}
	if entries := store.History(); len(entries) != 1 || entries[0].URL != "https://example.com/new" {
		t.Fatalf("history after period deletion = %#v", entries)
	}
	if err := store.ClearHistory(); err != nil {
		t.Fatal(err)
	}
	if len(store.History()) != 0 || len(store.SuggestionSnapshot(context.Background(), "").History) != 0 {
		t.Fatal("clear history left data or index candidates")
	}
}
