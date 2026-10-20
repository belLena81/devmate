// Package config loads application settings from built-in defaults, an optional JSON file, and DEVMATE_* environment variables.
package config

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"time"
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
	BaseURL      string `json:"base_url"`
	GeneratePath string `json:"generate_path"`

	Model string `json:"model"`

	RequestTimeoutSec int   `json:"request_timeout_sec"`
	MaxResponseBytes  int64 `json:"max_response_bytes"`
}

// RequestTimeout converts RequestTimeoutSec to a time.Duration.
func (o OllamaConfig) RequestTimeout() time.Duration {
	return time.Duration(o.RequestTimeoutSec) * time.Second
}

// ServiceConfig holds settings for the service layer.
type ServiceConfig struct {
	// ChunkThreshold is the diff size in bytes above which map-reduce is used.
	ChunkThreshold int `json:"chunk_threshold"`

	// MaxConcurrency is the maximum number of parallel LLM calls; 0 means runtime.NumCPU().
	MaxConcurrency int `json:"max_concurrency"`

	MaxRetries int `json:"max_retries"`

	RetryBaseDelaySec int `json:"retry_base_delay_sec"`
}

// RetryBaseDelay converts RetryBaseDelaySec to a time.Duration.
func (s ServiceConfig) RetryBaseDelay() time.Duration {
	return time.Duration(s.RetryBaseDelaySec) * time.Second
}

// CacheConfig holds settings for the disk cache.
type CacheConfig struct {
	Dir string `json:"dir"`
}

// LogConfig holds logging settings.
type LogConfig struct {
	Level string `json:"level"`
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

// ─── Default constructor ─────────────────────────────────────────────────────

// defaults returns a Config pre-filled with all built-in defaults.
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
		Cache: CacheConfig{
			Dir: defaultCacheDir(),
		},
		Log: LogConfig{
			Level: DefaultLogLevel,
		},
	}
}

// ─── Load ────────────────────────────────────────────────────────────────────

// Load builds the resolved Config by layering defaults, then the config file, then env vars.
func Load() (Config, error) {
	log := bootstrapLogger()

	cfg := defaults()
	log.Debug("config defaults applied")

	path := configFilePath()
	log.Debug("loading config file", "path", path)

	fileLoaded, err := loadFile(path, &cfg, log)
	if err != nil {
		return cfg, fmt.Errorf("config: loading %s: %w", path, err)
	}
	if !fileLoaded {
		log.Debug("config file not found, using built-in defaults", "path", path)
	}

	envCount, err := applyEnv(&cfg, log)
	if err != nil {
		return cfg, err
	}
	if envCount > 0 {
		log.Debug("env var overrides applied", "count", envCount)
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

// bootstrapLogger returns the stderr logger used during Load, before the configured log level is known.
func bootstrapLogger() *slog.Logger {
	level := slog.LevelDebug
	if v := os.Getenv("DEVMATE_LOG_LEVEL"); v != "debug" && v != "" {
		level = slog.LevelError + 1
	}
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: level,
	})).With("component", "config")
}

// ─── File loading ─────────────────────────────────────────────────────────────

func configFilePath() string {
	if p := os.Getenv("DEVMATE_CONFIG"); p != "" {
		return p
	}
	return filepath.Join("config", "config.json")
}

// loadFile reads a JSON config file into cfg; a missing file returns (false, nil).
func loadFile(path string, cfg *Config, log *slog.Logger) (loaded bool, err error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer f.Close()

	before := *cfg

	dec := json.NewDecoder(f)
	if err := dec.Decode(cfg); err != nil {
		return false, fmt.Errorf("parse error: %w", err)
	}

	log.Debug("config file loaded", "path", path)
	logFileOverrides(log, before, *cfg)
	return true, nil
}

