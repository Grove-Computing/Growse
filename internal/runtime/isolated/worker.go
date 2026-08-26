package isolated

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"runtime/debug"
	"sync"
	"time"

	"github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/events"
	"github.com/Grove-Computing/Growse/internal/network"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
	"github.com/Grove-Computing/Growse/internal/runtime/javascript"
	"github.com/Grove-Computing/Growse/internal/runtime/workerproto"
	"github.com/Grove-Computing/Growse/internal/runtime/yaegi"
	storagecore "github.com/Grove-Computing/Growse/internal/storage"
)

const (
	brokerStorageSourceID = ^uint64(0)
	maxWorkerHeapBytes    = 256 << 20
)

type pageEventRuntime interface {
	DispatchPageEvent(func() bool) bool
}

type animationFrameRuntime interface {
	RunAnimationFrame(time.Time) bool
	HasAnimationFrameCallbacks() bool
}

type backgroundRuntime interface {
	SetBackground(bool)
}

type locationRuntime interface {
	UpdateLocation(*url.URL)
}

type navigationEventRuntime interface {
	DispatchPopState(string)
	DispatchHashChange(string, string)
}

type workerState struct {
	peer    *peer
	sandbox sandboxStatusResponse

	commandMu sync.Mutex
	mu        sync.Mutex
	runtime   runtimemodel.Runtime
	document  *dom.Document
	events    *events.Dispatcher
	local     *storagecore.Area
	session   *storagecore.Area
	cancel    context.CancelFunc
	unsub     []func()
	loaded    bool
	started   bool
}

func init() {
	if os.Getenv(workerEnvironmentKey) != "1" {
		return
	}
	if err := runWorkerProcess(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "runtime worker failed: %v\n", err)
		os.Exit(70)
	}
	os.Exit(0)
}

func runWorkerProcess() error {
	debug.SetMemoryLimit(maxWorkerHeapBytes)
	constraints, err := applyWorkerPlatformSandbox()
	if err != nil {
		return err
	}
	p := newPeer(os.Stdin, os.Stdout)
	state := &workerState{peer: p, sandbox: workerSandboxStatus(constraints)}
	state.installHandlers()
	<-p.done
	_ = state.stop()
	p.mu.Lock()
	err = p.readErr
	p.mu.Unlock()
	if err != nil && !errors.Is(err, os.ErrClosed) {
		return nil
	}
	return nil
}

func (state *workerState) installHandlers() {
	state.peer.handleRequest("sandbox.status", func(context.Context, json.RawMessage) (any, error) {
		return state.sandbox, nil
	})
	state.peer.handleRequest("runtime.load", state.load)
	state.peer.handleRequest("runtime.start", state.start)
	state.peer.handleRequest("runtime.stop", func(context.Context, json.RawMessage) (any, error) { return nil, state.stop() })
	state.peer.handleRequest("runtime.event", state.dispatchEvent)
	state.peer.handleRequest("runtime.frame", state.runFrame)
	state.peer.handleRequest("runtime.has-frame", state.hasFrame)
	state.peer.handleEvent("runtime.background", state.setBackground)
	state.peer.handleEvent("runtime.location", state.updateLocation)
	state.peer.handleEvent("runtime.popstate", state.dispatchPopState)
	state.peer.handleEvent("runtime.hashchange", state.dispatchHashChange)
	state.peer.handleEvent("storage.external", state.applyExternalStorage)
}

