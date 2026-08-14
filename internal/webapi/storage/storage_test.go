package storage

import (
	"errors"
	"testing"

	storagecore "github.com/Grove-Computing/Growse/internal/storage"
)

func TestAPIDistinguishesLocalAndSessionStorage(t *testing.T) {
	api := New(storagecore.NewArea(), storagecore.NewArea())
	if err := api.Local().Set("mode", "local"); err != nil {
		t.Fatal(err)
	}
	if err := api.Session().Set("mode", "session"); err != nil {
		t.Fatal(err)
	}
	if got, found, err := api.Local().Get("mode"); err != nil || !found || got != "local" {
		t.Fatalf("Local.Get() = (%q, %v, %v)", got, found, err)
	}
	if got, found, err := api.Session().Get("mode"); err != nil || !found || got != "session" {
		t.Fatalf("Session.Get() = (%q, %v, %v)", got, found, err)
	}
	if err := api.Local().Remove("mode"); err != nil || api.Local().Length() != 0 {
		t.Fatalf("Local.Remove() error = %v length = %d", err, api.Local().Length())
	}
	if err := api.Session().Clear(); err != nil || api.Session().Length() != 0 {
		t.Fatalf("Session.Clear() error = %v length = %d", err, api.Session().Length())
	}
}

func TestUnavailableStorageReturnsError(t *testing.T) {
	storage := New(nil, nil).Local()
	if err := storage.Set("key", "value"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Set() error = %v", err)
	}
}
