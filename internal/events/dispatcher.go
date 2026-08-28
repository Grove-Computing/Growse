// Package events はGrowseのDOMイベント配信を管理する。
package events

import (
	"log/slog"
	"sync"

	"github.com/Grove-Computing/Growse/internal/dom"
)

type Type string

const (
	Click      Type = "click"
	Input      Type = "input"
	Change     Type = "change"
	Submit     Type = "submit"
	Reset      Type = "reset"
	Focus      Type = "focus"
	Blur       Type = "blur"
	MouseEnter Type = "mouseenter"
	MouseLeave Type = "mouseleave"
	Load       Type = "load"
	Error      Type = "error"
)

type Phase uint8

const (
	PhaseNone      Phase = 0
	PhaseCapturing Phase = 1
	PhaseAtTarget  Phase = 2
	PhaseBubbling  Phase = 3
)

type Event struct {
	Type          Type
	Target        dom.NodeID
	X             float32
	Y             float32
	Value         string
	RuntimeID     uint64
	BubblesValue  bool
	BubblesSet    bool
	CancelableSet bool
	defaultAction *defaultAction
	dispatch      *dispatchState
}

type defaultAction struct {
	mu        sync.Mutex
	prevented bool
}

type dispatchState struct {
	mu                 sync.Mutex
	currentTarget      dom.NodeID
	phase              Phase
	propagationStopped bool
	immediateStopped   bool
	passive            bool
}

func Cancelable(eventType Type, target dom.NodeID) Event {
	return Event{Type: eventType, Target: target, defaultAction: new(defaultAction)}
}

// New constructs an Event with explicit bubbles and cancelable flags.
func New(eventType Type, target dom.NodeID, bubbles, cancelable bool) Event {
	event := Event{Type: eventType, Target: target, BubblesValue: bubbles, BubblesSet: true, CancelableSet: true}
	if cancelable {
		event.defaultAction = new(defaultAction)
	}
	return event
}

func (event Event) PreventDefault() {
	if event.defaultAction == nil || event.PassiveListener() {
		return
	}
	event.defaultAction.mu.Lock()
	event.defaultAction.prevented = true
	event.defaultAction.mu.Unlock()
}

func (event Event) IsCancelable() bool { return event.defaultAction != nil }

func (event Event) DefaultPrevented() bool {
	if event.defaultAction == nil {
		return false
	}
	event.defaultAction.mu.Lock()
	defer event.defaultAction.mu.Unlock()
	return event.defaultAction.prevented
}

func (event Event) Bubbles() bool {
	if event.BubblesSet {
		return event.BubblesValue
	}
	switch event.Type {
	case Click, Input, Change, Submit, Reset:
		return true
	default:
		return false
	}
}

// StopImmediatePropagation stops remaining listeners on the current target and propagation.
func (event Event) StopImmediatePropagation() {
	if event.dispatch == nil {
		return
	}
	event.dispatch.mu.Lock()
	event.dispatch.immediateStopped = true
	event.dispatch.propagationStopped = true
	event.dispatch.mu.Unlock()
}

// ImmediatePropagationStopped reports whether the current dispatch was stopped immediately.
func (event Event) ImmediatePropagationStopped() bool {
	if event.dispatch == nil {
		return false
	}
	event.dispatch.mu.Lock()
	defer event.dispatch.mu.Unlock()
	return event.dispatch.immediateStopped
}

// PassiveListener reports whether the currently invoked listener is passive.
func (event Event) PassiveListener() bool {
	if event.dispatch == nil {
		return false
	}
	event.dispatch.mu.Lock()
	defer event.dispatch.mu.Unlock()
	return event.dispatch.passive
}

func (event Event) StopPropagation() {
	if event.dispatch == nil {
		return
	}
	event.dispatch.mu.Lock()
	event.dispatch.propagationStopped = true
	event.dispatch.mu.Unlock()
}

func (event Event) PropagationStopped() bool {
	if event.dispatch == nil {
		return false
	}
	event.dispatch.mu.Lock()
	defer event.dispatch.mu.Unlock()
	return event.dispatch.propagationStopped
}

func (event Event) CurrentTarget() dom.NodeID {
	if event.dispatch == nil {
		return 0
	}
	event.dispatch.mu.Lock()
	defer event.dispatch.mu.Unlock()
	return event.dispatch.currentTarget
}

func (event Event) EventPhase() Phase {
	if event.dispatch == nil {
		return PhaseNone
	}
	event.dispatch.mu.Lock()
	defer event.dispatch.mu.Unlock()
	return event.dispatch.phase
}

func (event *Event) setDispatch(currentTarget dom.NodeID, phase Phase) {
	if event.dispatch == nil {
		event.dispatch = new(dispatchState)
	}
	event.dispatch.mu.Lock()
	event.dispatch.currentTarget = currentTarget
	event.dispatch.phase = phase
	event.dispatch.mu.Unlock()
}

func (event Event) setPassive(passive bool) {
	if event.dispatch == nil {
		return
	}
	event.dispatch.mu.Lock()
	event.dispatch.passive = passive
	event.dispatch.mu.Unlock()
}

type Listener func(Event)
type ListenerID uint64

