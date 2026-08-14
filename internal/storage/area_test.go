package storage

import "testing"

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
