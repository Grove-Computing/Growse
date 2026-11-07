package homeconfig

import (
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
)

func testSettings(t *testing.T, title, rawURL, background string) Settings {
	t.Helper()
	settings := Defaults()
	settings.Background = background
	if _, err := settings.Add(title, rawURL); err != nil {
		t.Fatal(err)
	}
	return settings
}

func TestStoreRestartsAndRecoversBackupOrDefaults(t *testing.T) {
	root := t.TempDir()
	store, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	first := testSettings(t, "First", "https://first.example/", BackgroundMist)
	second := testSettings(t, "Second", "https://second.example/", BackgroundDusk)
	if err := store.Save(first); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(second); err != nil {
		t.Fatal(err)
	}
	restarted, _ := OpenStore(root)
	if got := restarted.Load(); !reflect.DeepEqual(got, second) {
		t.Fatalf("restart = %#v, want %#v", got, second)
	}
	if err := os.WriteFile(filepath.Join(root, "home-settings.json"), []byte("{broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := restarted.Load(); !reflect.DeepEqual(got, first) {
		t.Fatalf("backup recovery = %#v, want %#v", got, first)
	}
	if err := os.WriteFile(filepath.Join(root, "home-settings.json.bak"), []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := restarted.Load(); !reflect.DeepEqual(got, Defaults()) {
		t.Fatalf("default recovery = %#v", got)
	}
}

func TestStoreWriteFailureLeavesPreviousSettings(t *testing.T) {
	root := t.TempDir()
	store, _ := OpenStore(root)
	first := testSettings(t, "First", "https://first.example/", BackgroundSolid)
	if err := store.Save(first); err != nil {
		t.Fatal(err)
	}
	store.path = root
	if err := store.Save(testSettings(t, "Second", "https://second.example/", BackgroundForest)); err == nil {
		t.Fatal("Save succeeded with directory as destination")
	}
	store.path = filepath.Join(root, "home-settings.json")
	if got := store.Load(); !reflect.DeepEqual(got, first) {
		t.Fatalf("failed write changed settings: %#v", got)
	}
}

func TestStoreSerializesMultipleWindowWriters(t *testing.T) {
	root := t.TempDir()
	left, _ := OpenStore(root)
	right, _ := OpenStore(root)
	values := []Settings{
		testSettings(t, "Left", "https://left.example/", BackgroundForest),
		testSettings(t, "Right", "https://right.example/", BackgroundSunrise),
	}
	var wait sync.WaitGroup
	for index, store := range []*Store{left, right} {
		wait.Add(1)
		go func(store *Store, settings Settings) {
			defer wait.Done()
			for count := 0; count < 20; count++ {
				if err := store.Save(settings); err != nil {
					t.Errorf("concurrent Save: %v", err)
					return
				}
			}
		}(store, values[index])
	}
	wait.Wait()
	got := left.Load()
	if !reflect.DeepEqual(got, values[0]) && !reflect.DeepEqual(got, values[1]) {
		t.Fatalf("concurrent result is invalid: %#v", got)
	}
}
