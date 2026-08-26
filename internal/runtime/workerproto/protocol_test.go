package workerproto

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
)

func TestCodecRoundTripsVersionedBoundedMessage(t *testing.T) {
	payload, err := MarshalPayload(struct {
		Engine string `json:"engine"`
	}{Engine: "javascript"})
	if err != nil {
		t.Fatal(err)
	}
	var wire bytes.Buffer
	codec := NewCodec(&wire, &wire)
	want := Envelope{Version: Version, Kind: KindRequest, ID: 7, Method: "load", Payload: payload}
	if err := codec.Write(want); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	got, err := codec.Read()
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if got.Version != Version || got.Kind != KindRequest || got.ID != 7 || got.Method != "load" {
		t.Fatalf("message = %#v", got)
	}
}

func TestCodecRejectsOversizedFrameBeforeAllocation(t *testing.T) {
	var wire bytes.Buffer
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], MaxMessageBytes+1)
	wire.Write(header[:])
	if _, err := NewCodec(&wire, nil).Read(); err == nil || !strings.Contains(err.Error(), "outside limit") {
		t.Fatalf("Read() error = %v", err)
	}
}

func TestCodecRejectsWrongVersionAndMalformedKind(t *testing.T) {
	for _, message := range []Envelope{
		{Version: Version + 1, Kind: KindRequest, ID: 1, Method: "load"},
		{Version: Version, Kind: KindRequest, Method: "load"},
		{Version: Version, Kind: KindEvent, ID: 1, Method: "mutation"},
	} {
		var wire bytes.Buffer
		if err := NewCodec(nil, &wire).Write(message); err == nil {
			t.Fatalf("Write(%#v) accepted malformed envelope", message)
		}
	}
}
