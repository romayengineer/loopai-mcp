package main

import (
	"os"
	"testing"
)

func TestCaptureConstants(t *testing.T) {
	if readBufSize <= 0 {
		t.Fatalf("expected positive readBufSize, got %d", readBufSize)
	}
	if captureDirMode&os.ModePerm == 0 {
		t.Fatalf("expected non-zero captureDirMode, got %o", captureDirMode)
	}
	if captureFileMode&os.ModePerm == 0 {
		t.Fatalf("expected non-zero captureFileMode, got %o", captureFileMode)
	}
}
