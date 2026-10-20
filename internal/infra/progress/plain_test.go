package progress_test

import (
	"bytes"
	"devmate/internal/infra/progress"
	"strings"
	"testing"
)

func TestPlain_PrintsEachStatusChangeOnce(t *testing.T) {
	var buf bytes.Buffer
	p := progress.NewPlain(&buf)

	p.Status("step 1")
	p.Status("step 1")
	p.Status("step 2")
	p.Done("finished")

	got := buf.String()
	if want := "step 1\nstep 2\nfinished\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if strings.Contains(got, "\033") || strings.Contains(got, "\r") {
		t.Error("plain reporter must not emit terminal escapes")
	}
}

func TestPlain_DoneWithoutMessage_PrintsNothing(t *testing.T) {
	var buf bytes.Buffer
	p := progress.NewPlain(&buf)
	p.Done("")
	if buf.Len() != 0 {
		t.Errorf("expected no output, got %q", buf.String())
	}
}
