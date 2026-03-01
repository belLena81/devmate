// Package config provides the application's unified configuration.
//
// Settings are resolved in priority order (highest to lowest):
//
//  1. Environment variables (DEVMATE_*)
//  2. Config file            (./config/config.json, or $DEVMATE_CONFIG)
//  3. Built-in defaults
//
// The config file path defaults to ./config/config.json relative to the
// working directory (the project root). This lets teams commit a shared
// config.json in the repo. Override with DEVMATE_CONFIG for other paths.
//
// The config file is optional. If it does not exist the defaults are used as-is.
// Unknown keys in the file are silently ignored so older binaries can read newer
// config files without breaking.
//
// Example config file:
//
//		{
//		  "ollama": {
//		    "base_url": "http://localhost:11434",
//		    "model":    "llama3.2:3b",
//		    "request_timeout_sec": 180,
//		  },
//		  "service": {
//		    "chunk_threshold": 3000,
//		    "max_concurrency": 2,
//		    "max_retries": 0,
//	     "retry_base_delay_sec": 1,
//		  },
//		  "cache": {
//		    "dir": ""
//		  },
//		  "log": {
//		    "level": "info"
//		  }
//		}
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

// These defaults mirror the values used by each subsystem when no config is
// provided. They live here as the single authoritative reference that both
// the config file documentation and the defaults() function use.
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
// All fields have safe zero values; Load() fills them with defaults and then
// overlays the config file and environment variables.
type Config struct {
	Ollama  OllamaConfig  `json:"ollama"`
	Service ServiceConfig `json:"service"`
	Cache   CacheConfig   `json:"cache"`
	Log     LogConfig     `json:"log"`
}

// OllamaConfig holds settings for the Ollama LLM client.
type OllamaConfig struct {
	// BaseURL is the URL of the Ollama server.
	// Env: DEVMATE_OLLAMA_BASE_URL
	BaseURL string `json:"base_url"`
	// GeneratePath is the path part of the Ollama request URL.
	// Env: DEVMATE_OLLAMA_GENERATE_PATH
	GeneratePath string `json:"generate_path"`

	// Model is the Ollama model tag to use (e.g. "llama3.2:3b").
	// Env: DEVMATE_OLLAMA_MODEL
	Model string `json:"model"`

	// RequestTimeoutSec is the per-request timeout in seconds.
	// Env: DEVMATE_REQUEST_TIMEOUT_SEC
	RequestTimeoutSec int `json:"request_timeout_sec"`
	//MaxResponseBytes is the maximum number of bytes to return from the server.
	// Env: DEVMATE_MAX_RESPONSE_BYTES
	MaxResponseBytes int64 `json:"max_response_bytes"`
}

// RequestTimeout converts RequestTimeoutSec to a time.Duration.
func (o OllamaConfig) RequestTimeout() time.Duration {
	return time.Duration(o.RequestTimeoutSec) * time.Second
}

// ServiceConfig holds settings for the service layer.
type ServiceConfig struct {
	// ChunkThreshold is the diff size in bytes above which map-reduce is used.
	// Env: DEVMATE_CHUNK_THRESHOLD
	ChunkThreshold int `json:"chunk_threshold"`

	// MaxConcurrency is the maximum number of parallel LLM calls.
	// 0 means use runtime.NumCPU().
	// Env: DEVMATE_MAX_CONCURRENCY
	MaxConcurrency int `json:"max_concurrency"`

	// MaxRetries is the number of service-level retries for failed LLM calls.
	// Env: DEVMATE_MAX_RETRIES
	MaxRetries int `json:"max_retries"`

	// RetryBaseDelaySec is the initial exponential back-off delay in seconds.
	// Env: DEVMATE_RETRY_BASE_DELAY_SEC
	RetryBaseDelaySec int `json:"retry_base_delay_sec"`
}

// RetryBaseDelay converts RetryBaseDelaySec to a time.Duration.
func (s ServiceConfig) RetryBaseDelay() time.Duration {
	return time.Duration(s.RetryBaseDelaySec) * time.Second
}

