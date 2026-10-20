package domain

import "context"

// LLM is a text-generation backend; implementations must abort in-flight requests when ctx is done.
type LLM interface {
	Generate(ctx context.Context, prompt string) (string, error)
}
