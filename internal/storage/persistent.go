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
	"unicode/utf8"
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
		return storageIOError("create directory")
	}
	if err := os.Chmod(path, 0o700); err != nil { // #nosec G302 -- this is a directory and 0700 denies group and other access.
		return storageIOError("protect directory")
	}
	return nil
}

func loadPersistentArea(directory, origin string) (*Area, error) {
	path := persistentFilePath(directory, origin)
	file, err := os.Open(path) // #nosec G304 -- path is derived from the SHA-256 filename produced by persistentFilePath.
	if errors.Is(err, os.ErrNotExist) {
		return newPersistentArea(nil, func(entries []Entry) error {
			return savePersistentArea(directory, origin, entries)
		}), nil
	}
	if err != nil {
		return nil, storageIOError("open data")
	}
	defer file.Close()
	limited := io.LimitReader(file, maxStorageFileBytes+1)
	content, err := io.ReadAll(limited)
	if err != nil || len(content) > maxStorageFileBytes {
		return nil, storageIOError("read data")
	}
	var data persistentData
	if err := json.Unmarshal(content, &data); err != nil {
		return nil, ErrCorruptData
	}
	if data.Version != storageSchemaVersion {
		return nil, ErrSchemaMismatch
	}
	if data.Origin != origin || !validPersistentEntries(data.Entries) {
		return nil, ErrCorruptData
	}
	return newPersistentArea(data.Entries, func(entries []Entry) error {
		return savePersistentArea(directory, origin, entries)
	}), nil
}

func savePersistentArea(directory, origin string, entries []Entry) error {
	content, err := json.Marshal(persistentData{Version: storageSchemaVersion, Origin: origin, Entries: entries})
	if err != nil {
		return storageIOError("encode data")
	}
	targetPath := persistentFilePath(directory, origin)
	if exceedsProfileQuota(directory, targetPath, int64(len(content))) {
		return ErrQuotaExceeded
	}
	temporary, err := os.CreateTemp(directory, ".storage-*.tmp")
	if err != nil {
		return storageIOError("create transaction")
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
		return storageIOError("protect transaction")
	}
	if _, err := temporary.Write(content); err != nil {
		return storageIOError("write transaction")
	}
	if err := temporary.Sync(); err != nil {
		return storageIOError("sync transaction")
	}
	if err := temporary.Close(); err != nil {
		return storageIOError("close transaction")
	}
	if err := os.Rename(temporaryPath, targetPath); err != nil {
		return storageIOError("commit transaction")
	}
	committed = true
	if directoryHandle, err := os.Open(directory); err == nil { // #nosec G304 -- directory is the configured private storage root.
		_ = directoryHandle.Sync()
		_ = directoryHandle.Close()
	}
	return nil
}

func validPersistentEntries(entries []Entry) bool {
	seen := make(map[string]struct{}, len(entries))
	total := 0
	for _, entry := range entries {
		if ValidateKey(entry.Key) != nil || !utf8.ValidString(entry.Value) || len(entry.Value) > MaxValueBytes {
			return false
		}
		if _, duplicate := seen[entry.Key]; duplicate {
			return false
		}
		seen[entry.Key] = struct{}{}
		total += len(entry.Key) + len(entry.Value)
		if total > MaxOriginStorageBytes {
			return false
		}
	}
	return true
}

func storageIOError(operation string) error {
	return fmt.Errorf("%w: %s", ErrStorageIO, operation)
}

func exceedsProfileQuota(directory, targetPath string, replacementBytes int64) bool {
	total := replacementBytes
	files, err := os.ReadDir(directory)
	if err != nil {
		return true
	}
	for _, file := range files {
		path := filepath.Join(directory, file.Name())
		if path == targetPath {
			continue
		}
		info, err := file.Info()
		if err != nil {
			return true
		}
		if info.Mode().IsRegular() {
			total += info.Size()
			if total > MaxProfileStorageBytes {
				return true
			}
		}
	}
	return total > MaxProfileStorageBytes
}

func persistentFilePath(directory, origin string) string {
	digest := sha256.Sum256([]byte(origin))
	return filepath.Join(directory, hex.EncodeToString(digest[:])+".json")
}
