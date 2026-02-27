// Package config provides the application's unified configuration.
//
// Settings are resolved in priority order (highest to lowest):
//
//  1. Environment variables  (DEVMATE_*)
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
//	{
//	  "ollama": {
//	    "base_url": "http://localhost:11434",
//	    "model":    "llama3.2:3b",
//	    "request_timeout_sec": 180,
//	    "http_max_retries": 3,
//	    "retry_base_delay_sec": 2
//	  },
//	  "service": {
//	    "chunk_threshold": 3000,
//	    "max_concurrency": 2,
//	    "max_retries": 0
//	  },
//	  "cache": {
//	    "dir": ""
//	  },
//	  "log": {
//	    "level": "info"
//	  }
//	}
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
	DefaultOllamaBaseURL         = "http://localhost:11434"
	DefaultOllamaModel           = "llama3.2:3b"
	DefaultOllamaRequestTimeout  = 3 * time.Minute
	DefaultOllamaHTTPMaxRetries  = 3
	DefaultOllamaRetryBaseDelay  = 2 * time.Second
	DefaultServiceChunkThreshold = 3000 // bytes; matches service.DefaultChunkThreshold
	DefaultServiceMaxConcurrency = 2    // matches service.DefaultServiceMaxConcurrency
	DefaultServiceMaxRetries     = 0
	DefaultLogLevel              = "info"
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

	// Model is the Ollama model tag to use (e.g. "llama3.2:3b").
	// Env: DEVMATE_OLLAMA_MODEL
	Model string `json:"model"`

	// RequestTimeoutSec is the per-request timeout in seconds.
	// Env: DEVMATE_OLLAMA_REQUEST_TIMEOUT_SEC
	RequestTimeoutSec int `json:"request_timeout_sec"`

	// HTTPMaxRetries is the number of HTTP-level retries on transient errors.
	// Env: DEVMATE_OLLAMA_HTTP_MAX_RETRIES
	HTTPMaxRetries int `json:"http_max_retries"`

	// RetryBaseDelaySec is the initial back-off delay between retries in seconds.
	// Env: DEVMATE_OLLAMA_RETRY_BASE_DELAY_SEC
	RetryBaseDelaySec int `json:"retry_base_delay_sec"`
}

// RequestTimeout converts RequestTimeoutSec to a time.Duration.
func (o OllamaConfig) RequestTimeout() time.Duration {
	return time.Duration(o.RequestTimeoutSec) * time.Second
}

// RetryBaseDelay converts RetryBaseDelaySec to a time.Duration.
func (o OllamaConfig) RetryBaseDelay() time.Duration {
	return time.Duration(o.RetryBaseDelaySec) * time.Second
}

