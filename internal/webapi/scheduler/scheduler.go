// Package scheduler provides page-scoped timing APIs exposed to WebGo programs.
package scheduler

import (
	"container/heap"
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"
)

// TimerID identifies one timeout or interval within a page.
type TimerID uint64

// FrameID identifies one animation frame callback within a page.
type FrameID uint64

// Timestamp is elapsed monotonic time on the page animation timeline.
type Timestamp time.Duration

// WebGo programs can build time.Duration values without importing the Go
// standard library, which is intentionally not part of the page API surface.
const (
	Millisecond = time.Millisecond
	Second      = time.Second
	// MaxTimerDuration bounds retained callbacks and deadline arithmetic.
	MaxTimerDuration = 365 * 24 * time.Hour
	// MaxTimersPerPage bounds callbacks retained by timeouts and intervals.
	MaxTimersPerPage = 10000
	// MaxFrameCallbacksPerPage bounds callbacks retained until the next frame.
	MaxFrameCallbacksPerPage = 10000
	// MaxCallbacksPerTurn prevents one deadline from starving the Page queue.
	MaxCallbacksPerTurn = 1000
)

// Clock supplies monotonic time to the scheduler.
type Clock interface {
	Now() time.Time
}

type systemClock struct{}

func (systemClock) Now() time.Time {
	return time.Now()
}

type timerEntry struct {
	id       TimerID
	deadline time.Time
	interval time.Duration
	repeat   bool
	callback func()
	sequence uint64
	index    int
	nesting  int
	canceled bool
}

type timerQueue []*timerEntry

func (queue timerQueue) Len() int { return len(queue) }

func (queue timerQueue) Less(left, right int) bool {
	if queue[left].deadline.Equal(queue[right].deadline) {
		return queue[left].sequence < queue[right].sequence
	}
	return queue[left].deadline.Before(queue[right].deadline)
}

func (queue timerQueue) Swap(left, right int) {
	queue[left], queue[right] = queue[right], queue[left]
	queue[left].index = left
	queue[right].index = right
}

func (queue *timerQueue) Push(value any) {
	entry := value.(*timerEntry)
	entry.index = len(*queue)
	*queue = append(*queue, entry)
}

func (queue *timerQueue) Pop() any {
	previous := *queue
	last := len(previous) - 1
	entry := previous[last]
	previous[last] = nil
	entry.index = -1
	*queue = previous[:last]
	return entry
}

// API owns the active timers for one page.
type API struct {
	ctx     context.Context
	cancel  context.CancelFunc
	clock   Clock
	enqueue func(func()) bool

	mu       sync.Mutex
	timers   map[TimerID]*timerEntry
	queue    timerQueue
	nextID   TimerID
	sequence uint64
	lastNow  time.Time
	nesting  int
	closed   bool

	frameOrigin    time.Time
	frameCallbacks map[FrameID]func(Timestamp)
	frameOrder     []FrameID
	nextFrameID    FrameID
	requestFrame   func()
	frameScope     func(time.Time, func())

	wake chan struct{}
	done chan struct{}
	auto bool
}

// NewPage creates a scheduler that waits for deadlines and delivers callbacks
// through enqueue.
func NewPage(parent context.Context, enqueue func(func()) bool, requestFrame func()) *API {
	return newAPI(parent, systemClock{}, enqueue, requestFrame, true)
}

// NewPageWithClock creates a manually driven page Scheduler. Embedders and
// tests advance timers through RunDue, so no wall-clock goroutine is started.
func NewPageWithClock(parent context.Context, clock Clock, enqueue func(func()) bool, requestFrame func()) *API {
	return newAPI(parent, clock, enqueue, requestFrame, false)
}

func newAPI(parent context.Context, clock Clock, enqueue func(func()) bool, requestFrame func(), auto bool) *API {
	if parent == nil {
		parent = context.Background()
	}
	if clock == nil {
		clock = systemClock{}
	}
	ctx, cancel := context.WithCancel(parent)
	api := &API{
		ctx:            ctx,
		cancel:         cancel,
		clock:          clock,
		enqueue:        enqueue,
		timers:         make(map[TimerID]*timerEntry),
		frameOrigin:    clock.Now(),
		frameCallbacks: make(map[FrameID]func(Timestamp)),
		requestFrame:   requestFrame,
		wake:           make(chan struct{}, 1),
		done:           make(chan struct{}),
		auto:           auto,
	}
	heap.Init(&api.queue)
	if auto {
		go api.run()
	} else {
		close(api.done)
	}
	return api
}

// SetFrameScope configures the page-owned scope used while frame callbacks run.
func (api *API) SetFrameScope(scope func(time.Time, func())) {
	if api == nil {
		return
	}
	api.mu.Lock()
	api.frameScope = scope
	api.mu.Unlock()
}

