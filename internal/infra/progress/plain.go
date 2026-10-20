package progress

import (
	"fmt"
	"io"
	"sync"
)

// Plain prints each status change on its own line without terminal escapes; it implements domain.Progress.
type Plain struct {
	mu   sync.Mutex
	w    io.Writer
	last string
}

// NewPlain returns a Plain reporter writing to w; use it when the output is not a terminal.
func NewPlain(w io.Writer) *Plain {
	return &Plain{w: w}
}

func (p *Plain) Status(msg string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if msg == p.last {
		return
	}
	p.last = msg
	fmt.Fprintln(p.w, msg)
}

func (p *Plain) Done(msg string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.last = ""
	if msg != "" {
		fmt.Fprintln(p.w, msg)
	}
}
