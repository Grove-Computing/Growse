// Package isolated runs Growse page runtimes behind a bounded child-process
// protocol. The browser remains the sole owner of DOM, network, storage, and
// navigation policy.
package isolated

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Grove-Computing/Growse/internal/events"
	"github.com/Grove-Computing/Growse/internal/network"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
	"github.com/Grove-Computing/Growse/internal/runtime/workerproto"
	storagecore "github.com/Grove-Computing/Growse/internal/storage"
)

const (
	workerEnvironmentKey = "GROWSE_RUNTIME_WORKER"
	workerStopTimeout    = time.Second
	maxWorkerStderrBytes = 64 << 10
	defaultTaskTimeout   = 5 * time.Second
	maxSessionWorkers    = 32
)

var activeWorkers atomic.Int64

// Runtime is a browser-side proxy for one isolated Go or JavaScript runtime.
type Runtime struct {
	mu sync.Mutex

	engine      runtimemodel.Engine
	environment runtimemodel.Environment
	peer        *peer
	command     *exec.Cmd
	stdin       io.WriteCloser
	processDone chan error
	stderr      *limitedBuffer
	loaded      bool
	started     bool
	stopped     bool
	unsubscribe func()
	taskTimeout time.Duration
	sandbox     runtimemodel.SandboxStatus
}

// New returns a runtime proxy for engine. The worker is started by Load.
func New(engine runtimemodel.Engine) *Runtime {
	return &Runtime{engine: runtimemodel.NormalizeEngine(engine), taskTimeout: defaultTaskTimeout}
}

