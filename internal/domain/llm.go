package domain

import "context"

// LLM is the interface for any text-generation backend.
// ctx must be respected: implementations should cancel in-flight requests as
// soon as ctx is done. This enables Ctrl-C cancellation and parent timeouts to
// propagate all the way down to the HTTP layer.
type LLM interface {
	Generate(ctx context.Context, prompt string) (string, error)
}
