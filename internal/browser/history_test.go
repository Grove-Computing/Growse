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
