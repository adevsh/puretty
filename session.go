package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
)

const bufferCapacity = 1 << 20 // 1 MiB

// OutputBuffer is a thread-safe ring buffer of PTY output bytes.
// Multiple readers can block on ReadSince concurrently; a single
// writer (the PTY read loop) appends via Write.
type OutputBuffer struct {
	mu      sync.Mutex
	buf     []byte // circular buffer, len == cap
	written int64  // total bytes ever written (monotonic, never wraps)
	cap     int    // capacity in bytes
	notify  chan struct{}
}

// NewOutputBuffer creates an OutputBuffer with the given capacity in bytes.
func NewOutputBuffer(capacity int) *OutputBuffer {
	return &OutputBuffer{
		buf:    make([]byte, capacity),
		cap:    capacity,
		notify: make(chan struct{}),
	}
}

// Write appends data to the buffer, evicting oldest bytes if needed.
// Wakes every goroutine blocked on ReadSince.
func (b *OutputBuffer) Write(data []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()

	dl := len(data)
	if dl == 0 {
		return
	}

	// Copy into circular buffer, handling wrap-around.
	pos := int(b.written % int64(b.cap))
	end := pos + dl
	if end <= b.cap {
		copy(b.buf[pos:end], data)
	} else {
		// Wrap: copy tail then head.
		n := copy(b.buf[pos:], data)
		copy(b.buf[:end-b.cap], data[n:])
	}

	b.written += int64(dl)

	// Wake all waiters.
	close(b.notify)
	b.notify = make(chan struct{})
}

// ReadSince returns all bytes written since offset, blocking until new
// data arrives, ctx is cancelled, or timeout elapses.
//
// On timeout with no data, returns (nil, offset, nil).
// On ctx cancellation, returns (nil, offset, ctx.Err()).
// If offset is behind the buffer start (data evicted), offset is
// advanced to the oldest available byte.
func (b *OutputBuffer) ReadSince(ctx context.Context, offset int64, timeout time.Duration) ([]byte, int64, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Block until data is available, timeout, or context cancelled.
	for offset >= b.written {
		ch := b.notify
		b.mu.Unlock()

		select {
		case <-ch:
			// Data written — reacquire lock and recheck.
		case <-time.After(timeout):
			b.mu.Lock()
			return nil, offset, nil
		case <-ctx.Done():
			b.mu.Lock()
			return nil, offset, ctx.Err()
		}

		b.mu.Lock()
	}

	// Clamp offset if client is too far behind (data evicted).
	oldest := b.written - int64(b.cap)
	if offset < oldest {
		offset = oldest
	}

	// Read available bytes.
	avail := int(b.written - offset)
	if avail > b.cap {
		avail = b.cap
	}

	result := make([]byte, avail)
	pos := int(offset % int64(b.cap))
	end := pos + avail
	if end <= b.cap {
		copy(result, b.buf[pos:end])
	} else {
		n := copy(result, b.buf[pos:])
		copy(result[n:], b.buf[:end-b.cap])
	}

	return result, b.written, nil
}

// Session owns the shell process and its PTY master.
// One Session per binary run; every connected browser tab reads
// and writes the same session.
type Session struct {
	output *OutputBuffer
	done   chan struct{} // closed when read loop exits (EOF or error)
	err    error         // set by read loop on exit
	mu     sync.Mutex    // guards err
	pty    *os.File
	cmd    *exec.Cmd
}

// NewSession starts a shell in a PTY and returns the Session.
//
// # Errors
//
// Returns an error if the shell cannot be started or the PTY cannot be allocated.
func NewSession(shell string) (*Session, error) {
	cmd := exec.Command(shell)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid:  true,
		Setctty: true,
	}

	ptyFile, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 24, Cols: 80})
	if err != nil {
		return nil, fmt.Errorf("start pty: %w", err)
	}

	s := &Session{
		output: NewOutputBuffer(bufferCapacity),
		done:   make(chan struct{}),
		pty:    ptyFile,
		cmd:    cmd,
	}

	go s.readLoop()

	return s, nil
}

// readLoop reads from the PTY master and writes into the output buffer.
// Exits when the PTY returns EOF or an error.
func (s *Session) readLoop() {
	defer close(s.done)

	buf := make([]byte, 4096)
	for {
		n, err := s.pty.Read(buf)
		if n > 0 {
			s.output.Write(buf[:n])
		}
		if err != nil {
			if err != io.EOF {
				s.mu.Lock()
				s.err = err
				s.mu.Unlock()
			}
			return
		}
	}
}

// Write sends b to the PTY master.
//
// # Errors
//
// Returns an error if the session has already been closed.
func (s *Session) Write(b []byte) (int, error) {
	return s.pty.Write(b)
}

// Resize changes the PTY window size.
//
// # Errors
//
// Returns an error if the session is closed or the ioctl fails.
func (s *Session) Resize(rows, cols int) error {
	return pty.Setsize(s.pty, &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)})
}

// Output returns the session's output ring buffer.
func (s *Session) Output() *OutputBuffer {
	return s.output
}

// Done returns a channel that is closed when the shell exits.
func (s *Session) Done() <-chan struct{} {
	return s.done
}

// Close terminates the shell process group and closes the PTY master.
// Safe to call multiple times.
func (s *Session) Close() error {
	// Send SIGTERM to the process group (negative PID).
	_ = syscall.Kill(-s.cmd.Process.Pid, syscall.SIGTERM)

	// Wait briefly, then force-kill if still alive.
	waitDone := make(chan struct{})
	go func() {
		_ = s.cmd.Wait()
		close(waitDone)
	}()

	select {
	case <-waitDone:
	case <-time.After(500 * time.Millisecond):
		_ = syscall.Kill(-s.cmd.Process.Pid, syscall.SIGKILL)
		<-waitDone
	}

	// Close the PTY master.
	if err := s.pty.Close(); err != nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.err == nil {
			s.err = err
		}
		return s.err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}
