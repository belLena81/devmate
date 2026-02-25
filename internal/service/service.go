package service

import (
	"devmate/internal/domain"
)

type Service struct {
	Git domain.GitClient
	LLM domain.LLM
}
type CommitOptions struct {
	domain.Options
}

type BranchOptions struct {
	Task string
	domain.Options
}

type PrOptions struct {
	SourceBranch      string
	DestinationBranch string
	domain.Options
}

func (s *Service) DraftMessage(o CommitOptions) (string, error) {
	diff, err := s.Git.DiffCached()
	if err != nil {
		return "", err
	}

	prompt := BuildCommitPrompt(diff, o)
	return s.LLM.Generate(prompt)
}

func BuildCommitPrompt(diff string, o CommitOptions) string {
	return diff
}
