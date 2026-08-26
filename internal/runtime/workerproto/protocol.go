// Package workerproto defines the bounded wire format shared by the browser
// broker and isolated runtime workers.
package workerproto

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
)

const (
	Version         = 1
	MaxMessageBytes = 1 << 20
)

type Kind string

const (
	KindRequest  Kind = "request"
	KindResponse Kind = "response"
	KindEvent    Kind = "event"
)

// Envelope is one versioned request, response, or one-way event.
type Envelope struct {
	Version int             `json:"version"`
	Kind    Kind            `json:"kind"`
	ID      uint64          `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
	Error   string          `json:"error,omitempty"`
}

// Codec reads and writes big-endian length-prefixed JSON messages.
type Codec struct {
	reader io.Reader
	writer io.Writer
	write  sync.Mutex
}

func NewCodec(reader io.Reader, writer io.Writer) *Codec {
	return &Codec{reader: reader, writer: writer}
}

func MarshalPayload(value any) (json.RawMessage, error) {
	if value == nil {
		return nil, nil
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal worker payload: %w", err)
	}
	if len(payload) > MaxMessageBytes {
		return nil, errors.New("worker payload exceeds message limit")
	}
	return payload, nil
}

func DecodePayload(payload json.RawMessage, target any) error {
	if target == nil || len(payload) == 0 {
		return nil
	}
	if err := json.Unmarshal(payload, target); err != nil {
		return fmt.Errorf("decode worker payload: %w", err)
	}
	return nil
}

func (codec *Codec) Write(message Envelope) error {
	if codec == nil || codec.writer == nil {
		return errors.New("worker codec writer is unavailable")
	}
	if message.Version == 0 {
		message.Version = Version
	}
	if err := validateEnvelope(message); err != nil {
		return err
	}
	encoded, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("encode worker message: %w", err)
	}
	if len(encoded) == 0 || len(encoded) > MaxMessageBytes {
		return errors.New("worker message exceeds size limit")
	}
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(encoded)))
	codec.write.Lock()
	defer codec.write.Unlock()
	if _, err := codec.writer.Write(header[:]); err != nil {
		return fmt.Errorf("write worker message header: %w", err)
	}
	if _, err := codec.writer.Write(encoded); err != nil {
		return fmt.Errorf("write worker message: %w", err)
	}
	return nil
}

func (codec *Codec) Read() (Envelope, error) {
	if codec == nil || codec.reader == nil {
		return Envelope{}, errors.New("worker codec reader is unavailable")
	}
	var header [4]byte
	if _, err := io.ReadFull(codec.reader, header[:]); err != nil {
		return Envelope{}, fmt.Errorf("read worker message header: %w", err)
	}
	size := binary.BigEndian.Uint32(header[:])
	if size == 0 || size > MaxMessageBytes {
		return Envelope{}, fmt.Errorf("worker message size %d is outside limit", size)
	}
	encoded := make([]byte, size)
	if _, err := io.ReadFull(codec.reader, encoded); err != nil {
		return Envelope{}, fmt.Errorf("read worker message: %w", err)
	}
	var message Envelope
	if err := json.Unmarshal(encoded, &message); err != nil {
		return Envelope{}, fmt.Errorf("decode worker message: %w", err)
	}
	if err := validateEnvelope(message); err != nil {
		return Envelope{}, err
	}
	return message, nil
}

func validateEnvelope(message Envelope) error {
	if message.Version != Version {
		return fmt.Errorf("unsupported worker protocol version %d", message.Version)
	}
	switch message.Kind {
	case KindRequest:
		if message.ID == 0 || message.Method == "" {
			return errors.New("worker request requires ID and method")
		}
	case KindResponse:
		if message.ID == 0 {
			return errors.New("worker response requires ID")
		}
	case KindEvent:
		if message.ID != 0 || message.Method == "" {
			return errors.New("worker event requires method and no ID")
		}
	default:
		return fmt.Errorf("unsupported worker message kind %q", message.Kind)
	}
	if len(message.Payload) > MaxMessageBytes {
		return errors.New("worker payload exceeds message limit")
	}
	return nil
}
