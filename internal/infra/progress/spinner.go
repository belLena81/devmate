package progress

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// frames is the spinner animation sequence.
var frames = [...]string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// LockedWriter wraps an io.Writer with a mutex so that multiple callers
// (e.g. the spinner animation loop and a slog handler) never interleave writes.
// Pass the same LockedWriter to both progress.NewWriter and slog.NewTextHandler
// to eliminate fd-level write contention on stderr.
type LockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

// NewLockedWriter wraps w in a LockedWriter.
func NewLockedWriter(w io.Writer) *LockedWriter { return &LockedWriter{w: w} }

// Write implements io.Writer.
func (lw *LockedWriter) Write(p []byte) (int, error) {
	lw.mu.Lock()
	defer lw.mu.Unlock()
	return lw.w.Write(p)
}

// Spinner writes animated progress lines to a writer (typically os.Stderr).
// It implements domain.Progress and is safe for concurrent use.
type Spinner struct {
	w       io.Writer
	mu      sync.Mutex
	msg     string // current status text
	stop    chan struct{}
	stopped chan struct{}
	active  bool
}

// New returns a Spinner that writes to os.Stderr.
func New() *Spinner {
	return NewWriter(os.Stderr)
}

// NewWriter returns a Spinner that writes to w.
// Useful for testing or redirecting output.
func NewWriter(w io.Writer) *Spinner {
	return &Spinner{w: w}
}

// Status sets the current status message and starts the spinner if needed.
func (s *Spinner) Status(msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.msg = msg
	if !s.active {
		s.stop = make(chan struct{})
		s.stopped = make(chan struct{})
		s.active = true
		go s.loop()
	}
}

// Done stops the spinner, clears the line, and optionally prints a final message.
// It returns as soon as the animation goroutine exits — no longer than one tick (80ms).
func (s *Spinner) Done(msg string) {
	s.mu.Lock()
	if !s.active {
		s.mu.Unlock()
		if msg != "" {
			fmt.Fprintf(s.w, "\r\033[K%s\n", msg)
		}
		return
	}
	close(s.stop)
	s.active = false
	s.mu.Unlock()

	<-s.stopped // wait for loop to exit — at most one 80ms tick

	// Clear the spinner line.
	fmt.Fprint(s.w, "\r\033[K")
	if msg != "" {
		fmt.Fprintf(s.w, "%s\n", msg)
	}
}

// loop animates the spinner at ~80ms per frame until stop is closed.
// The write to w is done while holding mu so that concurrent Status/Done
// calls and external writers sharing a LockedWriter never interleave.
func (s *Spinner) loop() {
	defer close(s.stopped)

	ticker := time.NewTicker(80 * time.Millisecond)
	defer ticker.Stop()

	frame := 0
	for {
		s.mu.Lock()
		msg := s.msg
		// Write inside the lock so that when w is a LockedWriter shared
		// with slog, our \r\033[K escape sequences are never interleaved.
		fmt.Fprintf(s.w, "\r\033[K%s %s", frames[frame%len(frames)], msg)
		s.mu.Unlock()
		frame++

		select {
		case <-s.stop:
			return
		case <-ticker.C:
		}
	}
}
