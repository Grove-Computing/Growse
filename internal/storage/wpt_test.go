package storage

import "testing"

// WPT source: webstorage/storage_key.window.js.
func TestWPTStorageKeyOrderSurvivesValueReplacement(t *testing.T) {
	area := NewArea()
	for _, entry := range []Entry{{Key: "name", Value: "user1"}, {Key: "age", Value: "20"}, {Key: "a", Value: "1"}, {Key: "b", Value: "2"}} {
		if err := area.Set(entry.Key, entry.Value); err != nil {
			t.Fatal(err)
		}
	}
	if err := area.Set("name", "user2"); err != nil {
		t.Fatal(err)
	}
	for index, want := range []string{"name", "age", "a", "b"} {
		if got, ok := area.Key(index); !ok || got != want {
			t.Fatalf("Key(%d) = %q, %t; want %q", index, got, ok, want)
		}
	}
	for _, index := range []int{-1, 4} {
		if key, ok := area.Key(index); ok || key != "" {
			t.Fatalf("Key(%d) = %q, %t; want out of range", index, key, ok)
		}
	}
}

// WPT source: webstorage/event_basic.js.
func TestWPTLocalStorageEventCarriesCommittedOldAndNewValuesOnce(t *testing.T) {
	area := NewArea()
	var sourceEvents, targetEvents []Change
	stopSource := area.Subscribe(1, func(change Change) { sourceEvents = append(sourceEvents, change) })
	defer stopSource()
	stopTarget := area.Subscribe(2, func(change Change) { targetEvents = append(targetEvents, change) })
	defer stopTarget()
	source := MutationSource{ID: 1, URL: "https://example.test/source"}
	if err := area.SetFrom(source, "name", "first"); err != nil {
		t.Fatal(err)
	}
	if err := area.SetFrom(source, "name", "second"); err != nil {
		t.Fatal(err)
	}
	if len(sourceEvents) != 0 || len(targetEvents) != 2 {
		t.Fatalf("source/target Storage Events = %d/%d", len(sourceEvents), len(targetEvents))
	}
	first, second := targetEvents[0], targetEvents[1]
	if first.HasOldValue || first.NewValue != "first" || !second.HasOldValue || second.OldValue != "first" || second.NewValue != "second" {
		t.Fatalf("Storage Event payloads = %+v / %+v", first, second)
	}
	if first.Sequence != 1 || second.Sequence != 2 || second.SourceURL != source.URL {
		t.Fatalf("Storage Event order/source = %+v / %+v", first, second)
	}
}
