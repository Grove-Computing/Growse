// Package javascript implements the page-scoped Growse JavaScript Runtime.
package javascript

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	dommodel "github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/network"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
	domapi "github.com/Grove-Computing/Growse/internal/webapi/dom"
	"github.com/dop251/goja"
)

const (
	MaxEventListeners = 10_000
	maxCallStackSize  = 1_000
	callbackQueueSize = 64
)

var errRuntimeStopped = errors.New("javascript runtime is stopped")

type task struct {
	run    func(*goja.Runtime) error
	result chan error
}

// Runtime is one JavaScript VM and serialized callback queue owned by a Page.
type Runtime struct {
	mu sync.Mutex

	vm         *goja.Runtime
	cancel     context.CancelFunc
	runtimeCtx context.Context
	queue      chan task
	done       chan struct{}

	scripts     []runtimemodel.Script
	environment runtimemodel.Environment
	domAPI      *domapi.API

	elements      map[*goja.Object]*domapi.Element
	elementByID   map[uint64]*goja.Object
	listeners     []listenerRecord
	listenerCount int
	maxListeners  int

	loaded    bool
	started   bool
	stopped   bool
	executing atomic.Bool
}

type listenerRecord struct {
	elementID uint64
	eventType string
	function  goja.Value
}

// New returns an unloaded page-scoped JavaScript Runtime.
func New() *Runtime {
	return &Runtime{maxListeners: MaxEventListeners}
}

// Load prepares a VM and host objects without evaluating Page scripts.
func (runtime *Runtime) Load(ctx context.Context, scripts []runtimemodel.Script, environment runtimemodel.Environment) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(scripts) == 0 {
		return errors.New("no JavaScript scripts to load")
	}
	for _, script := range scripts {
		if runtimemodel.NormalizeEngine(script.Engine) != runtimemodel.EngineJavaScript {
			return fmt.Errorf("script uses engine %q; want %q", script.Engine, runtimemodel.EngineJavaScript)
		}
	}

	runtime.mu.Lock()
	if runtime.loaded || runtime.started {
		runtime.mu.Unlock()
		return errors.New("runtime already loaded")
	}
	if runtime.stopped {
		runtime.mu.Unlock()
		return errRuntimeStopped
	}
	runtimeContext, cancel := context.WithCancel(ctx)
	vm := goja.New()
	vm.SetMaxCallStackSize(maxCallStackSize)
	runtime.vm = vm
	runtime.cancel = cancel
	runtime.runtimeCtx = runtimeContext
	runtime.queue = make(chan task, callbackQueueSize)
	runtime.done = make(chan struct{})
	runtime.scripts = cloneScripts(scripts)
	runtime.environment = environment
	runtime.domAPI = domapi.New(environment.Document, environment.Events, environment.OnMutation)
	runtime.elements = make(map[*goja.Object]*domapi.Element)
	runtime.elementByID = make(map[uint64]*goja.Object)
	runtime.listeners = nil
	runtime.listenerCount = 0
	runtime.loaded = true
	queue, done := runtime.queue, runtime.done
	runtime.mu.Unlock()

	go runtime.run(runtimeContext, vm, queue, done)
	go runtime.interruptOnCancel(runtimeContext, vm)
	if err := runtime.runSync(ctx, func(vm *goja.Runtime) error {
		if err := runtime.installConsole(vm); err != nil {
			return err
		}
		return runtime.installDOM(vm)
	}); err != nil {
		_ = runtime.Stop()
		return fmt.Errorf("install JavaScript host API: %w", err)
	}
	return nil
}

