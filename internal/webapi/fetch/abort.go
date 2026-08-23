package fetch

import "sync"

// AbortController owns one signal used to cancel Fetch requests.
type AbortController struct{ signal *AbortSignal }

// AbortSignal is a read-only cancellation state for a Fetch request.
type AbortSignal struct {
	mu        sync.Mutex
	aborted   bool
	listeners []func()
}

// NewAbortController creates a controller with an active signal.
func NewAbortController() *AbortController { return &AbortController{signal: &AbortSignal{}} }

// Signal returns this controller's signal.
func (controller *AbortController) Signal() *AbortSignal {
	if controller == nil {
		return nil
	}
	return controller.signal
}

// Abort cancels the signal. Repeated calls are harmless.
func (controller *AbortController) Abort() {
	if controller != nil && controller.signal != nil {
		controller.signal.abort()
	}
}

// Aborted reports whether the signal has been canceled.
func (signal *AbortSignal) Aborted() bool {
	if signal == nil {
		return false
	}
	signal.mu.Lock()
	defer signal.mu.Unlock()
	return signal.aborted
}

func (signal *AbortSignal) subscribe(listener func()) func() {
	if signal == nil {
		return func() {}
	}
	signal.mu.Lock()
	if signal.aborted {
		signal.mu.Unlock()
		listener()
		return func() {}
	}
	index := len(signal.listeners)
	signal.listeners = append(signal.listeners, listener)
	signal.mu.Unlock()
	return func() {
		signal.mu.Lock()
		if index < len(signal.listeners) {
			signal.listeners[index] = nil
		}
		signal.mu.Unlock()
	}
}

func (signal *AbortSignal) abort() {
	signal.mu.Lock()
	if signal.aborted {
		signal.mu.Unlock()
		return
	}
	signal.aborted = true
	listeners := append([]func(){}, signal.listeners...)
	signal.listeners = nil
	signal.mu.Unlock()
	for _, listener := range listeners {
		if listener != nil {
			listener()
		}
	}
}
