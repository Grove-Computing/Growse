package storage

import (
	"net/url"
	"sync"

	"github.com/Grove-Computing/Growse/internal/network"
)

// Manager は1つのBrowser Windowに属するLocal / Session Storageを保持する。
type Manager struct {
	mu      sync.Mutex
	local   map[string]*Area
	session map[string]*Area
}

// NewManager は空のStorage Managerを生成する。
func NewManager() *Manager {
	return &Manager{local: make(map[string]*Area), session: make(map[string]*Area)}
}

// Areas はDocument Originに対応するLocal / Session Storageを返す。
func (manager *Manager) Areas(documentURL *url.URL) (local, session *Area, ok bool) {
	if manager == nil {
		return nil, nil, false
	}
	origin, err := network.OriginFromURL(documentURL)
	if err != nil {
		return nil, nil, false
	}
	key := origin.String()
	manager.mu.Lock()
	defer manager.mu.Unlock()
	local = manager.local[key]
	if local == nil {
		local = NewArea()
		manager.local[key] = local
	}
	session = manager.session[key]
	if session == nil {
		session = NewArea()
		manager.session[key] = session
	}
	return local, session, true
}
