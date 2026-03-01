package domain

import (
	"errors"
)

// ─── Validation errors ────────────────────────────────────────────────────────

// ErrInvalidCmdType is returned when a commit/branch/PR type string does not
// match any of the recognised conventional-commit types.
var ErrInvalidCmdType = errors.New("invalid commit type, must be one of [feat fix chore docs refactor]")

// ErrMissingTaskDescription is returned when the branch command receives an
// empty task string.
var ErrMissingTaskDescription = errors.New("missing task description")

// ErrMissingSourceBranch is returned when the PR command receives an empty
// source-branch argument.
var ErrMissingSourceBranch = errors.New("missing source branch")

// ErrMissingTargetBranch is returned when the PR command receives an empty
// target-branch argument.
var ErrMissingTargetBranch = errors.New("missing target branch")

// ErrServiceNotInitialized is returned when a CLI command is invoked before
// the backing service has been wired up.
var ErrServiceNotInitialized = errors.New("service not initialized")

// ─── Business-logic errors ────────────────────────────────────────────────────

// ErrEmptyPR is returned by DraftPrDescription when the source branch contains
// no commits that are not already present in the destination branch.
var ErrEmptyPR = errors.New("no unique commits found between branches — nothing to describe")

// ─── Infrastructure errors ────────────────────────────────────────────────────

// ErrNotGitRepository is returned when devmate is run outside a git repository.
var ErrNotGitRepository = errors.New("not inside a git repository")

// ErrInvalidRef is returned when a git ref argument would cause option
// injection (e.g. a ref starting with "-").
var ErrInvalidRef = errors.New("invalid git ref")

// ErrEmptyRef is returned when an empty string is passed as a git ref.
var ErrEmptyRef = errors.New("ref name must not be empty")

// ─── Service errors ───────────────────────────────────────────────────────────

// ErrLLMNoAttemptsSucceed is returned when generateWithRetry is configured with a
// non-positive attempt count — this indicates a programmer error.
var ErrLLMNoAttemptsSucceed = errors.New("LLM generate: all attempts failed")
var ErrLLMNoConfigured = errors.New("service: llm is not configured (nil)")
var ErrLLMRequestFailed = errors.New("LLM request failed")
var ErrLLMMarshalRequestFailed = errors.New("LLM marshal request failed")
var ErrLLMBuildRequestFailed = errors.New("LLM build request failed")
var ErrLLMDecodeResponseFailed = errors.New("LLM decode response failed")

// ErrChunkFailed is the sentinel wrapped by errors produced when an individual
// map-reduce chunk fails. Use errors.Is to detect this class of error.
var ErrChunkFailed = errors.New("chunk summarization failed")

// ErrReduceFailed is the sentinel wrapped by errors produced when a
// map-reduce reduce-group call fails.
var ErrReduceFailed = errors.New("summary reduction failed")
