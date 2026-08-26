package serviceworker

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Grove-Computing/Growse/internal/network"
)

const (
	persistentSchemaVersion = 1
	maxPersistentOriginFile = 512 << 20
)

var (
	ErrPersistenceIO      = errors.New("service worker persistence I/O failure")
	ErrPersistenceCorrupt = errors.New("corrupt service worker profile data")
)

type persistentOrigin struct {
	Version       int                      `json:"version"`
	Origin        string                   `json:"origin"`
	Registrations []persistentRegistration `json:"registrations,omitempty"`
	Caches        []persistentCache        `json:"caches,omitempty"`
}

type persistentRegistration struct {
	ID            uint64 `json:"id"`
	Scope         string `json:"scope"`
	ScriptURL     string `json:"scriptUrl"`
	ActiveURL     string `json:"activeUrl,omitempty"`
	WaitingURL    string `json:"waitingUrl,omitempty"`
	ActiveScript  []byte `json:"activeScript,omitempty"`
	WaitingScript []byte `json:"waitingScript,omitempty"`
	Claimed       bool   `json:"claimed,omitempty"`
	Generation    uint64 `json:"generation"`
}

type persistentCache struct {
	Name    string                 `json:"name"`
	Entries []persistentCacheEntry `json:"entries,omitempty"`
}

type persistentCacheEntry struct {
	RequestURL string             `json:"requestUrl"`
	Response   persistentResponse `json:"response"`
}

type persistentResponse struct {
	URL         string      `json:"url,omitempty"`
	StatusCode  int         `json:"statusCode"`
	Status      string      `json:"status,omitempty"`
	Header      http.Header `json:"header,omitempty"`
	ContentType string      `json:"contentType,omitempty"`
	Body        []byte      `json:"body,omitempty"`
	Redirected  bool        `json:"redirected,omitempty"`
	CacheStatus string      `json:"cacheStatus,omitempty"`
}

// NewPersistentManager restores Service Worker profile state below dataRoot.
// A corrupt file is quarantined without suppressing healthy Origins.
func NewPersistentManager(dataRoot string) (*Manager, error) {
	if dataRoot == "" || !filepath.IsAbs(dataRoot) {
		return nil, errors.New("service worker data root must be an absolute path")
	}
	directory := filepath.Join(dataRoot, "service-workers")
	if err := ensurePrivateServiceWorkerDirectory(directory); err != nil {
		return nil, err
	}
	manager := NewManager()
	manager.dataDir = directory
	files, err := os.ReadDir(directory)
	if err != nil {
		return nil, persistenceIOError("list profile")
	}
	sort.Slice(files, func(left, right int) bool { return files[left].Name() < files[right].Name() })
	for _, file := range files {
		if file.IsDir() || !isPersistentOriginFilename(file.Name()) {
			continue
		}
		data, loadErr := loadPersistentOrigin(filepath.Join(directory, file.Name()))
		if loadErr != nil {
			if errors.Is(loadErr, ErrPersistenceCorrupt) {
				quarantinePersistentOrigin(directory, file.Name())
				continue
			}
			return nil, loadErr
		}
		if persistentOriginFilename(data.Origin) != file.Name() || manager.registrationCount()+len(data.Registrations) > MaxRegistrations {
			quarantinePersistentOrigin(directory, file.Name())
			continue
		}
		if err := manager.restoreOrigin(data); err != nil {
			quarantinePersistentOrigin(directory, file.Name())
		}
	}
	return manager, nil
}

func (manager *Manager) registrationCount() int {
	manager.mu.RLock()
	count := len(manager.registrations)
	manager.mu.RUnlock()
	return count
}

