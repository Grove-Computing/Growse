package serviceworker

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
	"time"

	"github.com/Grove-Computing/Growse/internal/network"
	"github.com/Grove-Computing/Growse/internal/runtime/isolated"
	"github.com/Grove-Computing/Growse/internal/runtime/workerproto"
)

const (
	defaultServiceWorkerIdleTimeout = 30 * time.Second
	defaultServiceWorkerTaskTimeout = 5 * time.Second
	serviceWorkerStopTimeout        = time.Second
	maxServiceWorkerStderrBytes     = 64 << 10
)

var ErrServiceWorkerProcess = errors.New("service worker process failure")

type serviceWorkerProcess struct {
	manager     *Manager
	origin      string
	key         string
	peer        *workerPeer
	blobs       *workerBlobStore
	command     *exec.Cmd
	stdin       io.WriteCloser
	done        chan error
	stderr      *serviceWorkerLimitedBuffer
	constraints []string

	callMu  sync.Mutex
	hostMu  sync.RWMutex
	host    serviceWorkerEventHost
	stopMu  sync.Mutex
	stopped bool
	idle    *time.Timer
}

type serviceWorkerEventHost struct {
	ctx      context.Context
	fallback NetworkFallback
}

type serviceWorkerLimitedBuffer struct {
	mu    sync.Mutex
	limit int
	data  bytes.Buffer
}

func (buffer *serviceWorkerLimitedBuffer) Write(value []byte) (int, error) {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	written := len(value)
	remaining := buffer.limit - buffer.data.Len()
	if remaining > 0 {
		if len(value) > remaining {
			value = value[:remaining]
		}
		_, _ = buffer.data.Write(value)
	}
	return written, nil
}

func (buffer *serviceWorkerLimitedBuffer) String() string {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return strings.TrimSpace(buffer.data.String())
}

func startServiceWorkerProcess(manager *Manager, key, origin string, scriptURL *url.URL, source []byte, host serviceWorkerEventHost) (*serviceWorkerProcess, error) {
	if !isolated.AcquireAuxiliaryWorkerSlot() {
		return nil, errors.New("service worker process limit exceeded")
	}
	release := true
	defer func() {
		if release {
			isolated.ReleaseAuxiliaryWorkerSlot()
		}
	}()
	executable, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("resolve service worker executable: %w", err)
	}
	command := exec.Command(executable) // #nosec G204 -- os.Executable is the verified current Growse/test binary.
	command.Env = serviceWorkerEnvironment()
	if err := isolated.ConfigureAuxiliaryWorkerCommand(command); err != nil {
		return nil, fmt.Errorf("configure service worker sandbox: %w", err)
	}
	stdin, err := command.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, err
	}
	stderr := &serviceWorkerLimitedBuffer{limit: maxServiceWorkerStderrBytes}
	command.Stderr = stderr
	if err := command.Start(); err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("start service worker: %w", err)
	}
	done := make(chan error, 1)
	go func() {
		done <- command.Wait()
		isolated.ReleaseAuxiliaryWorkerSlot()
	}()
	release = false
	peer := newPausedWorkerPeer(stdout, stdin)
	process := &serviceWorkerProcess{
		manager: manager, origin: origin, key: key, peer: peer, command: command, stdin: stdin, done: done, stderr: stderr, host: host,
	}
	process.blobs = newWorkerBlobStore(peer)
	process.installHostHandlers()
	peer.start()
	loadContext, cancel := context.WithTimeout(context.Background(), manager.serviceWorkerTaskTimeout())
	defer cancel()
	process.setHost(serviceWorkerEventHost{ctx: loadContext, fallback: host.fallback})
	defer process.clearHost()
	sourceID, err := process.blobs.send(loadContext, source)
	if err == nil {
		var response workerLoadResponse
		err = peer.call(loadContext, "worker.load", workerLoadRequest{ScriptURL: scriptURL.String(), Source: sourceID}, &response)
		process.constraints = append([]string(nil), response.Constraints...)
	}
	if err != nil {
		process.stop()
		return nil, fmt.Errorf("load service worker process: %w%s", err, process.stderrSuffix())
	}
	return process, nil
}

