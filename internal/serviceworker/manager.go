// Package serviceworker owns origin-scoped Service Worker registrations.
package serviceworker

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/Grove-Computing/Growse/internal/network"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
)

const (
	MaxRegistrations     = 64
	MaxWorkerScriptBytes = 2 << 20
)

var (
	ErrInsecureContext = errors.New("service worker requires a secure context")
	ErrCrossOrigin     = errors.New("service worker script and scope must be same-origin")
	ErrScope           = errors.New("service worker scope exceeds the script directory")
	ErrQuota           = errors.New("service worker registration quota exceeded")
)

// FetchScript retrieves one Service Worker script through Browser-owned I/O.
type FetchScript func(context.Context, *network.Request) (*network.Response, error)

// NetworkFallback performs a request without re-entering Service Worker dispatch.
type NetworkFallback func(context.Context, *network.Request) (*network.Response, error)

type registration struct {
	id            uint64
	origin        string
	scope         *url.URL
	scriptURL     *url.URL
	activeScript  []byte
	activeDigest  [sha256.Size]byte
	waitingScript []byte
	waitingDigest [sha256.Size]byte
	activeURL     *url.URL
	waitingURL    *url.URL
	active        bool
	waiting       bool
	claimed       bool
	generation    uint64
}

// Manager stores registrations for one browser profile.
type Manager struct {
	mu            sync.RWMutex
	operationMu   sync.Mutex
	persistMu     sync.Mutex
	workerMu      sync.Mutex
	registrations map[string]*registration
	nextID        uint64
	caches        *CacheStorage
	dataDir       string
	workers       map[string]*serviceWorkerProcess
	idleTimeout   time.Duration
	taskTimeout   time.Duration
	workerStarts  uint64
}

// NewManager creates an empty in-memory registration store.
func NewManager() *Manager {
	return &Manager{
		registrations: make(map[string]*registration), caches: newCacheStorage(), workers: make(map[string]*serviceWorkerProcess),
		idleTimeout: defaultServiceWorkerIdleTimeout, taskTimeout: defaultServiceWorkerTaskTimeout,
	}
}

// Caches returns the origin-partitioned Cache Storage owned by this profile.
func (manager *Manager) Caches() *CacheStorage {
	if manager == nil {
		return nil
	}
	manager.mu.Lock()
	if manager.caches == nil {
		manager.caches = newCacheStorage()
	}
	result := manager.caches
	manager.mu.Unlock()
	return result
}

// IsSecureContext accepts HTTPS and HTTP loopback development origins.
func IsSecureContext(target *url.URL) bool {
	if target == nil {
		return false
	}
	if strings.EqualFold(target.Scheme, "https") {
		return true
	}
	host := strings.ToLower(strings.TrimSuffix(target.Hostname(), "."))
	return strings.EqualFold(target.Scheme, "http") && (host == "localhost" || host == "127.0.0.1" || host == "::1")
}

