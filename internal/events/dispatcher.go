// Package events はGrowseのDOMイベント配信を管理する。
package events

import (
	"log/slog"
	"sync"

	"github.com/saku0512/growse/internal/dom"
)

// Type はDOMイベントの種類を表す。
type Type string

const (
	// Click はポインターによるクリックイベントを表す。
	Click Type = "click"
	// Input はユーザー操作で入力値が変化したイベントを表す。
	Input Type = "input"
	// Change は入力の編集が確定したイベントを表す。
	Change Type = "change"
	// Submit はformの送信操作を表す。
	Submit Type = "submit"
	// MouseEnter はポインターが要素のhover経路へ入ったことを表す。
	MouseEnter Type = "mouseenter"
	// MouseLeave はポインターが要素のhover経路から外れたことを表す。
	MouseLeave Type = "mouseleave"
)

// Event はDOM要素へ配信するイベント情報を保持する。
type Event struct {
	Type   Type
	Target dom.NodeID
	X      float32
	Y      float32
	Value  string
}

// Listener はイベントを処理する関数である。
type Listener func(Event)

// Dispatcher はNodeIDとイベント種類ごとのリスナーを保持する。
type Dispatcher struct {
	mu        sync.RWMutex
	listeners map[dom.NodeID]map[Type][]Listener
}

// NewDispatcher は空のDispatcherを生成する。
func NewDispatcher() *Dispatcher {
	return &Dispatcher{listeners: make(map[dom.NodeID]map[Type][]Listener)}
}

// AddEventListener は対象ノードへイベントリスナーを追加する。
func (dispatcher *Dispatcher) AddEventListener(nodeID dom.NodeID, eventType Type, listener Listener) {
	if dispatcher == nil || listener == nil {
		return
	}
	dispatcher.mu.Lock()
	defer dispatcher.mu.Unlock()
	byType := dispatcher.listeners[nodeID]
	if byType == nil {
		byType = make(map[Type][]Listener)
		dispatcher.listeners[nodeID] = byType
	}
	byType[eventType] = append(byType[eventType], listener)
}

// RemoveEventListeners は指定したノードに登録された全リスナーを削除する。
func (dispatcher *Dispatcher) RemoveEventListeners(nodeIDs ...dom.NodeID) {
	if dispatcher == nil {
		return
	}
	dispatcher.mu.Lock()
	defer dispatcher.mu.Unlock()
	for _, nodeID := range nodeIDs {
		delete(dispatcher.listeners, nodeID)
	}
}

// Dispatch は対象ノードへ登録されたリスナーを登録順に呼び出す。
func (dispatcher *Dispatcher) Dispatch(event Event) bool {
	if dispatcher == nil {
		return false
	}
	dispatcher.mu.RLock()
	listeners := append([]Listener(nil), dispatcher.listeners[event.Target][event.Type]...)
	dispatcher.mu.RUnlock()
	for _, listener := range listeners {
		invoke(listener, event)
	}
	return len(listeners) > 0
}

func invoke(listener Listener, event Event) {
	defer func() {
		if recovered := recover(); recovered != nil {
			slog.Error("WebGoイベントハンドラーでpanicが発生しました",
				"component", "events", "type", event.Type, "target", event.Target, "panic", recovered)
		}
	}()
	listener(event)
}
