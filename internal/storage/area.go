// Package storage はOriginごとのWeb Storage data modelを提供する。
package storage

import "sync"

// Area は挿入順を保持する1つのStorage namespaceである。
type Area struct {
	mu      sync.RWMutex
	values  map[string]string
	ordered []string
}

// NewArea は空のStorage Areaを生成する。
func NewArea() *Area {
	return &Area{values: make(map[string]string)}
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
func (area *Area) Set(key, value string) {
	if area == nil {
		return
	}
	area.mu.Lock()
	defer area.mu.Unlock()
	if _, exists := area.values[key]; !exists {
		area.ordered = append(area.ordered, key)
	}
	area.values[key] = value
}

// Remove はkeyが存在する場合に削除する。
func (area *Area) Remove(key string) {
	if area == nil {
		return
	}
	area.mu.Lock()
	defer area.mu.Unlock()
	if _, exists := area.values[key]; !exists {
		return
	}
	delete(area.values, key)
	for index, current := range area.ordered {
		if current == key {
			area.ordered = append(area.ordered[:index], area.ordered[index+1:]...)
			break
		}
	}
}

// Clear はすべてのentryを削除する。
func (area *Area) Clear() {
	if area == nil {
		return
	}
	area.mu.Lock()
	area.values = make(map[string]string)
	area.ordered = nil
	area.mu.Unlock()
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
