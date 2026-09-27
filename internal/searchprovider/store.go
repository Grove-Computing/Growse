package searchprovider

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const MaxSettingsBytes = 64 * 1024

var ErrStorage = errors.New("検索設定を保存できません")
var stores sync.Map

// Store serializes writers by profile. An OS directory lock also excludes
// other processes, and times out rather than blocking the browser forever.
type Store struct {
	path string
	mu   *sync.Mutex
}

func OpenStore(root string) (*Store, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, ErrStorage
	}
	if err := os.MkdirAll(absolute, 0700); err != nil {
		return nil, ErrStorage
	}
	path := filepath.Join(absolute, "search-providers.json")
	value, _ := stores.LoadOrStore(path, &sync.Mutex{})
	return &Store{path: path, mu: value.(*sync.Mutex)}, nil
}
func readSettings(path string) (Settings, bool) {
	f, err := os.Open(path) // #nosec G304 -- path is the fixed settings filename in the browser-owned OS profile, never page input.
	if err != nil {
		return Settings{}, false
	}
	defer f.Close()
	body, err := io.ReadAll(io.LimitReader(f, MaxSettingsBytes+1))
	if err != nil || len(body) > MaxSettingsBytes {
		return Settings{}, false
	}
	var data struct {
		Version  int      `json:"version"`
		Settings Settings `json:"settings"`
	}
	if json.Unmarshal(body, &data) != nil || data.Version != 1 || data.Settings.Validate() != nil {
		return Settings{}, false
	}
	return data.Settings, true
}
func (s *Store) Load() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	if settings, ok := readSettings(s.path); ok {
		return settings
	}
	if settings, ok := readSettings(s.path + ".bak"); ok {
		settings.RemoteSuggestions = false
		return settings
	}
	return Defaults()
}
func encodeSettings(settings Settings) ([]byte, error) {
	if settings.Validate() != nil {
		return nil, ErrInvalid
	}
	body, err := json.Marshal(struct {
		Version  int      `json:"version"`
		Settings Settings `json:"settings"`
	}{1, settings})
	if err != nil || len(body) > MaxSettingsBytes {
		return nil, ErrStorage
	}
	return body, nil
}
func atomicSettings(path string, body []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".search-settings-*.tmp")
	if err != nil {
		return ErrStorage
	}
	temporary := f.Name()
	defer os.Remove(temporary)
	if _, err = f.Write(body); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		return ErrStorage
	}
	if os.Rename(temporary, path) != nil {
		return ErrStorage
	}
	return nil
}
func (s *Store) Save(settings Settings) error {
	body, err := encodeSettings(settings)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	lock := s.path + ".lock"
	deadline := time.Now().Add(2 * time.Second)
	for {
		err := os.Mkdir(lock, 0700)
		if err == nil {
			break
		}
		if !errors.Is(err, os.ErrExist) || time.Now().After(deadline) {
			return ErrStorage
		}
		time.Sleep(10 * time.Millisecond)
	}
	defer os.Remove(lock)
	if previous, ok := readSettings(s.path); ok {
		backup, err := encodeSettings(previous)
		if err != nil || atomicSettings(s.path+".bak", backup) != nil {
			return ErrStorage
		}
	}
	if err := atomicSettings(s.path, body); err != nil {
		return err
	}
	// Rename is the commit point. Directory sync is best effort on platforms
	// that do not support fsync on a directory.
	if directory, err := os.Open(filepath.Dir(s.path)); err == nil { // #nosec G304 -- fixed browser-owned profile directory, opened only to sync its rename.
		_ = directory.Sync()
		_ = directory.Close()
	}
	return nil
}
