package serviceworker

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"

	"github.com/Grove-Computing/Growse/internal/runtime/workerproto"
)

const (
	serviceWorkerBlobChunk = 512 << 10
	maxServiceWorkerBlob   = 4 << 20
	maxPendingBlobBytes    = 16 << 20
)

type blobChunkRequest struct {
	ID    uint64 `json:"id"`
	Data  []byte `json:"data,omitempty"`
	Final bool   `json:"final,omitempty"`
}

type workerBlobStore struct {
	peer *workerPeer
	mu   sync.Mutex
	next atomic.Uint64
	data map[uint64][]byte
	done map[uint64]bool
	size int
}

func newWorkerBlobStore(peer *workerPeer) *workerBlobStore {
	store := &workerBlobStore{peer: peer, data: make(map[uint64][]byte), done: make(map[uint64]bool)}
	peer.handleRequest("blob.put", store.receive)
	return store
}

func (store *workerBlobStore) send(ctx context.Context, value []byte) (uint64, error) {
	if len(value) == 0 {
		return 0, nil
	}
	if len(value) > maxServiceWorkerBlob {
		return 0, errors.New("service worker blob exceeds transfer limit")
	}
	id := store.next.Add(1)
	if id == 0 {
		return 0, errors.New("service worker blob ID exhausted")
	}
	for offset := 0; offset < len(value); offset += serviceWorkerBlobChunk {
		end := min(offset+serviceWorkerBlobChunk, len(value))
		request := blobChunkRequest{ID: id, Data: append([]byte(nil), value[offset:end]...), Final: end == len(value)}
		if err := store.peer.call(ctx, "blob.put", request, nil); err != nil {
			return 0, err
		}
	}
	return id, nil
}

func (store *workerBlobStore) receive(_ context.Context, payload json.RawMessage) (any, error) {
	var request blobChunkRequest
	if err := workerproto.DecodePayload(payload, &request); err != nil {
		return nil, err
	}
	if request.ID == 0 || len(request.Data) > serviceWorkerBlobChunk {
		return nil, errors.New("invalid service worker blob chunk")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.done[request.ID] || len(store.data[request.ID])+len(request.Data) > maxServiceWorkerBlob || store.size+len(request.Data) > maxPendingBlobBytes {
		store.size -= len(store.data[request.ID])
		delete(store.data, request.ID)
		delete(store.done, request.ID)
		return nil, errors.New("service worker blob quota exceeded")
	}
	store.data[request.ID] = append(store.data[request.ID], request.Data...)
	store.size += len(request.Data)
	if request.Final {
		store.done[request.ID] = true
	}
	return nil, nil
}

func (store *workerBlobStore) take(id uint64) ([]byte, error) {
	if id == 0 {
		return nil, nil
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if !store.done[id] {
		return nil, errors.New("service worker blob is incomplete")
	}
	value := store.data[id]
	delete(store.data, id)
	delete(store.done, id)
	store.size -= len(value)
	return value, nil
}
