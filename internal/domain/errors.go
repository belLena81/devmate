package domain

import (
	"errors"
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

// ErrLLMNoAttemptsSucceed is wrapped together with the last LLM error once all retries fail.
var ErrLLMNoAttemptsSucceed = errors.New("LLM generate: all attempts failed")
var ErrLLMNoConfigured = errors.New("service: llm is not configured (nil)")
var ErrLLMRequestFailed = errors.New("LLM request failed")
var ErrLLMMarshalRequestFailed = errors.New("LLM marshal request failed")
var ErrLLMBuildRequestFailed = errors.New("LLM build request failed")
var ErrLLMDecodeResponseFailed = errors.New("LLM decode response failed")
var ErrChunkFailed = errors.New("chunk summarization failed")
var ErrReduceFailed = errors.New("summary reduction failed")
