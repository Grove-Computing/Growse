package browser

import "testing"

func TestHistoryPushTruncatesForwardEntries(t *testing.T) {
	history := newHistory()
	history.push(mustParseURL(t, "https://example.com/one"))
	history.push(mustParseURL(t, "https://example.com/two"))
	history.push(mustParseURL(t, "https://example.com/three"))

	_, history.index, _ = history.target(-1)
	history.push(mustParseURL(t, "https://example.com/new"))

	if history.canForward() {
		t.Fatal("forward history was not truncated")
	}
	if got, want := len(history.entries), 3; got != want {
		t.Fatalf("entry count = %d, want %d", got, want)
	}
	if got, want := history.entries[2].String(), "https://example.com/new"; got != want {
		t.Fatalf("last entry = %q, want %q", got, want)
	}
}

func TestHistoryTargetDoesNotMoveIndex(t *testing.T) {
	history := newHistory()
	history.push(mustParseURL(t, "https://example.com/one"))
	history.push(mustParseURL(t, "https://example.com/two"))

	target, index, ok := history.target(-1)
	if !ok || target.String() != "https://example.com/one" || index != 0 {
		t.Fatalf("target(-1) = (%v, %d, %v)", target, index, ok)
	}
	if history.index != 1 {
		t.Fatalf("history index moved before navigation succeeded: %d", history.index)
	}
}

func TestHistoryRebindPageUpdatesRelatedEntries(t *testing.T) {
	history := newHistory()
	history.pushEntry(&historyEntry{URL: mustParseURL(t, "https://example.com/one"), PageID: 7})
	history.pushEntry(&historyEntry{URL: mustParseURL(t, "https://example.com/two"), PageID: 7, SameDocument: true})
	history.pushEntry(&historyEntry{URL: mustParseURL(t, "https://example.com/other"), PageID: 8})

	history.rebindPage(7, 9)
	if history.entries[0].PageID != 9 || history.entries[1].PageID != 9 || history.entries[2].PageID != 8 {
		t.Fatalf("page IDs = %d, %d, %d", history.entries[0].PageID, history.entries[1].PageID, history.entries[2].PageID)
	}
}
