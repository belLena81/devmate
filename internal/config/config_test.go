package config_test

import (
	"devmate/internal/config"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoad_Defaults(t *testing.T) {
	// Point at a non-existent file so we exercise the defaults path.
	t.Setenv("DEVMATE_CONFIG", filepath.Join(t.TempDir(), "nonexistent.json"))

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}

	if cfg.Ollama.BaseURL != config.DefaultOllamaBaseURL {
		t.Errorf("BaseURL: got %q, want %q", cfg.Ollama.BaseURL, config.DefaultOllamaBaseURL)
	}
	if cfg.Ollama.Model != config.DefaultOllamaModel {
		t.Errorf("Model: got %q, want %q", cfg.Ollama.Model, config.DefaultOllamaModel)
	}
	if cfg.Ollama.RequestTimeout() != config.DefaultOllamaRequestTimeout {
		t.Errorf("RequestTimeout: got %v, want %v", cfg.Ollama.RequestTimeout(), config.DefaultOllamaRequestTimeout)
	}
	if cfg.Service.ChunkThreshold != config.DefaultServiceChunkThreshold {
		t.Errorf("ChunkThreshold: got %d, want %d", cfg.Service.ChunkThreshold, config.DefaultServiceChunkThreshold)
	}
	if cfg.Service.MaxConcurrency != config.DefaultServiceMaxConcurrency {
		t.Errorf("MaxConcurrency: got %d, want %d", cfg.Service.MaxConcurrency, config.DefaultServiceMaxConcurrency)
	}
	if cfg.Service.MaxRetries != config.DefaultServiceMaxRetries {
		t.Errorf("MaxRetries: got %d, want %d", cfg.Service.MaxRetries, config.DefaultServiceMaxRetries)
	}
	if cfg.Log.Level != config.DefaultLogLevel {
		t.Errorf("Log.Level: got %q, want %q", cfg.Log.Level, config.DefaultLogLevel)
	}
}

func TestLoad_FromFile(t *testing.T) {
	data := map[string]any{
		"ollama": map[string]any{
			"base_url":             "http://remote:11434",
			"model":                "mistral:7b",
			"request_timeout_sec":  60,
			"http_max_retries":     5,
			"retry_base_delay_sec": 4,
		},
		"service": map[string]any{
			"chunk_threshold": 8000,
			"max_concurrency": 4,
			"max_retries":     2,
		},
		"cache": map[string]any{
			"dir": "/tmp/devmate-cache",
		},
		"log": map[string]any{
			"level": "debug",
		},
	}

	f := writeTempConfig(t, data)
	t.Setenv("DEVMATE_CONFIG", f)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}

	if got, want := cfg.Ollama.BaseURL, "http://remote:11434"; got != want {
		t.Errorf("BaseURL: got %q, want %q", got, want)
	}
	if got, want := cfg.Ollama.Model, "mistral:7b"; got != want {
		t.Errorf("Model: got %q, want %q", got, want)
	}
	if got, want := cfg.Ollama.RequestTimeout(), 60*time.Second; got != want {
		t.Errorf("RequestTimeout: got %v, want %v", got, want)
	}
	if got, want := cfg.Ollama.HTTPMaxRetries, 5; got != want {
		t.Errorf("HTTPMaxRetries: got %d, want %d", got, want)
	}
	if got, want := cfg.Service.ChunkThreshold, 8000; got != want {
		t.Errorf("ChunkThreshold: got %d, want %d", got, want)
	}
	if got, want := cfg.Service.MaxConcurrency, 4; got != want {
		t.Errorf("MaxConcurrency: got %d, want %d", got, want)
	}
	if got, want := cfg.Service.MaxRetries, 2; got != want {
		t.Errorf("MaxRetries: got %d, want %d", got, want)
	}
	if got, want := cfg.Cache.Dir, "/tmp/devmate-cache"; got != want {
		t.Errorf("Cache.Dir: got %q, want %q", got, want)
	}
	if got, want := cfg.Log.Level, "debug"; got != want {
		t.Errorf("Log.Level: got %q, want %q", got, want)
	}
}

func TestLoad_EnvOverridesFile(t *testing.T) {
	data := map[string]any{
		"ollama": map[string]any{
			"model": "file-model",
		},
	}
	f := writeTempConfig(t, data)
	t.Setenv("DEVMATE_CONFIG", f)
	t.Setenv("DEVMATE_OLLAMA_MODEL", "env-model")
	t.Setenv("DEVMATE_SERVICE_CHUNK_THRESHOLD", "9999")
	t.Setenv("DEVMATE_LOG_LEVEL", "warn")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}

	if got, want := cfg.Ollama.Model, "env-model"; got != want {
		t.Errorf("Model: env should override file: got %q, want %q", got, want)
	}
	if got, want := cfg.Service.ChunkThreshold, 9999; got != want {
		t.Errorf("ChunkThreshold: env should override default: got %d, want %d", got, want)
	}
	if got, want := cfg.Log.Level, "warn"; got != want {
		t.Errorf("Log.Level: env should override default: got %q, want %q", got, want)
	}
}

func TestLoad_BadFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	os.WriteFile(path, []byte(`{ "ollama": { "unknown_key": 1 } }`), 0o644)
	t.Setenv("DEVMATE_CONFIG", path)

	_, err := config.Load()
	if err == nil {
		t.Fatal("Load() expected error for unknown key, got nil")
	}
}

func TestLogConfig_SlogLevel(t *testing.T) {
	tests := []struct {
		level string
		want  string
	}{
		{"debug", "DEBUG"},
		{"info", "INFO"},
		{"warn", "WARN"},
		{"error", "ERROR"},
		{"", "INFO"},        // zero value → info
		{"garbage", "INFO"}, // unknown → info
	}
	for _, tc := range tests {
		lc := config.LogConfig{Level: tc.level}
		if got := lc.SlogLevel().String(); got != tc.want {
			t.Errorf("SlogLevel(%q) = %q, want %q", tc.level, got, tc.want)
		}
	}
}

// writeTempConfig writes data as JSON to a temp file and returns its path.
func writeTempConfig(t *testing.T, data any) string {
	t.Helper()
	b, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatalf("write config file: %v", err)
	}
	return path
}