// Register fetches and atomically installs or updates one scope.
func (manager *Manager) Register(ctx context.Context, clientURL *url.URL, scriptValue, scopeValue string, fetch FetchScript) (runtimemodel.ServiceWorkerRegistration, error) {
	if manager == nil || !IsSecureContext(clientURL) {
		return runtimemodel.ServiceWorkerRegistration{}, ErrInsecureContext
	}
	if fetch == nil {
		return runtimemodel.ServiceWorkerRegistration{}, errors.New("service worker script loader is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	manager.operationMu.Lock()
	defer manager.operationMu.Unlock()
	scriptURL, scopeURL, origin, err := resolveRegistration(clientURL, scriptValue, scopeValue)
	if err != nil {
		return runtimemodel.ServiceWorkerRegistration{}, err
	}
	response, err := fetch(ctx, &network.Request{
		Method: http.MethodGet, URL: scriptURL, SiteURL: clientURL, Kind: network.RequestServiceWorker, Credentials: network.CredentialsInclude,
	})
	if err != nil {
		return runtimemodel.ServiceWorkerRegistration{}, fmt.Errorf("fetch service worker script: %w", err)
	}
	if err := validateScriptResponse(scriptURL, response); err != nil {
		return runtimemodel.ServiceWorkerRegistration{}, err
	}
	digest := sha256.Sum256(response.Body)
	key := registrationKey(origin, scopeURL)
	manager.mu.Lock()
	existing := manager.registrations[key]
	if existing != nil && (existing.activeDigest == digest || existing.waiting && existing.waitingDigest == digest) {
		value := snapshot(existing)
		manager.mu.Unlock()
		return value, nil
	}
	if existing == nil && len(manager.registrations) >= MaxRegistrations {
		manager.mu.Unlock()
		return runtimemodel.ServiceWorkerRegistration{}, ErrQuota
	}
	activateWithoutWaiting := existing == nil || !existing.active
	manager.mu.Unlock()
	candidate, lifecycle, err := manager.startCandidateWorker(ctx, key, origin, response.URL, response.Body, activateWithoutWaiting, NetworkFallback(fetch))
	if err != nil {
		return runtimemodel.ServiceWorkerRegistration{}, err
	}
	manager.mu.Lock()
	existing = manager.registrations[key]
	if existing == nil {
		manager.nextID++
		existing = &registration{id: manager.nextID, origin: origin, scope: scopeURL}
		manager.registrations[key] = existing
	}
	existing.scriptURL = cloneURL(response.URL)
	existing.generation++
	activateCandidate := !existing.active || lifecycle.skipWaiting
	if activateCandidate {
		existing.active = true
		existing.waiting = false
		existing.activeURL = cloneURL(response.URL)
		existing.activeScript = append(existing.activeScript[:0], response.Body...)
		existing.activeDigest = digest
		existing.waitingURL = nil
		existing.waitingScript = nil
		existing.waitingDigest = [sha256.Size]byte{}
		existing.claimed = lifecycle.claim
	} else {
		existing.waiting = true
		existing.waitingURL = cloneURL(response.URL)
		existing.waitingScript = append(existing.waitingScript[:0], response.Body...)
		existing.waitingDigest = digest
	}
	value := snapshot(existing)
	manager.mu.Unlock()
	if activateCandidate {
		manager.installServiceWorker(key, candidate)
	} else {
		candidate.stop()
	}
	if err := manager.persistOrigin(origin); err != nil {
		return runtimemodel.ServiceWorkerRegistration{}, err
	}
	return value, nil
}

// Update revalidates the currently configured script URL.
func (manager *Manager) Update(ctx context.Context, clientURL *url.URL, scopeValue string, fetch FetchScript) (runtimemodel.ServiceWorkerRegistration, error) {
	registration, ok := manager.registrationForScope(clientURL, scopeValue)
	if !ok {
		return runtimemodel.ServiceWorkerRegistration{}, errors.New("service worker registration was not found")
	}
	return manager.Register(ctx, clientURL, registration.scriptURL.String(), registration.scope.String(), fetch)
}

// Unregister removes one same-origin scope and makes its workers redundant.
func (manager *Manager) Unregister(clientURL *url.URL, scopeValue string) (bool, error) {
	if manager == nil {
		return false, nil
	}
	manager.operationMu.Lock()
	defer manager.operationMu.Unlock()
	registration, ok := manager.registrationForScope(clientURL, scopeValue)
	if !ok {
		return false, nil
	}
	manager.mu.Lock()
	delete(manager.registrations, registrationKey(registration.origin, registration.scope))
	manager.mu.Unlock()
	manager.evictServiceWorker(registrationKey(registration.origin, registration.scope), nil)
	if err := manager.persistOrigin(registration.origin); err != nil {
		return false, err
	}
	return true, nil
}

// GetRegistration returns the longest active scope matching clientValue.
func (manager *Manager) GetRegistration(clientURL *url.URL, clientValue string) (*runtimemodel.ServiceWorkerRegistration, error) {
	if manager == nil || !IsSecureContext(clientURL) {
		return nil, ErrInsecureContext
	}
	target := clientURL
	if strings.TrimSpace(clientValue) != "" {
		reference, err := url.Parse(clientValue)
		if err != nil {
			return nil, errors.New("invalid Service Worker client URL")
		}
		target = clientURL.ResolveReference(reference)
	}
	origin, err := network.OriginFromURL(target)
	if err != nil {
		return nil, ErrCrossOrigin
	}
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	var best *registration
	for _, item := range manager.registrations {
		if item.origin != origin.String() || !strings.HasPrefix(target.EscapedPath(), item.scope.EscapedPath()) {
			continue
		}
		if best == nil || len(item.scope.EscapedPath()) > len(best.scope.EscapedPath()) {
			best = item
		}
	}
	if best == nil {
		return nil, nil
	}
	value := snapshot(best)
	return &value, nil
}

// GetRegistrations returns same-origin registrations ordered by scope.
func (manager *Manager) GetRegistrations(clientURL *url.URL) ([]runtimemodel.ServiceWorkerRegistration, error) {
	if manager == nil || !IsSecureContext(clientURL) {
		return nil, ErrInsecureContext
	}
	origin, err := network.OriginFromURL(clientURL)
	if err != nil {
		return nil, ErrCrossOrigin
	}
	manager.mu.RLock()
	result := make([]runtimemodel.ServiceWorkerRegistration, 0)
	for _, item := range manager.registrations {
		if item.origin == origin.String() {
			result = append(result, snapshot(item))
		}
	}
	manager.mu.RUnlock()
	sortRegistrations(result)
	return result, nil
}

// Controller returns the active claimed worker for a client, if any.
func (manager *Manager) Controller(clientURL *url.URL) *runtimemodel.ServiceWorkerRegistration {
	value, _ := manager.GetRegistration(clientURL, "")
	if value == nil || value.Active != runtimemodel.ServiceWorkerActivated || !value.Claimed {
		return nil
	}
	return value
}

// DispatchFetch delivers a controlled request to the longest matching active
// worker. A worker that does not synchronously call respondWith uses fallback.
func (manager *Manager) DispatchFetch(ctx context.Context, request *network.Request, fallback NetworkFallback) (*network.Response, error) {
	if fallback == nil {
		return nil, errors.New("service worker network fallback is unavailable")
	}
	if request == nil || request.URL == nil || request.Kind == network.RequestServiceWorker || request.Kind == network.RequestServiceWorkerFetch {
		return fallback(ctx, request)
	}
	controlURL := request.SiteURL
	if request.Kind == network.RequestNavigation || request.Kind == network.RequestForm || controlURL == nil {
		controlURL = request.URL
	}
	worker, ok := manager.activeWorkerFor(controlURL)
	if !ok {
		return fallback(ctx, request)
	}
	process, processErr := manager.activeServiceWorker(worker, fallback)
	var response *network.Response
	var fetchErr error
	if processErr == nil {
		response, fetchErr = process.fetch(ctx, request, fallback)
		if errors.Is(fetchErr, ErrServiceWorkerProcess) {
			manager.crashServiceWorker(worker.key, process)
			if !errors.Is(fetchErr, context.DeadlineExceeded) && !errors.Is(fetchErr, context.Canceled) {
				if restarted, restartErr := manager.activeServiceWorker(worker, fallback); restartErr == nil {
					response, fetchErr = restarted.fetch(ctx, request, fallback)
					if errors.Is(fetchErr, ErrServiceWorkerProcess) {
						manager.crashServiceWorker(worker.key, restarted)
					}
				} else {
					fetchErr = errors.Join(fetchErr, restartErr)
				}
			}
		}
	} else {
		fetchErr = processErr
	}
	persistErr := manager.persistOrigin(worker.origin)
	if fetchErr != nil || persistErr != nil {
		return nil, errors.Join(fetchErr, persistErr)
	}
	return response, nil
}

type activeWorker struct {
	origin    string
	key       string
	scriptURL *url.URL
	source    []byte
}

func (manager *Manager) activeWorkerFor(clientURL *url.URL) (activeWorker, bool) {
	if manager == nil || !IsSecureContext(clientURL) {
		return activeWorker{}, false
	}
	origin, err := network.OriginFromURL(clientURL)
	if err != nil {
		return activeWorker{}, false
	}
	manager.mu.RLock()
	var best *registration
	for _, item := range manager.registrations {
		if !item.active || item.origin != origin.String() || !strings.HasPrefix(clientURL.EscapedPath(), item.scope.EscapedPath()) {
			continue
		}
		if best == nil || len(item.scope.EscapedPath()) > len(best.scope.EscapedPath()) {
			best = item
		}
	}
	if best == nil {
		manager.mu.RUnlock()
		return activeWorker{}, false
	}
	result := activeWorker{
		origin: best.origin, key: registrationKey(best.origin, best.scope),
		scriptURL: cloneURL(best.activeURL), source: append([]byte(nil), best.activeScript...),
	}
	manager.mu.RUnlock()
	return result, result.scriptURL != nil
}

func (manager *Manager) registrationForScope(clientURL *url.URL, scopeValue string) (*registration, bool) {
	if manager == nil || !IsSecureContext(clientURL) {
		return nil, false
	}
	scopeURL := clientURL.ResolveReference(&url.URL{Path: scopeValue})
	if parsed, err := url.Parse(scopeValue); err == nil && scopeValue != "" {
		scopeURL = clientURL.ResolveReference(parsed)
	}
	origin, err := network.OriginFromURL(scopeURL)
	if err != nil {
		return nil, false
	}
	manager.mu.RLock()
	item := manager.registrations[registrationKey(origin.String(), scopeURL)]
	if item != nil {
		copy := *item
		copy.scope = cloneURL(item.scope)
		copy.scriptURL = cloneURL(item.scriptURL)
		copy.activeURL = cloneURL(item.activeURL)
		copy.waitingURL = cloneURL(item.waitingURL)
		copy.activeScript = append([]byte(nil), item.activeScript...)
		copy.waitingScript = append([]byte(nil), item.waitingScript...)
		item = &copy
	}
	manager.mu.RUnlock()
	return item, item != nil
}

func resolveRegistration(clientURL *url.URL, scriptValue, scopeValue string) (*url.URL, *url.URL, string, error) {
	scriptReference, err := url.Parse(strings.TrimSpace(scriptValue))
	if err != nil || scriptValue == "" {
		return nil, nil, "", errors.New("invalid service worker script URL")
	}
	scriptURL := clientURL.ResolveReference(scriptReference)
	scriptOrigin, scriptErr := network.OriginFromURL(scriptURL)
	clientOrigin, clientErr := network.OriginFromURL(clientURL)
	if scriptErr != nil || clientErr != nil || scriptOrigin != clientOrigin || scriptURL.User != nil {
		return nil, nil, "", ErrCrossOrigin
	}
	directory := path.Dir(scriptURL.EscapedPath()) + "/"
	scopeURL := cloneURL(scriptURL)
	scopeURL.Path, scopeURL.RawPath, scopeURL.RawQuery, scopeURL.Fragment = directory, "", "", ""
	if strings.TrimSpace(scopeValue) != "" {
		scopeReference, parseErr := url.Parse(scopeValue)
		if parseErr != nil {
			return nil, nil, "", errors.New("invalid service worker scope URL")
		}
		scopeURL = clientURL.ResolveReference(scopeReference)
	}
	scopeOrigin, scopeErr := network.OriginFromURL(scopeURL)
	if scopeErr != nil || scopeOrigin != clientOrigin || scopeURL.User != nil {
		return nil, nil, "", ErrCrossOrigin
	}
	if !strings.HasPrefix(scopeURL.EscapedPath(), directory) {
		return nil, nil, "", ErrScope
	}
	return scriptURL, scopeURL, clientOrigin.String(), nil
}

func validateScriptResponse(requested *url.URL, response *network.Response) error {
	if response == nil || response.URL == nil || response.StatusCode < 200 || response.StatusCode > 299 || !network.SameOrigin(requested, response.URL) {
		return errors.New("invalid service worker script response")
	}
	mediaType, _, err := mime.ParseMediaType(response.ContentType)
	if err != nil || mediaType != "text/javascript" && mediaType != "application/javascript" && mediaType != "application/ecmascript" {
		return errors.New("service worker script MIME type is unsupported")
	}
	if len(response.Body) > MaxWorkerScriptBytes {
		return errors.New("service worker script exceeds 2 MiB")
	}
	return nil
}

func snapshot(item *registration) runtimemodel.ServiceWorkerRegistration {
	value := runtimemodel.ServiceWorkerRegistration{
		ID: item.id, Generation: item.generation, Scope: item.scope.String(), ScriptURL: item.scriptURL.String(), Claimed: item.claimed,
	}
	if item.active {
		value.Active = runtimemodel.ServiceWorkerActivated
		if item.activeURL != nil {
			value.ActiveScriptURL = item.activeURL.String()
		}
	}
	if item.waiting {
		value.Waiting = runtimemodel.ServiceWorkerInstalled
		if item.waitingURL != nil {
			value.WaitingScriptURL = item.waitingURL.String()
		}
	}
	return value
}

func registrationKey(origin string, scope *url.URL) string { return origin + " " + scope.EscapedPath() }

func cloneURL(source *url.URL) *url.URL {
	if source == nil {
		return nil
	}
	copy := *source
	return &copy
}

func sortRegistrations(values []runtimemodel.ServiceWorkerRegistration) {
	for index := 1; index < len(values); index++ {
		for current := index; current > 0 && values[current].Scope < values[current-1].Scope; current-- {
			values[current], values[current-1] = values[current-1], values[current]
		}
	}
}
