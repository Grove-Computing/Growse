package storage

import (
	"errors"
	"net/url"
	"path/filepath"
	"sync"

	"github.com/Grove-Computing/Growse/internal/network"
)

var ErrInvalidOrigin = errors.New("invalid storage origin")

// Manager は1つのBrowser Windowに属するLocal / Session Storageを保持する。
type Manager struct {
	mu       sync.Mutex
	local    map[string]*Area
	session  map[string]*Area
	localDir string
}

// NewManager は永続化しない空のStorage Managerを生成する。
func NewManager() *Manager {
	return &Manager{local: make(map[string]*Area), session: make(map[string]*Area)}
}

// NewPersistentManager はdataRoot配下へLocal Storageを永続化するManagerを生成する。
func NewPersistentManager(dataRoot string) (*Manager, error) {
	if dataRoot == "" || !filepath.IsAbs(dataRoot) {
		return nil, errors.New("storage data root must be an absolute path")
	}
	manager := NewManager()
	manager.localDir = filepath.Join(dataRoot, "local-storage")
	if err := ensurePrivateDirectory(manager.localDir); err != nil {
		return nil, err
	}
	return manager, nil
}

// Areas はDocument Originに対応するLocal / Session Storageを返す。
func (manager *Manager) Areas(documentURL *url.URL) (local, session *Area, err error) {
	if manager == nil {
		return nil, nil, ErrInvalidOrigin
	}
	origin, originErr := network.OriginFromURL(documentURL)
	if originErr != nil {
		return nil, nil, ErrInvalidOrigin
	}
	key := origin.String()
	manager.mu.Lock()
	defer manager.mu.Unlock()
	local = manager.local[key]
	if local == nil {
		if len(manager.local) >= MaxStorageOrigins {
			return nil, nil, ErrQuotaExceeded
		}
		if manager.localDir == "" {
			local = NewArea()
		} else {
			local, err = loadPersistentArea(manager.localDir, key)
			if err != nil {
				return nil, nil, err
			}
		}
		manager.local[key] = local
	}
	session = manager.session[key]
	if session == nil {
		session = NewArea()
		manager.session[key] = session
	}
	return local, session, nil
}