func (manager *Manager) persistOrigin(origin string) error {
	if manager == nil || manager.dataDir == "" || origin == "" {
		return nil
	}
	manager.persistMu.Lock()
	defer manager.persistMu.Unlock()
	data := manager.snapshotPersistentOrigin(origin)
	target := filepath.Join(manager.dataDir, persistentOriginFilename(origin))
	if len(data.Registrations) == 0 && len(data.Caches) == 0 {
		if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
			return persistenceIOError("remove Origin")
		}
		return syncServiceWorkerDirectory(manager.dataDir)
	}
	content, err := json.Marshal(data)
	if err != nil || len(content) > maxPersistentOriginFile {
		return persistenceIOError("encode Origin")
	}
	temporary, err := os.CreateTemp(manager.dataDir, ".service-worker-*.tmp")
	if err != nil {
		return persistenceIOError("create transaction")
	}
	temporaryPath := temporary.Name()
	committed := false
	defer func() {
		_ = temporary.Close()
		if !committed {
			_ = os.Remove(temporaryPath)
		}
	}()
	if temporary.Chmod(0o600) != nil {
		return persistenceIOError("protect transaction")
	}
	if _, err := temporary.Write(content); err != nil || temporary.Sync() != nil || temporary.Close() != nil {
		return persistenceIOError("write transaction")
	}
	if err := os.Rename(temporaryPath, target); err != nil {
		return persistenceIOError("commit transaction")
	}
	committed = true
	return syncServiceWorkerDirectory(manager.dataDir)
}

func (manager *Manager) snapshotPersistentOrigin(origin string) persistentOrigin {
	data := persistentOrigin{Version: persistentSchemaVersion, Origin: origin}
	manager.mu.RLock()
	for _, item := range manager.registrations {
		if item.origin != origin {
			continue
		}
		data.Registrations = append(data.Registrations, persistentRegistration{
			ID: item.id, Scope: item.scope.String(), ScriptURL: item.scriptURL.String(),
			ActiveURL: urlString(item.activeURL), WaitingURL: urlString(item.waitingURL),
			ActiveScript: append([]byte(nil), item.activeScript...), WaitingScript: append([]byte(nil), item.waitingScript...),
			Claimed: item.claimed, Generation: item.generation,
		})
	}
	manager.mu.RUnlock()
	sort.Slice(data.Registrations, func(left, right int) bool { return data.Registrations[left].Scope < data.Registrations[right].Scope })
	caches := manager.Caches()
	caches.mu.RLock()
	partition := caches.origins[origin]
	if partition != nil {
		names := make([]string, 0, len(partition.buckets))
		for name := range partition.buckets {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			stored := persistentCache{Name: name}
			bucket := partition.buckets[name]
			keys := make([]string, 0, len(bucket.entries))
			for key := range bucket.entries {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				entry := bucket.entries[key]
				stored.Entries = append(stored.Entries, persistentCacheEntry{
					RequestURL: entry.request.URL.String(), Response: persistentResponseFromNetwork(entry.response),
				})
			}
			data.Caches = append(data.Caches, stored)
		}
	}
	caches.mu.RUnlock()
	return data
}

func (manager *Manager) restoreOrigin(data persistentOrigin) error {
	originURL, err := url.Parse(data.Origin)
	if err != nil || originString(originURL) != data.Origin || !IsSecureContext(originURL) {
		return ErrPersistenceCorrupt
	}
	registrations := make([]*registration, 0, len(data.Registrations))
	seenScopes := make(map[string]struct{}, len(data.Registrations))
	for _, stored := range data.Registrations {
		item, err := restoreRegistration(data.Origin, stored)
		if err != nil {
			return err
		}
		key := registrationKey(data.Origin, item.scope)
		if _, duplicate := seenScopes[key]; duplicate {
			return ErrPersistenceCorrupt
		}
		seenScopes[key] = struct{}{}
		registrations = append(registrations, item)
	}
	partition, err := restoreCaches(data.Origin, data.Caches)
	if err != nil {
		return err
	}
	manager.mu.Lock()
	for _, item := range registrations {
		manager.registrations[registrationKey(data.Origin, item.scope)] = item
		if item.id > manager.nextID {
			manager.nextID = item.id
		}
	}
	manager.mu.Unlock()
	if partition != nil {
		manager.caches.mu.Lock()
		manager.caches.origins[data.Origin] = partition
		manager.caches.mu.Unlock()
	}
	return nil
}

