package isolated

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"strings"
	"testing"

	"github.com/Grove-Computing/Growse/internal/runtime/workerproto"
)

func TestPeerSupportsNestedBidirectionalRequests(t *testing.T) {
	leftConnection, rightConnection := net.Pipe()
	t.Cleanup(func() { _ = leftConnection.Close(); _ = rightConnection.Close() })
	left := newPeer(leftConnection, leftConnection)
	right := newPeer(rightConnection, rightConnection)
	left.handleRequest("double", func(ctx context.Context, payload json.RawMessage) (any, error) {
		var value int
		if err := decode(payload, &value); err != nil {
			return nil, err
		}
		return value * 2, nil
	})
	right.handleRequest("nested", func(ctx context.Context, payload json.RawMessage) (any, error) {
		var value int
		if err := decode(payload, &value); err != nil {
			return nil, err
		}
		var doubled int
		if err := right.call(ctx, "double", value, &doubled); err != nil {
			return nil, err
		}
		return doubled + 1, nil
	})
	var got int
	if err := left.call(context.Background(), "nested", 20, &got); err != nil {
		t.Fatalf("call() error = %v", err)
	}
	if got != 41 {
		t.Fatalf("nested result = %d, want 41", got)
	}
}

func TestPeerClosesPendingCallWhenTransportFails(t *testing.T) {
	leftConnection, rightConnection := net.Pipe()
	left := newPeer(leftConnection, leftConnection)
	started := make(chan struct{})
	right := newPeer(rightConnection, rightConnection)
	right.handleRequest("block", func(context.Context, json.RawMessage) (any, error) {
		close(started)
		select {}
	})
	result := make(chan error, 1)
	go func() { result <- left.call(context.Background(), "block", nil, nil) }()
	<-started
	_ = rightConnection.Close()
	if err := <-result; err == nil || !strings.Contains(err.Error(), "closed") && !strings.Contains(err.Error(), io.EOF.Error()) {
		t.Fatalf("pending call error = %v", err)
	}
}

func TestPeerRejectsUnregisteredHostMethod(t *testing.T) {
	leftConnection, rightConnection := net.Pipe()
	t.Cleanup(func() { _ = leftConnection.Close(); _ = rightConnection.Close() })
	left := newPeer(leftConnection, leftConnection)
	_ = newPeer(rightConnection, rightConnection)
	if err := left.call(context.Background(), "host.filesystem", nil, nil); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("unregistered host call error = %v", err)
	}
}

func TestPeerContainsHandlerPanicAndContinues(t *testing.T) {
	leftConnection, rightConnection := net.Pipe()
	t.Cleanup(func() { _ = leftConnection.Close(); _ = rightConnection.Close() })
	left := newPeer(leftConnection, leftConnection)
	right := newPeer(rightConnection, rightConnection)
	right.handleRequest("panic", func(context.Context, json.RawMessage) (any, error) { panic("contained") })
	right.handleRequest("ok", func(context.Context, json.RawMessage) (any, error) { return "alive", nil })
	if err := left.call(context.Background(), "panic", nil, nil); err == nil || !strings.Contains(err.Error(), "handler panic") {
		t.Fatalf("panic call error = %v", err)
	}
	var result string
	if err := left.call(context.Background(), "ok", nil, &result); err != nil || result != "alive" {
		t.Fatalf("call after panic = %q, %v", result, err)
	}
}

func TestPeerRejectsPendingRequestsBeyondLimit(t *testing.T) {
	leftConnection, rightConnection := net.Pipe()
	t.Cleanup(func() { _ = leftConnection.Close(); _ = rightConnection.Close() })
	p := newPeer(leftConnection, io.Discard)
	p.mu.Lock()
	for id := uint64(1); id <= maxPendingRequests; id++ {
		p.pending[id] = make(chan workerproto.Envelope, 1)
	}
	p.mu.Unlock()
	if err := p.call(context.Background(), "overflow", nil, nil); err == nil || !strings.Contains(err.Error(), "pending request limit") {
		t.Fatalf("pending limit error = %v", err)
	}
}

func decode(payload []byte, target any) error {
	return workerproto.DecodePayload(payload, target)
}