// RequestAnimationFrame registers callback for the next rendered frame.
func (api *API) RequestAnimationFrame(callback func(Timestamp)) (FrameID, error) {
	if api == nil || callback == nil {
		return 0, errors.New("animation frame callback is required")
	}
	api.mu.Lock()
	if api.closed {
		api.mu.Unlock()
		return 0, errors.New("scheduler is closed")
	}
	if len(api.frameCallbacks) >= MaxFrameCallbacksPerPage {
		api.mu.Unlock()
		return 0, errors.New("animation frame callback limit exceeded")
	}
	api.nextFrameID++
	if api.nextFrameID == 0 {
		api.nextFrameID++
	}
	id := api.nextFrameID
	api.frameCallbacks[id] = callback
	api.frameOrder = append(api.frameOrder, id)
	requestFrame := api.requestFrame
	api.mu.Unlock()
	if requestFrame != nil {
		requestFrame()
	}
	return id, nil
}

// CancelAnimationFrame removes a callback that has not started.
func (api *API) CancelAnimationFrame(id FrameID) bool {
	if api == nil || id == 0 {
		return false
	}
	api.mu.Lock()
	_, exists := api.frameCallbacks[id]
	if exists {
		delete(api.frameCallbacks, id)
	}
	api.mu.Unlock()
	return exists
}

// RunAnimationFrame delivers the callbacks captured for one frame. Callers
// must invoke it from the page event queue.
func (api *API) RunAnimationFrame(current time.Time) bool {
	if api == nil {
		return false
	}
	api.mu.Lock()
	if api.closed || len(api.frameCallbacks) == 0 {
		api.mu.Unlock()
		return false
	}
	current = api.observeLocked(current)
	elapsed := current.Sub(api.frameOrigin)
	if elapsed < 0 {
		elapsed = 0
	}
	order := api.frameOrder
	callbacks := api.frameCallbacks
	frameScope := api.frameScope
	api.frameOrder = nil
	api.frameCallbacks = make(map[FrameID]func(Timestamp))
	api.mu.Unlock()

	deliver := func() {
		for _, id := range order {
			if callback := callbacks[id]; callback != nil {
				invokeSchedulerCallback("frame", func() { callback(Timestamp(elapsed)) })
			}
		}
	}
	if frameScope != nil {
		frameScope(current, deliver)
	} else {
		deliver()
	}
	return true
}

// HasAnimationFrameCallbacks reports whether another rendered frame is needed.
func (api *API) HasAnimationFrameCallbacks() bool {
	if api == nil {
		return false
	}
	api.mu.Lock()
	defer api.mu.Unlock()
	return !api.closed && len(api.frameCallbacks) != 0
}

// SetTimeout registers callback to run once after delay.
func (api *API) SetTimeout(delay time.Duration, callback func()) (TimerID, error) {
	return api.schedule(delay, 0, false, callback)
}

// SetInterval registers callback to run repeatedly after each interval.
func (api *API) SetInterval(interval time.Duration, callback func()) (TimerID, error) {
	if interval < 0 {
		interval = 0
	}
	return api.schedule(interval, interval, true, callback)
}

// ClearTimer cancels a timeout or interval before its next callback starts.
func (api *API) ClearTimer(id TimerID) bool {
	if api == nil || id == 0 {
		return false
	}
	api.mu.Lock()
	entry, exists := api.timers[id]
	if !exists || entry.canceled {
		api.mu.Unlock()
		return false
	}
	delete(api.timers, id)
	entry.canceled = true
	entry.callback = nil
	if entry.index >= 0 {
		heap.Remove(&api.queue, entry.index)
	}
	api.mu.Unlock()
	api.signal()
	return true
}

func (api *API) schedule(delay, interval time.Duration, repeat bool, callback func()) (TimerID, error) {
	if api == nil || callback == nil {
		return 0, errors.New("scheduler callback is required")
	}
	if delay < 0 {
		delay = 0
	}

	api.mu.Lock()
	if api.closed {
		api.mu.Unlock()
		return 0, errors.New("scheduler is closed")
	}
	if len(api.timers) >= MaxTimersPerPage {
		api.mu.Unlock()
		return 0, errors.New("scheduler timer limit exceeded")
	}
	nesting := api.nesting + 1
	delay, err := normalizeDelay(delay, nesting)
	if err != nil {
		api.mu.Unlock()
		return 0, err
	}
	if repeat {
		interval, err = normalizeDelay(interval, nesting)
		if err != nil {
			api.mu.Unlock()
			return 0, err
		}
	}
	api.nextID++
	if api.nextID == 0 {
		api.nextID++
	}
	api.sequence++
	entry := &timerEntry{
		id:       api.nextID,
		deadline: api.nowLocked().Add(delay),
		interval: interval,
		repeat:   repeat,
		callback: callback,
		sequence: api.sequence,
		index:    -1,
		nesting:  nesting,
	}
	api.timers[entry.id] = entry
	heap.Push(&api.queue, entry)
	api.mu.Unlock()
	api.signal()
	return entry.id, nil
}

