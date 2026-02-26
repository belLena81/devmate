package service

import (
	"devmate/internal/domain"
	"log/slog"
)

type Service struct {
	Git domain.GitClient
	LLM domain.LLM
	Log *slog.Logger
}

func New(git domain.GitClient, llm domain.LLM, log *slog.Logger) *Service {
	return &Service{
		Git: git,
		LLM: llm,
		Log: log.With("component", "service"),
	}
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
	s.Log.Debug("drafting commit message", "type", o.Type, "mode", o.Mode)

	diff, err := s.Git.DiffCached()
	if err != nil {
		s.Log.Error("failed to get diff", "error", err)
		return "", err
	}

	s.Log.Debug("got diff", "bytes", len(diff)) // size not content
	prompt := BuildCommitPrompt(diff, o)
	result, err := s.LLM.Generate(prompt)
	if err != nil {
		s.Log.Error("LLM generation failed", "error", err)
		return "", err
	}

	s.Log.Debug("message drafted successfully")
	return result, nil
}

func BuildCommitPrompt(diff string, o CommitOptions) string {
	return diff
}