// ServiceConfig holds settings for the service layer.
type ServiceConfig struct {
	// ChunkThreshold is the diff size in bytes above which map-reduce is used.
	// Env: DEVMATE_SERVICE_CHUNK_THRESHOLD
	ChunkThreshold int `json:"chunk_threshold"`

	// MaxConcurrency is the maximum number of parallel LLM calls.
	// 0 means use runtime.NumCPU().
	// Env: DEVMATE_SERVICE_MAX_CONCURRENCY
	MaxConcurrency int `json:"max_concurrency"`

	// MaxRetries is the number of service-level retries for failed LLM calls,
	// on top of any HTTP-level retries performed by the Ollama client.
	// Env: DEVMATE_SERVICE_MAX_RETRIES
	MaxRetries int `json:"max_retries"`
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
			Model:             DefaultOllamaModel,
			RequestTimeoutSec: int(DefaultOllamaRequestTimeout.Seconds()),
			HTTPMaxRetries:    DefaultOllamaHTTPMaxRetries,
			RetryBaseDelaySec: int(DefaultOllamaRetryBaseDelay.Seconds()),
		},
		Service: ServiceConfig{
			ChunkThreshold: DefaultServiceChunkThreshold,
			MaxConcurrency: DefaultServiceMaxConcurrency,
			MaxRetries:     DefaultServiceMaxRetries,
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

	envCount := applyEnv(&cfg, log)
	if envCount > 0 {
		log.Debug("env var overrides applied", "count", envCount)
	}

	log.Debug("config resolved",
		"ollama.base_url", cfg.Ollama.BaseURL,
		"ollama.model", cfg.Ollama.Model,
		"ollama.request_timeout_sec", cfg.Ollama.RequestTimeoutSec,
		"ollama.http_max_retries", cfg.Ollama.HTTPMaxRetries,
		"ollama.retry_base_delay_sec", cfg.Ollama.RetryBaseDelaySec,
		"service.chunk_threshold", cfg.Service.ChunkThreshold,
		"service.max_concurrency", cfg.Service.MaxConcurrency,
		"service.max_retries", cfg.Service.MaxRetries,
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

// MustLoad is like Load but panics on error. Suitable for use in main().
func MustLoad() Config {
	cfg, err := Load()
	if err != nil {
		panic(err)
	}
	return cfg
}

// ─── File loading ─────────────────────────────────────────────────────────────

// configFilePath returns the path to the config file to load.
//
// Resolution order:
//  1. DEVMATE_CONFIG env var (absolute or relative path)
//  2. ./config/config.json  (relative to the current working directory,
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
	dec.DisallowUnknownFields() // fail fast on typos in the config file
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
	if before.Ollama.Model != after.Ollama.Model {
		log.Debug("file override", "key", "ollama.model", "value", after.Ollama.Model)
	}
	if before.Ollama.RequestTimeoutSec != after.Ollama.RequestTimeoutSec {
		log.Debug("file override", "key", "ollama.request_timeout_sec", "value", after.Ollama.RequestTimeoutSec)
	}
	if before.Ollama.HTTPMaxRetries != after.Ollama.HTTPMaxRetries {
		log.Debug("file override", "key", "ollama.http_max_retries", "value", after.Ollama.HTTPMaxRetries)
	}
	if before.Ollama.RetryBaseDelaySec != after.Ollama.RetryBaseDelaySec {
		log.Debug("file override", "key", "ollama.retry_base_delay_sec", "value", after.Ollama.RetryBaseDelaySec)
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
	if before.Cache.Dir != after.Cache.Dir {
		log.Debug("file override", "key", "cache.dir", "value", after.Cache.Dir)
	}
	if before.Log.Level != after.Log.Level {
		log.Debug("file override", "key", "log.level", "value", after.Log.Level)
	}
}

// ─── Env-var overlay ──────────────────────────────────────────────────────────

// applyEnv overrides any field for which the corresponding DEVMATE_* env var
// is set to a non-empty string. Returns the number of overrides applied.
// Each override is logged individually so the operator can see exactly which
// env vars are in effect.
func applyEnv(cfg *Config, log *slog.Logger) int {
	count := 0
	override := func(key, val string, apply func()) {
		log.Debug("env override", "key", key, "value", val)
		apply()
		count++
	}

	if v := os.Getenv("DEVMATE_OLLAMA_BASE_URL"); v != "" {
		override("ollama.base_url", v, func() { cfg.Ollama.BaseURL = v })
	}
	if v := os.Getenv("DEVMATE_OLLAMA_MODEL"); v != "" {
		override("ollama.model", v, func() { cfg.Ollama.Model = v })
	}
	if v := os.Getenv("DEVMATE_OLLAMA_REQUEST_TIMEOUT_SEC"); v != "" {
		n := mustInt(v, "DEVMATE_OLLAMA_REQUEST_TIMEOUT_SEC")
		override("ollama.request_timeout_sec", v, func() { cfg.Ollama.RequestTimeoutSec = n })
	}
	if v := os.Getenv("DEVMATE_OLLAMA_HTTP_MAX_RETRIES"); v != "" {
		n := mustInt(v, "DEVMATE_OLLAMA_HTTP_MAX_RETRIES")
		override("ollama.http_max_retries", v, func() { cfg.Ollama.HTTPMaxRetries = n })
	}
	if v := os.Getenv("DEVMATE_OLLAMA_RETRY_BASE_DELAY_SEC"); v != "" {
		n := mustInt(v, "DEVMATE_OLLAMA_RETRY_BASE_DELAY_SEC")
		override("ollama.retry_base_delay_sec", v, func() { cfg.Ollama.RetryBaseDelaySec = n })
	}
	if v := os.Getenv("DEVMATE_SERVICE_CHUNK_THRESHOLD"); v != "" {
		n := mustInt(v, "DEVMATE_SERVICE_CHUNK_THRESHOLD")
		override("service.chunk_threshold", v, func() { cfg.Service.ChunkThreshold = n })
	}
	if v := os.Getenv("DEVMATE_SERVICE_MAX_CONCURRENCY"); v != "" {
		n := mustInt(v, "DEVMATE_SERVICE_MAX_CONCURRENCY")
		override("service.max_concurrency", v, func() { cfg.Service.MaxConcurrency = n })
	}
	if v := os.Getenv("DEVMATE_SERVICE_MAX_RETRIES"); v != "" {
		n := mustInt(v, "DEVMATE_SERVICE_MAX_RETRIES")
		override("service.max_retries", v, func() { cfg.Service.MaxRetries = n })
	}
	if v := os.Getenv("DEVMATE_CACHE_DIR"); v != "" {
		override("cache.dir", v, func() { cfg.Cache.Dir = v })
	}
	if v := os.Getenv("DEVMATE_LOG_LEVEL"); v != "" {
		override("log.level", v, func() { cfg.Log.Level = v })
	}
	return count
}

// mustInt parses s as an integer. It panics with a clear message on failure so
// misconfigured env vars are caught at startup rather than causing silent bugs.
func mustInt(s, name string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		panic(fmt.Sprintf("config: %s=%q is not a valid integer: %v", name, s, err))
	}
	return n
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func defaultCacheDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".cache", "devmate")
}
