package progress

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

var frames = [...]string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// LockedWriter serialises writes to an io.Writer shared by the spinner and other writers such as slog.
type LockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func NewLockedWriter(w io.Writer) *LockedWriter { return &LockedWriter{w: w} }

func (lw *LockedWriter) Write(p []byte) (int, error) {
	lw.mu.Lock()
	defer lw.mu.Unlock()
	return lw.w.Write(p)
}

// Spinner writes animated progress lines to a writer; it implements domain.Progress.
type Spinner struct {
	w       io.Writer
	mu      sync.Mutex
	msg     string
	stop    chan struct{}
	stopped chan struct{}
	active  bool
}

// New returns a Spinner that writes to os.Stderr.
func New() *Spinner {
	return NewWriter(os.Stderr)
}

func NewWriter(w io.Writer) *Spinner {
	return &Spinner{w: w}
}

// Status sets the status message and starts the spinner if it is not running.
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

// Done stops the spinner, clears the line, and prints msg if non-empty.
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

	<-s.stopped

	fmt.Fprint(s.w, "\r\033[K")
	if msg != "" {
		fmt.Fprintf(s.w, "%s\n", msg)
	}
}

func (s *Spinner) loop() {
	defer close(s.stopped)

	ticker := time.NewTicker(80 * time.Millisecond)
	defer ticker.Stop()

	frame := 0
	for {
		s.mu.Lock()
		msg := s.msg
		// Write inside the lock so the escape sequences never interleave with Status/Done output.
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
