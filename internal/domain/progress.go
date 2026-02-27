package domain

// Progress receives status updates from the service layer.
// Implementations control how (or whether) updates reach the user.
// All methods must be safe for concurrent use.
type Progress interface {
	// Status prints a transient status line (overwritten by the next call).
	Status(msg string)
	// Done clears any transient status and optionally prints a final message.
	Done(msg string)
}

// NoopProgress silently discards all progress updates.
// Used when progress reporting is disabled or in tests.
type NoopProgress struct{}

func (NoopProgress) Status(string) {}
func (NoopProgress) Done(string)   {}
