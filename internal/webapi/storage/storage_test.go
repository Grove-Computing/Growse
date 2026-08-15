package storage

import (
	"errors"
	"testing"

	storagecore "github.com/Grove-Computing/Growse/internal/storage"
)

func TestLocalStorageCommitNotifiesOtherPage(t *testing.T) {
	local := storagecore.NewArea()
	source := NewPage(local, storagecore.NewArea(), storagecore.MutationSource{ID: 1, URL: "https://example.test/source"}, nil)
	defer source.Close()
	target := NewPage(local, storagecore.NewArea(), storagecore.MutationSource{ID: 2, URL: "https://example.test/target"}, nil)
	defer target.Close()

	var sourceEvents, targetEvents []Event
	source.OnChange(func(event Event) { sourceEvents = append(sourceEvents, event) })
	target.OnChange(func(event Event) { targetEvents = append(targetEvents, event) })
	if err := source.Local().Set("theme", "dark"); err != nil {
		t.Fatal(err)
	}

	if len(sourceEvents) != 0 {
		t.Fatalf("updating page received %d events", len(sourceEvents))
	}
	if len(targetEvents) != 1 {
		t.Fatalf("other page received %d events, want 1", len(targetEvents))
	}
	event := targetEvents[0]
	if event.Key != "theme" || event.HasOldValue || !event.HasNewValue || event.NewValue != "dark" || event.Cleared {
		t.Fatalf("event = %+v", event)
	}
	if event.SourceURL != "https://example.test/source" || event.Sequence != 1 {
		t.Fatalf("event source/order = (%q, %d)", event.SourceURL, event.Sequence)
	}
}

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

func TestGetDistinguishesEmptyValueFromMissingKey(t *testing.T) {
	storage := New(storagecore.NewArea(), storagecore.NewArea()).Local()
	if err := storage.Set("empty", ""); err != nil {
		t.Fatal(err)
	}
	if value, found, err := storage.Get("empty"); err != nil || !found || value != "" {
		t.Fatalf("Get(empty) = (%q, %v, %v)", value, found, err)
	}
	if value, found, err := storage.Get("missing"); err != nil || found || value != "" {
		t.Fatalf("Get(missing) = (%q, %v, %v)", value, found, err)
	}
}

func TestFailedAreaPreservesSpecificInitializationError(t *testing.T) {
	storage := New(storagecore.NewFailedArea(storagecore.ErrCorruptData), storagecore.NewArea()).Local()
	if _, _, err := storage.Get("key"); !errors.Is(err, storagecore.ErrCorruptData) {
		t.Fatalf("Get() error = %v", err)
	}
	if err := storage.Set("key", "value"); !errors.Is(err, storagecore.ErrCorruptData) {
		t.Fatalf("Set() error = %v", err)
	}
}
