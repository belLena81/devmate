// Package config loads application settings from built-in defaults, an optional JSON file, and DEVMATE_* environment variables.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/caarlos0/env/v11"
)

// ─── Defaults ─────────────────────────────────────────────────────────────────

const (
	DefaultOllamaBaseURL          = "http://localhost:11434"
	DefaultGeneratePath           = "/api/generate"
	DefaultOllamaModel            = "llama3.2:3b"
	DefaultOllamaRequestTimeout   = 3 * time.Minute
	DefaultOllamaMaxResponseBytes = 10 << 20 // 10 MiB
	DefaultServiceChunkThreshold  = 3000
	DefaultServiceMaxConcurrency  = 2
	DefaultServiceMaxRetries      = 0
	DefaultLogLevel               = "info"
	DefaultRetryBaseDelay         = 1 * time.Second

	appName        = "devmate"
	configFileName = "config.json"
)

// ─── Config struct ────────────────────────────────────────────────────────────

// Config is the top-level application configuration.
type Config struct {
	Ollama  OllamaConfig  `json:"ollama"`
	Service ServiceConfig `json:"service"`
	Cache   CacheConfig   `json:"cache"`
	Log     LogConfig     `json:"log"`
}

// OllamaConfig holds settings for the Ollama LLM client.
type OllamaConfig struct {
	BaseURL           string `json:"base_url"            env:"DEVMATE_OLLAMA_BASE_URL"`
	GeneratePath      string `json:"generate_path"       env:"DEVMATE_OLLAMA_GENERATE_PATH"`
	Model             string `json:"model"               env:"DEVMATE_OLLAMA_MODEL"`
	RequestTimeoutSec int    `json:"request_timeout_sec" env:"DEVMATE_REQUEST_TIMEOUT_SEC"`
	MaxResponseBytes  int64  `json:"max_response_bytes"  env:"DEVMATE_MAX_RESPONSE_BYTES"`
}

// RequestTimeout converts RequestTimeoutSec to a time.Duration.
func (o OllamaConfig) RequestTimeout() time.Duration {
	return time.Duration(o.RequestTimeoutSec) * time.Second
}

// ServiceConfig holds settings for the service layer.
type ServiceConfig struct {
	// ChunkThreshold is the diff size in bytes above which map-reduce is used.
	ChunkThreshold int `json:"chunk_threshold" env:"DEVMATE_CHUNK_THRESHOLD"`
	// MaxConcurrency is the maximum number of parallel LLM calls; 0 means runtime.NumCPU().
	MaxConcurrency    int `json:"max_concurrency"      env:"DEVMATE_MAX_CONCURRENCY"`
	MaxRetries        int `json:"max_retries"          env:"DEVMATE_MAX_RETRIES"`
	RetryBaseDelaySec int `json:"retry_base_delay_sec" env:"DEVMATE_RETRY_BASE_DELAY_SEC"`
}

// RetryBaseDelay converts RetryBaseDelaySec to a time.Duration.
func (s ServiceConfig) RetryBaseDelay() time.Duration {
	return time.Duration(s.RetryBaseDelaySec) * time.Second
}

// CacheConfig holds settings for the disk cache.
type CacheConfig struct {
	Dir string `json:"dir" env:"DEVMATE_CACHE_DIR"`
}

// LogConfig holds logging settings.
type LogConfig struct {
	Level string `json:"level" env:"DEVMATE_LOG_LEVEL"`
}

// SlogLevel converts Level to a slog.Level.
func (l LogConfig) SlogLevel() slog.Level {
	switch l.Level {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func defaults() Config {
	return Config{
		Ollama: OllamaConfig{
			BaseURL:           DefaultOllamaBaseURL,
			GeneratePath:      DefaultGeneratePath,
			Model:             DefaultOllamaModel,
			RequestTimeoutSec: int(DefaultOllamaRequestTimeout.Seconds()),
			MaxResponseBytes:  DefaultOllamaMaxResponseBytes,
		},
		Service: ServiceConfig{
			ChunkThreshold:    DefaultServiceChunkThreshold,
			MaxConcurrency:    DefaultServiceMaxConcurrency,
			MaxRetries:        DefaultServiceMaxRetries,
			RetryBaseDelaySec: int(DefaultRetryBaseDelay.Seconds()),
		},
		Cache: CacheConfig{Dir: DefaultCacheDir()},
		Log:   LogConfig{Level: DefaultLogLevel},
	}
}

// ─── Paths ────────────────────────────────────────────────────────────────────

// DefaultPath returns the per-user config file location, or "" when the user config dir is unknown.
func DefaultPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, appName, configFileName)
}

