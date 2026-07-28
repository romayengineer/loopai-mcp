package proto_test

import (
	"encoding/json"
	"testing"

	"github.com/romayengineer/loopai-mcp/internal/proto"
)

func TestNewMessage(t *testing.T) {
	msg := proto.NewMessage(proto.MsgStarted, proto.StartedPayload{Pid: 42, Client: "claude"})
	if msg.Type != proto.MsgStarted {
		t.Fatalf("expected type %q, got %q", proto.MsgStarted, msg.Type)
	}
	if msg.Payload == nil {
		t.Fatal("expected non-nil payload")
	}
	var p proto.StartedPayload
	if err := json.Unmarshal(msg.Payload, &p); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if p.Pid != 42 || p.Client != "claude" {
		t.Fatalf("unexpected payload: %+v", p)
	}
}

func TestNewMessageNilPayload(t *testing.T) {
	msg := proto.NewMessage(proto.MsgIdle, proto.IdlePayload{})
	if msg.Type != proto.MsgIdle {
		t.Fatalf("expected type %q, got %q", proto.MsgIdle, msg.Type)
	}
}

func TestMessageRoundTrip(t *testing.T) {
	orig := proto.NewMessage(proto.MsgOutput, proto.OutputPayload{Data: []byte("hello\nworld\n")})
	b, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded proto.Message
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Type != proto.MsgOutput {
		t.Fatalf("expected type %q, got %q", proto.MsgOutput, decoded.Type)
	}
	var p proto.OutputPayload
	if err := json.Unmarshal(decoded.Payload, &p); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if string(p.Data) != "hello\nworld\n" {
		t.Fatalf("unexpected data: %q", p.Data)
	}
}

func TestMessageTypesAreStrings(t *testing.T) {
	types := []proto.MessageType{
		proto.MsgOutput, proto.MsgIdle, proto.MsgExited,
		proto.MsgStarted, proto.MsgType, proto.MsgCtrlC, proto.MsgShutdown,
	}
	for _, mt := range types {
		if string(mt) == "" {
			t.Fatal("empty message type string")
		}
	}
}

func TestOutputPayloadRoundTrip(t *testing.T) {
	in := proto.OutputPayload{Data: []byte{0x00, 0x01, 0x02, 0xFF}}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out proto.OutputPayload
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out.Data) != 4 || out.Data[0] != 0x00 || out.Data[3] != 0xFF {
		t.Fatalf("unexpected data: %v", out.Data)
	}
}

func TestMessageJSONStructure(t *testing.T) {
	b := []byte(`{"type":"type","payload":{"text":"hello world"}}`)
	var msg proto.Message
	if err := json.Unmarshal(b, &msg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if msg.Type != proto.MsgType {
		t.Fatalf("expected %q, got %q", proto.MsgType, msg.Type)
	}
	var p proto.TypePayload
	if err := json.Unmarshal(msg.Payload, &p); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if p.Text != "hello world" {
		t.Fatalf("expected 'hello world', got %q", p.Text)
	}
}

func TestExitedPayload(t *testing.T) {
	msg := proto.NewMessage(proto.MsgExited, proto.ExitedPayload{Code: 1, Signal: "SIGTERM"})
	if msg.Type != proto.MsgExited {
		t.Fatalf("expected %q, got %q", proto.MsgExited, msg.Type)
	}
}
