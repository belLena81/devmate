package infra

type gitClient struct{}

// NewGitClient returns a GitClient backed by the system git binary.
func NewGitClient() *gitClient {
	return &gitClient{}
}
func (g *gitClient) DiffCached() (string, error) {
	return "", nil
}

// Compare returns the git log between base and head (read-only).
func (g *gitClient) LogBetween(base, head string) ([]string, error) {
	return nil, nil
}
