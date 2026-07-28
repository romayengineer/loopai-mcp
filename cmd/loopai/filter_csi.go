package main

import (
	"bytes"
	"io"
)

// FilterState tracks the CSI parsing state machine.
type FilterState int

const (
	stateNormal FilterState = iota
	stateESC                // ESC seen
	stateCSI                // ESC [ seen, buffering CSI params
	stateCSIu               // ESC [ < seen, buffering CSI u params
	stateSS3                // ESC O seen, waiting for final byte
)

// filterCSI copies data from src to dst, stripping terminal escape sequences
// that would otherwise appear as raw artifacts on the client's TUI.
//
// In raw terminal mode (-interactive), the user's terminal generates escape
// sequences for special keys (arrows, function keys, modifiers). These must
// be stripped before forwarding to the PTY, otherwise the client sees raw
// escape artifacts like ^[[A, ^[[<35;5u, etc.
//
// Stripped sequences:
//   - CSI: ESC [ [params] [intermediate] final_byte    (0x40-0x7E)
//     Examples: ^[[A (up), ^[[1;5C (Ctrl+right), ^[[<35;5u (F1 CSI u)
//   - SS3: ESC O final_byte                            (0x40-0x7E)
//     Examples: ^[OP (F1), ^[OQ (F2)
func filterCSI(dst io.Writer, src io.Reader) (written int64, err error) {
	buf := make([]byte, 4096)
	var pending bytes.Buffer
	state := stateNormal

	for {
		n, rerr := src.Read(buf)
		if n > 0 {
			data := buf[:n]
			for _, b := range data {
				switch state {
				case stateNormal:
					if b == 0x1B {
						pending.Reset()
						pending.WriteByte(b)
						state = stateESC
					} else {
						nw, werr := dst.Write([]byte{b})
						written += int64(nw)
						if werr != nil {
							return written, werr
						}
					}

				case stateESC:
					pending.WriteByte(b)
					switch b {
					case '[':
						state = stateCSI
					case 'O':
						state = stateSS3
					default:
						// Not a known escape sequence start — flush and reset.
						nw, werr := dst.Write(pending.Bytes())
						written += int64(nw)
						if werr != nil {
							return written, werr
						}
						pending.Reset()
						state = stateNormal
					}

				case stateCSI:
					if b == '<' {
						// CSI u sequence: ESC [ < params... u
						pending.WriteByte(b)
						state = stateCSIu
					} else if b >= 0x40 && b <= 0x7E {
						// Final byte of CSI — discard the whole sequence.
						pending.Reset()
						state = stateNormal
					} else if b >= 0x20 && b <= 0x3F {
						// Parameter or intermediate byte — keep buffering.
						pending.WriteByte(b)
					} else if b == 0x1B {
						// Nested ESC — flush pending and start over.
						nw, werr := dst.Write(pending.Bytes())
						written += int64(nw)
						if werr != nil {
							return written, werr
						}
						pending.Reset()
						pending.WriteByte(b)
						state = stateESC
					} else {
						// Unexpected — flush pending and reset.
						pending.WriteByte(b)
						nw, werr := dst.Write(pending.Bytes())
						written += int64(nw)
						if werr != nil {
							return written, werr
						}
						pending.Reset()
						state = stateNormal
					}

				case stateCSIu:
					if b == 'u' {
						// Complete CSI u — discard.
						pending.Reset()
						state = stateNormal
					} else if (b >= '0' && b <= '9') || b == ';' {
						pending.WriteByte(b)
					} else if b >= 0x40 && b <= 0x7E {
						// Non-u final byte — probably malformed CSI u, discard.
						pending.Reset()
						state = stateNormal
					} else if b == 0x1B {
						nw, werr := dst.Write(pending.Bytes())
						written += int64(nw)
						if werr != nil {
							return written, werr
						}
						pending.Reset()
						pending.WriteByte(b)
						state = stateESC
					} else {
						// Unexpected — flush and reset.
						pending.WriteByte(b)
						nw, werr := dst.Write(pending.Bytes())
						written += int64(nw)
						if werr != nil {
							return written, werr
						}
						pending.Reset()
						state = stateNormal
					}

				case stateSS3:
					// After ESC O, the next byte is the final byte.
					// Discard the sequence regardless of what the byte was.
					pending.Reset()
					state = stateNormal
				}
			}
		}
		if rerr != nil {
			if rerr == io.EOF && pending.Len() > 0 {
				nw, werr := dst.Write(pending.Bytes())
				written += int64(nw)
				if werr != nil {
					return written, werr
				}
			}
			return written, rerr
		}
	}
}
