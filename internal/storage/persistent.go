package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const (
	storageSchemaVersion = 1
	maxStorageFileBytes  = 16 * 1024 * 1024
)

type persistentData struct {
	Version int     `json:"version"`
	Origin  string  `json:"origin"`
	Entries []Entry `json:"entries"`
}

// DefaultDataRoot はOS標準のUser Config Directory配下にGrowse data rootを返す。
func DefaultDataRoot() (string, error) {
	root, err := os.UserConfigDir()
	if err != nil {
		return "", errors.New("resolve Growse data directory")
	}
	if root == "" || !filepath.IsAbs(root) {
		return "", errors.New("resolve Growse data directory")
	}
	return filepath.Join(root, "growse"), nil
}

func ensurePrivateDirectory(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return fmt.Errorf("create storage directory: %w", err)
	}
	if err := os.Chmod(path, 0o700); err != nil {
		return fmt.Errorf("protect storage directory: %w", err)
	}
	return nil
}

func loadPersistentArea(directory, origin string) (*Area, error) {
	path := persistentFilePath(directory, origin)
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return newPersistentArea(nil, func(entries []Entry) error {
			return savePersistentArea(directory, origin, entries)
		}), nil
	}
	if err != nil {
		return nil, errors.New("open local storage")
	}
	defer file.Close()
	limited := io.LimitReader(file, maxStorageFileBytes+1)
	content, err := io.ReadAll(limited)
	if err != nil || len(content) > maxStorageFileBytes {
		return nil, errors.New("read local storage")
	}
	var data persistentData
	if err := json.Unmarshal(content, &data); err != nil || data.Version != storageSchemaVersion || data.Origin != origin {
		return nil, errors.New("invalid local storage data")
	}
	return newPersistentArea(data.Entries, func(entries []Entry) error {
		return savePersistentArea(directory, origin, entries)
	}), nil
}

func savePersistentArea(directory, origin string, entries []Entry) error {
	content, err := json.Marshal(persistentData{Version: storageSchemaVersion, Origin: origin, Entries: entries})
	if err != nil {
		return errors.New("encode local storage")
	}
	temporary, err := os.CreateTemp(directory, ".storage-*.tmp")
	if err != nil {
		return errors.New("create local storage transaction")
	}
	temporaryPath := temporary.Name()
	committed := false
	defer func() {
		_ = temporary.Close()
		if !committed {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return errors.New("protect local storage transaction")
	}
	if _, err := temporary.Write(content); err != nil {
		return errors.New("write local storage transaction")
	}
	if err := temporary.Sync(); err != nil {
		return errors.New("sync local storage transaction")
	}
	if err := temporary.Close(); err != nil {
		return errors.New("close local storage transaction")
	}
	if err := os.Rename(temporaryPath, persistentFilePath(directory, origin)); err != nil {
		return errors.New("commit local storage transaction")
	}
	committed = true
	if directoryHandle, err := os.Open(directory); err == nil {
		_ = directoryHandle.Sync()
		_ = directoryHandle.Close()
	}
	return nil
}

func persistentFilePath(directory, origin string) string {
	digest := sha256.Sum256([]byte(origin))
	return filepath.Join(directory, hex.EncodeToString(digest[:])+".json")
}
