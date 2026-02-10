package commit

import "github.com/belLena81/devmate/domain"

type Service struct {
	Git domain.GitClient
	LLM domain.LLM
}

/*func (s *Service) DraftMessage() (string, error) {
	diff, err := s.Git.DiffCached()
	if err != nil {
		return "", err
	}

	prompt := BuildCommitPrompt(diff)
	return s.LLM.Generate(prompt)
}*/
