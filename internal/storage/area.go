// Package storage はOriginごとのWeb Storage data modelを提供する。
package storage

import (
	"errors"
	"sync"
	"unicode/utf8"
)

const (
	MaxKeyBytes            = 4 * 1024
	MaxValueBytes          = 1024 * 1024
	MaxOriginStorageBytes  = 5 * 1024 * 1024
	MaxProfileStorageBytes = 50 * 1024 * 1024
	MaxStorageOrigins      = 128
)

var (
	ErrQuotaExceeded = errors.New("storage quota exceeded")
	ErrInvalidString = errors.New("invalid storage string")
)

// Area は挿入順を保持する1つのStorage namespaceである。
type Area struct {
	mu      sync.RWMutex
	values  map[string]string
	ordered []string
	commit  func([]Entry) error
}

// Entry は永続化可能な挿入順key/valueである。
type Entry struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// NewArea は空のStorage Areaを生成する。
func NewArea() *Area {
	return &Area{values: make(map[string]string)}
}

func newPersistentArea(entries []Entry, commit func([]Entry) error) *Area {
	area := NewArea()
	area.commit = commit
	for _, entry := range entries {
		if _, exists := area.values[entry.Key]; !exists {
			area.ordered = append(area.ordered, entry.Key)
		}
		area.values[entry.Key] = entry.Value
	}
	return area
}

// Get はkeyのvalueと存在有無を返す。
func (area *Area) Get(key string) (string, bool) {
	if area == nil {
		return "", false
	}
	area.mu.RLock()
	defer area.mu.RUnlock()
	value, ok := area.values[key]
	return value, ok
}

// Set はkeyのvalueを追加または更新する。
func (area *Area) Set(key, value string) error {
	if area == nil {
		return nil
	}
	if err := ValidateKey(key); err != nil {
		return err
	}
	if !utf8.ValidString(value) {
		return ErrInvalidString
	}
	if len(value) > MaxValueBytes {
		return ErrQuotaExceeded
	}
	area.mu.Lock()
	defer area.mu.Unlock()
	if current, exists := area.values[key]; exists && current == value {
		return nil
	}
	previous := area.entriesLocked()
	projected := area.bytesLocked() + len(key) + len(value)
	if current, exists := area.values[key]; exists {
		projected -= len(key) + len(current)
	}
	if projected > MaxOriginStorageBytes {
		return ErrQuotaExceeded
	}
	if _, exists := area.values[key]; !exists {
		area.ordered = append(area.ordered, key)
	}
	area.values[key] = value
	if err := area.commitLocked(); err != nil {
		area.restoreLocked(previous)
		return err
	}
	return nil
}

// Remove はkeyが存在する場合に削除する。
func (area *Area) Remove(key string) error {
	if area == nil {
		return nil
	}
	if err := ValidateKey(key); err != nil {
		return err
	}
	area.mu.Lock()
	defer area.mu.Unlock()
	if _, exists := area.values[key]; !exists {
		return nil
	}
	previous := area.entriesLocked()
	delete(area.values, key)
	for index, current := range area.ordered {
		if current == key {
			area.ordered = append(area.ordered[:index], area.ordered[index+1:]...)
			break
		}
	}
	if err := area.commitLocked(); err != nil {
		area.restoreLocked(previous)
		return err
	}
	return nil
}

// ValidateKey はWeb Storage keyのUTF-8とsize上限を検証する。
func ValidateKey(key string) error {
	if !utf8.ValidString(key) {
		return ErrInvalidString
	}
	if len(key) > MaxKeyBytes {
		return ErrQuotaExceeded
	}
	return nil
}

// Clear はすべてのentryを削除する。
func (area *Area) Clear() error {
	if area == nil {
		return nil
	}
	area.mu.Lock()
	defer area.mu.Unlock()
	if len(area.ordered) == 0 {
		return nil
	}
	previous := area.entriesLocked()
	area.values = make(map[string]string)
	area.ordered = nil
	if err := area.commitLocked(); err != nil {
		area.restoreLocked(previous)
		return err
	}
	return nil
}

func (area *Area) entriesLocked() []Entry {
	entries := make([]Entry, 0, len(area.ordered))
	for _, key := range area.ordered {
		entries = append(entries, Entry{Key: key, Value: area.values[key]})
	}
	return entries
}

func (area *Area) bytesLocked() int {
	total := 0
	for key, value := range area.values {
		total += len(key) + len(value)
	}
	return total
}

func (area *Area) commitLocked() error {
	if area.commit == nil {
		return nil
	}
	return area.commit(area.entriesLocked())
}

func (area *Area) restoreLocked(entries []Entry) {
	area.values = make(map[string]string, len(entries))
	area.ordered = make([]string, 0, len(entries))
	for _, entry := range entries {
		area.values[entry.Key] = entry.Value
		area.ordered = append(area.ordered, entry.Key)
	}
}

// Len はentry数を返す。
func (area *Area) Len() int {
	if area == nil {
		return 0
	}
	area.mu.RLock()
	defer area.mu.RUnlock()
	return len(area.ordered)
}

// Key は挿入順indexのkeyを返す。
func (area *Area) Key(index int) (string, bool) {
	if area == nil || index < 0 {
		return "", false
	}
	area.mu.RLock()
	defer area.mu.RUnlock()
	if index >= len(area.ordered) {
		return "", false
	}
	return area.ordered[index], true
}