func (state *workerState) load(ctx context.Context, payload json.RawMessage) (any, error) {
	state.commandMu.Lock()
	defer state.commandMu.Unlock()
	var request loadRequest
	if err := workerproto.DecodePayload(payload, &request); err != nil {
		return nil, err
	}
	request.Engine = runtimemodel.NormalizeEngine(request.Engine)
	if !request.Engine.Valid() {
		return nil, fmt.Errorf("invalid runtime engine %q", request.Engine)
	}
	document, err := dom.NewDocumentFromSnapshot(request.Document)
	if err != nil {
		return nil, err
	}
	baseURL, err := url.Parse(request.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse runtime base URL: %w", err)
	}
	scripts := make([]runtimemodel.Script, len(request.Scripts))
	for index, script := range request.Scripts {
		scripts[index] = runtimemodel.Script{Engine: script.Engine, Source: script.Source, Inline: script.Inline}
		if script.SourceURL != "" {
			scripts[index].SourceURL, err = url.Parse(script.SourceURL)
			if err != nil {
				return nil, fmt.Errorf("parse script URL: %w", err)
			}
		}
	}
	local := storageArea(request.LocalStorage)
	session := storageArea(request.SessionStorage)
	dispatcher := events.NewDispatcher()
	var pageRuntime runtimemodel.Runtime
	switch request.Engine {
	case runtimemodel.EngineGo:
		pageRuntime = yaegi.New()
	case runtimemodel.EngineJavaScript:
		pageRuntime = javascript.New()
	}
	if pageRuntime == nil {
		return nil, errors.New("runtime worker could not create engine")
	}
	runtimeContext, cancel := context.WithCancel(context.Background())
	state.mu.Lock()
	if state.loaded || state.runtime != nil {
		state.mu.Unlock()
		cancel()
		return nil, errors.New("runtime worker is already loaded")
	}
	state.runtime, state.document, state.events = pageRuntime, document, dispatcher
	state.local, state.session, state.cancel = local, session, cancel
	state.mu.Unlock()

	environment := runtimemodel.Environment{
		Document: document, Events: dispatcher, BaseURL: baseURL,
		LocalStorage: local, SessionStorage: session, StorageSource: request.StorageSource,
		OnMutation: func() {
			_ = state.peer.event("dom.mutation", mutationEvent{Document: document.Snapshot()})
		},
		ConsoleRecord: func(level, message string) {
			_ = state.peer.event("console.record", consoleEvent{Level: level, Message: message})
		},
		RequestFrame: func() { _ = state.peer.event("frame.request", nil) },
		FrameScope: func(_ time.Time, callback func()) {
			if callback != nil {
				callback()
			}
		},
		Fetch: state.fetch,
		Navigate: func(target *url.URL) error {
			return state.peer.call(runtimeContext, "host.navigate", navigationRequest{URL: target.String()}, nil)
		},
		HistoryPush: func(value string, target *url.URL) error {
			return state.historyState(runtimeContext, "host.history-push", value, target)
		},
		HistoryReplace: func(value string, target *url.URL) error {
			return state.historyState(runtimeContext, "host.history-replace", value, target)
		},
		HistoryTraverse: func(delta int) error {
			return state.peer.call(runtimeContext, "host.history-traverse", historyTraverseRequest{Delta: delta}, nil)
		},
		HistoryInfo: func() (int, string) {
			var response historyInfoResponse
			if err := state.peer.call(runtimeContext, "host.history-info", nil, &response); err != nil {
				return request.HistoryLength, request.HistoryState
			}
			return response.Length, response.State
		},
	}
	if err := pageRuntime.Load(runtimeContext, scripts, environment); err != nil {
		cancel()
		_ = pageRuntime.Stop()
		state.reset()
		return nil, err
	}
	state.mu.Lock()
	state.loaded = true
	state.unsub = []func(){
		local.Subscribe(brokerStorageSourceID, func(change storagecore.Change) {
			_ = state.peer.event("storage.change", storageChangeEvent{Area: "local", Change: change})
		}),
		session.Subscribe(brokerStorageSourceID, func(change storagecore.Change) {
			_ = state.peer.event("storage.change", storageChangeEvent{Area: "session", Change: change})
		}),
	}
	state.mu.Unlock()
	return nil, nil
}

func (state *workerState) start(ctx context.Context, _ json.RawMessage) (any, error) {
	state.commandMu.Lock()
	defer state.commandMu.Unlock()
	state.mu.Lock()
	runtime, loaded, started := state.runtime, state.loaded, state.started
	if loaded && !started {
		state.started = true
	}
	state.mu.Unlock()
	if runtime == nil || !loaded {
		return nil, errors.New("runtime worker is not loaded")
	}
	if started {
		return nil, errors.New("runtime worker is already started")
	}
	return nil, runtime.Start(ctx)
}

func (state *workerState) stop() error {
	state.commandMu.Lock()
	defer state.commandMu.Unlock()
	state.mu.Lock()
	runtime, cancel, unsub := state.runtime, state.cancel, state.unsub
	state.runtime, state.cancel, state.unsub = nil, nil, nil
	state.loaded, state.started = false, false
	state.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	for _, unsubscribe := range unsub {
		unsubscribe()
	}
	var err error
	if runtime != nil {
		err = runtime.Stop()
	}
	state.reset()
	return err
}

func (state *workerState) reset() {
	state.mu.Lock()
	state.runtime, state.document, state.events = nil, nil, nil
	state.local, state.session, state.cancel = nil, nil, nil
	state.loaded, state.started = false, false
	state.mu.Unlock()
}

func (state *workerState) dispatchEvent(_ context.Context, payload json.RawMessage) (any, error) {
	state.commandMu.Lock()
	defer state.commandMu.Unlock()
	var request eventRequest
	if err := workerproto.DecodePayload(payload, &request); err != nil {
		return nil, err
	}
	state.mu.Lock()
	runtime, document, dispatcher := state.runtime, state.document, state.events
	state.mu.Unlock()
	if runtime == nil || document == nil || dispatcher == nil {
		return nil, errors.New("runtime worker is unavailable")
	}
	if err := document.ApplySnapshot(request.Document); err != nil {
		return nil, err
	}
	event := events.Event{Type: request.Type, Target: request.Target, X: request.X, Y: request.Y, Value: request.Value}
	if request.Cancelable {
		event = events.Cancelable(request.Type, request.Target)
		event.X, event.Y, event.Value = request.X, request.Y, request.Value
	}
	handled := false
	if queued, ok := runtime.(pageEventRuntime); ok {
		handled = queued.DispatchPageEvent(func() bool { return dispatcher.Dispatch(event) })
	} else {
		handled = dispatcher.Dispatch(event)
	}
	return eventResponse{Handled: handled, DefaultPrevented: event.DefaultPrevented()}, nil
}

