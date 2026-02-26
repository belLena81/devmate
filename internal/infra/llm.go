package infra

type ollamaClient struct {
	url   string
	model string
}

// NewOllamaClient returns an LLM backed by the local Ollama server.
func NewOllamaClient() *ollamaClient {
	return &ollamaClient{
		url:   "",
		model: "",
	}
}
func (o *ollamaClient) Generate(prompt string) (string, error) {
	return prompt, nil
}
