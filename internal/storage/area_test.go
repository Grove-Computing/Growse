package storage

import (
	"errors"
	"strings"
	"testing"
)

func TestAreaSupportsCRUDLengthAndKey(t *testing.T) {
	area := NewArea()
	area.Set("theme", "dark")
	area.Set("draft", "hello")
	if got, ok := area.Get("theme"); !ok || got != "dark" {
		t.Fatalf("Get(theme) = (%q, %v)", got, ok)
	}
	if area.Len() != 2 {
		t.Fatalf("Len() = %d, want 2", area.Len())
	}
	if got, ok := area.Key(1); !ok || got != "draft" {
		t.Fatalf("Key(1) = (%q, %v)", got, ok)
	}
	area.Remove("theme")
	if _, ok := area.Get("theme"); ok || area.Len() != 1 {
		t.Fatal("Remove() did not delete theme")
	}
	area.Clear()
	if area.Len() != 0 {
		t.Fatalf("Len() after Clear = %d", area.Len())
	}
}

func TestAreaDistinguishesEmptyMissingAndPreservesEnumerationOrder(t *testing.T) {
	area := NewArea()
	area.Set("first", "")
	area.Set("second", "two")
	if value, found := area.Get("first"); !found || value != "" {
		t.Fatalf("empty value = (%q, %v), want empty and found", value, found)
	}
	if value, found := area.Get("missing"); found || value != "" {
		t.Fatalf("missing value = (%q, %v), want empty and not found", value, found)
	}

	area.Set("first", "updated")
	if key, _ := area.Key(0); key != "first" {
		t.Fatalf("updated key moved to %q", key)
	}
	area.Remove("first")
	area.Set("first", "re-added")
	if first, _ := area.Key(0); first != "second" {
		t.Fatalf("first key = %q, want second", first)
	}
	if second, _ := area.Key(1); second != "first" {
		t.Fatalf("second key = %q, want re-added first", second)
	}
	if _, ok := area.Key(-1); ok {
		t.Fatal("Key(-1) unexpectedly exists")
	}
	if _, ok := area.Key(2); ok {
		t.Fatal("Key(2) unexpectedly exists")
	}
}

func TestAreaAppliesKeyValueAndOriginQuotasWithoutMutation(t *testing.T) {
	area := NewArea()
	invalidUTF8 := string([]byte{0xff})
	for _, test := range []struct{ key, value string }{
		{key: strings.Repeat("k", MaxKeyBytes+1), value: "value"},
		{key: invalidUTF8, value: "value"},
		{key: "key", value: strings.Repeat("v", MaxValueBytes+1)},
		{key: "key", value: invalidUTF8},
	} {
		if err := area.Set(test.key, test.value); err == nil {
			t.Fatal("Set() accepted invalid or oversized input")
		}
	}
	if area.Len() != 0 {
		t.Fatalf("rejected input changed area length = %d", area.Len())
	}

	chunk := strings.Repeat("v", MaxValueBytes)
	for index := 0; index < 4; index++ {
		if err := area.Set(string(rune('a'+index)), chunk); err != nil {
			t.Fatal(err)
		}
	}
	before := area.Len()
	if err := area.Set("overflow", chunk); !errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("Origin quota error = %v", err)
	}
	if area.Len() != before {
		t.Fatal("Origin quota failure mutated Area")
	}
}