func (api *API) run() {
	defer close(api.done)
	var timer *time.Timer
	for {
		api.mu.Lock()
		if api.closed {
			api.mu.Unlock()
			if timer != nil {
				timer.Stop()
			}
			return
		}
		var delay time.Duration
		hasDeadline := len(api.queue) != 0
		if hasDeadline {
			delay = api.queue[0].deadline.Sub(api.nowLocked())
			if delay < 0 {
				delay = 0
			}
		}
		api.mu.Unlock()

		if !hasDeadline {
			select {
			case <-api.ctx.Done():
				return
			case <-api.wake:
				continue
			}
		}
		if timer == nil {
			timer = time.NewTimer(delay)
		} else {
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(delay)
		}
		select {
		case <-api.ctx.Done():
			timer.Stop()
			return
		case <-api.wake:
			continue
		case <-timer.C:
			api.runDue(api.clock.Now())
		}
	}
}

func (api *API) runDue(current time.Time) {
	api.mu.Lock()
	current = api.observeLocked(current)
	api.mu.Unlock()
	for delivered := 0; delivered < MaxCallbacksPerTurn; delivered++ {
		api.mu.Lock()
		if api.closed || len(api.queue) == 0 || api.queue[0].deadline.After(current) {
			api.mu.Unlock()
			return
		}
		entry := heap.Pop(&api.queue).(*timerEntry)
		if entry.canceled {
			api.mu.Unlock()
			continue
		}
		api.mu.Unlock()

		accepted := api.enqueue != nil && api.enqueue(func() {
			api.execute(entry)
		})
		if !accepted {
			api.mu.Lock()
			delete(api.timers, entry.id)
			entry.canceled = true
			entry.callback = nil
			api.mu.Unlock()
		}
	}
}

// RunDue synchronously queues all timers whose deadline is at or before
// current. It is intended for a manually driven Scheduler.
func (api *API) RunDue(current time.Time) {
	if api == nil {
		return
	}
	api.runDue(current)
}

func (api *API) execute(entry *timerEntry) {
	api.mu.Lock()
	if api.closed || entry.canceled {
		api.mu.Unlock()
		return
	}
	callback := entry.callback
	previousNesting := api.nesting
	api.nesting = entry.nesting
	if !entry.repeat {
		delete(api.timers, entry.id)
		entry.canceled = true
		entry.callback = nil
	}
	api.mu.Unlock()

	if callback != nil {
		invokeSchedulerCallback("timer", callback)
	}

	api.mu.Lock()
	api.nesting = previousNesting
	if api.closed || entry.canceled || !entry.repeat {
		api.mu.Unlock()
		return
	}
	api.sequence++
	entry.sequence = api.sequence
	entry.nesting++
	interval, err := normalizeDelay(entry.interval, entry.nesting)
	if err != nil {
		delete(api.timers, entry.id)
		entry.canceled = true
		entry.callback = nil
		api.mu.Unlock()
		return
	}
	entry.deadline = api.nowLocked().Add(interval)
	heap.Push(&api.queue, entry)
	api.mu.Unlock()
	api.signal()
}

func invokeSchedulerCallback(callbackType string, callback func()) {
	defer func() {
		if recover() != nil {
			slog.Error("WebGo Scheduler callbackでpanicが発生しました", "component", "scheduler", "type", callbackType)
		}
	}()
	callback()
}

func normalizeDelay(delay time.Duration, nesting int) (time.Duration, error) {
	if delay < 0 {
		delay = 0
	}
	if delay > MaxTimerDuration {
		return 0, errors.New("scheduler duration exceeds the safety limit")
	}
	if nesting > 5 && delay < 4*time.Millisecond {
		delay = 4 * time.Millisecond
	}
	return delay, nil
}

func (api *API) nowLocked() time.Time {
	return api.observeLocked(api.clock.Now())
}

func (api *API) observeLocked(current time.Time) time.Time {
	if !api.lastNow.IsZero() && current.Before(api.lastNow) {
		return api.lastNow
	}
	api.lastNow = current
	return current
}

func (api *API) signal() {
	if api == nil || !api.auto {
		return
	}
	select {
	case api.wake <- struct{}{}:
	default:
	}
}

// Close cancels every timer and releases page callback references.
func (api *API) Close() {
	if api == nil {
		return
	}
	api.mu.Lock()
	if api.closed {
		api.mu.Unlock()
		return
	}
	api.closed = true
	for _, entry := range api.timers {
		entry.canceled = true
		entry.callback = nil
	}
	clear(api.timers)
	api.queue = nil
	clear(api.frameCallbacks)
	api.frameCallbacks = nil
	api.frameOrder = nil
	api.requestFrame = nil
	api.frameScope = nil
	api.mu.Unlock()
	api.cancel()
	api.signal()
	<-api.done
}
