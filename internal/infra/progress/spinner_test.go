package progress_test

import (
	"bytes"
	"devmate/internal/infra/progress"
	"strings"
	"testing"
	"time"
)

func TestSpinner_StatusAndDone_WritesToWriter(t *testing.T) {
	var buf bytes.Buffer
	s := progress.NewWriter(&buf)

	s.Status("loading...")
	// Give the loop a moment to write at least one frame.
	time.Sleep(200 * time.Millisecond)
	s.Done("")

	out := buf.String()
	if !strings.Contains(out, "loading...") {
		t.Errorf("expected 'loading...' in output, got: %q", out)
	}
}

func TestSpinner_Done_ClearsLine(t *testing.T) {
	var buf bytes.Buffer
	s := progress.NewWriter(&buf)

	s.Status("working")
	time.Sleep(100 * time.Millisecond)
	s.Done("")

	out := buf.String()
	// \033[K is the ANSI "erase to end of line" escape used to clear.
	if !strings.Contains(out, "\033[K") {
		t.Error("expected ANSI clear sequence in output")
	}
}

func TestSpinner_Done_WithMessage_PrintsFinalLine(t *testing.T) {
	var buf bytes.Buffer
	s := progress.NewWriter(&buf)

	s.Status("working")
	time.Sleep(100 * time.Millisecond)
	s.Done("all done!")

	out := buf.String()
	if !strings.Contains(out, "all done!") {
		t.Errorf("expected final message in output, got: %q", out)
	}
}

func TestSpinner_StatusUpdate_ChangesMessage(t *testing.T) {
	var buf bytes.Buffer
	s := progress.NewWriter(&buf)

	s.Status("step 1")
	time.Sleep(150 * time.Millisecond)
	s.Status("step 2")
	time.Sleep(150 * time.Millisecond)
	s.Done("")

	out := buf.String()
	if !strings.Contains(out, "step 1") {
		t.Error("expected 'step 1' in output")
	}
	if !strings.Contains(out, "step 2") {
		t.Error("expected 'step 2' in output")
	}
}

func TestSpinner_Done_WithoutStatus_DoesNotPanic(t *testing.T) {
	var buf bytes.Buffer
	s := progress.NewWriter(&buf)

	// Calling Done without ever calling Status should not panic.
	s.Done("final")

	if !strings.Contains(buf.String(), "final") {
		t.Error("expected final message even without prior Status")
	}
}

func TestSpinner_MultipleDone_DoesNotPanic(t *testing.T) {
	var buf bytes.Buffer
	s := progress.NewWriter(&buf)

	s.Status("work")
	time.Sleep(100 * time.Millisecond)
	s.Done("")
	s.Done("") // second Done should not panic or hang
}
