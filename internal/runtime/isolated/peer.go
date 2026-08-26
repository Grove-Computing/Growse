package isolated

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
	maxConcurrentHandlers = 64
	maxPendingRequests    = 256
)

type requestHandler func(context.Context, json.RawMessage) (any, error)
type eventHandler func(json.RawMessage)

type peer struct {
	codec *workerproto.Codec

	mu       sync.Mutex
	pending  map[uint64]chan workerproto.Envelope
	requests map[string]requestHandler
	events   map[string]eventHandler
	closed   bool
	readErr  error

	nextID   atomic.Uint64
	done     chan struct{}
	handlers chan struct{}
	readOnce sync.Once
}

func newPeer(reader io.Reader, writer io.Writer) *peer {
	p := newPausedPeer(reader, writer)
	p.start()
	return p
}

func newPausedPeer(reader io.Reader, writer io.Writer) *peer {
	p := &peer{
		codec: workerproto.NewCodec(reader, writer), pending: make(map[uint64]chan workerproto.Envelope),
		requests: make(map[string]requestHandler), events: make(map[string]eventHandler),
		done: make(chan struct{}), handlers: make(chan struct{}, maxConcurrentHandlers),
	}
	return p
}

func (p *peer) start() {
	p.readOnce.Do(func() { go p.readLoop() })
}

func (p *peer) handleRequest(method string, handler requestHandler) {
	p.mu.Lock()
	p.requests[method] = handler
	p.mu.Unlock()
}

func (p *peer) handleEvent(method string, handler eventHandler) {
	p.mu.Lock()
	p.events[method] = handler
	p.mu.Unlock()
}

func (p *peer) call(ctx context.Context, method string, request, response any) error {
	if ctx == nil {
		ctx = context.Background()
	}
	payload, err := workerproto.MarshalPayload(request)
	if err != nil {
		return err
	}
	id := p.nextID.Add(1)
	if id == 0 {
		return errors.New("worker request ID exhausted")
	}
	replies := make(chan workerproto.Envelope, 1)
	p.mu.Lock()
	if p.closed {
		err := p.readErr
		p.mu.Unlock()
		if err == nil {
			err = errors.New("worker connection is closed")
		}
		return err
	}
	if len(p.pending) >= maxPendingRequests {
		p.mu.Unlock()
		return errors.New("worker pending request limit exceeded")
	}
	p.pending[id] = replies
	p.mu.Unlock()

	if err := p.codec.Write(workerproto.Envelope{Kind: workerproto.KindRequest, ID: id, Method: method, Payload: payload}); err != nil {
		p.removePending(id)
		return err
	}
	select {
	case reply, ok := <-replies:
		if !ok {
			p.mu.Lock()
			err := p.readErr
			p.mu.Unlock()
			if err == nil {
				err = errors.New("worker connection closed before response")
			}
			return err
		}
		if reply.Error != "" {
			return errors.New(reply.Error)
		}
		return workerproto.DecodePayload(reply.Payload, response)
	case <-ctx.Done():
		p.removePending(id)
		return ctx.Err()
	case <-p.done:
		p.mu.Lock()
		err := p.readErr
		p.mu.Unlock()
		if err == nil {
			err = errors.New("worker connection closed")
		}
		return err
	}
}

func (p *peer) event(method string, value any) error {
	payload, err := workerproto.MarshalPayload(value)
	if err != nil {
		return err
	}
	p.mu.Lock()
	closed := p.closed
	readErr := p.readErr
	p.mu.Unlock()
	if closed {
		if readErr != nil {
			return readErr
		}
		return errors.New("worker connection is closed")
	}
	return p.codec.Write(workerproto.Envelope{Kind: workerproto.KindEvent, Method: method, Payload: payload})
}

func (p *peer) closeWithError(err error) {
	if err == nil {
		err = io.EOF
	}
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.closed = true
	p.readErr = err
	pending := p.pending
	p.pending = make(map[uint64]chan workerproto.Envelope)
	close(p.done)
	p.mu.Unlock()
	for _, replies := range pending {
		close(replies)
	}
}

func (p *peer) removePending(id uint64) {
	p.mu.Lock()
	delete(p.pending, id)
	p.mu.Unlock()
}

func (p *peer) readLoop() {
	for {
		message, err := p.codec.Read()
		if err != nil {
			p.closeWithError(err)
			return
		}
		switch message.Kind {
		case workerproto.KindResponse:
			p.mu.Lock()
			replies := p.pending[message.ID]
			delete(p.pending, message.ID)
			p.mu.Unlock()
			if replies != nil {
				replies <- message
			}
		case workerproto.KindRequest:
			p.dispatchRequest(message)
		case workerproto.KindEvent:
			p.mu.Lock()
			handler := p.events[message.Method]
			p.mu.Unlock()
			if handler != nil {
				handler(message.Payload)
			}
		}
	}
}

func (p *peer) dispatchRequest(message workerproto.Envelope) {
	p.mu.Lock()
	handler := p.requests[message.Method]
	p.mu.Unlock()
	select {
	case p.handlers <- struct{}{}:
		go func() {
			defer func() { <-p.handlers }()
			result, err := invokeRequestHandler(handler, message)
			payload, marshalErr := workerproto.MarshalPayload(result)
			if err == nil && marshalErr != nil {
				err = marshalErr
			}
			reply := workerproto.Envelope{Kind: workerproto.KindResponse, ID: message.ID, Payload: payload}
			if err != nil {
				reply.Error = err.Error()
			}
			if writeErr := p.codec.Write(reply); writeErr != nil {
				p.closeWithError(writeErr)
			}
		}()
	default:
		reply := workerproto.Envelope{Kind: workerproto.KindResponse, ID: message.ID, Error: "worker request concurrency limit exceeded"}
		if err := p.codec.Write(reply); err != nil {
			p.closeWithError(err)
		}
	}
}

func invokeRequestHandler(handler requestHandler, message workerproto.Envelope) (result any, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			result = nil
			err = fmt.Errorf("worker handler panic: %v", recovered)
		}
	}()
	if handler == nil {
		return nil, fmt.Errorf("unsupported worker method %q", message.Method)
	}
	return handler(context.Background(), message.Payload)
}
