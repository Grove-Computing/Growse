package url

import (
	"errors"
	"strings"
	"testing"
)

func TestURLSearchParamsPreservesOrderDuplicatesAndEmptyValues(t *testing.T) {
	params, err := Parse("tag=go&empty=&tag=web+api&novalue&%E3%81%82=%2B")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if got, want := params.GetAll("tag"), []string{"go", "web api"}; !equalStrings(got, want) {
		t.Fatalf("GetAll(tag) = %v, want %v", got, want)
	}
	if got, ok := params.Get("empty"); !ok || got != "" {
		t.Fatalf("Get(empty) = %q, %t", got, ok)
	}
	if got, ok := params.Get("novalue"); !ok || got != "" {
		t.Fatalf("Get(novalue) = %q, %t", got, ok)
	}
	if got, want := params.Entries(), []Entry{{"tag", "go"}, {"empty", ""}, {"tag", "web api"}, {"novalue", ""}, {"あ", "+"}}; !equalEntries(got, want) {
		t.Fatalf("Entries() = %#v, want %#v", got, want)
	}
	if got, err := params.Encode(); err != nil || got != "tag=go&empty=&tag=web+api&novalue=&%E3%81%82=%2B" {
		t.Fatalf("Encode() = %q, %v", got, err)
	}
}

func TestURLSearchParamsMutationsAreAtomic(t *testing.T) {
	params := New()
	if err := params.Append("tag", "one"); err != nil {
		t.Fatal(err)
	}
	if err := params.Append("mode", "old"); err != nil {
		t.Fatal(err)
	}
	if err := params.Append("tag", "two"); err != nil {
		t.Fatal(err)
	}
	if err := params.Set("tag", "new"); err != nil {
		t.Fatal(err)
	}
	if got, want := params.Entries(), []Entry{{"tag", "new"}, {"mode", "old"}}; !equalEntries(got, want) {
		t.Fatalf("Set entries = %#v, want %#v", got, want)
	}
	params.Delete("tag")
	if params.Has("tag") || !params.Has("mode") {
		t.Fatalf("Delete/Has produced %#v", params.Entries())
	}
	entries := params.Entries()
	entries[0].Value = "changed"
	if value, _ := params.Get("mode"); value != "old" {
		t.Fatalf("Entries leaked mutation: %q", value)
	}
	before := params.Entries()
	if err := params.Append("large", strings.Repeat("x", MaxEncodedSize)); !errors.Is(err, ErrQueryTooLarge) {
		t.Fatalf("Append large error = %v", err)
	}
	if got := params.Entries(); !equalEntries(got, before) {
		t.Fatalf("failed Append modified entries: %#v", got)
	}
}

func TestURLSearchParamsRejectsMalformedAndInvalidUTF8(t *testing.T) {
	for _, raw := range []string{"%", "%ZZ", "%FF", string([]byte{'a', 0xff})} {
		if _, err := Parse(raw); !errors.Is(err, ErrInvalidQuery) {
			t.Errorf("Parse(%q) error = %v, want ErrInvalidQuery", raw, err)
		}
	}
	if _, err := Parse(strings.Repeat("a", MaxEncodedSize+1)); !errors.Is(err, ErrQueryTooLarge) {
		t.Fatalf("large Parse error = %v", err)
	}
	params := New()
	if err := params.Append(string([]byte{0xff}), "value"); !errors.Is(err, ErrInvalidQuery) {
		t.Fatalf("Append invalid UTF-8 error = %v", err)
	}
}

func equalStrings(left, right []string) bool {
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

func equalEntries(left, right []Entry) bool {
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
