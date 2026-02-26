package domain

type GitClient interface {
	DiffCached() (string, error)
	LogBetween(base, head string) ([]string, error)
}
