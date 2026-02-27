package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

	requestTimeout = 3 * time.Minute

	DefaultMaxRetries = 3

	retryBaseDelay = 2 * time.Second
)

// OllamaClient calls a local Ollama server to generate text.
// It implements domain.LLM.
type OllamaClient struct {
	baseURL    string
	model      string
	http       *http.Client
	log        *slog.Logger
	maxRetries int
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

// WithMaxRetries sets how many times Generate retries a transient failure.
// Pass 0 to disable retries entirely.
func WithMaxRetries(n int) Option {
	return func(c *OllamaClient) { c.maxRetries = n }
}

// NewOllamaClient returns a ready-to-use OllamaClient.
// Call with no arguments for sensible defaults; use Option functions to override.
func NewOllamaClient(opts ...Option) *OllamaClient {
	c := &OllamaClient{
		baseURL:    defaultBaseURL,
		model:      defaultModel,
		http:       &http.Client{}, // no client-level timeout; each Generate call sets its own
		log:        slog.Default().With("component", "ollama"),
		maxRetries: DefaultMaxRetries,
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

// isTransient reports whether err is a failure worth retrying:
// a 5xx status from Ollama or a network/context timeout.
// Context cancellation (user abort or parent cancellation) is NOT retried.
func isTransient(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var statusErr *ollamaStatusError
	return errors.As(err, &statusErr) && statusErr.code >= 500
}

// ollamaStatusError carries the HTTP status code so isTransient can inspect it.
type ollamaStatusError struct {
	code int
}

func (e *ollamaStatusError) Error() string {
	return fmt.Sprintf("ollama: unexpected status %d", e.code)
}

// Generate sends prompt to Ollama and returns the model's response text.
// Streaming is disabled — the full response is returned in one call.
// Transient failures (5xx, timeout) are retried up to c.maxRetries times
// with exponential back-off. Non-transient errors (4xx, cancelled context)
// are returned immediately.
func (c *OllamaClient) Generate(prompt string) (string, error) {
	body, err := json.Marshal(generateRequest{
		Model:  c.model,
		Prompt: prompt,
		Stream: false,
	})
	if err != nil {
		return "", fmt.Errorf("ollama: marshal request: %w", err)
	}

	var lastErr error
	delay := retryBaseDelay

	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			c.log.Debug("retrying generate request",
				"attempt", attempt,
				"of", c.maxRetries,
				"after", delay,
				"last_error", lastErr,
			)
			time.Sleep(delay)
			delay *= 2
		}

		c.log.Debug("sending generate request",
			"model", c.model,
			"prompt_bytes", len(prompt),
			"attempt", attempt+1,
		)

		text, err := c.doGenerate(body)
		if err == nil {
			return text, nil
		}

		lastErr = err
		if !isTransient(err) {
			// Non-transient: 4xx, cancelled — bail out immediately.
			return "", lastErr
		}
		// Transient: loop and retry (unless we've exhausted attempts).
	}

	return "", fmt.Errorf("ollama: %d attempts failed, last error: %w", c.maxRetries+1, lastErr)
}

// doGenerate performs a single HTTP round-trip to /api/generate.
func (c *OllamaClient) doGenerate(body []byte) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+generatePath, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("ollama: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		// Unwrap context errors so isTransient can inspect them directly.
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("ollama: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", &ollamaStatusError{code: resp.StatusCode}
	}

	var result generateResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("ollama: decode response: %w", err)
	}

	text := strings.TrimSpace(result.Response)
	c.log.Debug("generate response received", "response_bytes", len(text))
	return text, nil
}
