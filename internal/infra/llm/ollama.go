package llm

import (
	"bytes"
	"context"
	"devmate/internal/domain"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// OllamaClient calls a local Ollama server to generate text.
// It implements domain.LLM.
type OllamaClient struct {
	baseURL          string
	generatePath     string
	model            string
	http             *http.Client
	log              *slog.Logger
	requestTimeout   time.Duration
	maxResponseBytes int64
}

// Option configures an OllamaClient.
type Option func(*OllamaClient)

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
// In production, prefer NewOllamaClientFromConfig to wire all settings from Config.
func NewOllamaClient(url, path string, timeout time.Duration, maxResponse int64, opts ...Option) *OllamaClient {
	c := &OllamaClient{
		baseURL:          url,
		generatePath:     path,
		http:             &http.Client{}, // no client-level timeout; each Generate call sets its own
		log:              slog.Default().With("component", "ollama"),
		requestTimeout:   timeout,
		maxResponseBytes: maxResponse,
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// ClientConfig holds the configuration values NewOllamaClientFromConfig uses.
// Populate it from your application's config package.
type ClientConfig struct {
	BaseURL          string
	GeneratePath     string
	Model            string
	RequestTimeout   time.Duration
	MaxResponseBytes int64
}

// NewOllamaClientFromConfig returns an OllamaClient fully configured from cfg.
// Additional opts are applied after the config values, so they can still override.
func NewOllamaClientFromConfig(cfg ClientConfig, opts ...Option) *OllamaClient {
	return NewOllamaClient(cfg.BaseURL, cfg.GeneratePath, cfg.RequestTimeout, cfg.MaxResponseBytes, append([]Option{
		WithModel(cfg.Model),
	}, opts...)...)
}

// BaseURL returns the server base URL this client is configured to use.
func (c *OllamaClient) BaseURL() string { return c.baseURL }

// GeneratePath returns the server generate path this client is configured to use.
func (c *OllamaClient) GeneratePath() string { return c.generatePath }

// Model returns the model name this client is configured to use.
func (c *OllamaClient) Model() string { return c.model }

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

// ollamaStatusError carries the HTTP status code so callers can inspect it.
type ollamaStatusError struct {
	code int
}

func (e *ollamaStatusError) Error() string {
	return fmt.Sprintf("ollama: unexpected status %d", e.code)
}

// Generate sends prompt to Ollama and returns the model's response text.
// Streaming is disabled — the full response is returned in one call.
// ctx is forwarded to the HTTP request so callers (the service layer) can
// cancel in-flight requests on the first error or via Ctrl-C.
// Retry logic lives exclusively in the service layer (generateWithRetry),
// so this method makes exactly one attempt and returns whatever happens.
func (c *OllamaClient) Generate(ctx context.Context, prompt string) (string, error) {
	body, err := json.Marshal(generateRequest{
		Model:  c.model,
		Prompt: prompt,
		Stream: false,
	})
	if err != nil {
		return "", fmt.Errorf("ollama: %w: %w", domain.ErrLLMMarshalRequestFailed, err)
	}

	c.log.Debug("sending generate request",
		"model", c.model,
		"prompt_bytes", len(prompt),
	)

	return c.doGenerate(ctx, body)
}

// doGenerate performs a single HTTP round-trip to /api/generate.
// It wraps ctx with a per-request timeout so that even a context.Background()
// caller gets bounded by c.requestTimeout, while a shorter-lived parent context
// will still win and cancel the request first.
func (c *OllamaClient) doGenerate(ctx context.Context, body []byte) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, c.requestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+c.generatePath, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("ollama: %w: %w", domain.ErrLLMBuildRequestFailed, err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		// Unwrap context errors so the caller can inspect them directly.
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("ollama: %w: %w", domain.ErrLLMRequestFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", &ollamaStatusError{code: resp.StatusCode}
	}

	var result generateResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, c.maxResponseBytes)).Decode(&result); err != nil {
		return "", fmt.Errorf("ollama: %w: %w", domain.ErrLLMDecodeResponseFailed, err)
	}

	text := strings.TrimSpace(result.Response)
	c.log.Debug("generate response received", "response_bytes", len(text))
	return text, nil
}
