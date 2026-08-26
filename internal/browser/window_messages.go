package browser

import (
	"errors"
	"net/url"
	"sync"

	"github.com/Grove-Computing/Growse/internal/network"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
)

const (
	maxWindowMessageBytes = 1 << 20
	maxPendingMessages    = 256
)

type windowKey struct {
	id         uint64
	generation uint64
}

type queuedWindowMessage struct {
	source       runtimemodel.WindowReference
	target       runtimemodel.WindowReference
	targetOrigin string
	payload      []byte
}

type windowRegistry struct {
	mu       sync.Mutex
	contexts map[windowKey]runtimemodel.WindowReference
	runtimes map[windowKey]runtimemodel.Runtime
	latest   map[uint64]uint64
	queue    []queuedWindowMessage
	draining bool
	closed   bool
}

func newWindowRegistry() *windowRegistry {
	return &windowRegistry{
		contexts: make(map[windowKey]runtimemodel.WindowReference),
		runtimes: make(map[windowKey]runtimemodel.Runtime),
		latest:   make(map[uint64]uint64),
	}
}

func (registry *windowRegistry) define(reference runtimemodel.WindowReference) {
	if registry == nil {
		return
	}
	key := windowKey{id: reference.ID, generation: reference.Generation}
	registry.mu.Lock()
	if !registry.closed {
		registry.contexts[key] = reference
		if reference.Generation > registry.latest[reference.ID] {
			registry.latest[reference.ID] = reference.Generation
		}
	}
	registry.mu.Unlock()
}

func (registry *windowRegistry) register(reference runtimemodel.WindowReference, pageRuntime runtimemodel.Runtime) {
	if registry == nil || pageRuntime == nil {
		return
	}
	key := windowKey{id: reference.ID, generation: reference.Generation}
	registry.mu.Lock()
	if registry.closed {
		registry.mu.Unlock()
		return
	}
	registry.contexts[key] = reference
	registry.runtimes[key] = pageRuntime
	if reference.Generation > registry.latest[reference.ID] {
		registry.latest[reference.ID] = reference.Generation
	}
	registry.startDrainLocked()
	registry.mu.Unlock()
}

func (registry *windowRegistry) unregister(reference runtimemodel.WindowReference) {
	if registry == nil {
		return
	}
	registry.mu.Lock()
	delete(registry.runtimes, windowKey{id: reference.ID, generation: reference.Generation})
	registry.mu.Unlock()
}

func (registry *windowRegistry) post(source, target runtimemodel.WindowReference, targetOrigin string, payload []byte) error {
	if registry == nil {
		return errors.New("window message broker is unavailable")
	}
	if len(payload) > maxWindowMessageBytes {
		return errors.New("window message exceeds 1 MiB")
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if registry.closed {
		return errors.New("window message broker is closed")
	}
	actualSource, sourceExists := registry.contexts[windowKey{id: source.ID, generation: source.Generation}]
	actualTarget, targetExists := registry.contexts[windowKey{id: target.ID, generation: target.Generation}]
	if !sourceExists || !targetExists || registry.latest[source.ID] > source.Generation || registry.latest[target.ID] > target.Generation {
		return errors.New("stale window message endpoint")
	}
	if !targetOriginMatches(targetOrigin, actualSource.Origin, actualTarget.Origin) {
		return errors.New("postMessage targetOrigin does not match target origin")
	}
	if len(registry.queue) >= maxPendingMessages {
		return errors.New("window message queue limit exceeded")
	}
	registry.queue = append(registry.queue, queuedWindowMessage{
		source: actualSource, target: actualTarget, targetOrigin: targetOrigin, payload: append([]byte(nil), payload...),
	})
	registry.startDrainLocked()
	return nil
}

func (registry *windowRegistry) startDrainLocked() {
	if registry.draining || registry.closed || len(registry.queue) == 0 {
		return
	}
	registry.draining = true
	go registry.drain()
}

func (registry *windowRegistry) drain() {
	for {
		registry.mu.Lock()
		if registry.closed || len(registry.queue) == 0 {
			registry.draining = false
			registry.mu.Unlock()
			return
		}
		message := registry.queue[0]
		targetKey := windowKey{id: message.target.ID, generation: message.target.Generation}
		actualTarget, contextExists := registry.contexts[targetKey]
		targetRuntime := registry.runtimes[targetKey]
		if !contextExists || registry.latest[message.target.ID] > message.target.Generation {
			registry.queue[0] = queuedWindowMessage{}
			registry.queue = registry.queue[1:]
			registry.mu.Unlock()
			continue
		}
		if targetRuntime == nil {
			registry.draining = false
			registry.mu.Unlock()
			return
		}
		registry.queue[0] = queuedWindowMessage{}
		registry.queue = registry.queue[1:]
		actualSource, sourceExists := registry.contexts[windowKey{id: message.source.ID, generation: message.source.Generation}]
		matches := sourceExists && targetOriginMatches(message.targetOrigin, actualSource.Origin, actualTarget.Origin)
		registry.mu.Unlock()
		if !matches {
			continue
		}
		dispatcher, ok := targetRuntime.(runtimemodel.MessageDispatcher)
		if !ok {
			continue
		}
		source := actualSource
		source.SameOrigin = originsMatch(actualSource.Origin, actualTarget.Origin)
		_ = dispatcher.DispatchMessage(runtimemodel.MessageEvent{
			Data: append([]byte(nil), message.payload...), Origin: actualSource.Origin, Source: source,
		})
	}
}

func (registry *windowRegistry) close() {
	if registry == nil {
		return
	}
	registry.mu.Lock()
	registry.closed = true
	registry.queue = nil
	registry.contexts = nil
	registry.runtimes = nil
	registry.mu.Unlock()
}

func targetOriginMatches(targetOrigin, sourceOrigin, targetOriginValue string) bool {
	switch targetOrigin {
	case "*":
		return true
	case "/", "":
		return sourceOrigin != "null" && sourceOrigin == targetOriginValue
	}
	if targetOriginValue == "null" {
		return false
	}
	parsed, err := url.Parse(targetOrigin)
	if err != nil || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Path != "" && parsed.Path != "/" {
		return false
	}
	origin, err := network.OriginFromURL(parsed)
	return err == nil && origin.String() == targetOriginValue
}

func originsMatch(left, right string) bool {
	return left != "" && left != "null" && left == right
}
