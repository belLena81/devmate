package cli

// helper_test.go contains shared test doubles used across the cli package's
// _test.go files. None of these symbols are exported; they are
// package-internal test utilities compiled only during `go test`.

import (
	"context"
	"devmate/internal/service"
)

// fakeCommitService is a test double for CommitService. It records the options
// it was called with so tests can assert on them, and it returns a configured
// response or error.
type fakeCommitService struct {
	response string
	err      error
	options  service.CommitOptions
}

func (f *fakeCommitService) DraftMessage(_ context.Context, o service.CommitOptions) (string, error) {
	f.options = o
	return f.response, f.err
}

type fakeBranchService struct {
	response string
	err      error
	options  service.BranchOptions
}

func (f *fakeBranchService) DraftBranchName(_ context.Context, o service.BranchOptions) (string, error) {
	f.options = o
	return f.response, f.err
}

type fakePrService struct {
	response string
	err      error
	options  service.PrOptions
}

func (f *fakePrService) DraftPrDescription(_ context.Context, o service.PrOptions) (string, error) {
	f.options = o
	return f.response, f.err
}
