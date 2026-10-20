package cli

import (
	"context"
	"devmate/internal/service"
)

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