func (r *Runtime) Load(ctx context.Context, scripts []runtimemodel.Script, environment runtimemodel.Environment) error {
	if r == nil {
		return errors.New("isolated runtime is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	if r.loaded || r.started {
		r.mu.Unlock()
		return errors.New("isolated runtime already loaded")
	}
	if r.stopped || !r.engine.Valid() {
		r.mu.Unlock()
		return errors.New("isolated runtime is stopped or invalid")
	}
	r.environment = environment
	r.mu.Unlock()
	if environment.Document == nil {
		return errors.New("isolated runtime requires a document")
	}

	workerPeer, command, stdin, processDone, stderr, err := startWorkerProcess()
	if err != nil {
		return err
	}
	r.mu.Lock()
	r.peer, r.command, r.stdin, r.processDone, r.stderr = workerPeer, command, stdin, processDone, stderr
	r.mu.Unlock()
	r.installHostHandlers(workerPeer)
	r.monitorWorker(workerPeer)
	if err := r.verifySandbox(ctx); err != nil {
		r.mu.Lock()
		r.sandbox.Failure = err.Error()
		r.sandbox.Ready = false
		r.mu.Unlock()
		_ = r.Stop()
		return err
	}

	request := loadRequest{
		Engine: r.engine, Document: environment.Document.Snapshot(), StorageSource: environment.StorageSource,
		ImportMap: cloneStringMap(environment.ImportMap), Frames: frameAccessToWire(environment.Frames), FramePolicy: environment.FramePolicy,
	}
	if environment.BaseURL != nil {
		request.BaseURL = publicRuntimeURL(environment.BaseURL).String()
	}
	if environment.LocalStorage != nil {
		request.LocalStorage = environment.LocalStorage.Entries()
	}
	if environment.SessionStorage != nil {
		request.SessionStorage = environment.SessionStorage.Entries()
	}
	if environment.HistoryInfo != nil {
		request.HistoryLength, request.HistoryState = environment.HistoryInfo()
	}
	request.Scripts = make([]wireScript, len(scripts))
	for index, script := range scripts {
		request.Scripts[index] = wireScript{
			Engine: script.Engine, Kind: script.Kind, Source: script.Source, Inline: script.Inline,
			Integrity: script.Integrity, CrossOrigin: script.CrossOrigin,
			Schedule: script.Schedule, DocumentOrder: script.DocumentOrder, FetchOrder: script.FetchOrder,
		}
		if script.SourceURL != nil {
			request.Scripts[index].SourceURL = publicRuntimeURL(script.SourceURL).String()
		}
	}
	if err := r.callTask(ctx, "runtime.load", request, nil); err != nil {
		_ = r.Stop()
		return fmt.Errorf("load isolated %s runtime: %w", r.engine, err)
	}
	if environment.LocalStorage != nil {
		r.unsubscribe = environment.LocalStorage.Subscribe(environment.StorageSource.ID, func(change storagecore.Change) {
			_ = workerPeer.event("storage.external", storageExternalEvent{Area: "local", Change: change})
		})
	}
	r.mu.Lock()
	r.loaded = true
	r.mu.Unlock()
	go func() {
		<-ctx.Done()
		_ = r.Stop()
	}()
	return nil
}

func cloneStringMap(source map[string]string) map[string]string {
	if len(source) == 0 {
		return nil
	}
	result := make(map[string]string, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func (r *Runtime) Start(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	r.mu.Lock()
	if !r.loaded || r.peer == nil {
		r.mu.Unlock()
		return errors.New("isolated runtime is not loaded")
	}
	if r.started {
		r.mu.Unlock()
		return errors.New("isolated runtime already started")
	}
	if r.stopped {
		r.mu.Unlock()
		return errors.New("isolated runtime is stopped")
	}
	r.started = true
	r.mu.Unlock()
	if err := r.callTask(ctx, "runtime.start", nil, nil); err != nil {
		return fmt.Errorf("start isolated %s runtime: %w", r.engine, err)
	}
	return nil
}

// SandboxStatus returns the last verified worker capability report.
func (r *Runtime) SandboxStatus() runtimemodel.SandboxStatus {
	if r == nil {
		return runtimemodel.SandboxStatus{Failure: "isolated runtime is nil"}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	status := r.sandbox
	status.Constraints = append([]string(nil), status.Constraints...)
	return status
}

func (r *Runtime) Stop() error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	if r.stopped {
		r.mu.Unlock()
		return nil
	}
	r.stopped = true
	p, stdin, command, processDone := r.peer, r.stdin, r.command, r.processDone
	unsubscribe := r.unsubscribe
	r.unsubscribe = nil
	r.mu.Unlock()
	if unsubscribe != nil {
		unsubscribe()
	}
	if p != nil {
		ctx, cancel := context.WithTimeout(context.Background(), workerStopTimeout)
		_ = p.call(ctx, "runtime.stop", nil, nil)
		cancel()
	}
	if stdin != nil {
		_ = stdin.Close()
	}
	if processDone != nil {
		select {
		case <-processDone:
		case <-time.After(workerStopTimeout):
			if command != nil && command.Process != nil {
				_ = command.Process.Kill()
			}
			<-processDone
		}
	}
	return nil
}

// DispatchDOMEvent synchronizes current browser DOM state and invokes worker listeners.
func (r *Runtime) DispatchDOMEvent(event events.Event) bool {
	r.mu.Lock()
	p, environment, stopped := r.peer, r.environment, r.stopped
	r.mu.Unlock()
	if p == nil || stopped || environment.Document == nil {
		return false
	}
	request := eventRequest{
		Document: environment.Document.Snapshot(), Type: event.Type, Target: event.Target,
		X: event.X, Y: event.Y, Value: event.Value, Cancelable: event.IsCancelable(),
	}
	var response eventResponse
	if err := r.callTask(context.Background(), "runtime.event", request, &response); err != nil {
		return false
	}
	if response.DefaultPrevented {
		event.PreventDefault()
	}
	return response.Handled
}

// DispatchPageEvent preserves the Runtime interface used by serialized browser inspection.
func (r *Runtime) DispatchPageEvent(callback func() bool) bool {
	return callback != nil && callback()
}

func (r *Runtime) RunAnimationFrame(current time.Time) bool {
	r.mu.Lock()
	p, environment, stopped := r.peer, r.environment, r.stopped
	r.mu.Unlock()
	if p == nil || stopped || environment.Document == nil {
		return false
	}
	var response boolResponse
	request := frameRequest{UnixNano: current.UnixNano(), Document: environment.Document.Snapshot()}
	return r.callTask(context.Background(), "runtime.frame", request, &response) == nil && response.Value
}

func (r *Runtime) HasAnimationFrameCallbacks() bool {
	r.mu.Lock()
	p, stopped := r.peer, r.stopped
	r.mu.Unlock()
	if p == nil || stopped {
		return false
	}
	var response boolResponse
	return r.callTask(context.Background(), "runtime.has-frame", nil, &response) == nil && response.Value
}

func (r *Runtime) SetBackground(background bool) {
	r.mu.Lock()
	p, stopped := r.peer, r.stopped
	r.mu.Unlock()
	if p != nil && !stopped {
		_ = p.event("runtime.background", backgroundEvent{Background: background})
	}
}

func (r *Runtime) UpdateLocation(location *url.URL) {
	if location != nil {
		r.sendEvent("runtime.location", locationEvent{URL: location.String()})
	}
}

func (r *Runtime) DispatchPopState(state string) {
	r.sendEvent("runtime.popstate", popStateEvent{State: state})
}

func (r *Runtime) DispatchHashChange(oldURL, newURL string) {
	r.sendEvent("runtime.hashchange", hashChangeEvent{OldURL: oldURL, NewURL: newURL})
}

func (r *Runtime) sendEvent(method string, value any) {
	r.mu.Lock()
	p, stopped := r.peer, r.stopped
	r.mu.Unlock()
	if p != nil && !stopped {
		_ = p.event(method, value)
	}
}

func (r *Runtime) callTask(ctx context.Context, method string, request, response any) error {
	if ctx == nil {
		ctx = context.Background()
	}
	r.mu.Lock()
	p, timeout, stopped := r.peer, r.taskTimeout, r.stopped
	r.mu.Unlock()
	if p == nil || stopped {
		return errors.New("isolated runtime worker is unavailable")
	}
	if timeout <= 0 {
		timeout = defaultTaskTimeout
	}
	taskContext, cancel := context.WithTimeout(ctx, timeout)
	err := p.call(taskContext, method, request, response)
	timedOut := errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil
	cancel()
	if timedOut {
		r.terminateWorker()
		return fmt.Errorf("runtime worker task %s exceeded %s", method, timeout)
	}
	return err
}

func (r *Runtime) verifySandbox(ctx context.Context) error {
	var response sandboxStatusResponse
	if err := r.callTask(ctx, "sandbox.status", nil, &response); err != nil {
		return fmt.Errorf("verify runtime sandbox: %w", err)
	}
	if err := validateSandboxStatus(response, os.Getpid()); err != nil {
		return err
	}
	r.mu.Lock()
	r.sandbox = response.SandboxStatus
	r.mu.Unlock()
	return nil
}

func (r *Runtime) terminateWorker() {
	r.mu.Lock()
	command := r.command
	r.mu.Unlock()
	if command != nil && command.Process != nil {
		_ = command.Process.Kill()
	}
}

func (r *Runtime) monitorWorker(p *peer) {
	go func() {
		<-p.done
		p.mu.Lock()
		err := p.readErr
		p.mu.Unlock()
		r.mu.Lock()
		stopped := r.stopped
		failure := r.environment.RuntimeFailure
		r.mu.Unlock()
		if !stopped && failure != nil {
			if err == nil {
				err = errors.New("runtime worker exited unexpectedly")
			}
			failure(err)
		}
	}()
}

func (r *Runtime) installHostHandlers(p *peer) {
	p.handleEvent("dom.mutation", func(payload json.RawMessage) {
		var event mutationEvent
		if workerproto.DecodePayload(payload, &event) != nil {
			return
		}
		r.mu.Lock()
		environment, stopped := r.environment, r.stopped
		r.mu.Unlock()
		if stopped || environment.Document == nil || environment.Document.ApplySnapshot(event.Document) != nil {
			return
		}
		if environment.OnMutation != nil {
			environment.OnMutation()
		}
	})
	p.handleEvent("console.record", func(payload json.RawMessage) {
		var event consoleEvent
		if workerproto.DecodePayload(payload, &event) != nil {
			return
		}
		r.mu.Lock()
		record := r.environment.ConsoleRecord
		r.mu.Unlock()
		if record != nil {
			record(event.Level, event.Message)
		}
	})
	p.handleEvent("frame.request", func(json.RawMessage) {
		r.mu.Lock()
		request := r.environment.RequestFrame
		r.mu.Unlock()
		if request != nil {
			request()
		}
	})
	p.handleEvent("storage.change", r.applyStorageChange)
	p.handleRequest("host.fetch", r.handleFetch)
	p.handleRequest("host.navigate", r.handleNavigate)
	p.handleRequest("host.history-push", r.handleHistoryPush)
	p.handleRequest("host.history-replace", r.handleHistoryReplace)
	p.handleRequest("host.history-traverse", r.handleHistoryTraverse)
	p.handleRequest("host.history-info", r.handleHistoryInfo)
	p.handleRequest("host.frame-mutation", r.handleFrameMutation)
}

func (r *Runtime) handleFrameMutation(_ context.Context, payload json.RawMessage) (any, error) {
	var request frameMutationRequest
	if err := workerproto.DecodePayload(payload, &request); err != nil {
		return nil, err
	}
	r.mu.Lock()
	mutate := r.environment.FrameMutation
	r.mu.Unlock()
	if mutate == nil {
		return nil, errors.New("frame mutation broker is unavailable")
	}
	return nil, mutate(request.ID, request.Generation, request.Document)
}

// UpdateFrames replaces generation-scoped child access inside the worker.
func (r *Runtime) UpdateFrames(frames []runtimemodel.FrameAccess) {
	if r == nil {
		return
	}
	r.mu.Lock()
	p, stopped := r.peer, r.stopped
	r.environment.Frames = append([]runtimemodel.FrameAccess(nil), frames...)
	r.mu.Unlock()
	if p != nil && !stopped {
		_ = p.event("runtime.frames", frameAccessToWire(frames))
	}
}

func (r *Runtime) handleFetch(ctx context.Context, payload json.RawMessage) (any, error) {
	var request fetchRequest
	if err := workerproto.DecodePayload(payload, &request); err != nil {
		return nil, err
	}
	r.mu.Lock()
	do := r.environment.Fetch
	r.mu.Unlock()
	if do == nil {
		return nil, errors.New("browser fetch broker is unavailable")
	}
	target, err := url.Parse(request.URL)
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	pageURL, engine := r.environment.BaseURL, string(r.engine)
	r.mu.Unlock()
	brokered := &network.Request{
		Method: request.Method, URL: target, Header: request.Header, Body: request.Body,
		Kind: request.Kind, Engine: request.Engine, Credentials: request.Credentials,
	}
	if err := validateBrokeredFetch(brokered, pageURL, engine); err != nil {
		return nil, err
	}
	response, err := do(ctx, brokered)
	if err != nil {
		return nil, err
	}
	result := fetchResponse{
		StatusCode: response.StatusCode, Status: response.Status, Header: response.Header,
		ContentType: response.ContentType, Body: response.Body, Redirected: response.Redirected, CacheStatus: response.CacheStatus,
	}
	if response.URL != nil {
		result.URL = response.URL.String()
	}
	return result, nil
}

func (r *Runtime) handleNavigate(_ context.Context, payload json.RawMessage) (any, error) {
	var request navigationRequest
	if err := workerproto.DecodePayload(payload, &request); err != nil {
		return nil, err
	}
	target, err := url.Parse(request.URL)
	if err != nil {
		return nil, err
	}
	if !httpURL(target) || target.User != nil {
		return nil, errors.New("sandbox navigation URL must be HTTP(S) without userinfo")
	}
	r.mu.Lock()
	navigate := r.environment.Navigate
	r.mu.Unlock()
	if navigate == nil {
		return nil, errors.New("navigation broker is unavailable")
	}
	return nil, navigate(target)
}

func (r *Runtime) handleHistoryPush(_ context.Context, payload json.RawMessage) (any, error) {
	return nil, r.handleHistoryState(payload, false)
}

func (r *Runtime) handleHistoryReplace(_ context.Context, payload json.RawMessage) (any, error) {
	return nil, r.handleHistoryState(payload, true)
}

func (r *Runtime) handleHistoryState(payload json.RawMessage, replace bool) error {
	var request historyStateRequest
	if err := workerproto.DecodePayload(payload, &request); err != nil {
		return err
	}
	var target *url.URL
	var err error
	if request.URL != "" {
		target, err = url.Parse(request.URL)
		if err != nil {
			return err
		}
	}
	r.mu.Lock()
	push, replaceHandler := r.environment.HistoryPush, r.environment.HistoryReplace
	r.mu.Unlock()
	if replace {
		if replaceHandler == nil {
			return errors.New("history replace broker is unavailable")
		}
		return replaceHandler(request.State, target)
	}
	if push == nil {
		return errors.New("history push broker is unavailable")
	}
	return push(request.State, target)
}

func (r *Runtime) handleHistoryTraverse(_ context.Context, payload json.RawMessage) (any, error) {
	var request historyTraverseRequest
	if err := workerproto.DecodePayload(payload, &request); err != nil {
		return nil, err
	}
	r.mu.Lock()
	traverse := r.environment.HistoryTraverse
	r.mu.Unlock()
	if traverse == nil {
		return nil, errors.New("history traversal broker is unavailable")
	}
	return nil, traverse(request.Delta)
}

func (r *Runtime) handleHistoryInfo(context.Context, json.RawMessage) (any, error) {
	r.mu.Lock()
	info := r.environment.HistoryInfo
	r.mu.Unlock()
	if info == nil {
		return historyInfoResponse{}, nil
	}
	length, state := info()
	return historyInfoResponse{Length: length, State: state}, nil
}

func (r *Runtime) applyStorageChange(payload json.RawMessage) {
	var event storageChangeEvent
	if workerproto.DecodePayload(payload, &event) != nil {
		return
	}
	r.mu.Lock()
	environment := r.environment
	r.mu.Unlock()
	area := environment.LocalStorage
	if event.Area == "session" {
		area = environment.SessionStorage
	}
	applyStorageChange(area, environment.StorageSource, event.Change)
}

func startWorkerProcess() (*peer, *exec.Cmd, io.WriteCloser, chan error, *limitedBuffer, error) {
	if activeWorkers.Add(1) > maxSessionWorkers {
		activeWorkers.Add(-1)
		return nil, nil, nil, nil, nil, errors.New("runtime worker session limit exceeded")
	}
	releaseWorker := true
	defer func() {
		if releaseWorker {
			activeWorkers.Add(-1)
		}
	}()
	executable, err := os.Executable()
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("resolve runtime worker executable: %w", err)
	}
	command := exec.Command(executable) // #nosec G204 -- os.Executable is the verified current Growse/test binary.
	command.Env = workerEnvironment()
	if err := configureWorkerCommand(command); err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("configure runtime worker sandbox: %w", err)
	}
	stdin, err := command.StdinPipe()
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, nil, nil, nil, nil, err
	}
	stderr := &limitedBuffer{limit: maxWorkerStderrBytes}
	command.Stderr = stderr
	if err := command.Start(); err != nil {
		_ = stdin.Close()
		return nil, nil, nil, nil, nil, fmt.Errorf("start runtime worker: %w", err)
	}
	done := make(chan error, 1)
	releaseWorker = false
	go func() {
		done <- command.Wait()
		activeWorkers.Add(-1)
	}()
	return newPeer(stdout, stdin), command, stdin, done, stderr, nil
}

