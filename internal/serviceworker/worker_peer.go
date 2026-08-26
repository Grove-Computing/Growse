package serviceworker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"

	"github.com/Grove-Computing/Growse/internal/runtime/workerproto"
)

const (
	maxServiceWorkerHandlers = 16
	maxServiceWorkerPending  = 256
)

type workerRequestHandler func(context.Context, json.RawMessage) (any, error)

type workerRemoteError struct{ message string }

func (err workerRemoteError) Error() string { return err.message }

type workerPeer struct {
	codec *workerproto.Codec

	mu       sync.Mutex
	pending  map[uint64]chan workerproto.Envelope
	requests map[string]workerRequestHandler
	closed   bool
	readErr  error
	nextID   atomic.Uint64
	done     chan struct{}
	handlers chan struct{}
	readOnce sync.Once
}

func newPausedWorkerPeer(reader io.Reader, writer io.Writer) *workerPeer {
	return &workerPeer{
		codec: workerproto.NewCodec(reader, writer), pending: make(map[uint64]chan workerproto.Envelope),
		requests: make(map[string]workerRequestHandler), done: make(chan struct{}), handlers: make(chan struct{}, maxServiceWorkerHandlers),
	}
}

func (peer *workerPeer) start() { peer.readOnce.Do(func() { go peer.readLoop() }) }

func (peer *workerPeer) handleRequest(method string, handler workerRequestHandler) {
	peer.mu.Lock()
	peer.requests[method] = handler
	peer.mu.Unlock()
}

func (peer *workerPeer) call(ctx context.Context, method string, request, response any) error {
	if ctx == nil {
		ctx = context.Background()
	}
	payload, err := workerproto.MarshalPayload(request)
	if err != nil {
		return err
	}
	id := peer.nextID.Add(1)
	if id == 0 {
		return errors.New("service worker request ID exhausted")
	}
	replies := make(chan workerproto.Envelope, 1)
	peer.mu.Lock()
	if peer.closed {
		err := peer.readErr
		peer.mu.Unlock()
		if err == nil {
			err = errors.New("service worker connection is closed")
		}
		return err
	}
	if len(peer.pending) >= maxServiceWorkerPending {
		peer.mu.Unlock()
		return errors.New("service worker pending request limit exceeded")
	}
	peer.pending[id] = replies
	peer.mu.Unlock()
	if err := peer.codec.Write(workerproto.Envelope{Kind: workerproto.KindRequest, ID: id, Method: method, Payload: payload}); err != nil {
		peer.removePending(id)
		return err
	}
	select {
	case reply, ok := <-replies:
		if !ok {
			peer.mu.Lock()
			err := peer.readErr
			peer.mu.Unlock()
			if err == nil {
				err = errors.New("service worker connection closed before response")
			}
			return err
		}
		if reply.Error != "" {
			return workerRemoteError{message: reply.Error}
		}
		return workerproto.DecodePayload(reply.Payload, response)
	case <-ctx.Done():
		peer.removePending(id)
		return context.Cause(ctx)
	case <-peer.done:
		peer.mu.Lock()
		err := peer.readErr
		peer.mu.Unlock()
		if err == nil {
			err = errors.New("service worker connection is closed")
		}
		return err
	}
}

func (peer *workerPeer) removePending(id uint64) {
	peer.mu.Lock()
	delete(peer.pending, id)
	peer.mu.Unlock()
}

func (peer *workerPeer) closeWithError(err error) {
	if err == nil {
		err = io.EOF
	}
	peer.mu.Lock()
	if peer.closed {
		peer.mu.Unlock()
		return
	}
	peer.closed = true
	peer.readErr = err
	pending := peer.pending
	peer.pending = make(map[uint64]chan workerproto.Envelope)
	close(peer.done)
	peer.mu.Unlock()
	for _, replies := range pending {
		close(replies)
	}
}

func (peer *workerPeer) readLoop() {
	for {
		message, err := peer.codec.Read()
		if err != nil {
			peer.closeWithError(err)
			return
		}
		switch message.Kind {
		case workerproto.KindResponse:
			peer.mu.Lock()
			replies := peer.pending[message.ID]
			delete(peer.pending, message.ID)
			peer.mu.Unlock()
			if replies != nil {
				replies <- message
			}
		case workerproto.KindRequest:
			peer.dispatchRequest(message)
		}
	}
}

func (peer *workerPeer) dispatchRequest(message workerproto.Envelope) {
	peer.mu.Lock()
	handler := peer.requests[message.Method]
	peer.mu.Unlock()
	select {
	case peer.handlers <- struct{}{}:
		go func() {
			defer func() { <-peer.handlers }()
			result, err := invokeWorkerRequestHandler(handler, message)
			payload, marshalErr := workerproto.MarshalPayload(result)
			if err == nil && marshalErr != nil {
				err = marshalErr
			}
			reply := workerproto.Envelope{Kind: workerproto.KindResponse, ID: message.ID, Payload: payload}
			if err != nil {
				reply.Error = err.Error()
			}
			if writeErr := peer.codec.Write(reply); writeErr != nil {
				peer.closeWithError(writeErr)
			}
		}()
	default:
		reply := workerproto.Envelope{Kind: workerproto.KindResponse, ID: message.ID, Error: "service worker request concurrency limit exceeded"}
		if err := peer.codec.Write(reply); err != nil {
			peer.closeWithError(err)
		}
	}
}

func invokeWorkerRequestHandler(handler workerRequestHandler, message workerproto.Envelope) (result any, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			result = nil
			err = fmt.Errorf("service worker handler panic: %v", recovered)
		}
	}()
	if handler == nil {
		return nil, fmt.Errorf("unsupported service worker method %q", message.Method)
	}
	return handler(context.Background(), message.Payload)
}
