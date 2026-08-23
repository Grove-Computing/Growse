package fetch

import (
	"errors"
	"strings"
	"testing"
)

func TestHeadersPreserveOrderValuesAndClone(t *testing.T) {
	headers := NewHeaders()
	for _, entry := range []HeaderEntry{{"X-Mode", "one"}, {"Accept", "application/json"}, {"x-mode", "two"}} {
		if err := headers.Append(entry.Name, entry.Value); err != nil {
			t.Fatal(err)
		}
	}
	if got, want := headers.Values("X-MODE"), []string{"one", "two"}; !equalStrings(got, want) {
		t.Fatalf("Values = %v, want %v", got, want)
	}
	if err := headers.Set("x-mode", "updated"); err != nil {
		t.Fatal(err)
	}
	if got, want := headers.Entries(), []HeaderEntry{{"x-mode", "updated"}, {"Accept", "application/json"}}; !equalHeaderEntries(got, want) {
		t.Fatalf("Entries = %#v, want %#v", got, want)
	}
	clone := headers.Clone()
	clone.Delete("accept")
	if !headers.Has("accept") || clone.Has("accept") {
		t.Fatal("Clone or Delete changed the wrong collection")
	}
	entries := headers.Entries()
	entries[0].Value = "changed"
	if got, _ := headers.Get("x-mode"); got != "updated" {
		t.Fatalf("Entries leaked mutation: %q", got)
	}
}

func TestHeadersRejectInvalidForbiddenAndOversizeWithoutMutation(t *testing.T) {
	headers := NewHeaders()
	if err := headers.Append("X-Good", "safe"); err != nil {
		t.Fatal(err)
	}
	before := headers.Entries()
	for _, test := range []struct {
		name, value string
		want        error
	}{
		{"Bad Header", "value", ErrInvalidHeader}, {"X-Test", "bad\r\nvalue", ErrInvalidHeader}, {"Host", "other.test", ErrForbiddenHeader}, {"Sec-Fetch-Site", "same-origin", ErrForbiddenHeader}, {strings.Repeat("x", maxHeaderNameSize+1), "value", ErrInvalidHeader}, {"X-Large", strings.Repeat("x", maxHeaderValueSize+1), ErrInvalidHeader},
	} {
		if err := headers.Append(test.name, test.value); !errors.Is(err, test.want) {
			t.Errorf("Append(%q) error = %v, want %v", test.name, err, test.want)
		}
	}
	if got := headers.Entries(); !equalHeaderEntries(got, before) {
		t.Fatalf("failed mutations changed headers: %#v", got)
	}
	for index := 0; index < maxHeaderFields-1; index++ {
		if err := headers.Append("X-Field-"+strings.Repeat("x", 1), "value"); err != nil {
			t.Fatal(err)
		}
	}
	if err := headers.Append("X-Overflow", "value"); !errors.Is(err, ErrHeadersTooLarge) {
		t.Fatalf("overflow error = %v", err)
	}
}

func equalHeaderEntries(left, right []HeaderEntry) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
