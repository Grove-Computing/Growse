package searchdata

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestBookmarkAddEditDeduplicateAndDelete(t *testing.T) {
	store := NewMemoryStore()
	clock := time.Unix(100, 0).UTC()
	store.now = func() time.Time { return clock }
	first, err := store.SaveBookmark("", "https://Example.com:443/docs/#part", "Docs")
	if err != nil {
		t.Fatal(err)
	}
	clock = time.Unix(200, 0).UTC()
	updated, err := store.SaveBookmark(first.URL, "https://example.com/guide", "Guide")
	if err != nil {
		t.Fatal(err)
	}
	if updated.CreatedAt != first.CreatedAt || !updated.UpdatedAt.Equal(clock) {
		t.Fatalf("edited bookmark timestamps = %#v", updated)
	}
	clock = time.Unix(300, 0).UTC()
	if _, err := store.SaveBookmark("", "https://example.com/guide/", "Guide updated"); err != nil {
		t.Fatal(err)
	}
	bookmarks := store.Bookmarks()
	if len(bookmarks) != 1 || bookmarks[0].Title != "Guide updated" {
		t.Fatalf("deduplicated bookmarks = %#v", bookmarks)
	}
	if got := store.SuggestionSnapshot(context.Background(), "guide updated"); len(got.Bookmarks) != 1 {
		t.Fatalf("bookmark index = %#v", got.Bookmarks)
	}
	if err := store.DeleteBookmark("https://EXAMPLE.com:443/guide#ignored"); err != nil {
		t.Fatal(err)
	}
	if len(store.Bookmarks()) != 0 || len(store.SuggestionSnapshot(context.Background(), "guide").Bookmarks) != 0 {
		t.Fatal("deleted bookmark remained in data or index")
	}
}

func TestBookmarkLimitPreservesExistingData(t *testing.T) {
	store := NewMemoryStore()
	when := time.Unix(1, 0).UTC()
	store.state.data.Bookmarks = make([]Bookmark, MaxBookmarkEntries)
	for index := range store.state.data.Bookmarks {
		store.state.data.Bookmarks[index] = Bookmark{URL: fmt.Sprintf("https://example.com/%d", index), Title: "entry", CreatedAt: when, UpdatedAt: when}
	}
	store.state.index = rebuildIndex(store.state.data)
	if _, err := store.SaveBookmark("", "https://new.example/", "new"); err != ErrLimit {
		t.Fatalf("bookmark limit error = %v, want %v", err, ErrLimit)
	}
	if len(store.Bookmarks()) != MaxBookmarkEntries {
		t.Fatal("bookmark limit failure mutated collection")
	}
}
