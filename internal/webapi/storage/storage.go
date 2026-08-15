// Package storage はWebGoへLocal StorageとSession Storageを公開する。
package storage

import (
	"errors"
	"sync"

	storagecore "github.com/Grove-Computing/Growse/internal/storage"
)

var ErrUnavailable = errors.New("storage is unavailable")

// Storage はWebGoから操作する1つのStorage Areaである。
type Storage struct {
	area   *storagecore.Area
	source storagecore.MutationSource
}

// Event is delivered when another same-origin page commits Local Storage.
type Event = storagecore.Change

// API はPageのLocal / Session Storageを明示的に分離する。
type API struct {
	mu          sync.RWMutex
	local       *Storage
	session     *Storage
	listeners   []func(Event)
	unsubscribe func()
	enqueue     func(func()) bool
}

// New はPageに対応するWeb Storage APIを生成する。
func New(local, session *storagecore.Area) *API {
	return &API{local: &Storage{area: local}, session: &Storage{area: session}}
}

// NewPage creates a page-scoped API and subscribes it to Local Storage events.
func NewPage(local, session *storagecore.Area, source storagecore.MutationSource, enqueue func(func()) bool) *API {
	api := &API{
		local: &Storage{area: local, source: source}, session: &Storage{area: session}, enqueue: enqueue,
	}
	if local != nil {
		api.unsubscribe = local.Subscribe(source.ID, api.dispatch)
	}
	return api
}

func (api *API) Local() *Storage {
	if api == nil {
		return &Storage{}
	}
	return api.local
}

func (api *API) Session() *Storage {
	if api == nil {
		return &Storage{}
	}
	return api.session
}

func (storage *Storage) Get(key string) (string, bool, error) {
	if storage == nil || storage.area == nil {
		return "", false, ErrUnavailable
	}
	if err := storage.area.Error(); err != nil {
		return "", false, err
	}
	if err := storagecore.ValidateKey(key); err != nil {
		return "", false, err
	}
	value, ok := storage.area.Get(key)
	return value, ok, nil
}

func (storage *Storage) Set(key, value string) error {
	if storage == nil || storage.area == nil {
		return ErrUnavailable
	}
	return storage.area.SetFrom(storage.source, key, value)
}

func (storage *Storage) Remove(key string) error {
	if storage == nil || storage.area == nil {
		return ErrUnavailable
	}
	return storage.area.RemoveFrom(storage.source, key)
}

func (storage *Storage) Clear() error {
	if storage == nil || storage.area == nil {
		return ErrUnavailable
	}
	return storage.area.ClearFrom(storage.source)
}

// OnChange registers a listener for committed changes from another page.
func (api *API) OnChange(listener func(Event)) bool {
	if api == nil || listener == nil {
		return false
	}
	api.mu.Lock()
	api.listeners = append(api.listeners, listener)
	api.mu.Unlock()
	return true
}

func (api *API) dispatch(event Event) {
	api.mu.RLock()
	listeners := append([]func(Event){}, api.listeners...)
	enqueue := api.enqueue
	api.mu.RUnlock()
	for _, listener := range listeners {
		current := listener
		callback := func() { current(event) }
		if enqueue == nil || !enqueue(callback) {
			if enqueue == nil {
				callback()
			}
		}
	}
}

// Close stops Local Storage event delivery to this page.
func (api *API) Close() {
	if api == nil {
		return
	}
	api.mu.Lock()
	unsubscribe := api.unsubscribe
	api.unsubscribe = nil
	api.listeners = nil
	api.mu.Unlock()
	if unsubscribe != nil {
		unsubscribe()
	}
}

func (storage *Storage) Length() int {
	if storage == nil || storage.area == nil {
		return 0
	}
	return storage.area.Len()
}

func (storage *Storage) Key(index int) (string, bool) {
	if storage == nil || storage.area == nil {
		return "", false
	}
	return storage.area.Key(index)
}
