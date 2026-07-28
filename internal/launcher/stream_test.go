package launcher

import (
	"bytes"
	"context"
	"errors"
	"os"
	"sync"
	"testing"

	"github.com/romayengineer/loopai-mcp/internal/proto"
)

type mockSender struct {
	mu       sync.Mutex
	messages []proto.Message
	err      error
}

func (s *mockSender) Send(_ context.Context, msg proto.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return s.err
	}
	s.messages = append(s.messages, msg)
	return nil
}

type mockReceiver struct {
	mu   sync.Mutex
	msgs []proto.Message
	idx  int
	err  error
}

func (r *mockReceiver) Receive(_ context.Context) (proto.Message, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return proto.Message{}, r.err
	}
	if r.idx >= len(r.msgs) {
		return proto.Message{}, errors.New("no more messages")
	}
	msg := r.msgs[r.idx]
	r.idx++
	return msg, nil
}

type mockPtyWriter struct {
	mu        sync.Mutex
	buf       bytes.Buffer
	signals   []os.Signal
	writeErr  error
	signalErr error
}

func (w *mockPtyWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.writeErr != nil {
		return 0, w.writeErr
	}
	return w.buf.Write(p)
}

func (w *mockPtyWriter) Signal(sig os.Signal) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.signalErr != nil {
		return w.signalErr
	}
	w.signals = append(w.signals, sig)
	return nil
}

func (w *mockPtyWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

func (w *mockPtyWriter) Signals() []os.Signal {
	w.mu.Lock()
	defer w.mu.Unlock()
	result := make([]os.Signal, len(w.signals))
	copy(result, w.signals)
	return result
}

type mockResetter struct {
	mu    sync.Mutex
	count int
}

func (r *mockResetter) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.count++
}

func TestPipePTYToBackendHappyPath(t *testing.T) {
	var sender mockSender
	var resetter mockResetter
	input := []byte("hello from PTY\n")
	r := bytes.NewReader(input)

	err := PipePTYToBackend(context.Background(), &sender, r, &resetter)
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if len(sender.messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(sender.messages))
	}
	if sender.messages[0].Type != proto.MsgOutput {
		t.Fatalf("expected MsgOutput, got %s", sender.messages[0].Type)
	}
	if resetter.count != 1 {
		t.Fatalf("expected 1 reset, got %d", resetter.count)
	}
}

func TestPipePTYToBackendMultipleChunks(t *testing.T) {
	var sender mockSender
	var resetter mockResetter
	r := bytes.NewReader([]byte("chunk1\nchunk2\nchunk3\n"))

	err := PipePTYToBackend(context.Background(), &sender, r, &resetter)
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if len(sender.messages) == 0 {
		t.Fatal("expected at least one message")
	}
	if resetter.count == 0 {
		t.Fatal("expected at least one reset")
	}
}

func TestPipePTYToBackendContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var sender mockSender
	var resetter mockResetter
	r := bytes.NewReader([]byte("data"))

	err := PipePTYToBackend(ctx, &sender, r, &resetter)
	if err == nil {
		t.Fatal("expected context error, got nil")
	}
}

func TestPipePTYToBackendSendError(t *testing.T) {
	sender := &mockSender{err: errors.New("send failed")}
	var resetter mockResetter
	r := bytes.NewReader([]byte("data"))

	err := PipePTYToBackend(context.Background(), sender, r, &resetter)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestPipePTYToBackendReadError(t *testing.T) {
	var sender mockSender
	var resetter mockResetter
	r := &errorReader{err: errors.New("read failed")}

	err := PipePTYToBackend(context.Background(), &sender, r, &resetter)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

type errorReader struct {
	err error
}

func (r *errorReader) Read([]byte) (int, error) {
	return 0, r.err
}

func TestPipeBackendToPTYType(t *testing.T) {
	msg, err := proto.NewMessage(proto.MsgType, proto.TypePayload{Text: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	shutdownMsg, _ := proto.NewMessage(proto.MsgShutdown, proto.ShutdownPayload{})
	receiver := &mockReceiver{msgs: []proto.Message{msg, shutdownMsg}}
	var pty mockPtyWriter

	err = PipeBackendToPTY(context.Background(), receiver, &pty)
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if pty.String() != "hello\r" {
		t.Fatalf("expected 'hello\\r', got %q", pty.String())
	}
}

func TestPipeBackendToPTYCtrlC(t *testing.T) {
	msg, err := proto.NewMessage(proto.MsgCtrlC, proto.CtrlCPayload{})
	if err != nil {
		t.Fatal(err)
	}
	shutdownMsg, _ := proto.NewMessage(proto.MsgShutdown, proto.ShutdownPayload{})
	receiver := &mockReceiver{msgs: []proto.Message{msg, shutdownMsg}}
	var pty mockPtyWriter

	err = PipeBackendToPTY(context.Background(), receiver, &pty)
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	signals := pty.Signals()
	if len(signals) != 1 {
		t.Fatalf("expected 1 signal, got %d", len(signals))
	}
	if signals[0] != os.Interrupt {
		t.Fatalf("expected Interrupt, got %v", signals[0])
	}
}

func TestPipeBackendToPTYShutdown(t *testing.T) {
	msg, err := proto.NewMessage(proto.MsgShutdown, proto.ShutdownPayload{})
	if err != nil {
		t.Fatal(err)
	}
	receiver := &mockReceiver{msgs: []proto.Message{msg}}
	var pty mockPtyWriter

	err = PipeBackendToPTY(context.Background(), receiver, &pty)
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestPipeBackendToPTYContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	receiver := &mockReceiver{msgs: []proto.Message{}}
	var pty mockPtyWriter

	err := PipeBackendToPTY(ctx, receiver, &pty)
	if err == nil {
		t.Fatal("expected context error, got nil")
	}
}

func TestPipeBackendToPTYReceiveError(t *testing.T) {
	receiver := &mockReceiver{err: errors.New("receive failed")}
	var pty mockPtyWriter

	err := PipeBackendToPTY(context.Background(), receiver, &pty)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestPipeBackendToPTYWriteError(t *testing.T) {
	msg, err := proto.NewMessage(proto.MsgType, proto.TypePayload{Text: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	shutdownMsg, _ := proto.NewMessage(proto.MsgShutdown, proto.ShutdownPayload{})
	receiver := &mockReceiver{msgs: []proto.Message{msg, shutdownMsg}}
	pty := &mockPtyWriter{writeErr: errors.New("write failed")}

	err = PipeBackendToPTY(context.Background(), receiver, pty)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestPipeBackendToPTYUnknownMessage(t *testing.T) {
	msg := proto.Message{Type: "unknown_type"}
	shutdownMsg, _ := proto.NewMessage(proto.MsgShutdown, proto.ShutdownPayload{})
	receiver := &mockReceiver{msgs: []proto.Message{msg, shutdownMsg}}
	var pty mockPtyWriter

	err := PipeBackendToPTY(context.Background(), receiver, &pty)
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestPipeBackendToPTYBadTypePayload(t *testing.T) {
	msg := proto.Message{Type: proto.MsgType, Payload: []byte(`{invalid json}`)}
	shutdownMsg, _ := proto.NewMessage(proto.MsgShutdown, proto.ShutdownPayload{})
	receiver := &mockReceiver{msgs: []proto.Message{msg, shutdownMsg}}
	var pty mockPtyWriter

	err := PipeBackendToPTY(context.Background(), receiver, &pty)
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if pty.String() != "" {
		t.Fatalf("expected empty buffer, got %q", pty.String())
	}
}