func (state *workerState) runFrame(_ context.Context, payload json.RawMessage) (any, error) {
	state.commandMu.Lock()
	defer state.commandMu.Unlock()
	var request frameRequest
	if err := workerproto.DecodePayload(payload, &request); err != nil {
		return nil, err
	}
	state.mu.Lock()
	runtime, document := state.runtime, state.document
	state.mu.Unlock()
	if document != nil {
		if err := document.ApplySnapshot(request.Document); err != nil {
			return nil, err
		}
	}
	frame, ok := runtime.(animationFrameRuntime)
	return boolResponse{Value: ok && frame.RunAnimationFrame(frameTime(request))}, nil
}

func (state *workerState) hasFrame(context.Context, json.RawMessage) (any, error) {
	state.mu.Lock()
	runtime := state.runtime
	state.mu.Unlock()
	frame, ok := runtime.(animationFrameRuntime)
	return boolResponse{Value: ok && frame.HasAnimationFrameCallbacks()}, nil
}

func (state *workerState) setBackground(payload json.RawMessage) {
	var event backgroundEvent
	if workerproto.DecodePayload(payload, &event) != nil {
		return
	}
	state.mu.Lock()
	runtime := state.runtime
	state.mu.Unlock()
	if target, ok := runtime.(backgroundRuntime); ok {
		target.SetBackground(event.Background)
	}
}

func (state *workerState) updateLocation(payload json.RawMessage) {
	var event locationEvent
	if workerproto.DecodePayload(payload, &event) != nil {
		return
	}
	target, err := url.Parse(event.URL)
	if err != nil {
		return
	}
	state.mu.Lock()
	runtime := state.runtime
	state.mu.Unlock()
	if updater, ok := runtime.(locationRuntime); ok {
		updater.UpdateLocation(target)
	}
}

func (state *workerState) dispatchPopState(payload json.RawMessage) {
	var event popStateEvent
	if workerproto.DecodePayload(payload, &event) != nil {
		return
	}
	state.mu.Lock()
	runtime := state.runtime
	state.mu.Unlock()
	if dispatcher, ok := runtime.(navigationEventRuntime); ok {
		dispatcher.DispatchPopState(event.State)
	}
}

func (state *workerState) dispatchHashChange(payload json.RawMessage) {
	var event hashChangeEvent
	if workerproto.DecodePayload(payload, &event) != nil {
		return
	}
	state.mu.Lock()
	runtime := state.runtime
	state.mu.Unlock()
	if dispatcher, ok := runtime.(navigationEventRuntime); ok {
		dispatcher.DispatchHashChange(event.OldURL, event.NewURL)
	}
}

func (state *workerState) applyExternalStorage(payload json.RawMessage) {
	var event storageExternalEvent
	if workerproto.DecodePayload(payload, &event) != nil {
		return
	}
	state.mu.Lock()
	area := state.local
	if event.Area == "session" {
		area = state.session
	}
	state.mu.Unlock()
	applyStorageChange(area, storagecore.MutationSource{ID: brokerStorageSourceID, URL: event.Change.SourceURL}, event.Change)
}

func (state *workerState) fetch(ctx context.Context, request *network.Request) (*network.Response, error) {
	if request == nil || request.URL == nil {
		return nil, errors.New("runtime worker fetch request is invalid")
	}
	wire := fetchRequest{
		Method: request.Method, URL: request.URL.String(), Header: request.Header, Body: request.Body,
		Kind: request.Kind, Engine: request.Engine, Credentials: request.Credentials,
	}
	if request.SiteURL != nil {
		wire.SiteURL = request.SiteURL.String()
	}
	var response fetchResponse
	if err := state.peer.call(ctx, "host.fetch", wire, &response); err != nil {
		return nil, err
	}
	result := &network.Response{
		StatusCode: response.StatusCode, Status: response.Status, Header: response.Header,
		ContentType: response.ContentType, Body: response.Body, Redirected: response.Redirected, CacheStatus: response.CacheStatus,
	}
	if response.URL != "" {
		result.URL, _ = url.Parse(response.URL)
	}
	return result, nil
}

func (state *workerState) historyState(ctx context.Context, method, value string, target *url.URL) error {
	request := historyStateRequest{State: value}
	if target != nil {
		request.URL = target.String()
	}
	return state.peer.call(ctx, method, request, nil)
}

func storageArea(entries []storagecore.Entry) *storagecore.Area {
	area := storagecore.NewArea()
	for _, entry := range entries {
		_ = area.SetFrom(storagecore.MutationSource{ID: brokerStorageSourceID}, entry.Key, entry.Value)
	}
	return area
}