// Start evaluates selected Page scripts in document order.
func (runtime *Runtime) Start(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	runtime.mu.Lock()
	if !runtime.loaded || runtime.vm == nil {
		runtime.mu.Unlock()
		return errors.New("runtime is not loaded")
	}
	if runtime.started {
		runtime.mu.Unlock()
		return errors.New("runtime already started")
	}
	if runtime.stopped {
		runtime.mu.Unlock()
		return errRuntimeStopped
	}
	runtime.started = true
	scripts := cloneScripts(runtime.scripts)
	runtime.mu.Unlock()

	err := runtime.runSync(ctx, func(vm *goja.Runtime) error {
		for index, script := range scripts {
			name := fmt.Sprintf("inline-script-%03d.js", index)
			if script.SourceURL != nil {
				name = network.RedactedURL(script.SourceURL)
			}
			if _, err := vm.RunScript(name, script.Source); err != nil {
				return fmt.Errorf("execute %s: %w", name, err)
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("execute JavaScript scripts: %w", err)
	}
	return nil
}

// Stop interrupts execution and releases all Page-owned host references.
func (runtime *Runtime) Stop() error {
	if runtime == nil {
		return nil
	}
	runtime.mu.Lock()
	if runtime.stopped {
		runtime.mu.Unlock()
		return nil
	}
	runtime.stopped = true
	cancel := runtime.cancel
	vm := runtime.vm
	done := runtime.done
	runtime.cancel = nil
	runtime.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if vm != nil {
		vm.Interrupt(context.Canceled)
	}
	if done != nil {
		<-done
	}
	runtime.mu.Lock()
	dispatcher := runtime.environment.Events
	listenerIDs := make([]dommodel.NodeID, 0, len(runtime.listeners))
	seenListenerIDs := make(map[dommodel.NodeID]struct{}, len(runtime.listeners))
	for _, listener := range runtime.listeners {
		id := dommodel.NodeID(listener.elementID)
		if _, seen := seenListenerIDs[id]; seen {
			continue
		}
		seenListenerIDs[id] = struct{}{}
		listenerIDs = append(listenerIDs, id)
	}
	runtime.vm = nil
	runtime.runtimeCtx = nil
	runtime.queue = nil
	runtime.done = nil
	runtime.scripts = nil
	runtime.environment = runtimemodel.Environment{}
	runtime.domAPI = nil
	runtime.elements = nil
	runtime.elementByID = nil
	runtime.listeners = nil
	runtime.listenerCount = 0
	runtime.loaded = false
	runtime.mu.Unlock()
	if dispatcher != nil && len(listenerIDs) != 0 {
		dispatcher.RemoveEventListeners(listenerIDs...)
	}
	return nil
}

// DispatchPageEvent serializes a browser Event with JavaScript callbacks.
func (runtime *Runtime) DispatchPageEvent(callback func() bool) bool {
	if callback == nil {
		return false
	}
	result := false
	if err := runtime.runSync(context.Background(), func(*goja.Runtime) error {
		result = callback()
		return nil
	}); err != nil {
		return false
	}
	return result
}

func (runtime *Runtime) run(ctx context.Context, vm *goja.Runtime, queue <-chan task, done chan<- struct{}) {
	defer close(done)
	for {
		select {
		case <-ctx.Done():
			return
		case queued := <-queue:
			runtime.executing.Store(true)
			err := executeTask(vm, queued.run)
			runtime.executing.Store(false)
			queued.result <- err
		}
	}
}

func executeTask(vm *goja.Runtime, run func(*goja.Runtime) error) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("JavaScript host panic: %v", recovered)
		}
	}()
	return run(vm)
}

func (runtime *Runtime) runSync(ctx context.Context, run func(*goja.Runtime) error) error {
	if ctx == nil {
		ctx = context.Background()
	}
	runtime.mu.Lock()
	runtimeContext := runtime.runtimeCtx
	queue := runtime.queue
	stopped := runtime.stopped
	runtime.mu.Unlock()
	if stopped || runtimeContext == nil || queue == nil {
		return errRuntimeStopped
	}
	queued := task{run: run, result: make(chan error, 1)}
	select {
	case queue <- queued:
	case <-ctx.Done():
		return ctx.Err()
	case <-runtimeContext.Done():
		return context.Cause(runtimeContext)
	}
	select {
	case err := <-queued.result:
		return err
	case <-ctx.Done():
		return ctx.Err()
	case <-runtimeContext.Done():
		return context.Cause(runtimeContext)
	}
}

func (runtime *Runtime) interruptOnCancel(ctx context.Context, vm *goja.Runtime) {
	<-ctx.Done()
	vm.Interrupt(context.Cause(ctx))
}

func (runtime *Runtime) recordError(message string) {
	runtime.mu.Lock()
	record := runtime.environment.ConsoleRecord
	fallback := runtime.environment.ConsoleLog
	stopped := runtime.stopped
	runtime.mu.Unlock()
	if stopped {
		return
	}
	if record != nil {
		record("error", message)
	} else if fallback != nil {
		fallback(message)
	}
}

func cloneScripts(scripts []runtimemodel.Script) []runtimemodel.Script {
	result := make([]runtimemodel.Script, len(scripts))
	copy(result, scripts)
	for index := range result {
		if scripts[index].SourceURL != nil {
			copyURL := *scripts[index].SourceURL
			result[index].SourceURL = &copyURL
		}
	}
	return result
}
