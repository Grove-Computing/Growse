package homeconfig

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var stores sync.Map

type Store struct {
	path string
	mu   *sync.Mutex
}

func OpenStore(root string) (*Store, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, ErrInvalid
	}
	if err := os.MkdirAll(absolute, 0700); err != nil {
		return nil, ErrInvalid
	}
	path := filepath.Join(absolute, "home-settings.json")
	value, _ := stores.LoadOrStore(path, &sync.Mutex{})
	return &Store{path: path, mu: value.(*sync.Mutex)}, nil
}

func readSettings(path string) (Settings, bool) {
	file, err := os.Open(path) // #nosec G304 -- fixed profile filename, never page input.
	if err != nil {
		return Settings{}, false
	}
	defer file.Close()
	body, err := io.ReadAll(io.LimitReader(file, MaxSettingsBytes+1))
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

func (store *Store) Load() Settings {
	store.mu.Lock()
	defer store.mu.Unlock()
	if settings, ok := readSettings(store.path); ok {
		return settings
	}
	if settings, ok := readSettings(store.path + ".bak"); ok {
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
	}{Version: 1, Settings: settings})
	if err != nil || len(body) > MaxSettingsBytes {
		return nil, ErrInvalid
	}
	return body, nil
}

func atomicSettings(path string, body []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".home-settings-*.tmp")
	if err != nil {
		return ErrInvalid
	}
	temporary := file.Name()
	defer os.Remove(temporary)
	if _, err = file.Write(body); err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil || closeErr != nil {
		return ErrInvalid
	}
	if os.Rename(temporary, path) != nil {
		return ErrInvalid
	}
	return nil
}

func (store *Store) Save(settings Settings) error {
	body, err := encodeSettings(settings)
	if err != nil {
		return err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	lock, err := os.OpenFile(store.path+".lock", os.O_CREATE|os.O_RDWR, 0600) // #nosec G304 -- fixed profile lock filename.
	if err != nil {
		return ErrInvalid
	}
	defer lock.Close()
	deadline := time.Now().Add(2 * time.Second)
	for {
		locked, lockErr := tryWriterLock(lock)
		if lockErr != nil {
			return ErrInvalid
		}
		if locked {
			break
		}
		if time.Now().After(deadline) {
			return ErrInvalid
		}
		time.Sleep(10 * time.Millisecond)
	}
	defer releaseWriterLock(lock)
	if previous, ok := readSettings(store.path); ok {
		backup, encodeErr := encodeSettings(previous)
		if encodeErr != nil || atomicSettings(store.path+".bak", backup) != nil {
			return ErrInvalid
		}
	}
	if err := atomicSettings(store.path, body); err != nil {
		return err
	}
	if directory, err := os.Open(filepath.Dir(store.path)); err == nil { // #nosec G304 -- fixed profile directory.
		_ = directory.Sync()
		_ = directory.Close()
	}
	return nil
}