// DefaultCacheDir returns the per-user cache directory, or "" when the user cache dir is unknown.
func DefaultCacheDir() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, appName)
}

func resolvePath(explicit string) string {
	if explicit != "" {
		return explicit
	}
	if p := os.Getenv("DEVMATE_CONFIG"); p != "" {
		return p
	}
	return DefaultPath()
}

// ─── Load ────────────────────────────────────────────────────────────────────

// Options controls how Load locates the config file and reports progress.
type Options struct {
	// Path overrides the config file; empty means DEVMATE_CONFIG, then DefaultPath().
	Path string
	// Debug prints config-loading details to stderr.
	Debug bool
}

// Load is LoadWith using the default options.
func Load() (Config, error) {
	return LoadWith(Options{})
}

// LoadWith builds the resolved Config by layering defaults, then the config file, then env vars.
func LoadWith(o Options) (Config, error) {
	log := bootstrapLogger(o.Debug)
	cfg := defaults()

	path := resolvePath(o.Path)
	loaded, err := loadFile(path, &cfg, log)
	if err != nil {
		return cfg, fmt.Errorf("config: loading %s: %w", path, err)
	}
	if !loaded {
		log.Debug("config file not found, using built-in defaults", "path", path)
	}

	err = env.ParseWithOptions(&cfg, env.Options{
		OnSet: func(tag string, value any, _ bool) {
			if v, ok := os.LookupEnv(tag); ok && v != "" {
				log.Debug("env override", "key", tag, "value", value)
			}
		},
	})
	if err != nil {
		return cfg, fmt.Errorf("config: %w", err)
	}
	if cfg.Cache.Dir == "" {
		cfg.Cache.Dir = DefaultCacheDir()
	}

	log.Debug("config resolved",
		"ollama.base_url", cfg.Ollama.BaseURL,
		"ollama.generate_path", cfg.Ollama.GeneratePath,
		"ollama.model", cfg.Ollama.Model,
		"ollama.request_timeout_sec", cfg.Ollama.RequestTimeoutSec,
		"ollama.max_response_bytes", cfg.Ollama.MaxResponseBytes,
		"service.chunk_threshold", cfg.Service.ChunkThreshold,
		"service.max_concurrency", cfg.Service.MaxConcurrency,
		"service.max_retries", cfg.Service.MaxRetries,
		"service.retry_base_delay_sec", cfg.Service.RetryBaseDelaySec,
		"cache.dir", cfg.Cache.Dir,
		"log.level", cfg.Log.Level,
	)
	return cfg, nil
}

// bootstrapLogger logs config loading to stderr only when debug output was requested.
func bootstrapLogger(debug bool) *slog.Logger {
	level := slog.LevelError + 1
	if debug || os.Getenv("DEVMATE_LOG_LEVEL") == "debug" {
		level = slog.LevelDebug
	}
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})).
		With("component", "config")
}

// loadFile merges a JSON config file into cfg; a missing file returns (false, nil).
func loadFile(path string, cfg *Config, log *slog.Logger) (bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := json.Unmarshal(data, cfg); err != nil {
		return false, fmt.Errorf("parse error: %w", err)
	}
	log.Debug("config file loaded", "path", path)

	var sections map[string]map[string]any
	if json.Unmarshal(data, &sections) == nil {
		for section, keys := range sections {
			for key, value := range keys {
				log.Debug("file override", "key", section+"."+key, "value", value)
			}
		}
	}
	return true, nil
}
