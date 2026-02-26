package service

import (
	"devmate/internal/domain"
	"log/slog"
)

// Service orchestrates git data retrieval, caching, and LLM generation.
type Service struct {
	Git   domain.GitClient
	LLM   domain.LLM
	Cache Cache  // injected; use NoopCache() to disable caching
	Model string // model name used as part of cache keys
	Log   *slog.Logger
}

func New(git domain.GitClient, llm domain.LLM, cache Cache, model string, log *slog.Logger) *Service {
	return &Service{
		Git:   git,
		LLM:   llm,
		Cache: cache,
		Model: model,
		Log:   log.With("component", "service"),
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
	typeStr, _ := o.Type.String()
	modeStr := o.Mode.String()
	s.Log.Debug("drafting commit message", "type", typeStr, "mode", modeStr)

	diff, err := s.Git.DiffCached()
	if err != nil {
		s.Log.Error("failed to get diff", "error", err)
		return "", err
	}
	s.Log.Debug("got diff", "bytes", len(diff))

	key := commitCacheKey(s.Model, diff, typeStr, modeStr, o.Explain)
	if hit, ok := s.Cache.Get(key); ok {
		s.Log.Debug("commit message cache hit")
		return hit, nil
	}

	prompt := BuildCommitPrompt(diff, o)
	result, err := s.LLM.Generate(prompt)
	if err != nil {
		s.Log.Error("LLM generation failed", "error", err)
		return "", err
	}

	if err := s.Cache.Set(key, result); err != nil {
		// Cache write failure is non-fatal — log and continue.
		s.Log.Debug("failed to write cache entry", "error", err)
	}

	s.Log.Debug("message drafted successfully")
	return result, nil
}

func (s *Service) DraftBranchName(o BranchOptions) (string, error) {
	typeStr, _ := o.Type.String()
	modeStr := o.Mode.String()
	s.Log.Debug("drafting branch name", "task", o.Task, "type", typeStr, "mode", modeStr)

	key := branchCacheKey(s.Model, o.Task, typeStr, modeStr, o.Explain)
	if hit, ok := s.Cache.Get(key); ok {
		s.Log.Debug("branch name cache hit")
		return hit, nil
	}

	prompt := BuildBranchPrompt(o)
	result, err := s.LLM.Generate(prompt)
	if err != nil {
		s.Log.Error("LLM generation failed", "error", err)
		return "", err
	}

	if err := s.Cache.Set(key, result); err != nil {
		s.Log.Debug("failed to write cache entry", "error", err)
	}

	s.Log.Debug("branch name drafted successfully")
	return result, nil
}

func (s *Service) DraftPrDescription(o PrOptions) (string, error) {
	typeStr, _ := o.Type.String()
	modeStr := o.Mode.String()
	s.Log.Debug("drafting pr description", "source", o.SourceBranch, "destination", o.DestinationBranch, "type", typeStr, "mode", modeStr)

	commits, err := s.Git.LogBetween(o.DestinationBranch, o.SourceBranch)
	if err != nil {
		s.Log.Error("failed to get commits", "error", err)
		return "", err
	}
	s.Log.Debug("got commits", "count", len(commits))

	if len(commits) == 0 {
		s.Log.Debug("no unique commits between branches, skipping LLM call",
			"source", o.SourceBranch, "destination", o.DestinationBranch)
		return "", domain.ErrEmptyPR
	}

	key := prCacheKey(s.Model, commits, typeStr, modeStr, o.Explain)
	if hit, ok := s.Cache.Get(key); ok {
		s.Log.Debug("pr description cache hit")
		return hit, nil
	}

	prompt := BuildPrPrompt(commits, o)
	result, err := s.LLM.Generate(prompt)
	if err != nil {
		s.Log.Error("LLM generation failed", "error", err)
		return "", err
	}

	if err := s.Cache.Set(key, result); err != nil {
		s.Log.Debug("failed to write cache entry", "error", err)
	}

	s.Log.Debug("pr description drafted successfully")
	return result, nil
}

// NoopCache is a Cache that never stores anything.
// Use it when caching is disabled or in tests that don't need cache behaviour.
type NoopCache struct{}

func (NoopCache) Get(string) (string, bool) { return "", false }
func (NoopCache) Set(string, string) error  { return nil }
func (NoopCache) Clear() error              { return nil }
