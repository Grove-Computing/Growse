// Package javascript implements the page-scoped Growse JavaScript Runtime.
package javascript

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	dommodel "github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/events"
	"github.com/Grove-Computing/Growse/internal/network"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
	domapi "github.com/Grove-Computing/Growse/internal/webapi/dom"
	fetchapi "github.com/Grove-Computing/Growse/internal/webapi/fetch"
	navigationapi "github.com/Grove-Computing/Growse/internal/webapi/navigation"
	schedulerapi "github.com/Grove-Computing/Growse/internal/webapi/scheduler"
	storageapi "github.com/Grove-Computing/Growse/internal/webapi/storage"
	"github.com/dop251/goja"
)

const (
	MaxEventListeners    = 10_000
	maxModuleBytes       = 2 << 20
	maxCallStackSize     = 1_000
	callbackQueueSize    = 64
	maxMicrotaskQueue    = 4_096
	defaultModuleTimeout = 5 * time.Second
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

	scripts           []runtimemodel.Script
	environment       runtimemodel.Environment
	domAPI            *domapi.API
	fetchAPI          *fetchapi.API
	fetchClock        fetchapi.Clock
	navigationAPI     *navigationapi.API
	schedulerAPI      *schedulerapi.API
	schedulerClock    schedulerapi.Clock
	storageAPI        *storageapi.API
	wasmAPI           *wasmAPI
	responseValues    map[*goja.Object]fetchapi.Response
	frameAccess       map[uint64]runtimemodel.FrameAccess
	frameByElement    map[uint64]uint64
	frameWindows      map[frameObjectKey]*goja.Object
	frameDocuments    map[frameObjectKey]*goja.Object
	windowProxies     map[frameObjectKey]*goja.Object
	structuredClone   goja.Callable
	abortSignals      map[*goja.Object]*fetchapi.AbortSignal
	windowListeners   []listenerRecord
	documentListeners []listenerRecord
	microtasks        []goja.Value
	maxMicrotasks     int
	moduleTimeout     time.Duration

	elements       map[*goja.Object]*domapi.Element
	elementByID    map[uint64]*goja.Object
	listeners      []listenerRecord
	listenerCount  int
	maxListeners   int
	dynamicScripts map[uint64]struct{}

	loaded    bool
	started   bool
	stopped   bool
	executing atomic.Bool
}

type listenerRecord struct {
	elementID uint64
	eventType string
	function  goja.Value
	capture   bool
	token     events.ListenerID
}

// New returns an unloaded page-scoped JavaScript Runtime.
func New() *Runtime {
	return &Runtime{maxListeners: MaxEventListeners, maxMicrotasks: maxMicrotaskQueue, moduleTimeout: defaultModuleTimeout}
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
	resourceBaseURL := environment.ResourceBaseURL
	if resourceBaseURL == nil {
		resourceBaseURL = environment.BaseURL
	}
	if runtime.fetchClock != nil {
		runtime.fetchAPI = fetchapi.NewPageWithClock(runtimeContext, resourceBaseURL, environment.Fetch, runtime.enqueueCallback, runtime.fetchClock)
	} else {
		runtime.fetchAPI = fetchapi.NewPage(runtimeContext, resourceBaseURL, environment.Fetch, runtime.enqueueCallback)
	}
	runtime.fetchAPI.SetLimiter(environment.FetchLimiter)
	runtime.navigationAPI = navigationapi.NewPage(environment.BaseURL, environment.Navigate)
	runtime.navigationAPI.SetPushStateHandler(environment.HistoryPush)
	runtime.navigationAPI.SetReplaceStateHandler(environment.HistoryReplace)
	runtime.navigationAPI.SetTraversalHandler(environment.HistoryTraverse, environment.HistoryInfo)
	if runtime.schedulerClock != nil {
		runtime.schedulerAPI = schedulerapi.NewPageWithClock(runtimeContext, runtime.schedulerClock, runtime.enqueueCallback, environment.RequestFrame)
	} else {
		runtime.schedulerAPI = schedulerapi.NewPage(runtimeContext, runtime.enqueueCallback, environment.RequestFrame)
	}
	runtime.schedulerAPI.SetFrameScope(environment.FrameScope)
	runtime.storageAPI = storageapi.NewPage(environment.LocalStorage, environment.SessionStorage, environment.StorageSource, runtime.enqueueCallback)
	runtime.responseValues = make(map[*goja.Object]fetchapi.Response)
	runtime.setFrameAccess(environment.Frames)
	runtime.windowProxies = make(map[frameObjectKey]*goja.Object)
	runtime.wasmAPI = newWasmAPI(runtimeContext, runtime.responseValues)
	runtime.elements = make(map[*goja.Object]*domapi.Element)
	runtime.elementByID = make(map[uint64]*goja.Object)
	runtime.abortSignals = make(map[*goja.Object]*fetchapi.AbortSignal)
	runtime.listeners = nil
	runtime.dynamicScripts = make(map[uint64]struct{})
	runtime.windowListeners = nil
	runtime.documentListeners = nil
	runtime.microtasks = nil
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
		if err := runtime.installDOM(vm); err != nil {
			return err
		}
		if err := runtime.installFetch(vm); err != nil {
			return err
		}
		if err := runtime.installStorage(vm); err != nil {
			return err
		}
		if err := runtime.installNavigation(vm); err != nil {
			return err
		}
		if err := runtime.installGlobals(vm); err != nil {
			return err
		}
		if err := runtime.installServiceWorker(vm); err != nil {
			return err
		}
		if err := runtime.installMessaging(vm); err != nil {
			return err
		}
		if err := runtime.wasmAPI.install(vm); err != nil {
			return err
		}
		return runtime.installScheduler(vm)
	}); err != nil {
		_ = runtime.Stop()
		return fmt.Errorf("install JavaScript host API: %w", err)
	}
	return nil
}

