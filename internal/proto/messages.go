// Package proto defines shared message types and wire protocol for
// communication between the launcher and backend over a Unix socket.
// Messages are newline-delimited JSON.
package proto

import "encoding/json"

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

type Message struct {
	Type    MessageType      `json:"type"`
	Payload json.RawMessage  `json:"payload,omitempty"`
}

func NewMessage(typ MessageType, v interface{}) Message {
	b, err := json.Marshal(v)
	if err != nil {
		return Message{Type: typ}
	}
	return Message{Type: typ, Payload: b}
}

type OutputPayload struct {
	Data []byte `json:"data"`
}

type IdlePayload struct{}

type ExitedPayload struct {
	Code   int    `json:"code"`
	Signal string `json:"signal,omitempty"`
}

type StartedPayload struct {
	Pid    int    `json:"pid"`
	Client string `json:"client"`
}

type TypePayload struct {
	Text string `json:"text"`
}

type CtrlCPayload struct{}

type ShutdownPayload struct{}