func serviceWorkerEnvironment() []string {
	values := []string{serviceWorkerEnvironmentKey + "=1", "GOMAXPROCS=1"}
	if runtime.GOOS == "windows" {
		for _, name := range []string{"SystemRoot", "WINDIR"} {
			if value := os.Getenv(name); value != "" {
				values = append(values, name+"="+value)
			}
		}
	}
	return values
}

func (process *serviceWorkerProcess) installHostHandlers() {
	process.peer.handleRequest("host.network", process.handleNetwork)
	process.peer.handleRequest("host.cache.open", process.handleCacheOpen)
	process.peer.handleRequest("host.cache.match", process.handleCacheMatch)
	process.peer.handleRequest("host.cache.has", process.handleCacheHas)
	process.peer.handleRequest("host.cache.delete", process.handleCacheDelete)
	process.peer.handleRequest("host.cache.keys", process.handleCacheKeys)
	process.peer.handleRequest("host.cache.put", process.handleCachePut)
	process.peer.handleRequest("host.cache.entry-delete", process.handleCacheEntryDelete)
	process.peer.handleRequest("host.cache.entry-keys", process.handleCacheEntryKeys)
}

func (process *serviceWorkerProcess) lifecycle(ctx context.Context, activate bool, fallback NetworkFallback) (lifecycleResult, error) {
	var response workerLifecycleResponse
	err := process.call(ctx, fallback, "worker.lifecycle", workerLifecycleRequest{Activate: activate}, &response)
	return lifecycleResult{skipWaiting: response.SkipWaiting, claim: response.Claim}, err
}

func (process *serviceWorkerProcess) fetch(ctx context.Context, request *network.Request, fallback NetworkFallback) (*network.Response, error) {
	process.callMu.Lock()
	defer process.callMu.Unlock()
	process.stopIdleTimer()
	eventContext, cancel := process.eventContext(ctx)
	defer cancel()
	process.setHost(serviceWorkerEventHost{ctx: eventContext, fallback: fallback})
	defer process.clearHost()
	wireRequest, err := requestToWire(eventContext, process.blobs, request)
	if err != nil {
		return nil, process.normalizeCallError(err)
	}
	var result workerFetchResponse
	if err := process.peer.call(eventContext, "worker.fetch", workerFetchRequest{Request: wireRequest}, &result); err != nil {
		return nil, process.normalizeCallError(err)
	}
	response, err := responseFromWire(process.blobs, result.Response)
	if err != nil {
		return nil, process.callError(err)
	}
	process.scheduleIdle()
	return response, nil
}

func (process *serviceWorkerProcess) call(ctx context.Context, fallback NetworkFallback, method string, request, response any) error {
	process.callMu.Lock()
	defer process.callMu.Unlock()
	process.stopIdleTimer()
	eventContext, cancel := process.eventContext(ctx)
	defer cancel()
	process.setHost(serviceWorkerEventHost{ctx: eventContext, fallback: fallback})
	defer process.clearHost()
	if err := process.peer.call(eventContext, method, request, response); err != nil {
		return process.normalizeCallError(err)
	}
	process.scheduleIdle()
	return nil
}

func (process *serviceWorkerProcess) eventContext(parent context.Context) (context.Context, context.CancelFunc) {
	if parent != nil && parent.Err() != nil {
		eventContext, cancel := context.WithCancel(context.Background())
		cancel()
		return eventContext, func() {}
	}
	return context.WithTimeout(context.Background(), process.manager.serviceWorkerTaskTimeout())
}

func (process *serviceWorkerProcess) callError(err error) error {
	return fmt.Errorf("%w: %w%s", ErrServiceWorkerProcess, err, process.stderrSuffix())
}

func (process *serviceWorkerProcess) normalizeCallError(err error) error {
	var remote workerRemoteError
	if errors.As(err, &remote) {
		return remote
	}
	return process.callError(err)
}

