package devtools

import (
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Grove-Computing/Growse/internal/network"
)

const (
	DefaultMaxNetworkRecords        = 500
	DefaultMaxSessionNetworkRecords = 4000
)

// SessionStore bounds diagnostics across all pages and tabs in one session.
type SessionStore struct {
	mu      sync.Mutex
	network int
	max     int
}

// NewSessionStore creates a production-bounded session diagnostics budget.
func NewSessionStore() *SessionStore {
	return &SessionStore{max: DefaultMaxSessionNetworkRecords}
}

func (session *SessionStore) reserveNetwork() bool {
	if session == nil {
		return true
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.network >= session.max {
		return false
	}
	session.network++
	return true
}

// NetworkRecord is immutable request metadata. It never contains bodies or headers.
type NetworkRecord struct {
	Sequence      uint64
	Method        string
	URL           string
	Kind          string
	Engine        string
	StartedAt     time.Time
	Duration      time.Duration
	StatusCode    int
	Redirected    bool
	CacheStatus   string
	ResponseBytes int
	ErrorCategory string
}

// ObserveNetwork stores one body-free, credential-redacted observation.
func (store *PageStore) ObserveNetwork(observation network.Observation) {
	if store == nil {
		return
	}
	record := NetworkRecord{
		Method: strings.ToUpper(observation.Method), URL: redactedNetworkURL(observation.URL), Kind: requestKindName(observation.Kind), Engine: observation.Engine,
		StartedAt: observation.StartedAt, Duration: observation.Duration, StatusCode: observation.StatusCode,
		Redirected: observation.Redirected, CacheStatus: observation.CacheStatus, ResponseBytes: observation.ResponseBytes,
		ErrorCategory: observation.ErrorCategory,
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.closed || store.maxNetwork <= 0 || len(store.network) >= store.maxNetwork || !store.session.reserveNetwork() {
		return
	}
	store.nextNetwork++
	record.Sequence = store.nextNetwork
	store.network = append(store.network, record)
}

// Network returns retained request metadata in start-completion order.
func (store *PageStore) Network() []NetworkRecord {
	if store == nil {
		return nil
	}
	store.mu.RLock()
	defer store.mu.RUnlock()
	return append([]NetworkRecord(nil), store.network...)
}

// ClearNetwork removes retained request rows without resetting sequence numbers.
func (store *PageStore) ClearNetwork() {
	if store == nil {
		return
	}
	store.mu.Lock()
	store.network = nil
	store.mu.Unlock()
}

func requestKindName(kind network.RequestKind) string {
	switch kind {
	case network.RequestNavigation:
		return "navigation"
	case network.RequestForm:
		return "form"
	case network.RequestFetch:
		return "fetch"
	case network.RequestStylesheet:
		return "stylesheet"
	case network.RequestImage:
		return "image"
	case network.RequestScript:
		return "script"
	case network.RequestModule:
		return "module"
	default:
		return "subresource"
	}
}

func redactedNetworkURL(target *url.URL) string {
	if target == nil {
		return "unknown"
	}
	copy := *target
	copy.User = nil
	query := copy.Query()
	keys := make([]string, 0, len(query))
	for key := range query {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	redacted := make(url.Values, len(keys))
	for _, key := range keys {
		redacted[key] = []string{"[REDACTED]"}
	}
	copy.RawQuery = redacted.Encode()
	copy.Fragment = ""
	return copy.String()
}
