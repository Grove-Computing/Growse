package serviceworker

import (
	"context"
	"errors"
	"net/url"
	"time"
)

func (manager *Manager) startCandidateWorker(ctx context.Context, key, origin string, scriptURL *url.URL, source []byte, activate bool, fallback NetworkFallback) (*serviceWorkerProcess, lifecycleResult, error) {
	host := serviceWorkerEventHost{ctx: ctx, fallback: fallback}
	process, err := startServiceWorkerProcess(manager, key, origin, scriptURL, source, host)
	if err != nil {
		return nil, lifecycleResult{}, err
	}
	manager.workerMu.Lock()
	manager.workerStarts++
	manager.workerMu.Unlock()
	result, err := process.lifecycle(ctx, activate, fallback)
	if err != nil {
		process.stop()
		return nil, lifecycleResult{}, err
	}
	return process, result, nil
}

func (manager *Manager) activeServiceWorker(worker activeWorker, fallback NetworkFallback) (*serviceWorkerProcess, error) {
	manager.workerMu.Lock()
	if manager.workers == nil {
		manager.workers = make(map[string]*serviceWorkerProcess)
	}
	if process := manager.workers[worker.key]; process != nil {
		manager.workerMu.Unlock()
		return process, nil
	}
	manager.workerMu.Unlock()
	process, err := startServiceWorkerProcess(manager, worker.key, worker.origin, worker.scriptURL, worker.source, serviceWorkerEventHost{ctx: context.Background(), fallback: fallback})
	if err != nil {
		return nil, err
	}
	manager.workerMu.Lock()
	if existing := manager.workers[worker.key]; existing != nil {
		manager.workerMu.Unlock()
		process.stop()
		return existing, nil
	}
	var replaced []*serviceWorkerProcess
	for key, existing := range manager.workers {
		if existing != nil && existing.origin == worker.origin {
			delete(manager.workers, key)
			replaced = append(replaced, existing)
		}
	}
	manager.workers[worker.key] = process
	manager.workerStarts++
	manager.workerMu.Unlock()
	for _, existing := range replaced {
		existing.stopAfterCalls()
	}
	return process, nil
}

func (manager *Manager) installServiceWorker(key string, candidate *serviceWorkerProcess) {
	if manager == nil || candidate == nil {
		return
	}
	manager.workerMu.Lock()
	if manager.workers == nil {
		manager.workers = make(map[string]*serviceWorkerProcess)
	}
	var previous []*serviceWorkerProcess
	for existingKey, existing := range manager.workers {
		if existing != nil && existing.origin == candidate.origin {
			delete(manager.workers, existingKey)
			if existing != candidate {
				previous = append(previous, existing)
			}
		}
	}
	manager.workers[key] = candidate
	manager.workerMu.Unlock()
	for _, existing := range previous {
		existing.stopAfterCalls()
	}
}

func (manager *Manager) evictServiceWorker(key string, expected *serviceWorkerProcess) {
	if manager == nil {
		return
	}
	manager.workerMu.Lock()
	process := manager.workers[key]
	if process == nil || expected != nil && process != expected {
		manager.workerMu.Unlock()
		return
	}
	delete(manager.workers, key)
	manager.workerMu.Unlock()
	process.stopAfterCalls()
}

func (manager *Manager) crashServiceWorker(key string, expected *serviceWorkerProcess) {
	if manager == nil || expected == nil {
		return
	}
	manager.workerMu.Lock()
	if manager.workers[key] == expected {
		delete(manager.workers, key)
	}
	manager.workerMu.Unlock()
	expected.kill()
}

func (manager *Manager) serviceWorkerIdleTimeout() time.Duration {
	manager.workerMu.Lock()
	value := manager.idleTimeout
	manager.workerMu.Unlock()
	if value <= 0 {
		return defaultServiceWorkerIdleTimeout
	}
	return value
}

func (manager *Manager) serviceWorkerTaskTimeout() time.Duration {
	manager.workerMu.Lock()
	value := manager.taskTimeout
	manager.workerMu.Unlock()
	if value <= 0 {
		return defaultServiceWorkerTaskTimeout
	}
	return value
}

// Close stops every idle or active Service Worker process owned by the profile.
func (manager *Manager) Close() error {
	if manager == nil {
		return nil
	}
	manager.workerMu.Lock()
	workers := make([]*serviceWorkerProcess, 0, len(manager.workers))
	for _, process := range manager.workers {
		workers = append(workers, process)
	}
	manager.workers = make(map[string]*serviceWorkerProcess)
	manager.workerMu.Unlock()
	for _, process := range workers {
		process.stopAfterCalls()
	}
	return nil
}

func (manager *Manager) crashWorkerForTest(key string) error {
	manager.workerMu.Lock()
	process := manager.workers[key]
	manager.workerMu.Unlock()
	if process == nil || process.command == nil || process.command.Process == nil {
		return errors.New("service worker process is unavailable")
	}
	return process.command.Process.Kill()
}