func (process *serviceWorkerProcess) stderrSuffix() string {
	if process == nil || process.stderr == nil || process.stderr.String() == "" {
		return ""
	}
	return ": " + process.stderr.String()
}

func (process *serviceWorkerProcess) setHost(host serviceWorkerEventHost) {
	process.hostMu.Lock()
	process.host = host
	process.hostMu.Unlock()
}

func (process *serviceWorkerProcess) clearHost() {
	process.hostMu.Lock()
	process.host = serviceWorkerEventHost{}
	process.hostMu.Unlock()
}

func (process *serviceWorkerProcess) currentHost() serviceWorkerEventHost {
	process.hostMu.RLock()
	host := process.host
	process.hostMu.RUnlock()
	return host
}

func (process *serviceWorkerProcess) stopIdleTimer() {
	process.stopMu.Lock()
	if process.idle != nil {
		process.idle.Stop()
		process.idle = nil
	}
	process.stopMu.Unlock()
}

func (process *serviceWorkerProcess) scheduleIdle() {
	process.stopMu.Lock()
	if !process.stopped {
		if process.idle != nil {
			process.idle.Stop()
		}
		process.idle = time.AfterFunc(process.manager.serviceWorkerIdleTimeout(), func() {
			process.manager.evictServiceWorker(process.key, process)
		})
	}
	process.stopMu.Unlock()
}

func (process *serviceWorkerProcess) stop() {
	if process == nil {
		return
	}
	process.stopMu.Lock()
	if process.stopped {
		process.stopMu.Unlock()
		return
	}
	process.stopped = true
	if process.idle != nil {
		process.idle.Stop()
		process.idle = nil
	}
	process.stopMu.Unlock()
	stopContext, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	_ = process.peer.call(stopContext, "worker.stop", nil, nil)
	cancel()
	if process.stdin != nil {
		_ = process.stdin.Close()
	}
	select {
	case <-process.done:
	case <-time.After(serviceWorkerStopTimeout):
		if process.command != nil && process.command.Process != nil {
			_ = process.command.Process.Kill()
		}
		<-process.done
	}
}

func (process *serviceWorkerProcess) stopAfterCalls() {
	if process == nil {
		return
	}
	process.callMu.Lock()
	defer process.callMu.Unlock()
	process.stop()
}

func (process *serviceWorkerProcess) kill() {
	if process == nil {
		return
	}
	process.stopMu.Lock()
	if process.stopped {
		process.stopMu.Unlock()
		return
	}
	process.stopped = true
	if process.idle != nil {
		process.idle.Stop()
		process.idle = nil
	}
	process.stopMu.Unlock()
	if process.command != nil && process.command.Process != nil {
		_ = process.command.Process.Kill()
	}
	if process.stdin != nil {
		_ = process.stdin.Close()
	}
	<-process.done
}

func (process *serviceWorkerProcess) handleNetwork(_ context.Context, payload json.RawMessage) (any, error) {
	var request workerFetchRequest
	if err := workerproto.DecodePayload(payload, &request); err != nil {
		return nil, err
	}
	host := process.currentHost()
	if host.fallback == nil || host.ctx == nil {
		return nil, errors.New("service worker network broker is unavailable")
	}
	networkRequest, err := requestFromWire(process.blobs, request.Request)
	if err != nil {
		return nil, err
	}
	response, err := host.fallback(host.ctx, networkRequest)
	if err != nil {
		return nil, err
	}
	wireResponse, err := responseToWire(host.ctx, process.blobs, response)
	return workerFetchResponse{Response: wireResponse}, err
}

func (process *serviceWorkerProcess) handleCacheOpen(_ context.Context, payload json.RawMessage) (any, error) {
	var request workerCacheNameRequest
	if err := workerproto.DecodePayload(payload, &request); err != nil {
		return nil, err
	}
	_, err := process.manager.Caches().Open(process.origin, request.Name)
	return nil, err
}