// Start evaluates selected Page scripts according to classic script loading
// semantics and delivers the document lifecycle exactly once.
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

	blocking, deferred, asynchronous := orderClassicScripts(scripts)
	if err := runtime.evaluateScripts(ctx, blocking); err != nil {
		return fmt.Errorf("execute JavaScript scripts: %w", err)
	}
	if err := runtime.runSync(ctx, func(vm *goja.Runtime) error {
		runtime.setDocumentReadyState(vm, "interactive")
		return nil
	}); err != nil {
		return fmt.Errorf("execute JavaScript scripts: %w", err)
	}
	if err := runtime.evaluateScripts(ctx, deferred); err != nil {
		return fmt.Errorf("execute JavaScript scripts: %w", err)
	}
	if err := runtime.runSync(ctx, func(vm *goja.Runtime) error {
		runtime.dispatchLifecycleEvent(vm, "DOMContentLoaded", true)
		return nil
	}); err != nil {
		return fmt.Errorf("execute JavaScript scripts: %w", err)
	}
	if err := runtime.evaluateScripts(ctx, asynchronous); err != nil {
		return fmt.Errorf("execute JavaScript scripts: %w", err)
	}
	err := runtime.runSync(ctx, func(vm *goja.Runtime) error {
		runtime.setDocumentReadyState(vm, "complete")
		runtime.dispatchLifecycleEvent(vm, "load", false)
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
	fetch := runtime.fetchAPI
	scheduler := runtime.schedulerAPI
	storage := runtime.storageAPI
	wasm := runtime.wasmAPI
	runtime.cancel = nil
	runtime.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if fetch != nil {
		fetch.Close()
	}
	if scheduler != nil {
		scheduler.Close()
	}
	if storage != nil {
		storage.Close()
	}
	if wasm != nil {
		wasm.close()
	}
	if vm != nil {
		vm.Interrupt(context.Canceled)
	}
	if done != nil {
		<-done
	}
	runtime.mu.Lock()
	dispatcher := runtime.environment.Events
	listeners := append([]listenerRecord(nil), runtime.listeners...)
	runtime.vm = nil
	runtime.runtimeCtx = nil
	runtime.queue = nil
	runtime.done = nil
	runtime.scripts = nil
	runtime.environment = runtimemodel.Environment{}
	runtime.domAPI = nil
	runtime.fetchAPI = nil
	runtime.navigationAPI = nil
	runtime.schedulerAPI = nil
	runtime.storageAPI = nil
	runtime.wasmAPI = nil
	runtime.responseValues = nil
	runtime.frameAccess = nil
	runtime.frameByElement = nil
	runtime.frameWindows = nil
	runtime.frameDocuments = nil
	runtime.windowProxies = nil
	runtime.structuredClone = nil
	runtime.elements = nil
	runtime.elementByID = nil
	runtime.abortSignals = nil
	runtime.listeners = nil
	runtime.dynamicScripts = nil
	runtime.windowListeners = nil
	runtime.documentListeners = nil
	runtime.microtasks = nil
	runtime.listenerCount = 0
	runtime.loaded = false
	runtime.mu.Unlock()
	if dispatcher != nil {
		for _, listener := range listeners {
			dispatcher.RemoveEventListener(dommodel.NodeID(listener.elementID), events.Type(listener.eventType), listener.token)
		}
	}
	return nil
}

func orderClassicScripts(scripts []runtimemodel.Script) (blocking, deferred, asynchronous []runtimemodel.Script) {
	for _, script := range scripts {
		switch script.Schedule {
		case runtimemodel.ScriptDefer:
			deferred = append(deferred, script)
		case runtimemodel.ScriptAsync:
			asynchronous = append(asynchronous, script)
		default:
			blocking = append(blocking, script)
		}
	}
	byDocumentOrder := func(values []runtimemodel.Script) {
		sort.SliceStable(values, func(left, right int) bool { return values[left].DocumentOrder < values[right].DocumentOrder })
	}
	byDocumentOrder(blocking)
	byDocumentOrder(deferred)
	sort.SliceStable(asynchronous, func(left, right int) bool {
		leftOrder, rightOrder := asynchronous[left].FetchOrder, asynchronous[right].FetchOrder
		if leftOrder <= 0 {
			leftOrder = len(asynchronous) + asynchronous[left].DocumentOrder + 1
		}
		if rightOrder <= 0 {
			rightOrder = len(asynchronous) + asynchronous[right].DocumentOrder + 1
		}
		if leftOrder == rightOrder {
			return asynchronous[left].DocumentOrder < asynchronous[right].DocumentOrder
		}
		return leftOrder < rightOrder
	})
	return blocking, deferred, asynchronous
}

func (runtime *Runtime) evaluateScripts(ctx context.Context, scripts []runtimemodel.Script) error {
	for index, script := range scripts {
		name := fmt.Sprintf("inline-script-%03d.js", index)
		if script.SourceURL != nil {
			name = network.RedactedURL(script.SourceURL)
		}
		var settled chan error
		var containedErr error
		evaluationContext := ctx
		cancel := func() {}
		timeout := defaultModuleTimeout
		if script.Kind == runtimemodel.ScriptModule {
			runtime.mu.Lock()
			timeout = runtime.moduleTimeout
			runtime.mu.Unlock()
			if timeout <= 0 {
				timeout = defaultModuleTimeout
			}
			evaluationContext, cancel = context.WithTimeout(ctx, timeout)
		}
		err := runtime.runSync(evaluationContext, func(vm *goja.Runtime) error {
			source := script.Source
			if script.Kind != runtimemodel.ScriptModule {
				_, scriptErr := vm.RunScript(name, source)
				if scriptErr != nil {
					containedErr = scriptErr
				}
				return nil
			}
			runtime.mu.Lock()
			environment := runtime.environment
			runtime.mu.Unlock()
			var bundleErr error
			source, bundleErr = bundleModule(evaluationContext, script, environment)
			if bundleErr != nil {
				runtime.recordError(fmt.Sprintf("link %s: %v", name, bundleErr))
				return nil
			}
			value, scriptErr := vm.RunScript(name, source)
			if scriptErr != nil {
				containedErr = scriptErr
				return nil
			}
			promise, ok := value.Export().(*goja.Promise)
			if !ok {
				runtime.recordError(fmt.Sprintf("evaluate %s: module did not return a Promise", name))
				return nil
			}
			settled = make(chan error, 1)
			promiseObject := vm.ToValue(promise).ToObject(vm)
			then, ok := goja.AssertFunction(promiseObject.Get("then"))
			if !ok {
				return errors.New("module Promise has no then method")
			}
			resolve := vm.ToValue(func(goja.FunctionCall) goja.Value {
				settled <- nil
				return goja.Undefined()
			})
			reject := vm.ToValue(func(call goja.FunctionCall) goja.Value {
				settled <- errors.New(call.Argument(0).String())
				return goja.Undefined()
			})
			_, scriptErr = then(promiseObject, resolve, reject)
			return scriptErr
		})
		if err != nil {
			cancel()
			return err
		}
		if containedErr != nil {
			runtime.recordScriptError(name, containedErr)
		}
		if settled != nil {
			select {
			case evaluationErr := <-settled:
				if evaluationErr != nil {
					runtime.recordError(fmt.Sprintf("evaluate %s: %v", name, evaluationErr))
				}
			case <-evaluationContext.Done():
				if ctx.Err() != nil {
					cancel()
					return context.Cause(ctx)
				}
				runtime.recordError(fmt.Sprintf("evaluate %s: module graph exceeded %s", name, timeout))
			}
		}
		cancel()
	}
	return nil
}

func (runtime *Runtime) recordScriptError(name string, scriptErr error) {
	runtime.mu.Lock()
	runtimeContext := runtime.runtimeCtx
	runtime.mu.Unlock()
	if runtimeContext != nil && runtimeContext.Err() != nil {
		return
	}
	runtime.recordError(fmt.Sprintf("execute %s: %v", name, scriptErr))
}

// UpdateLocation reflects a same-document Navigation in JavaScript location.
func (runtime *Runtime) UpdateLocation(documentURL *url.URL) {
	runtime.mu.Lock()
	navigation := runtime.navigationAPI
	runtime.mu.Unlock()
	if navigation != nil {
		navigation.UpdateCurrent(documentURL)
	}
}

// DispatchPopState queues a Browser history traversal event for JavaScript.
func (runtime *Runtime) DispatchPopState(state string) {
	runtime.mu.Lock()
	navigation := runtime.navigationAPI
	runtime.mu.Unlock()
	if navigation != nil {
		runtime.enqueueCallback(func() { navigation.DispatchPopState(state) })
	}
}

// DispatchHashChange queues a same-document fragment event for JavaScript.
func (runtime *Runtime) DispatchHashChange(oldURL, newURL string) {
	runtime.mu.Lock()
	navigation := runtime.navigationAPI
	runtime.mu.Unlock()
	if navigation != nil {
		runtime.enqueueCallback(func() { navigation.DispatchHashChange(oldURL, newURL) })
	}
}

// RunAnimationFrame synchronously delivers one browser frame on the Page queue.
func (runtime *Runtime) RunAnimationFrame(current time.Time) bool {
	runtime.mu.Lock()
	scheduler := runtime.schedulerAPI
	runtime.mu.Unlock()
	if scheduler == nil || !scheduler.HasAnimationFrameCallbacks() {
		return false
	}
	ran := false
	if err := runtime.runSync(context.Background(), func(*goja.Runtime) error {
		ran = scheduler.RunAnimationFrame(current)
		return nil
	}); err != nil {
		return false
	}
	return ran
}

// HasAnimationFrameCallbacks reports whether the Page requested another frame.
func (runtime *Runtime) HasAnimationFrameCallbacks() bool {
	runtime.mu.Lock()
	scheduler := runtime.schedulerAPI
	runtime.mu.Unlock()
	return scheduler != nil && scheduler.HasAnimationFrameCallbacks()
}

// SetBackground applies hidden-Tab scheduling policy to the Page.
func (runtime *Runtime) SetBackground(background bool) {
	runtime.mu.Lock()
	scheduler := runtime.schedulerAPI
	runtime.mu.Unlock()
	if scheduler != nil {
		scheduler.SetBackground(background)
	}
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
		default:
		}
		select {
		case <-ctx.Done():
			return
		case queued := <-queue:
			select {
			case <-ctx.Done():
				return
			default:
			}
			runtime.executing.Store(true)
			err := executeTask(vm, queued.run)
			if ctx.Err() == nil {
				runtime.drainMicrotasks(vm)
			}
			runtime.executing.Store(false)
			queued.result <- err
		}
	}
}

func (runtime *Runtime) enqueueCallback(callback func()) bool {
	if callback == nil {
		return false
	}
	runtime.mu.Lock()
	runtimeContext := runtime.runtimeCtx
	queue := runtime.queue
	stopped := runtime.stopped
	runtime.mu.Unlock()
	if stopped || runtimeContext == nil || queue == nil {
		return false
	}
	queued := task{
		run: func(*goja.Runtime) error {
			callback()
			return nil
		},
		result: make(chan error, 1),
	}
	select {
	case <-runtimeContext.Done():
		return false
	default:
	}
	select {
	case queue <- queued:
		return true
	case <-runtimeContext.Done():
		return false
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
