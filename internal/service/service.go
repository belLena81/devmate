package service

import (
	"devmate/internal/domain"
	"log/slog"
	"strings"
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
	cmdType, _ := o.Type.String()
	mode := o.Mode.String()
	s.Log.Debug("drafting commit message", "type", cmdType, "mode", mode)

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

func (s *Service) DraftBranchName(o BranchOptions) (string, error) {
	cmdType, _ := o.Type.String()
	mode := o.Mode.String()
	s.Log.Debug("drafting branch name", "task", o.Task, "type", cmdType, "mode", mode)

	prompt := BuildBranchPrompt(o)
	result, err := s.LLM.Generate(prompt)
	if err != nil {
		s.Log.Error("LLM generation failed", "error", err)
		return "", err
	}

	s.Log.Debug("message drafted successfully")
	return result, nil
}

func (s *Service) DraftPrDescription(o PrOptions) (string, error) {
	cmdType, _ := o.Type.String()
	mode := o.Mode.String()
	s.Log.Debug("drafting pr description: branch names", "source", o.SourceBranch, "destination", o.DestinationBranch, "type", cmdType, "mode", mode)

	commits, err := s.Git.LogBetween(o.SourceBranch, o.DestinationBranch)
	if err != nil {
		s.Log.Error("failed to get diff", "error", err)
		return "", err
	}

	s.Log.Debug("calling LogBetween", "base", o.DestinationBranch, "head", o.SourceBranch)
	msgs, err := s.Git.LogBetween(o.DestinationBranch, o.SourceBranch)
	s.Log.Debug("LogBetween result", "msgs", msgs, "err", err)
	prompt := BuildPrPrompt(commits, o)
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

func BuildBranchPrompt(o BranchOptions) string {
	return o.Task
}

func BuildPrPrompt(commits []string, o PrOptions) string {
	return strings.Join(commits, "\n")
}
