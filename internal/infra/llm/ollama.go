package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

const (
	defaultBaseURL = "http://localhost:11434"
	defaultModel   = "llama3.2:3b"
	generatePath   = "/api/generate"
	requestTimeout = 120 * time.Second
)

// OllamaClient calls a local Ollama server to generate text.
// It implements domain.LLM.
type OllamaClient struct {
	baseURL string
	model   string
	http    *http.Client
	log     *slog.Logger
}

// Option configures an OllamaClient.
type Option func(*OllamaClient)

// WithBaseURL overrides the default Ollama server URL (http://localhost:11434).
// Useful for pointing at a remote instance or in tests.
func WithBaseURL(url string) Option {
	return func(c *OllamaClient) { c.baseURL = url }
}

// WithModel overrides the default model name.
func WithModel(model string) Option {
	return func(c *OllamaClient) { c.model = model }
}

// WithLogger attaches a structured logger. If not provided, logs are discarded.
func WithLogger(log *slog.Logger) Option {
	return func(c *OllamaClient) { c.log = log.With("component", "ollama") }
}

// NewOllamaClient returns a ready-to-use OllamaClient.
// Call with no arguments for sensible defaults; use Option functions to override.
func NewOllamaClient(opts ...Option) *OllamaClient {
	c := &OllamaClient{
		baseURL: defaultBaseURL,
		model:   defaultModel,
		http:    &http.Client{Timeout: requestTimeout},
		log:     slog.Default().With("component", "ollama"),
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Model returns the model name this client is configured to use.
func (c *OllamaClient) Model() string { return c.model }

// BaseURL returns the server base URL this client is configured to use.
func (c *OllamaClient) BaseURL() string { return c.baseURL }

// generateRequest is the JSON body sent to POST /api/generate.
type generateRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

// generateResponse is the JSON body returned by POST /api/generate
// when stream=false.
type generateResponse struct {
	Response string `json:"response"`
	Done     bool   `json:"done"`
}

// Generate sends prompt to Ollama and returns the model's response text.
// Streaming is disabled — the full response is returned in one call.
func (c *OllamaClient) Generate(prompt string) (string, error) {
	c.log.Debug("sending generate request", "model", c.model, "prompt_bytes", len(prompt))

	body, err := json.Marshal(generateRequest{
		Model:  c.model,
		Prompt: prompt,
		Stream: false,
	})
	if err != nil {
		return "", fmt.Errorf("ollama: marshal request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, c.baseURL+generatePath, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("ollama: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("ollama: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ollama: unexpected status %d", resp.StatusCode)
	}

	var result generateResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("ollama: decode response: %w", err)
	}

	text := strings.TrimSpace(result.Response)
	c.log.Debug("generate response received", "response_bytes", len(text))
	return text, nil
}
