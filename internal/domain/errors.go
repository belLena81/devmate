package domain

import (
	"errors"
	"fmt"
)

// ─── Validation errors ────────────────────────────────────────────────────────

var ErrInvalidCmdType = errors.New("invalid commit type, must be one of [feat fix chore docs refactor]")
var ErrMissingTaskDescription = errors.New("missing task description")
var ErrMissingSourceBranch = errors.New("missing source branch")
var ErrMissingTargetBranch = errors.New("missing target branch")
var ErrServiceNotInitialized = errors.New("service not initialized")

// ─── Business-logic errors ────────────────────────────────────────────────────

var ErrEmptyPR = errors.New("no unique commits found between branches — nothing to describe")

// ─── Infrastructure errors ────────────────────────────────────────────────────

var ErrNotGitRepository = errors.New("not inside a git repository")
var ErrInvalidRef = errors.New("invalid git ref")
var ErrEmptyRef = errors.New("ref name must not be empty")

// ─── Service errors ───────────────────────────────────────────────────────────

// ErrLLMNoAttemptsSucceed wraps the last LLM error once all retries are exhausted.
// Non-retryable errors (bad request, unknown model) are returned without this wrapper.
var ErrLLMNoAttemptsSucceed = errors.New("LLM generate: all attempts failed")
var ErrLLMNoConfigured = errors.New("service: llm is not configured (nil)")
var ErrLLMRequestFailed = errors.New("LLM request failed")
var ErrLLMMarshalRequestFailed = errors.New("LLM marshal request failed")
var ErrLLMBuildRequestFailed = errors.New("LLM build request failed")
var ErrLLMDecodeResponseFailed = errors.New("LLM decode response failed")
var ErrChunkFailed = errors.New("chunk summarization failed")
var ErrReduceFailed = errors.New("summary reduction failed")

// LLMStatusError is returned when the LLM server answers with a non-2xx status.
type LLMStatusError struct {
	Code    int
	Message string
}

func (e *LLMStatusError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("LLM server returned status %d: %s", e.Code, e.Message)
	}
	return fmt.Sprintf("LLM server returned status %d", e.Code)
}

// Retryable reports whether the status indicates a transient server-side condition.
func (e *LLMStatusError) Retryable() bool {
	return e.Code == 408 || e.Code == 429 || e.Code >= 500
}
