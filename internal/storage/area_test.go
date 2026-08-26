package storage

import (
	"errors"
	"fmt"
	"strings"
	"sync"
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

func TestAreaEntriesReturnsDefensiveInsertionOrderedSnapshot(t *testing.T) {
	area := NewArea()
	_ = area.Set("first", "one")
	_ = area.Set("second", "two")
	entries := area.Entries()
	if len(entries) != 2 || entries[0] != (Entry{Key: "first", Value: "one"}) || entries[1].Key != "second" {
		t.Fatalf("Entries() = %#v", entries)
	}
	entries[0].Value = "changed"
	if value, _ := area.Get("first"); value != "one" {
		t.Fatalf("mutated snapshot changed Area to %q", value)
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

func TestAreaCommitFailureRollsBackWithoutEvent(t *testing.T) {
	commitErr := errors.New("disk unavailable")
	for _, test := range []struct {
		name   string
		mutate func(*Area) error
	}{
		{name: "set", mutate: func(area *Area) error { return area.SetFrom(MutationSource{ID: 1}, "saved", "changed") }},
		{name: "remove", mutate: func(area *Area) error { return area.RemoveFrom(MutationSource{ID: 1}, "saved") }},
		{name: "clear", mutate: func(area *Area) error { return area.ClearFrom(MutationSource{ID: 1}) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			area := newPersistentArea([]Entry{{Key: "saved", Value: "original"}}, func([]Entry) error { return commitErr })
			events := 0
			unsubscribe := area.Subscribe(2, func(Change) { events++ })
			defer unsubscribe()

			if err := test.mutate(area); !errors.Is(err, commitErr) {
				t.Fatalf("mutation error = %v", err)
			}
			if value, found := area.Get("saved"); !found || value != "original" {
				t.Fatalf("rolled back value = (%q, %v)", value, found)
			}
			if events != 0 {
				t.Fatalf("events after failed commit = %d", events)
			}
		})
	}
}

func TestConcurrentAreaMutationsPreserveCommitAndEventOrder(t *testing.T) {
	const mutations = 16
	var commitOrder []string
	area := newPersistentArea(nil, func(entries []Entry) error {
		commitOrder = append(commitOrder, entries[0].Value)
		return nil
	})
	var eventOrder []string
	var sequences []uint64
	unsubscribe := area.Subscribe(100, func(change Change) {
		eventOrder = append(eventOrder, change.NewValue)
		sequences = append(sequences, change.Sequence)
	})
	defer unsubscribe()

	var wait sync.WaitGroup
	errors := make(chan error, mutations)
	for index := 0; index < mutations; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			value := fmt.Sprintf("value-%02d", index)
			if err := area.SetFrom(MutationSource{ID: uint64(index + 1)}, "shared", value); err != nil {
				errors <- err
			}
		}(index)
	}
	wait.Wait()
	close(errors)
	for err := range errors {
		t.Fatal(err)
	}

	if len(commitOrder) != mutations || len(eventOrder) != mutations {
		t.Fatalf("commit/event count = %d/%d, want %d", len(commitOrder), len(eventOrder), mutations)
	}
	for index := range commitOrder {
		if eventOrder[index] != commitOrder[index] {
			t.Fatalf("event[%d] = %q, commit = %q", index, eventOrder[index], commitOrder[index])
		}
		if sequences[index] != uint64(index+1) {
			t.Fatalf("sequence[%d] = %d", index, sequences[index])
		}
	}
}
