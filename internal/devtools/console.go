// Package devtools provides bounded, read-only diagnostics for one browser page.
package devtools

import (
	"strings"
	"sync"
	"unicode/utf8"
)

const (
	DefaultMaxConsoleRecords = 1000
	DefaultMaxMessageBytes   = 4 * 1024
)

// ConsoleLevel identifies the severity of a console record.
type ConsoleLevel string

const (
	ConsoleLog   ConsoleLevel = "log"
	ConsoleInfo  ConsoleLevel = "info"
	ConsoleWarn  ConsoleLevel = "warn"
	ConsoleError ConsoleLevel = "error"
)

// ConsoleRecord is an immutable snapshot of one page console message.
type ConsoleRecord struct {
	Sequence uint64
	Level    ConsoleLevel
	Source   string
	Message  string
}

// PageStore owns the diagnostics retained for a single loaded page.
type PageStore struct {
	mu          sync.RWMutex
	console     []ConsoleRecord
	next        uint64
	closed      bool
	maxRecords  int
	maxBytes    int
	network     []NetworkRecord
	nextNetwork uint64
	maxNetwork  int
	session     *SessionStore
}

// NewPageStore creates a store with production safety limits.
func NewPageStore() *PageStore {
	return NewPageStoreForSession(NewSessionStore())
}

// NewPageStoreForSession creates page diagnostics under a shared session budget.
func NewPageStoreForSession(session *SessionStore) *PageStore {
	store := newPageStore(DefaultMaxConsoleRecords, DefaultMaxMessageBytes)
	store.maxNetwork = DefaultMaxNetworkRecords
	store.session = session
	return store
}

func newPageStore(maxRecords, maxBytes int) *PageStore {
	return &PageStore{maxRecords: maxRecords, maxBytes: maxBytes}
}

// AddConsole appends a bounded console record. Calls after Close are ignored.
func (store *PageStore) AddConsole(level ConsoleLevel, source, message string) {
	if store == nil {
		return
	}
	if !validConsoleLevel(level) {
		level = ConsoleLog
	}
	message = truncateUTF8(message, store.maxBytes)
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.closed || store.maxRecords <= 0 {
		return
	}
	store.next++
	store.console = append(store.console, ConsoleRecord{Sequence: store.next, Level: level, Source: source, Message: message})
	if overflow := len(store.console) - store.maxRecords; overflow > 0 {
		copy(store.console, store.console[overflow:])
		store.console = store.console[:store.maxRecords]
	}
}

// Console returns a copy of all retained records in emission order.
func (store *PageStore) Console() []ConsoleRecord {
	if store == nil {
		return nil
	}
	store.mu.RLock()
	defer store.mu.RUnlock()
	return append([]ConsoleRecord(nil), store.console...)
}

// ClearConsole removes retained records without resetting sequence numbers.
func (store *PageStore) ClearConsole() {
	if store == nil {
		return
	}
	store.mu.Lock()
	store.console = nil
	store.mu.Unlock()
}

// Close discards records and prevents callbacks owned by a retired page from writing.
func (store *PageStore) Close() {
	if store == nil {
		return
	}
	store.mu.Lock()
	store.closed = true
	store.console = nil
	store.network = nil
	store.mu.Unlock()
}

func validConsoleLevel(level ConsoleLevel) bool {
	return level == ConsoleLog || level == ConsoleInfo || level == ConsoleWarn || level == ConsoleError
}

func truncateUTF8(value string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(value) <= maxBytes {
		return value
	}
	const suffix = "…"
	limit := maxBytes - len(suffix)
	if limit <= 0 {
		return strings.Repeat(".", maxBytes)
	}
	for limit > 0 && !utf8.ValidString(value[:limit]) {
		limit--
	}
	return value[:limit] + suffix
}
