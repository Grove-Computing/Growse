// Package scheduler provides page-scoped timing APIs exposed to WebGo programs.
package scheduler

import (
	"container/heap"
	"context"
	"errors"
	"sync"
	"time"
)

// TimerID identifies one timeout or interval within a page.
type TimerID uint64

// WebGo programs can build time.Duration values without importing the Go
// standard library, which is intentionally not part of the page API surface.
const (
	Millisecond = time.Millisecond
	Second      = time.Second
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
	closed   bool

	wake chan struct{}
	done chan struct{}
	auto bool
}

// NewPage creates a scheduler that waits for deadlines and delivers callbacks
// through enqueue.
func NewPage(parent context.Context, enqueue func(func()) bool) *API {
	return newAPI(parent, systemClock{}, enqueue, true)
}

func newAPI(parent context.Context, clock Clock, enqueue func(func()) bool, auto bool) *API {
	if parent == nil {
		parent = context.Background()
	}
	if clock == nil {
		clock = systemClock{}
	}
	ctx, cancel := context.WithCancel(parent)
	api := &API{
		ctx:     ctx,
		cancel:  cancel,
		clock:   clock,
		enqueue: enqueue,
		timers:  make(map[TimerID]*timerEntry),
		wake:    make(chan struct{}, 1),
		done:    make(chan struct{}),
		auto:    auto,
	}
	heap.Init(&api.queue)
	if auto {
		go api.run()
	} else {
		close(api.done)
	}
	return api
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
	api.nextID++
	if api.nextID == 0 {
		api.nextID++
	}
	api.sequence++
	entry := &timerEntry{
		id:       api.nextID,
		deadline: api.clock.Now().Add(delay),
		interval: interval,
		repeat:   repeat,
		callback: callback,
		sequence: api.sequence,
		index:    -1,
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
			delay = api.queue[0].deadline.Sub(api.clock.Now())
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
	for {
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

func (api *API) execute(entry *timerEntry) {
	api.mu.Lock()
	if api.closed || entry.canceled {
		api.mu.Unlock()
		return
	}
	callback := entry.callback
	if !entry.repeat {
		delete(api.timers, entry.id)
		entry.canceled = true
		entry.callback = nil
	}
	api.mu.Unlock()

	if callback != nil {
		callback()
	}

	api.mu.Lock()
	if api.closed || entry.canceled || !entry.repeat {
		api.mu.Unlock()
		return
	}
	api.sequence++
	entry.sequence = api.sequence
	entry.deadline = api.clock.Now().Add(entry.interval)
	heap.Push(&api.queue, entry)
	api.mu.Unlock()
	api.signal()
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
	api.mu.Unlock()
	api.cancel()
	api.signal()
	<-api.done
}