type listenerEntry struct {
	id       ListenerID
	listener Listener
	capture  bool
	once     bool
	passive  bool
}

type Dispatcher struct {
	mu        sync.RWMutex
	listeners map[dom.NodeID]map[Type][]listenerEntry
	nextID    ListenerID
}

func NewDispatcher() *Dispatcher {
	return &Dispatcher{listeners: make(map[dom.NodeID]map[Type][]listenerEntry)}
}

func (dispatcher *Dispatcher) AddEventListener(nodeID dom.NodeID, eventType Type, listener Listener) {
	dispatcher.AddEventListenerWithCapture(nodeID, eventType, false, listener)
}

func (dispatcher *Dispatcher) AddEventListenerWithCapture(nodeID dom.NodeID, eventType Type, capture bool, listener Listener) ListenerID {
	return dispatcher.AddEventListenerWithOptions(nodeID, eventType, capture, false, false, listener)
}

// AddEventListenerWithOptions registers propagation and lifecycle options.
func (dispatcher *Dispatcher) AddEventListenerWithOptions(nodeID dom.NodeID, eventType Type, capture, once, passive bool, listener Listener) ListenerID {
	if dispatcher == nil || listener == nil {
		return 0
	}
	dispatcher.mu.Lock()
	defer dispatcher.mu.Unlock()
	dispatcher.nextID++
	byType := dispatcher.listeners[nodeID]
	if byType == nil {
		byType = make(map[Type][]listenerEntry)
		dispatcher.listeners[nodeID] = byType
	}
	byType[eventType] = append(byType[eventType], listenerEntry{id: dispatcher.nextID, listener: listener, capture: capture, once: once, passive: passive})
	return dispatcher.nextID
}

func (dispatcher *Dispatcher) RemoveEventListener(nodeID dom.NodeID, eventType Type, id ListenerID) bool {
	if dispatcher == nil || id == 0 {
		return false
	}
	dispatcher.mu.Lock()
	defer dispatcher.mu.Unlock()
	entries := dispatcher.listeners[nodeID][eventType]
	for index, entry := range entries {
		if entry.id != id {
			continue
		}
		entries = append(entries[:index], entries[index+1:]...)
		if len(entries) == 0 {
			delete(dispatcher.listeners[nodeID], eventType)
		} else {
			dispatcher.listeners[nodeID][eventType] = entries
		}
		if len(dispatcher.listeners[nodeID]) == 0 {
			delete(dispatcher.listeners, nodeID)
		}
		return true
	}
	return false
}

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

func (dispatcher *Dispatcher) Dispatch(event Event) bool {
	if dispatcher == nil {
		return false
	}
	event.setDispatch(event.Target, PhaseAtTarget)
	handled := dispatcher.invokeNode(event, event.Target, true)
	handled = dispatcher.invokeNode(event, event.Target, false) || handled
	event.setDispatch(0, PhaseNone)
	return handled
}

func (dispatcher *Dispatcher) DispatchTree(document *dom.Document, event Event) bool {
	if dispatcher == nil || document == nil {
		return false
	}
	target, ok := document.NodeByID(event.Target)
	if !ok {
		return false
	}
	event.dispatch = new(dispatchState)
	path := make([]dom.NodeID, 0, 8)
	for current := target.Parent; current != nil; current = current.Parent {
		path = append(path, current.ID)
	}
	handled := false
	for index := len(path) - 1; index >= 0; index-- {
		event.setDispatch(path[index], PhaseCapturing)
		handled = dispatcher.invokeNode(event, path[index], true) || handled
		if event.PropagationStopped() {
			event.setDispatch(0, PhaseNone)
			return handled
		}
	}
	event.setDispatch(event.Target, PhaseAtTarget)
	handled = dispatcher.invokeNode(event, event.Target, true) || handled
	handled = dispatcher.invokeNode(event, event.Target, false) || handled
	if event.PropagationStopped() || !event.Bubbles() {
		event.setDispatch(0, PhaseNone)
		return handled
	}
	for _, nodeID := range path {
		event.setDispatch(nodeID, PhaseBubbling)
		handled = dispatcher.invokeNode(event, nodeID, false) || handled
		if event.PropagationStopped() {
			break
		}
	}
	event.setDispatch(0, PhaseNone)
	return handled
}

func (dispatcher *Dispatcher) invokeNode(event Event, nodeID dom.NodeID, capture bool) bool {
	dispatcher.mu.RLock()
	entries := append([]listenerEntry(nil), dispatcher.listeners[nodeID][event.Type]...)
	dispatcher.mu.RUnlock()
	handled := false
	for _, entry := range entries {
		if entry.capture != capture {
			continue
		}
		if event.ImmediatePropagationStopped() {
			break
		}
		handled = true
		if entry.once {
			dispatcher.RemoveEventListener(nodeID, event.Type, entry.id)
		}
		event.setPassive(entry.passive)
		invoke(entry.listener, event)
		event.setPassive(false)
	}
	return handled
}

func invoke(listener Listener, event Event) {
	defer func() {
		if recovered := recover(); recovered != nil {
			slog.Error("WebGoイベントハンドラーでpanicが発生しました", "component", "events", "type", event.Type, "target", event.Target, "panic", recovered)
		}
	}()
	listener(event)
}
