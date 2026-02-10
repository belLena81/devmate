package domain

type LLM interface {
	Generate(prompt string) (string, error)
}
