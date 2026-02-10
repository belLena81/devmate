package domain

type GitClient interface {
	DiffCached() (string, error)
	CurrentBranch() (string, error)
	Compare(base, head string) (string, error)
}
