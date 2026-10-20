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

// OllamaClient calls a local Ollama server to generate text; it implements domain.LLM.
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

// WithLogger attaches a structured logger.
func WithLogger(log *slog.Logger) Option {
	return func(c *OllamaClient) { c.log = log.With("component", "ollama") }
}

// NewOllamaClient returns an OllamaClient with the given settings, overridden by opts.
func NewOllamaClient(url, path string, timeout time.Duration, maxResponse int64, opts ...Option) *OllamaClient {
	c := &OllamaClient{
		baseURL:          url,
		generatePath:     path,
		http:             &http.Client{},
		log:              slog.Default().With("component", "ollama"),
		requestTimeout:   timeout,
		maxResponseBytes: maxResponse,
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// ClientConfig holds the settings NewOllamaClientFromConfig uses.
type ClientConfig struct {
	BaseURL          string
	GeneratePath     string
	Model            string
	RequestTimeout   time.Duration
	MaxResponseBytes int64
}

// NewOllamaClientFromConfig returns an OllamaClient configured from cfg, overridden by opts.
func NewOllamaClientFromConfig(cfg ClientConfig, opts ...Option) *OllamaClient {
	return NewOllamaClient(cfg.BaseURL, cfg.GeneratePath, cfg.RequestTimeout, cfg.MaxResponseBytes, append([]Option{
		WithModel(cfg.Model),
	}, opts...)...)
}

func (c *OllamaClient) BaseURL() string { return c.baseURL }

func (c *OllamaClient) GeneratePath() string { return c.generatePath }

func (c *OllamaClient) Model() string { return c.model }

type generateRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

type generateResponse struct {
	Response string `json:"response"`
	Done     bool   `json:"done"`
}

// Generate sends prompt to Ollama and returns the full response text in one
// non-streaming request. It makes a single attempt; retries live in the service layer.
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
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("ollama: %w: %w", domain.ErrLLMRequestFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", &domain.LLMStatusError{Code: resp.StatusCode, Message: readErrorMessage(resp.Body)}
	}

	var result generateResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, c.maxResponseBytes)).Decode(&result); err != nil {
		return "", fmt.Errorf("ollama: %w: %w", domain.ErrLLMDecodeResponseFailed, err)
	}

	text := strings.TrimSpace(result.Response)
	c.log.Debug("generate response received", "response_bytes", len(text))
	return text, nil
}

// readErrorMessage extracts the "error" field Ollama sends with non-2xx responses.
func readErrorMessage(r io.Reader) string {
	var body struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(r, 4096)).Decode(&body); err != nil {
		return ""
	}
	return strings.TrimSpace(body.Error)
}