func workerEnvironment() []string {
	environment := []string{workerEnvironmentKey + "=1", "GOMAXPROCS=1"}
	if runtime.GOOS == "windows" {
		for _, name := range []string{"SystemRoot", "WINDIR"} {
			if value := os.Getenv(name); value != "" {
				environment = append(environment, name+"="+value)
			}
		}
	}
	return environment
}

type limitedBuffer struct {
	mu    sync.Mutex
	limit int
	data  bytes.Buffer
}

func (buffer *limitedBuffer) Write(value []byte) (int, error) {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	written := len(value)
	remaining := buffer.limit - buffer.data.Len()
	if remaining > 0 {
		if remaining < len(value) {
			value = value[:remaining]
		}
		_, _ = buffer.data.Write(value)
	}
	return written, nil
}

func (buffer *limitedBuffer) String() string {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return strings.TrimSpace(buffer.data.String())
}

func applyStorageChange(area *storagecore.Area, source storagecore.MutationSource, change storagecore.Change) {
	if area == nil {
		return
	}
	if change.Cleared {
		_ = area.ClearFrom(source)
	} else if change.HasNewValue {
		_ = area.SetFrom(source, change.Key, change.NewValue)
	} else {
		_ = area.RemoveFrom(source, change.Key)
	}
}
