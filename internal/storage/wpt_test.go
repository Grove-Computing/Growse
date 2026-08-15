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
