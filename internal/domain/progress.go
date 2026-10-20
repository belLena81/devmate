package domain

// Progress receives status updates from the service layer; all methods must be safe for concurrent use.
type Progress interface {
	Status(msg string)
	Done(msg string)
}

// NoopProgress discards all progress updates.
type NoopProgress struct{}

func (NoopProgress) Status(string) {}
func (NoopProgress) Done(string)   {}