func restoreRegistration(origin string, stored persistentRegistration) (*registration, error) {
	scope, scopeErr := parsePersistentURL(stored.Scope, origin)
	scriptURL, scriptErr := parsePersistentURL(stored.ScriptURL, origin)
	activeURL, activeErr := parseOptionalPersistentURL(stored.ActiveURL, origin)
	waitingURL, waitingErr := parseOptionalPersistentURL(stored.WaitingURL, origin)
	if stored.ID == 0 || stored.Generation == 0 || scopeErr != nil || scriptErr != nil || activeErr != nil || waitingErr != nil || activeURL == nil ||
		len(stored.ActiveScript) > MaxWorkerScriptBytes || len(stored.WaitingScript) > MaxWorkerScriptBytes ||
		waitingURL == nil && len(stored.WaitingScript) != 0 {
		return nil, ErrPersistenceCorrupt
	}
	item := &registration{
		id: stored.ID, origin: origin, scope: scope, scriptURL: scriptURL,
		activeURL: activeURL, waitingURL: waitingURL,
		activeScript: append([]byte(nil), stored.ActiveScript...), waitingScript: append([]byte(nil), stored.WaitingScript...),
		active: activeURL != nil, waiting: waitingURL != nil, claimed: stored.Claimed, generation: stored.Generation,
	}
	item.activeDigest = sha256.Sum256(item.activeScript)
	item.waitingDigest = sha256.Sum256(item.waitingScript)
	return item, nil
}

func restoreCaches(origin string, values []persistentCache) (*originCaches, error) {
	if len(values) == 0 {
		return nil, nil
	}
	if len(values) > MaxCachesPerOrigin {
		return nil, ErrPersistenceCorrupt
	}
	partition := &originCaches{buckets: make(map[string]*cacheBucket)}
	for _, stored := range values {
		if len(stored.Name) > maxCacheNameBytes || partition.buckets[stored.Name] != nil {
			return nil, ErrPersistenceCorrupt
		}
		bucket := &cacheBucket{entries: make(map[string]cacheEntry)}
		for _, entry := range stored.Entries {
			requestURL, err := url.Parse(entry.RequestURL)
			request := &network.Request{Method: http.MethodGet, URL: requestURL, Credentials: network.CredentialsOmit}
			key, valid := cacheRequestKey(request)
			if err != nil || !valid || originString(requestURL) == "" || bucket.entries[key].request != nil {
				return nil, ErrPersistenceCorrupt
			}
			response, err := entry.Response.networkResponse()
			if err != nil || len(response.Body) > MaxCacheResponseBytes || hasCredentialHeader(response.Header) {
				return nil, ErrPersistenceCorrupt
			}
			partition.entries++
			partition.bytes += len(response.Body)
			if partition.entries > MaxCacheEntries || partition.bytes > MaxCacheOriginBytes {
				return nil, ErrPersistenceCorrupt
			}
			bucket.entries[key] = cacheEntry{request: cacheSafeRequest(request), response: cacheSafeResponse(response)}
		}
		partition.buckets[stored.Name] = bucket
	}
	return partition, nil
}

func persistentResponseFromNetwork(response *network.Response) persistentResponse {
	response = cacheSafeResponse(response)
	return persistentResponse{
		URL: urlString(response.URL), StatusCode: response.StatusCode, Status: response.Status,
		Header: response.Header.Clone(), ContentType: response.ContentType, Body: append([]byte(nil), response.Body...),
		Redirected: response.Redirected, CacheStatus: response.CacheStatus,
	}
}

