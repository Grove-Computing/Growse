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
	mu           sync.RWMutex
	mutationMu   sync.Mutex
	observerMu   sync.Mutex
	values       map[string]string
	ordered      []string
	commit       func([]Entry) error
	failure      error
	observers    map[uint64]areaObserver
	nextObserver uint64
	sequence     uint64
}

// MutationSource identifies the page that changed a Local Storage Area.
type MutationSource struct {
	ID  uint64
	URL string
}

// Change describes one successfully committed Storage mutation.
type Change struct {
	Key         string
	OldValue    string
	NewValue    string
	HasOldValue bool
	HasNewValue bool
	Cleared     bool
	SourceURL   string
	Sequence    uint64
}

type areaObserver struct {
	sourceID uint64
	notify   func(Change)
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

// NewFailedArea は初期化Errorを各操作から返す利用不能なAreaを生成する。
func NewFailedArea(err error) *Area {
	return &Area{values: make(map[string]string), failure: err}
}

// Error はArea初期化時のErrorを返す。
func (area *Area) Error() error {
	if area == nil {
		return ErrStorageIO
	}
	return area.failure
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
	return area.SetFrom(MutationSource{}, key, value)
}

// SetFrom adds or updates a key and notifies other pages after commit.
func (area *Area) SetFrom(source MutationSource, key, value string) error {
	if area == nil {
		return nil
	}
	if area.failure != nil {
		return area.failure
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
	area.mutationMu.Lock()
	defer area.mutationMu.Unlock()
	area.mu.Lock()
	if current, exists := area.values[key]; exists && current == value {
		area.mu.Unlock()
		return nil
	}
	oldValue, hadOldValue := area.values[key]
	previous := area.entriesLocked()
	projected := area.bytesLocked() + len(key) + len(value)
	if current, exists := area.values[key]; exists {
		projected -= len(key) + len(current)
	}
	if projected > MaxOriginStorageBytes {
		area.mu.Unlock()
		return ErrQuotaExceeded
	}
	if _, exists := area.values[key]; !exists {
		area.ordered = append(area.ordered, key)
	}
	area.values[key] = value
	if err := area.commitLocked(); err != nil {
		area.restoreLocked(previous)
		area.mu.Unlock()
		return err
	}
	area.sequence++
	change := Change{Key: key, OldValue: oldValue, NewValue: value, HasOldValue: hadOldValue, HasNewValue: true, SourceURL: source.URL, Sequence: area.sequence}
	area.mu.Unlock()
	area.notify(source.ID, change)
	return nil
}

// Remove はkeyが存在する場合に削除する。
func (area *Area) Remove(key string) error {
	return area.RemoveFrom(MutationSource{}, key)
}

// RemoveFrom deletes a key and notifies other pages after commit.
func (area *Area) RemoveFrom(source MutationSource, key string) error {
	if area == nil {
		return nil
	}
	if area.failure != nil {
		return area.failure
	}
	if err := ValidateKey(key); err != nil {
		return err
	}
	area.mutationMu.Lock()
	defer area.mutationMu.Unlock()
	area.mu.Lock()
	oldValue, exists := area.values[key]
	if !exists {
		area.mu.Unlock()
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
		area.mu.Unlock()
		return err
	}
	area.sequence++
	change := Change{Key: key, OldValue: oldValue, HasOldValue: true, SourceURL: source.URL, Sequence: area.sequence}
	area.mu.Unlock()
	area.notify(source.ID, change)
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
	return area.ClearFrom(MutationSource{})
}

// ClearFrom removes every key and notifies other pages after commit.
func (area *Area) ClearFrom(source MutationSource) error {
	if area == nil {
		return nil
	}
	if area.failure != nil {
		return area.failure
	}
	area.mutationMu.Lock()
	defer area.mutationMu.Unlock()
	area.mu.Lock()
	if len(area.ordered) == 0 {
		area.mu.Unlock()
		return nil
	}
	previous := area.entriesLocked()
	area.values = make(map[string]string)
	area.ordered = nil
	if err := area.commitLocked(); err != nil {
		area.restoreLocked(previous)
		area.mu.Unlock()
		return err
	}
	area.sequence++
	change := Change{Cleared: true, SourceURL: source.URL, Sequence: area.sequence}
	area.mu.Unlock()
	area.notify(source.ID, change)
	return nil
}

// Subscribe registers a page for changes made by other sources. The returned
// function is idempotent and stops future delivery.
func (area *Area) Subscribe(sourceID uint64, notify func(Change)) func() {
	if area == nil || notify == nil {
		return func() {}
	}
	area.observerMu.Lock()
	if area.observers == nil {
		area.observers = make(map[uint64]areaObserver)
	}
	area.nextObserver++
	token := area.nextObserver
	area.observers[token] = areaObserver{sourceID: sourceID, notify: notify}
	area.observerMu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			area.observerMu.Lock()
			delete(area.observers, token)
			area.observerMu.Unlock()
		})
	}
}

func (area *Area) notify(sourceID uint64, change Change) {
	area.observerMu.Lock()
	observers := make([]areaObserver, 0, len(area.observers))
	for _, observer := range area.observers {
		if observer.sourceID != sourceID {
			observers = append(observers, observer)
		}
	}
	area.observerMu.Unlock()
	for _, observer := range observers {
		observer.notify(change)
	}
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

func (area *Area) discard() {
	if area == nil {
		return
	}
	area.mutationMu.Lock()
	area.mu.Lock()
	area.values = make(map[string]string)
	area.ordered = nil
	area.mu.Unlock()
	area.mutationMu.Unlock()
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

// Entries returns a defensive insertion-ordered snapshot for persistence and
// isolated runtime synchronization.
func (area *Area) Entries() []Entry {
	if area == nil {
		return nil
	}
	area.mu.RLock()
	defer area.mu.RUnlock()
	return area.entriesLocked()
}
