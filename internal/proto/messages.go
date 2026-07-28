// Package proto defines shared message types and wire protocol for
// communication between the launcher and backend over a Unix socket.
// Messages are newline-delimited JSON.
package proto

import (
	"encoding/json"
	"fmt"
	"log/slog"
)

// MessageType identifies the kind of message in the wire protocol.
type MessageType string

const (
	MsgOutput   MessageType = "output"
	MsgIdle     MessageType = "idle"
	MsgExited   MessageType = "exited"
	MsgStarted  MessageType = "started"
	MsgType     MessageType = "type"
	MsgCtrlC    MessageType = "ctrl_c"
	MsgShutdown MessageType = "shutdown"
)

// Message is a newline-delimited JSON frame exchanged between launcher and backend.
type Message struct {
	Type    MessageType     `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// NewMessage builds a Message by JSON-marshaling the provided payload.
func NewMessage(typ MessageType, v interface{}) (Message, error) {
	b, err := json.Marshal(v)
	if err != nil {
		slog.Warn("marshal message payload", "type", typ, "error", err)
		return Message{Type: typ}, fmt.Errorf("marshal %s payload: %w", typ, err)
	}
	return Message{Type: typ, Payload: b}, nil
}

// OutputPayload carries terminal output bytes from the launcher to the backend.
type OutputPayload struct {
	Data []byte `json:"data"`
}

// IdlePayload signals that the launcher has detected an idle period in output.
type IdlePayload struct{}

// ExitedPayload reports that the spawned client process has exited.
type ExitedPayload struct {
	Code   int    `json:"code"`
	Signal string `json:"signal,omitempty"`
}

// StartedPayload is sent by the launcher when the client process starts.
type StartedPayload struct {
	Pid    int    `json:"pid"`
	Client string `json:"client"`
}

// TypePayload carries text to be typed into the PTY by the launcher.
type TypePayload struct {
	Text string `json:"text"`
}

// CtrlCPayload tells the launcher to send Ctrl+C to the PTY.
type CtrlCPayload struct{}

// ShutdownPayload tells the launcher to stop its receive loop.
type ShutdownPayload struct{}