func (stored persistentResponse) networkResponse() (*network.Response, error) {
	responseURL, err := parseOptionalURL(stored.URL)
	if err != nil || responseURL != nil && responseURL.User != nil || stored.StatusCode < 200 || stored.StatusCode > 599 {
		return nil, ErrPersistenceCorrupt
	}
	return &network.Response{
		URL: responseURL, StatusCode: stored.StatusCode, Status: stored.Status, Header: stored.Header.Clone(),
		ContentType: stored.ContentType, Body: append([]byte(nil), stored.Body...), Redirected: stored.Redirected, CacheStatus: stored.CacheStatus,
	}, nil
}

func loadPersistentOrigin(path string) (persistentOrigin, error) {
	file, err := os.Open(path) // #nosec G304 -- path is a private directory entry selected by a fixed filename grammar.
	if err != nil {
		return persistentOrigin{}, persistenceIOError("open Origin")
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, maxPersistentOriginFile+1))
	if err != nil {
		return persistentOrigin{}, persistenceIOError("read Origin")
	}
	if len(content) > maxPersistentOriginFile {
		return persistentOrigin{}, ErrPersistenceCorrupt
	}
	var data persistentOrigin
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&data); err != nil || data.Version != persistentSchemaVersion || data.Origin == "" {
		return persistentOrigin{}, ErrPersistenceCorrupt
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return persistentOrigin{}, ErrPersistenceCorrupt
	}
	return data, nil
}

func parsePersistentURL(raw, origin string) (*url.URL, error) {
	value, err := url.Parse(raw)
	if err != nil || value.User != nil || originString(value) != origin || !IsSecureContext(value) {
		return nil, ErrPersistenceCorrupt
	}
	return value, nil
}

func parseOptionalPersistentURL(raw, origin string) (*url.URL, error) {
	if raw == "" {
		return nil, nil
	}
	return parsePersistentURL(raw, origin)
}

func parseOptionalURL(raw string) (*url.URL, error) {
	if raw == "" {
		return nil, nil
	}
	return url.Parse(raw)
}

func hasCredentialHeader(header http.Header) bool {
	for _, name := range []string{"Authorization", "Proxy-Authorization", "Cookie", "Set-Cookie", "Set-Cookie2", "WWW-Authenticate", "Proxy-Authenticate"} {
		if header.Get(name) != "" {
			return true
		}
	}
	return false
}

func ensurePrivateServiceWorkerDirectory(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return persistenceIOError("create profile")
	}
	if err := os.Chmod(path, 0o700); err != nil { // #nosec G302 -- 0700 denies group and other access.
		return persistenceIOError("protect profile")
	}
	return nil
}

func quarantinePersistentOrigin(directory, name string) {
	quarantine := filepath.Join(directory, "quarantine")
	if ensurePrivateServiceWorkerDirectory(quarantine) != nil {
		return
	}
	source := filepath.Join(directory, name)
	target := filepath.Join(quarantine, name+".corrupt")
	for suffix := 1; ; suffix++ {
		if _, err := os.Stat(target); err != nil {
			break
		}
		target = filepath.Join(quarantine, fmt.Sprintf("%s.corrupt.%d", name, suffix))
	}
	_ = os.Rename(source, target)
}

func syncServiceWorkerDirectory(directory string) error {
	handle, err := os.Open(directory) // #nosec G304 -- directory is the configured private profile root.
	if err != nil {
		return nil
	}
	defer handle.Close()
	_ = handle.Sync()
	return nil
}

func persistenceIOError(operation string) error {
	return fmt.Errorf("%w: %s", ErrPersistenceIO, operation)
}

func persistentOriginFilename(origin string) string {
	digest := sha256.Sum256([]byte(origin))
	return hex.EncodeToString(digest[:]) + ".json"
}

func isPersistentOriginFilename(name string) bool {
	if len(name) != sha256.Size*2+len(".json") || !strings.HasSuffix(name, ".json") {
		return false
	}
	_, err := hex.DecodeString(strings.TrimSuffix(name, ".json"))
	return err == nil
}

func urlString(value *url.URL) string {
	if value == nil {
		return ""
	}
	return value.String()
}