func logFileOverrides(log *slog.Logger, before, after Config) {
	if before.Ollama.BaseURL != after.Ollama.BaseURL {
		log.Debug("file override", "key", "ollama.base_url", "value", after.Ollama.BaseURL)
	}
	if before.Ollama.GeneratePath != after.Ollama.GeneratePath {
		log.Debug("file override", "key", "ollama.generate_path", "value", after.Ollama.GeneratePath)
	}
	if before.Ollama.Model != after.Ollama.Model {
		log.Debug("file override", "key", "ollama.model", "value", after.Ollama.Model)
	}
	if before.Ollama.RequestTimeoutSec != after.Ollama.RequestTimeoutSec {
		log.Debug("file override", "key", "ollama.request_timeout_sec", "value", after.Ollama.RequestTimeoutSec)
	}
	if before.Ollama.MaxResponseBytes != after.Ollama.MaxResponseBytes {
		log.Debug("file override", "key", "ollama.max_response_bytes", "value", after.Ollama.MaxResponseBytes)
	}
	if before.Service.ChunkThreshold != after.Service.ChunkThreshold {
		log.Debug("file override", "key", "service.chunk_threshold", "value", after.Service.ChunkThreshold)
	}
	if before.Service.MaxConcurrency != after.Service.MaxConcurrency {
		log.Debug("file override", "key", "service.max_concurrency", "value", after.Service.MaxConcurrency)
	}
	if before.Service.MaxRetries != after.Service.MaxRetries {
		log.Debug("file override", "key", "service.max_retries", "value", after.Service.MaxRetries)
	}
	if before.Service.RetryBaseDelaySec != after.Service.RetryBaseDelaySec {
		log.Debug("file override", "key", "service.retry_base_delay_sec", "value", after.Service.RetryBaseDelaySec)
	}
	if before.Cache.Dir != after.Cache.Dir {
		log.Debug("file override", "key", "cache.dir", "value", after.Cache.Dir)
	}
	if before.Log.Level != after.Log.Level {
		log.Debug("file override", "key", "log.level", "value", after.Log.Level)
	}
}

// ─── Env-var overlay ──────────────────────────────────────────────────────────

// applyEnv overlays non-empty DEVMATE_* env vars onto cfg and returns the number of overrides applied.
func applyEnv(cfg *Config, log *slog.Logger) (int, error) {
	count := 0
	override := func(key, val string, apply func()) {
		log.Debug("env override", "key", key, "value", val)
		apply()
		count++
	}

	if v := os.Getenv("DEVMATE_OLLAMA_BASE_URL"); v != "" {
		override("ollama.base_url", v, func() { cfg.Ollama.BaseURL = v })
	}
	if v := os.Getenv("DEVMATE_OLLAMA_GENERATE_PATH"); v != "" {
		override("ollama.generate_path", v, func() { cfg.Ollama.GeneratePath = v })
	}
	if v := os.Getenv("DEVMATE_OLLAMA_MODEL"); v != "" {
		override("ollama.model", v, func() { cfg.Ollama.Model = v })
	}
	if v := os.Getenv("DEVMATE_REQUEST_TIMEOUT_SEC"); v != "" {
		n, err := parseIntEnv(v, "DEVMATE_REQUEST_TIMEOUT_SEC")
		if err != nil {
			return count, err
		}
		override("ollama.request_timeout_sec", v, func() { cfg.Ollama.RequestTimeoutSec = n })
	}
	if v := os.Getenv("DEVMATE_MAX_RESPONSE_BYTES"); v != "" {
		n, err := parseInt64Env(v, "DEVMATE_MAX_RESPONSE_BYTES")
		if err != nil {
			return count, err
		}
		override("ollama.max_response_bytes", v, func() { cfg.Ollama.MaxResponseBytes = n })
	}

	if v := os.Getenv("DEVMATE_CHUNK_THRESHOLD"); v != "" {
		n, err := parseIntEnv(v, "DEVMATE_CHUNK_THRESHOLD")
		if err != nil {
			return count, err
		}
		override("service.chunk_threshold", v, func() { cfg.Service.ChunkThreshold = n })
	}
	if v := os.Getenv("DEVMATE_MAX_CONCURRENCY"); v != "" {
		n, err := parseIntEnv(v, "DEVMATE_MAX_CONCURRENCY")
		if err != nil {
			return count, err
		}
		override("service.max_concurrency", v, func() { cfg.Service.MaxConcurrency = n })
	}
	if v := os.Getenv("DEVMATE_MAX_RETRIES"); v != "" {
		n, err := parseIntEnv(v, "DEVMATE_MAX_RETRIES")
		if err != nil {
			return count, err
		}
		override("service.max_retries", v, func() { cfg.Service.MaxRetries = n })
	}
	if v := os.Getenv("DEVMATE_RETRY_BASE_DELAY_SEC"); v != "" {
		n, err := parseIntEnv(v, "DEVMATE_RETRY_BASE_DELAY_SEC")
		if err != nil {
			return count, err
		}
		override("service.retry_base_delay_sec", v, func() { cfg.Service.RetryBaseDelaySec = n })
	}
	if v := os.Getenv("DEVMATE_CACHE_DIR"); v != "" {
		override("cache.dir", v, func() { cfg.Cache.Dir = v })
	}
	if v := os.Getenv("DEVMATE_LOG_LEVEL"); v != "" {
		override("log.level", v, func() { cfg.Log.Level = v })
	}
	return count, nil
}

func parseIntEnv(s, name string) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("config: %s=%q is not a valid integer: %w", name, s, err)
	}
	return n, nil
}

func parseInt64Env(s, name string) (int64, error) {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("config: %s=%q is not a valid integer: %w", name, s, err)
	}
	return n, nil
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func defaultCacheDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".cache", "devmate")
}