func (process *serviceWorkerProcess) handleCacheMatch(_ context.Context, payload json.RawMessage) (any, error) {
	var request workerCacheRequest
	if err := workerproto.DecodePayload(payload, &request); err != nil {
		return nil, err
	}
	cacheRequest, err := requestFromWire(process.blobs, request.Request)
	if err != nil {
		return nil, err
	}
	var response *network.Response
	var found bool
	if request.Name == "" {
		response, found = process.manager.Caches().Match(process.origin, cacheRequest)
	} else if cache, ok := process.manager.Caches().cache(process.origin, request.Name); ok {
		response, found = cache.Match(cacheRequest)
	}
	wireResponse, err := responseToWire(process.hostContext(), process.blobs, response)
	if err != nil {
		return nil, err
	}
	wireResponse.Found = found
	return workerCacheMatchResponse{Response: wireResponse}, nil
}

func (process *serviceWorkerProcess) handleCacheHas(_ context.Context, payload json.RawMessage) (any, error) {
	var request workerCacheNameRequest
	if err := workerproto.DecodePayload(payload, &request); err != nil {
		return nil, err
	}
	return workerBoolResponse{Value: process.manager.Caches().Has(process.origin, request.Name)}, nil
}

func (process *serviceWorkerProcess) handleCacheDelete(_ context.Context, payload json.RawMessage) (any, error) {
	var request workerCacheNameRequest
	if err := workerproto.DecodePayload(payload, &request); err != nil {
		return nil, err
	}
	return workerBoolResponse{Value: process.manager.Caches().Delete(process.origin, request.Name)}, nil
}

func (process *serviceWorkerProcess) handleCacheKeys(context.Context, json.RawMessage) (any, error) {
	return workerCacheNamesResponse{Names: process.manager.Caches().Keys(process.origin)}, nil
}

func (process *serviceWorkerProcess) handleCachePut(_ context.Context, payload json.RawMessage) (any, error) {
	var request workerCachePutRequest
	if err := workerproto.DecodePayload(payload, &request); err != nil {
		return nil, err
	}
	cache, ok := process.manager.Caches().cache(process.origin, request.Name)
	if !ok {
		return nil, errors.New("service worker Cache was deleted")
	}
	cacheRequest, err := requestFromWire(process.blobs, request.Request)
	if err != nil {
		return nil, err
	}
	response, err := responseFromWire(process.blobs, request.Response)
	if err != nil {
		return nil, err
	}
	return nil, cache.Put(cacheRequest, response)
}

func (process *serviceWorkerProcess) handleCacheEntryDelete(_ context.Context, payload json.RawMessage) (any, error) {
	var request workerCacheRequest
	if err := workerproto.DecodePayload(payload, &request); err != nil {
		return nil, err
	}
	cache, ok := process.manager.Caches().cache(process.origin, request.Name)
	if !ok {
		return workerBoolResponse{}, nil
	}
	cacheRequest, err := requestFromWire(process.blobs, request.Request)
	if err != nil {
		return nil, err
	}
	return workerBoolResponse{Value: cache.Delete(cacheRequest)}, nil
}

func (process *serviceWorkerProcess) handleCacheEntryKeys(_ context.Context, payload json.RawMessage) (any, error) {
	var request workerCacheNameRequest
	if err := workerproto.DecodePayload(payload, &request); err != nil {
		return nil, err
	}
	cache, ok := process.manager.Caches().cache(process.origin, request.Name)
	if !ok {
		return workerCacheKeysResponse{}, nil
	}
	keys := cache.Keys()
	result := workerCacheKeysResponse{Requests: make([]wireNetworkRequest, 0, len(keys))}
	for _, key := range keys {
		value, err := requestToWire(process.hostContext(), process.blobs, key)
		if err != nil {
			return nil, err
		}
		result.Requests = append(result.Requests, value)
	}
	return result, nil
}

func (process *serviceWorkerProcess) hostContext() context.Context {
	host := process.currentHost()
	if host.ctx != nil {
		return host.ctx
	}
	return context.Background()
}
