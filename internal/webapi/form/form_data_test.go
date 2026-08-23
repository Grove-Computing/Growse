package form

import (
	"errors"
	"strconv"
	"strings"
	"testing"
)

func TestFormDataMutatesAndEncodesOrderedFields(t *testing.T) {
	data := New()
	for _, entry := range []Entry{{"tag", "go"}, {"empty", ""}, {"tag", "web api"}} {
		if err := data.Append(entry.Name, entry.Value); err != nil {
			t.Fatal(err)
		}
	}
	if got, want := data.GetAll("tag"), []string{"go", "web api"}; !equalStrings(got, want) {
		t.Fatalf("GetAll = %v, want %v", got, want)
	}
	if err := data.Set("tag", "growse"); err != nil {
		t.Fatal(err)
	}
	if got, want := data.Entries(), []Entry{{"tag", "growse"}, {"empty", ""}}; !equalEntries(got, want) {
		t.Fatalf("Entries = %#v, want %#v", got, want)
	}
	if got, err := data.Encode(); err != nil || got != "tag=growse&empty=" {
		t.Fatalf("Encode = %q, %v", got, err)
	}
	data.Delete("tag")
	if data.Has("tag") || !data.Has("empty") {
		t.Fatalf("Delete/Has produced %#v", data.Entries())
	}
}

func TestFormDataRejectsInvalidAndOversizeAtomically(t *testing.T) {
	data := New()
	if err := data.Append("safe", "value"); err != nil {
		t.Fatal(err)
	}
	before := data.Entries()
	for _, entry := range []Entry{{"", "value"}, {"bad\r\nname", "value"}, {string([]byte{0xff}), "value"}, {"large", strings.Repeat("x", maxValueSize+1)}} {
		if err := data.Append(entry.Name, entry.Value); !errors.Is(err, ErrInvalidField) {
			t.Errorf("Append(%q) error = %v", entry.Name, err)
		}
	}
	if got := data.Entries(); !equalEntries(got, before) {
		t.Fatalf("failed mutation changed data: %#v", got)
	}
	for index := 0; index < maxFields-1; index++ {
		if err := data.Append("field"+strconv.Itoa(index), "x"); err != nil {
			t.Fatal(err)
		}
	}
	if err := data.Append("overflow", "x"); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("field limit error = %v", err)
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
