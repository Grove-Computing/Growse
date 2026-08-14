// Package storage はWebGoへLocal StorageとSession Storageを公開する。
package storage

import (
	"errors"

	storagecore "github.com/Grove-Computing/Growse/internal/storage"
)

var ErrUnavailable = errors.New("storage is unavailable")

// Storage はWebGoから操作する1つのStorage Areaである。
type Storage struct {
	area *storagecore.Area
}

// API はPageのLocal / Session Storageを明示的に分離する。
type API struct {
	local   *Storage
	session *Storage
}

// New はPageに対応するWeb Storage APIを生成する。
func New(local, session *storagecore.Area) *API {
	return &API{local: &Storage{area: local}, session: &Storage{area: session}}
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
	value, ok := storage.area.Get(key)
	return value, ok, nil
}

func (storage *Storage) Set(key, value string) error {
	if storage == nil || storage.area == nil {
		return ErrUnavailable
	}
	return storage.area.Set(key, value)
}

func (storage *Storage) Remove(key string) error {
	if storage == nil || storage.area == nil {
		return ErrUnavailable
	}
	return storage.area.Remove(key)
}

func (storage *Storage) Clear() error {
	if storage == nil || storage.area == nil {
		return ErrUnavailable
	}
	return storage.area.Clear()
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
