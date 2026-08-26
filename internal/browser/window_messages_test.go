package browser

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
)

type blockingMessageRuntime struct {
	started chan struct{}
	release chan struct{}
	calls   atomic.Int32
}

func (*blockingMessageRuntime) Load(context.Context, []runtimemodel.Script, runtimemodel.Environment) error {
	return nil
}
func (*blockingMessageRuntime) Start(context.Context) error { return nil }
func (*blockingMessageRuntime) Stop() error                 { return nil }
func (runtime *blockingMessageRuntime) DispatchMessage(runtimemodel.MessageEvent) error {
	runtime.calls.Add(1)
	select {
	case runtime.started <- struct{}{}:
	default:
	}
	<-runtime.release
	return nil
}

func TestWindowMessageBrokerEnforcesOriginsQueueAndGeneration(t *testing.T) {
	registry := newWindowRegistry()
	t.Cleanup(registry.close)
	source := runtimemodel.WindowReference{ID: 0, Generation: 1, Origin: "https://source.example"}
	target := runtimemodel.WindowReference{ID: 1, Generation: 1, Origin: "https://target.example"}
	registry.define(source)
	registry.define(target)
	if err := registry.post(source, target, "https://wrong.example", []byte(`null`)); err == nil {
		t.Fatal("mismatched targetOrigin was accepted")
	}
	if err := registry.post(source, target, "/", []byte(`null`)); err == nil {
		t.Fatal("same-origin default was accepted for cross-origin target")
	}

	runtime := &blockingMessageRuntime{started: make(chan struct{}, 1), release: make(chan struct{})}
	registry.register(target, runtime)
	if err := registry.post(source, target, "*", []byte(`{"first":true}`)); err != nil {
		t.Fatal(err)
	}
	select {
	case <-runtime.started:
	case <-time.After(time.Second):
		t.Fatal("message delivery did not start")
	}
	for index := 0; index < maxPendingMessages; index++ {
		if err := registry.post(source, target, "*", []byte(`null`)); err != nil {
			t.Fatalf("queue item %d rejected: %v", index, err)
		}
	}
	if err := registry.post(source, target, "*", []byte(`null`)); err == nil || !strings.Contains(err.Error(), "queue limit") {
		t.Fatalf("overflow error = %v", err)
	}
	close(runtime.release)

	newTarget := target
	newTarget.Generation++
	registry.define(newTarget)
	if err := registry.post(source, target, "*", []byte(`null`)); err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("stale generation error = %v", err)
	}
	oversized := make([]byte, maxWindowMessageBytes+1)
	if err := registry.post(source, newTarget, "*", oversized); err == nil || !strings.Contains(err.Error(), "1 MiB") {
		t.Fatalf("oversized message error = %v", err)
	}
}
