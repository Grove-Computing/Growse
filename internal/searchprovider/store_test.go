package searchprovider

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestAtomicSettingsRecovery(t *testing.T) {
	root := t.TempDir()
	store, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	if store.Load().DefaultID != "duckduckgo" {
		t.Fatal("missing defaults")
	}
	s := Defaults()
	s.RemoteSuggestions = true
	if err := store.Save(s); err != nil {
		t.Fatal(err)
	}
	reopened, _ := OpenStore(root)
	if !reopened.Load().RemoteSuggestions {
		t.Fatal("not persisted")
	}
	s.RemoteSuggestions = false
	if err := store.Save(s); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.path, []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if store.Load().RemoteSuggestions {
		t.Fatal("corrupt settings re-enabled suggestions")
	}
	if err := os.WriteFile(store.path+".bak", []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if store.Load().RemoteSuggestions {
		t.Fatal("unsafe recovery default")
	}
}
func TestConcurrentSettingsWritersAndFailure(t *testing.T) {
	root := t.TempDir()
	a, _ := OpenStore(root)
	b, _ := OpenStore(root)
	var wg sync.WaitGroup
	for i := range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s := Defaults()
			s.RemoteSuggestions = i%2 == 0
			if err := []*Store{a, b}[i%2].Save(s); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if _, ok := readSettings(a.path); !ok {
		t.Fatal("corrupt concurrent write")
	}
	if err := os.Mkdir(filepath.Join(root, "blocked"), 0700); err != nil {
		t.Fatal(err)
	}
	a.path = filepath.Join(root, "blocked")
	if err := a.Save(Defaults()); err == nil {
		t.Fatal("rename failure accepted")
	}
	entries, _ := os.ReadDir(root)
	for _, entry := range entries {
		if len(entry.Name()) > 0 && entry.Name()[0] == '.' {
			t.Fatal("temporary leaked", entry.Name())
		}
	}
}
