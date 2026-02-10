package commit

import "devmate/internal/domain"

type Service struct {
	Git domain.GitClient
	LLM domain.LLM
}

func (s *Service) DraftMessage() (string, error) {
	diff, err := s.Git.DiffCached()
	if err != nil {
		return "", err
	}

	prompt := BuildCommitPrompt(diff)
	return s.LLM.Generate(prompt)
}

func BuildCommitPrompt(diff string) string {
	return diff
}
