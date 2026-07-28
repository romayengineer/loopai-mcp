package launcher

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// DisablePTYEcho disables local ECHO on the PTY master. This prevents the
// PTY from echoing keystrokes back to the user when running in interactive
// mode (-interactive flag). Without this, every keystroke forwarded from
// stdin → PTY gets echoed back through the PTY output as raw escape
// sequences (e.g. ^[[<35u) and appears on the user's terminal alongside
// the client's TUI output.
//
// The PTY starts with ECHO enabled by default (matching real terminal
// behavior). Interactive mode forwards raw keystrokes to the PTY, but the
// terminal emulator on the user's side already echoes what the user types.
// Disabling ECHO on the PTY eliminates the double-echo and the raw escape
// artifacts while still letting the client receive all keystrokes normally.
func DisablePTYEcho(ptm *os.File) error {
	n, err := unix.IoctlGetInt(int(ptm.Fd()), unix.TIOCGPTN)
	if err != nil {
		return fmt.Errorf("get pty number: %w", err)
	}
	slaveName := fmt.Sprintf("/dev/pts/%d", n)
	slave, err := os.OpenFile(slaveName, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("open slave %s: %w", slaveName, err)
	}
	defer slave.Close()

	termios, err := unix.IoctlGetTermios(int(slave.Fd()), unix.TCGETS)
	if err != nil {
		return fmt.Errorf("get slave termios: %w", err)
	}
	termios.Lflag &^= unix.ECHO
	if err := unix.IoctlSetTermios(int(slave.Fd()), unix.TCSETS, termios); err != nil {
		return fmt.Errorf("set slave termios: %w", err)
	}
	return nil
}

// DisablePTYEcho disables terminal echo on this PTY. See the package-level
// DisablePTYEcho function for details on why this is needed.
func (p *PtyProcess) DisablePTYEcho() error {
	return DisablePTYEcho(p.PTY)
}