// CacheConfig holds settings for the disk cache.
type CacheConfig struct {
	// Dir is the directory where cached LLM responses are stored.
	// Defaults to ~/.cache/devmate. Set to "" to use the default.
	// Env: DEVMATE_CACHE_DIR
	Dir string `json:"dir"`
}

// LogConfig holds logging settings.
type LogConfig struct {
	// Level controls verbosity: "debug", "info", "warn", "error".
	// Env: DEVMATE_LOG_LEVEL
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

// Load builds the resolved Config by layering defaults → file → env vars.
// The config file path is taken from DEVMATE_CONFIG if set, otherwise
// defaults to ./config/config.json (relative to the working directory).
//
// Progress is logged to stderr at DEBUG level so the operator can see exactly
// which values came from the file, env vars, or built-in defaults.
// This log uses a fixed debug-level handler that is active before the
// configured log level is known — it is the bootstrap logger.
//
// An absent config file is not an error. Parse errors are returned.
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

// bootstrapLogger returns a debug-level logger that writes to stderr.
// It is used only during Load() — before the configured log level is known —
// so the operator can always see config loading progress when needed by
// setting DEVMATE_LOG_LEVEL=debug before running the binary.
//
// We honour DEVMATE_LOG_LEVEL here as a special case: if it is set to anything
// other than "debug" the bootstrap logger is silenced so config chatter does
// not appear in normal runs.
func bootstrapLogger() *slog.Logger {
	level := slog.LevelDebug
	if v := os.Getenv("DEVMATE_LOG_LEVEL"); v != "debug" && v != "" {
		// Non-debug level requested — suppress bootstrap output.
		level = slog.LevelError + 1 // above all standard levels → silent
	}
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: level,
	})).With("component", "config")
}

// ─── File loading ─────────────────────────────────────────────────────────────

// configFilePath returns the path to the config file to load.
//
// Resolution order:
//  1. DEVMATE_CONFIG env var (absolute or relative path)
//  2. ./config/config.json (relative to the current working directory,
//     i.e. the project root when running "devmate" from the repo)
//
// Using a repo-relative path as the default makes it easy to commit a shared
// team config alongside the source code without any per-user setup.
func configFilePath() string {
	if p := os.Getenv("DEVMATE_CONFIG"); p != "" {
		return p
	}
	return filepath.Join("config", "config.json")
}

// loadFile reads a JSON config file into cfg.
// Returns (true, nil) when the file was found and parsed successfully.
// Returns (false, nil) when the file does not exist — that is not an error.
// Logs each field that differs from the defaults so the operator can see
// exactly what the file changed.
func loadFile(path string, cfg *Config, log *slog.Logger) (loaded bool, err error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer f.Close()

	// Snapshot defaults so we can compare after decoding.
	before := *cfg

	dec := json.NewDecoder(f)
	if err := dec.Decode(cfg); err != nil {
		return false, fmt.Errorf("parse error: %w", err)
	}

	log.Debug("config file loaded", "path", path)
	logFileOverrides(log, before, *cfg)
	return true, nil
}

// logFileOverrides logs each field whose value changed after loading the file.
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

// applyEnv overrides any field for which the corresponding DEVMATE_* env var
// is set to a non-empty string. Returns (count, error) where count is the
// number of overrides applied and error is non-nil if any integer env var
// held an invalid value.
// Each override is logged individually so the operator can see exactly which
// env vars are in effect.
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

// parseIntEnv parses s as an integer for the named environment variable.
// It returns a descriptive error on failure so Load() can surface a clear
// message instead of crashing the process with a raw panic trace.
func parseIntEnv(s, name string) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("config: %s=%q is not a valid integer: %w", name, s, err)
	}
	return n, nil
}

// parseIntEnv parses s as an integer for the named environment variable.
// It returns a descriptive error on failure so Load() can surface a clear
// message instead of crashing the process with a raw panic trace.
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
